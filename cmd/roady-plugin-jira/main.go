package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	jira "github.com/felixgeelhaar/jirasdk"
	"github.com/felixgeelhaar/jirasdk/core/issue"
	"github.com/felixgeelhaar/jirasdk/core/search"
	"github.com/felixgeelhaar/roady/pkg/domain/planning"
	domainPlugin "github.com/felixgeelhaar/roady/pkg/domain/plugin"
	infraPlugin "github.com/felixgeelhaar/roady/pkg/plugin"
	goplugin "github.com/hashicorp/go-plugin"
)

type JiraSyncer struct {
	client     *jira.Client
	projectKey string
	baseURL    string
}

func (s *JiraSyncer) Init(config map[string]string) error {
	baseURL := config["domain"]
	if baseURL == "" {
		baseURL = os.Getenv("JIRA_DOMAIN")
	}
	s.projectKey = config["project_key"]
	if s.projectKey == "" {
		s.projectKey = os.Getenv("JIRA_PROJECT_KEY")
	}
	email := config["email"]
	if email == "" {
		email = os.Getenv("JIRA_EMAIL")
	}
	apiToken := config["api_token"]
	if apiToken == "" {
		apiToken = os.Getenv("JIRA_API_TOKEN")
	}

	if baseURL == "" || s.projectKey == "" || email == "" || apiToken == "" {
		return fmt.Errorf("jira configuration missing (domain, project_key, email, api_token required)")
	}

	if !strings.HasPrefix(baseURL, "http") {
		baseURL = "https://" + baseURL
	}
	s.baseURL = baseURL

	client, err := jira.NewClient(
		jira.WithBaseURL(baseURL),
		jira.WithAPIToken(email, apiToken),
		jira.WithTimeout(30*time.Second),
	)
	if err != nil {
		return fmt.Errorf("create jira client: %w", err)
	}
	s.client = client

	return nil
}

func (s *JiraSyncer) Sync(plan *planning.Plan, state *planning.ExecutionState) (*domainPlugin.SyncResult, error) {
	ctx := context.Background()
	result := &domainPlugin.SyncResult{
		StatusUpdates: make(map[string]planning.TaskStatus),
		LinkUpdates:   make(map[string]planning.ExternalRef),
	}

	// 1. Fetch all issues for the project using iterator
	existingIssues, err := s.fetchProjectIssues(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetch jira issues: %w", err)
	}

	// Index by roady-id marker in description
	issueByRoadyID := make(map[string]*issue.Issue)
	for i := range existingIssues {
		iss := existingIssues[i]
		if rid := extractRoadyID(iss.GetDescriptionText()); rid != "" {
			issueByRoadyID[rid] = iss
		}
	}

	// 2. Iterate through Roady tasks
	for _, task := range plan.Tasks {
		var targetIssue *issue.Issue

		// A. Check state links
		if res, ok := state.TaskStates[task.ID]; ok {
			if ref, ok := res.ExternalRefs["jira"]; ok {
				for i := range existingIssues {
					iss := existingIssues[i]
					if iss.ID == ref.ID || iss.Key == ref.Identifier {
						targetIssue = iss
						break
					}
				}
			}
		}

		// B. Match by roady-id marker
		if targetIssue == nil {
			if iss, ok := issueByRoadyID[task.ID]; ok {
				targetIssue = iss
			}
		}

		// C. Create if missing
		if targetIssue == nil {
			iss, err := s.createIssue(ctx, task)
			if err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("create jira issue for %s: %v", task.ID, err))
				continue
			}
			targetIssue = iss
			result.LinkUpdates[task.ID] = planning.ExternalRef{
				ID:           iss.ID,
				Identifier:   iss.Key,
				URL:          fmt.Sprintf("%s/browse/%s", s.baseURL, iss.Key),
				LastSyncedAt: time.Now(),
			}
		}

		// 3. Map Status
		newStatus := mapJiraStatus(targetIssue.GetStatusName())
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

func (s *JiraSyncer) fetchProjectIssues(ctx context.Context) ([]*issue.Issue, error) {
	var allIssues []*issue.Issue

	// Use the JQL iterator for automatic pagination
	iter := s.client.Search.NewSearchJQLIterator(ctx, &search.SearchJQLOptions{
		JQL:        fmt.Sprintf("project = '%s'", s.projectKey),
		Fields:     []string{"summary", "description", "status"},
		MaxResults: 100,
	})

	for iter.Next() {
		iss := iter.Issue()
		allIssues = append(allIssues, iss)
	}

	if err := iter.Err(); err != nil {
		return nil, err
	}

	return allIssues, nil
}

