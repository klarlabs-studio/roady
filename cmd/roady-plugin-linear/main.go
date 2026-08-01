package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/felixgeelhaar/roady/pkg/domain/planning"
	domainPlugin "github.com/felixgeelhaar/roady/pkg/domain/plugin"
	infraPlugin "github.com/felixgeelhaar/roady/pkg/plugin"
	goplugin "github.com/hashicorp/go-plugin"
)

type LinearSyncer struct {
	apiKey string
	teamID string
}

func (s *LinearSyncer) Init(config map[string]string) error {
	s.apiKey = config["api_key"]
	if s.apiKey == "" {
		s.apiKey = os.Getenv("LINEAR_API_KEY")
	}
	s.teamID = config["team_id"]
	if s.teamID == "" {
		s.teamID = os.Getenv("LINEAR_TEAM_ID")
	}

	if s.apiKey == "" {
		return fmt.Errorf("LINEAR_API_KEY is required")
	}
	if s.teamID == "" {
		return fmt.Errorf("LINEAR_TEAM_ID is required")
	}
	return nil
}

type linearIssue struct {
	ID          string `json:"id"`
	Identifier  string `json:"identifier"`
	Title       string `json:"title"`
	Description string `json:"description"`
	State       struct {
		Name string `json:"name"`
		Type string `json:"type"` // unstarted, started, completed, canceled, backlog, triage
	} `json:"state"`
	URL string `json:"url"`
}

func (s *LinearSyncer) Sync(plan *planning.Plan, state *planning.ExecutionState) (*domainPlugin.SyncResult, error) {
	result := &domainPlugin.SyncResult{
		StatusUpdates: make(map[string]planning.TaskStatus),
		LinkUpdates:   make(map[string]planning.ExternalRef),
	}

	// 1. Fetch existing issues for the team
	existingIssues, err := s.fetchTeamIssues()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch linear issues: %w", err)
	}

	// Create a map for lookup by roady-id (stored in description)
	issueByRoadyID := make(map[string]linearIssue)
	for _, issue := range existingIssues {
		if rid := extractRoadyID(issue.Description); rid != "" {
			issueByRoadyID[rid] = issue
		}
	}

	// 2. Iterate through Roady tasks
	for _, task := range plan.Tasks {
		var targetIssue *linearIssue

		// A. Check if we already have a link in state
		if res, ok := state.TaskStates[task.ID]; ok {
			if ref, ok := res.ExternalRefs["linear"]; ok {
				// Try to find by ID
				for _, issue := range existingIssues {
					if issue.ID == ref.ID {
						targetIssue = &issue
						break
					}
				}
			}
		}

		// B. Fallback to matching by description marker
		if targetIssue == nil {
			if issue, ok := issueByRoadyID[task.ID]; ok {
				targetIssue = &issue
			}
		}

		// C. Create issue if missing
		if targetIssue == nil {
			issue, err := s.createIssue(task)
			if err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("failed to create issue for %s: %v", task.ID, err))
				continue
			}
			targetIssue = issue
			result.LinkUpdates[task.ID] = planning.ExternalRef{
				ID:           issue.ID,
				Identifier:   issue.Identifier,
				URL:          issue.URL,
				LastSyncedAt: time.Now(),
			}
		}

		// 3. Map status from Linear to Roady
		newStatus := mapLinearStatus(targetIssue.State.Type, targetIssue.State.Name)
		currentStatus := planning.StatusPending
		if res, ok := state.TaskStates[task.ID]; ok {
			currentStatus = res.Status
		}

		if newStatus != currentStatus {
			result.StatusUpdates[task.ID] = newStatus
		}
	}

	return result, nil
}

func (s *LinearSyncer) query(query string, variables map[string]interface{}) (map[string]interface{}, error) {
	reqBody, _ := json.Marshal(map[string]interface{}{
		"query":     query,
		"variables": variables,
	})

	req, _ := http.NewRequest("POST", "https://api.linear.app/graphql", bytes.NewBuffer(reqBody))
	req.Header.Set("Authorization", s.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close() //nolint:errcheck // best-effort close on read body

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("linear api returned status %d: %s", resp.StatusCode, string(body))
	}

	var gqlResp struct {
		Data   map[string]interface{} `json:"data"`
		Errors []interface{}          `json:"errors"`
	}
	if err := json.Unmarshal(body, &gqlResp); err != nil {
		return nil, err
	}

	if len(gqlResp.Errors) > 0 {
		return nil, fmt.Errorf("linear graphql errors: %v", gqlResp.Errors)
	}

	return gqlResp.Data, nil
}

func (s *LinearSyncer) fetchTeamIssues() ([]linearIssue, error) {
	q := `query($teamId: String!) {
		team(id: $teamId) {
			issues {
				nodes {
					id
					identifier
					title
					description
					state {
						name
						type
					}
					url
				}
			}
		}
	}`
	data, err := s.query(q, map[string]interface{}{"teamId": s.teamID})
	if err != nil {
		return nil, err
	}

	team, ok := data["team"].(map[string]interface{})
	if !ok || team == nil {
		return nil, fmt.Errorf("team not found")
	}
	issuesData := team["issues"].(map[string]interface{})["nodes"]

	var issues []linearIssue
	marshaled, _ := json.Marshal(issuesData)
	_ = json.Unmarshal(marshaled, &issues)

	return issues, nil
}

