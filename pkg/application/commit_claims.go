package application

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/felixgeelhaar/roady/pkg/domain/drift"
	"github.com/felixgeelhaar/roady/pkg/domain/planning"
)

// Commits land without markers. A project that planned in roady and then
// committed its work plainly — mcp-go did, for 71 commits — looks to roady as
// if the plan fell behind the repository, when the plan described exactly
// that work. CommitClaims says which task, if any, claims a commit; drift
// counts only the ones none does, and roady git suggest / link let a person
// claim the rest.

// CommitClaims indexes the ways a task claims a commit: a [roady:<task>]
// marker in its subject naming a task in the plan, or its hash among a task's
// evidence.
type CommitClaims struct {
	tasks    map[string]bool
	evidence map[string]string // hash or hash prefix → task
}

var evidenceHash = regexp.MustCompile(`(?i)^(?:commit:\s*)?([0-9a-f]{7,40})$`)

// NewCommitClaims indexes plan and state; either may be nil.
func NewCommitClaims(plan *planning.Plan, state *planning.ExecutionState) *CommitClaims {
	c := &CommitClaims{tasks: map[string]bool{}, evidence: map[string]string{}}
	if plan != nil {
		for _, t := range plan.Tasks {
			c.tasks[t.ID] = true
		}
	}
	if state != nil {
		for id, r := range state.TaskStates {
			for _, e := range r.Evidence {
				if m := evidenceHash.FindStringSubmatch(strings.TrimSpace(e)); m != nil {
					c.evidence[strings.ToLower(m[1])] = id
				}
			}
		}
	}
	return c
}

// Claim returns the task that claims commit, if one does.
func (c *CommitClaims) Claim(commit drift.Commit) (string, bool) {
	for _, m := range roadyMarker.FindAllStringSubmatch(commit.Subject, -1) {
		if c.tasks[m[1]] {
			return m[1], true
		}
	}
	hash := strings.ToLower(commit.Hash)
	for prefix, task := range c.evidence {
		if strings.HasPrefix(hash, prefix) {
			return task, true
		}
	}
	return "", false
}

// countUnclaimed leaves claimed commits out of the staleness count.
func countUnclaimed(activity drift.RepoActivity, claims *CommitClaims) drift.RepoActivity {
	if activity.Commits == nil {
		return activity
	}
	unclaimed := 0
	for _, commit := range activity.Commits {
		if _, ok := claims.Claim(commit); !ok {
			unclaimed++
		}
	}
	activity.Claimed = len(activity.Commits) - unclaimed
	activity.CommitsSincePlan = unclaimed
	return activity
}

// CommitSuggestion is an unclaimed commit and the task it most likely served.
type CommitSuggestion struct {
	Commit    drift.Commit `json:"commit"`
	TaskID    string       `json:"task_id,omitempty"`
	TaskTitle string       `json:"task_title,omitempty"`
	// Shared are the words the subject and the task have in common: the
	// evidence for the guess, so a person can judge it.
	Shared []string `json:"shared,omitempty"`
}

// minSharedWords is how much a subject must have in common with a task to be
// suggested for it. One shared word matches too much ("server", "client").
const minSharedWords = 2