func (s *JiraSyncer) createIssue(ctx context.Context, task planning.Task) (*issue.Issue, error) {
	description := fmt.Sprintf("%s\n\nroady-id: %s", task.Description, task.ID)

	fields := &issue.IssueFields{
		Project:   &issue.Project{Key: s.projectKey},
		Summary:   task.Title,
		IssueType: &issue.IssueType{Name: "Task"},
	}
	fields.SetDescriptionText(description)

	created, err := s.client.Issue.Create(ctx, &issue.CreateInput{
		Fields: fields,
	})
	if err != nil {
		return nil, err
	}

	// Fetch full issue to get status
	return s.client.Issue.Get(ctx, created.Key, &issue.GetOptions{
		Fields: []string{"summary", "description", "status"},
	})
}

func extractRoadyID(desc string) string {
	if strings.Contains(desc, "roady-id: ") {
		idx := strings.Index(desc, "roady-id: ")
		remaining := desc[idx+10:]
		// Take until newline or end
		if nlIdx := strings.Index(remaining, "\n"); nlIdx != -1 {
			return strings.TrimSpace(remaining[:nlIdx])
		}
		return strings.TrimSpace(remaining)
	}
	return ""
}

func mapJiraStatus(jiraName string) planning.TaskStatus {
	name := strings.ToLower(jiraName)
	switch name {
	case "done", "resolved", "closed", "verified":
		return planning.StatusDone
	case "in progress", "started", "active", "doing":
		return planning.StatusInProgress
	case "blocked", "on hold":
		return planning.StatusBlocked
	default:
		return planning.StatusPending
	}
}

// Push sends status only. PushFields is preferred by the host when
// available; this remains for hosts that predate field support.
func (s *JiraSyncer) Push(taskID string, status planning.TaskStatus) error {
	return s.PushFields(taskID, domainPlugin.TaskFields{Status: status})
}

// PushFields implements domainPlugin.FieldSyncer, setting Jira's priority
// field alongside the workflow transition.
func (s *JiraSyncer) PushFields(taskID string, fields domainPlugin.TaskFields) error {
	status := fields.Status
	ctx := context.Background()

	// Find the issue by roady-id marker
	issues, err := s.fetchProjectIssues(ctx)
	if err != nil {
		return fmt.Errorf("fetch issues: %w", err)
	}

	var targetIssue *issue.Issue
	for _, iss := range issues {
		if extractRoadyID(iss.GetDescriptionText()) == taskID {
			targetIssue = iss
			break
		}
	}

	if targetIssue == nil {
		return fmt.Errorf("issue not found for task %s", taskID)
	}

	// Map Roady status to common Jira transition IDs
	// Note: These are standard IDs for Jira Software simplified workflow
	// Users may need to configure custom IDs for their workflow
	var transitionID string
	switch status {
	case planning.StatusDone:
		transitionID = "31" // "Done" in simplified workflow
	case planning.StatusInProgress:
		transitionID = "21" // "In Progress" in simplified workflow
	case planning.StatusPending:
		transitionID = "11" // "To Do" in simplified workflow
	case planning.StatusBlocked:
		// Blocked typically doesn't have a standard transition
		return fmt.Errorf("status 'blocked' requires custom workflow configuration")
	default:
		return fmt.Errorf("unsupported status: %s", status)
	}

	err = s.client.Issue.DoTransition(ctx, targetIssue.Key, &issue.TransitionInput{
		Transition: &issue.Transition{ID: transitionID},
	})
	if err != nil {
		return fmt.Errorf("do transition (ID=%s): %w - if this fails, your Jira workflow may use different transition IDs", transitionID, err)
	}

	// Priority is set separately from the transition. A failure here is
	// reported but does not undo the transition that already succeeded —
	// partially-applied is better than silently reverting a status the
	// caller asked for.
	if name := jiraPriorityName(fields.Priority); name != "" {
		if pErr := s.client.Issue.Update(ctx, targetIssue.Key, &issue.UpdateInput{
			Fields: map[string]interface{}{
				"priority": map[string]string{"name": name},
			},
		}); pErr != nil {
			return fmt.Errorf("status updated, but setting priority %q failed: %w", name, pErr)
		}
	}

	return nil
}

// jiraPriorityName maps Roady's three-value priority onto Jira's default
// scheme. An unset Roady priority returns "" so the field is left untouched
// rather than being reset — sync must not clear a value a human set in Jira
// just because Roady has nothing to say about it.
//
// Jira priority schemes are configurable per project; these are the defaults.
// A project using custom names needs its own mapping.
func jiraPriorityName(p planning.TaskPriority) string {
	switch p {
	case planning.PriorityHigh:
		return "High"
	case planning.PriorityMedium:
		return "Medium"
	case planning.PriorityLow:
		return "Low"
	default:
		return ""
	}
}

func main() {
	goplugin.Serve(&goplugin.ServeConfig{
		HandshakeConfig: infraPlugin.HandshakeConfig,
		Plugins: map[string]goplugin.Plugin{
			"syncer": &domainPlugin.SyncerPlugin{Impl: &JiraSyncer{}},
		},
	})
}