func (s *LinearSyncer) createIssue(task planning.Task) (*linearIssue, error) {
	q := `mutation($teamId: String!, $title: String!, $description: String!) {
		issueCreate(input: { teamId: $teamId, title: $title, description: $description }) {
			success
			issue {
				id
				identifier
				title
				description
				state {
					name
					type
				}
				url
			}
		}
	}`

	desc := fmt.Sprintf("%s\n\n---\nroady-id: %s", task.Description, task.ID)
	data, err := s.query(q, map[string]interface{}{
		"teamId":      s.teamID,
		"title":       task.Title,
		"description": desc,
	})
	if err != nil {
		return nil, err
	}

	createData := data["issueCreate"].(map[string]interface{})
	if !createData["success"].(bool) {
		return nil, fmt.Errorf("failed to create issue")
	}

	var issue linearIssue
	marshaled, _ := json.Marshal(createData["issue"])
	_ = json.Unmarshal(marshaled, &issue)

	return &issue, nil
}

func extractRoadyID(desc string) string {
	if strings.Contains(desc, "roady-id: ") {
		parts := strings.Split(desc, "roady-id: ")
		if len(parts) > 1 {
			return strings.TrimSpace(parts[1])
		}
	}
	return ""
}

func mapLinearStatus(linearType string, linearName string) planning.TaskStatus {
	switch linearType {
	case "completed":
		return planning.StatusDone
	case "started":
		return planning.StatusInProgress
	case "canceled":
		return planning.StatusBlocked // Or some other appropriate mapping
	case "backlog", "unstarted", "triage":
		return planning.StatusPending
	default:
		return planning.StatusPending
	}
}

type workflowState struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

// Push sends status only. PushFields is preferred by the host when
// available; this remains for hosts that predate field support.
func (s *LinearSyncer) Push(taskID string, status planning.TaskStatus) error {
	return s.PushFields(taskID, domainPlugin.TaskFields{Status: status})
}

// PushFields implements domainPlugin.FieldSyncer, mapping Roady's priority
// onto Linear's numeric scale alongside the workflow state.
func (s *LinearSyncer) PushFields(taskID string, fields domainPlugin.TaskFields) error {
	status := fields.Status
	// Find the issue by roady-id marker
	issues, err := s.fetchTeamIssues()
	if err != nil {
		return fmt.Errorf("fetch issues: %w", err)
	}

	var targetIssue *linearIssue
	for i := range issues {
		if extractRoadyID(issues[i].Description) == taskID {
			targetIssue = &issues[i]
			break
		}
	}

	if targetIssue == nil {
		return fmt.Errorf("issue not found for task %s", taskID)
	}

	// Get available workflow states for the team
	states, err := s.fetchWorkflowStates()
	if err != nil {
		return fmt.Errorf("fetch workflow states: %w", err)
	}

	// Map Roady status to Linear state type
	var targetStateType string
	switch status {
	case planning.StatusDone:
		targetStateType = "completed"
	case planning.StatusInProgress:
		targetStateType = "started"
	case planning.StatusPending:
		targetStateType = "unstarted"
	case planning.StatusBlocked:
		targetStateType = "canceled" // Linear doesn't have blocked, use canceled
	}

	// Find matching state
	var targetState *workflowState
	for i := range states {
		if states[i].Type == targetStateType {
			targetState = &states[i]
			break
		}
	}

	if targetState == nil {
		return fmt.Errorf("no workflow state found for status %s", status)
	}

	// Update the issue
	return s.updateIssue(targetIssue.ID, targetState.ID, linearPriority(fields.Priority))
}

// linearPriority maps Roady's three-value priority onto Linear's scale,
// where 0 means "no priority", 1 is urgent and 4 is low. An unset Roady
// priority returns -1 so the field is left untouched rather than being
// reset to "no priority" — sync must not clear a value a human set in
// Linear just because Roady has nothing to say about it.
func linearPriority(p planning.TaskPriority) int {
	switch p {
	case planning.PriorityHigh:
		return 2 // High
	case planning.PriorityMedium:
		return 3 // Medium
	case planning.PriorityLow:
		return 4 // Low
	default:
		return -1
	}
}

func (s *LinearSyncer) fetchWorkflowStates() ([]workflowState, error) {
	q := `query($teamId: String!) {
		team(id: $teamId) {
			states {
				nodes {
					id
					name
					type
				}
			}
		}
	}`
	data, err := s.query(q, map[string]interface{}{"teamId": s.teamID})
	if err != nil {
		return nil, err
	}

	team, ok := data["team"].(map[string]interface{})
	if !ok || team == nil {
		return nil, fmt.Errorf("team not found")
	}
	statesData := team["states"].(map[string]interface{})["nodes"]

	var states []workflowState
	marshaled, _ := json.Marshal(statesData)
	_ = json.Unmarshal(marshaled, &states)

	return states, nil
}

// updateIssue sets the workflow state, and the priority too when one is
// specified (priority < 0 means leave it alone).
func (s *LinearSyncer) updateIssue(issueID, stateID string, priority int) error {
	q := `mutation($issueId: String!, $stateId: String!) {
		issueUpdate(id: $issueId, input: { stateId: $stateId }) {
			success
		}
	}`
	vars := map[string]interface{}{
		"issueId": issueID,
		"stateId": stateID,
	}

	if priority >= 0 {
		q = `mutation($issueId: String!, $stateId: String!, $priority: Int!) {
			issueUpdate(id: $issueId, input: { stateId: $stateId, priority: $priority }) {
				success
			}
		}`
		vars["priority"] = priority
	}

	data, err := s.query(q, vars)
	if err != nil {
		return err
	}

	updateData := data["issueUpdate"].(map[string]interface{})
	if !updateData["success"].(bool) {
		return fmt.Errorf("failed to update issue state")
	}

	return nil
}

func main() {
	goplugin.Serve(&goplugin.ServeConfig{
		HandshakeConfig: infraPlugin.HandshakeConfig,
		Plugins: map[string]goplugin.Plugin{
			"syncer": &domainPlugin.SyncerPlugin{Impl: &LinearSyncer{}},
		},
	})
}