// Suggest lists commits since the plan was last updated that no task claims,
// newest first, each with the task whose id and title share most words with
// its subject. It writes nothing.
func (s *GitService) Suggest(limit int) ([]CommitSuggestion, error) {
	plan, err := s.repo.LoadPlan()
	if err != nil || plan == nil {
		return nil, fmt.Errorf("load plan: %w", err)
	}
	state, _ := s.repo.LoadState()
	commits, err := commitsSince(plan.UpdatedAt)
	if err != nil {
		return nil, err
	}
	claims := NewCommitClaims(plan, state)
	type taskWords struct {
		task  planning.Task
		words map[string]bool
	}
	// A requirement's task is titled "<requirement> (<feature>)", so every
	// task of a feature shares the feature's words; they say nothing about
	// which of them a commit served, and matched a plan commit to one.
	featureWords := map[string]map[string]bool{}
	if sp, err := s.repo.LoadSpec(); err == nil && sp != nil {
		for _, f := range sp.Features {
			featureWords[f.ID] = wordSet(f.Title)
		}
	}
	var tasks []taskWords
	for _, t := range plan.Tasks {
		words := wordSet(t.ID + " " + t.Title)
		for w := range featureWords[t.FeatureID] {
			delete(words, w)
		}
		tasks = append(tasks, taskWords{task: t, words: words})
	}
	out := []CommitSuggestion{}
	for _, c := range commits {
		if _, ok := claims.Claim(c); ok {
			continue
		}
		sug := CommitSuggestion{Commit: c}
		subject := wordSet(c.Subject)
		for _, tw := range tasks {
			var shared []string
			for w := range subject {
				if tw.words[w] {
					shared = append(shared, w)
				}
			}
			if len(shared) >= minSharedWords && len(shared) > len(sug.Shared) {
				sort.Strings(shared)
				sug.TaskID, sug.TaskTitle, sug.Shared = tw.task.ID, tw.task.Title, shared
			}
		}
		out = append(out, sug)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

// Link records a commit as evidence on a task, resolving ref to its full
// hash. linked is false when the task already had it.
func (s *GitService) Link(ref, taskID, actor string) (hash string, linked bool, err error) {
	hash, err = resolveCommit(ref)
	if err != nil {
		return "", false, err
	}
	plan, err := s.repo.LoadPlan()
	if err != nil || plan == nil {
		return "", false, fmt.Errorf("load plan: %w", err)
	}
	if _, ok := findTask(plan, taskID); !ok {
		return "", false, fmt.Errorf("no task %q in the plan", taskID)
	}
	state, err := s.repo.LoadState()
	if err != nil || state == nil {
		return "", false, fmt.Errorf("load state: %w", err)
	}
	evidence := "Commit: " + hash
	for _, e := range state.TaskStates[taskID].Evidence {
		if e == evidence {
			return hash, false, nil
		}
	}
	state.AddEvidence(taskID, evidence)
	if err := s.repo.SaveState(state); err != nil {
		return "", false, fmt.Errorf("save state: %w", err)
	}
	if s.taskSvc != nil && s.taskSvc.audit != nil {
		if err := s.taskSvc.audit.Log("task.evidence", actor, map[string]any{
			"task_id":  taskID,
			"evidence": evidence,
			"via":      "git-link",
		}); err != nil {
			return hash, true, fmt.Errorf("linked, but the audit log was not written: %w", err)
		}
	}
	return hash, true, nil
}

// commitsSince lists commits touching anything outside .roady/ since t,
// newest first — the commits staleness counts.
func commitsSince(t time.Time) ([]drift.Commit, error) {
	args := []string{"log", "--format=%H%x1f%s"}
	if !t.IsZero() {
		args = append(args, "--since="+t.Format(time.RFC3339))
	}
	args = append(args, "HEAD", "--", ".", ":(exclude).roady")
	out, err := runGit(args...)
	if err != nil {
		return nil, fmt.Errorf("read git log: %w", err)
	}
	var commits []drift.Commit
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if hash, subject, ok := strings.Cut(line, "\x1f"); ok && hash != "" {
			commits = append(commits, drift.Commit{Hash: hash, Subject: subject})
		}
	}
	return commits, nil
}

// resolveCommit turns a ref into the full hash of a commit that exists.
func resolveCommit(ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" || strings.HasPrefix(ref, "-") {
		return "", fmt.Errorf("name a commit to link")
	}
	out, err := runGit("rev-parse", "--verify", "--quiet", ref+"^{commit}")
	if err != nil {
		return "", fmt.Errorf("%q is not a commit in this repository", ref)
	}
	return strings.TrimSpace(out), nil
}

func runGit(args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	// #nosec G204 -- fixed subcommands; the only caller-supplied value is a ref, refused if it starts with "-"
	out, err := exec.CommandContext(ctx, "git", args...).Output()
	return string(out), err
}

// commitStopWords carry no meaning about which task a commit served.
var commitStopWords = map[string]bool{
	"feat": true, "fix": true, "chore": true, "docs": true, "test": true, "tests": true, "build": true,
	"refactor": true, "perf": true, "style": true, "task": true, "roady": true, "the": true, "and": true,
	"for": true, "with": true, "from": true, "into": true, "onto": true, "via": true, "add": true,
	"adds": true, "added": true, "make": true, "makes": true, "use": true, "uses": true, "when": true,
	"then": true, "that": true, "this": true, "not": true, "its": true, "are": true, "was": true,
	"has": true, "have": true, "also": true, "only": true, "now": true, "new": true, "all": true,
	"done": true,
}

// wordSet lowercases text into its meaningful words: three characters or
// more, stop words and pull-request numbers left out.
func wordSet(text string) map[string]bool {
	words := map[string]bool{}
	for _, w := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		if len(w) >= 3 && !commitStopWords[w] {
			words[w] = true
		}
	}
	return words
}
