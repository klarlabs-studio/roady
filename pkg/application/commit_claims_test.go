package application_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/felixgeelhaar/roady/pkg/application"
	"github.com/felixgeelhaar/roady/pkg/domain"
	"github.com/felixgeelhaar/roady/pkg/domain/drift"
	"github.com/felixgeelhaar/roady/pkg/domain/planning"
	"github.com/felixgeelhaar/roady/pkg/domain/spec"
)

// mcp-go committed its planned work without markers, so roady read the plan
// as 71 commits behind. A marker for a task in the plan, or the commit among a
// task's evidence, claims a commit; staleness counts only the rest.
func TestUntaggedCommitClaimsLeaveStalenessToUnclaimedWork(t *testing.T) {
	plan := &planning.Plan{Tasks: []planning.Task{{ID: "rev1"}, {ID: "rev2"}}}
	state := planning.NewExecutionState("p")
	state.AddEvidence("rev2", "Commit: bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	state.AddEvidence("rev1", "abc1234")
	claims := application.NewCommitClaims(plan, state)

	for _, tc := range []struct {
		commit drift.Commit
		task   string
	}{
		{drift.Commit{Hash: "1111111", Subject: "feat: certify [roady:rev1]"}, "rev1"},
		{drift.Commit{Hash: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Subject: "feat: stateless"}, "rev2"},
		{drift.Commit{Hash: "abc1234ffff", Subject: "short evidence prefix"}, "rev1"},
		{drift.Commit{Hash: "2222222", Subject: "marker for a task not in the plan [roady:gone]"}, ""},
		{drift.Commit{Hash: "3333333", Subject: "fix: typo"}, ""},
	} {
		got, _ := claims.Claim(tc.commit)
		if got != tc.task {
			t.Errorf("%q claimed by %q, want %q", tc.commit.Subject, got, tc.task)
		}
	}

	activity := drift.RepoActivity{CommitsSincePlan: 12}
	for i := 0; i < 12; i++ {
		activity.Commits = append(activity.Commits, drift.Commit{Hash: strings.Repeat("c", 40), Subject: "feat: work [roady:rev1]"})
	}
	activity.Commits[0].Subject = "fix: unplanned"
	issues := drift.NewDriftDetector().DetectStalenessDrift(&planning.Plan{ID: "p", Tasks: plan.Tasks}, application.CountUnclaimedForTest(activity, claims), time.Now())
	if len(issues) != 0 {
		t.Errorf("11 of 12 commits are claimed; staleness still reported: %+v", issues)
	}
}

// suggest names the task an unmarked commit most likely served; link records
// it, after which suggest no longer lists it.
func TestUntaggedCommitSuggestAndLink(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	// Staleness counts commits that change something outside .roady/, so
	// each commit touches a file.
	commit := func(n int, msg string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte(strings.Repeat("x", n)), 0o600); err != nil {
			t.Fatal(err)
		}
		run("add", "f.txt")
		run("commit", "-q", "-m", msg)
	}
	run("init", "-q")
	commit(1, "docs: protocol revisions summary")
	commit(2, "feat(protocol): certify 2025-03-26 streamable http")
	commit(3, "feat: stateless default [roady:rev4]")
	t.Chdir(dir)

	repo := &MockRepo{
		// Task titles end with their feature's title, as roady writes them;
		// those shared words must not decide a match.
		Spec: &spec.ProductSpec{ID: "s", Features: []spec.Feature{{ID: "revisions", Title: "Protocol revisions"}}},
		Plan: &planning.Plan{ID: "p", ApprovalStatus: planning.ApprovalApproved, Tasks: []planning.Task{
			{ID: "rev1-2025-03-26", Title: "Certify 2025-03-26 streamable http (Protocol revisions)", FeatureID: "revisions"},
			{ID: "rev4", Title: "Stateless rewrite (Protocol revisions)", FeatureID: "revisions"},
		}},
		State:  planning.NewExecutionState("p"),
		Policy: &domain.PolicyConfig{MaxWIP: 5},
	}
	audit := application.NewAuditService(repo)
	git := application.NewGitService(repo, application.NewTaskService(repo, audit, application.NewPolicyService(repo)))

	sugs, err := git.Suggest(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(sugs) != 2 {
		t.Fatalf("suggestions %+v, want the two unmarked commits", sugs)
	}
	certify := sugs[0]
	if certify.TaskID != "rev1-2025-03-26" || !strings.Contains(strings.Join(certify.Shared, ","), "2025") {
		t.Errorf("certify commit suggested %+v", certify)
	}
	if sugs[1].TaskID != "" {
		t.Errorf("a commit sharing only the feature's words was matched to %s", sugs[1].TaskID)
	}

	hash, linked, err := git.Link(certify.Commit.Hash[:8], "rev1-2025-03-26", "tester")
	if err != nil || !linked || hash != certify.Commit.Hash {
		t.Fatalf("link: %s %v %v", hash, linked, err)
	}
	if _, again, _ := git.Link(hash, "rev1-2025-03-26", "tester"); again {
		t.Error("linking the same commit twice added it twice")
	}
	if _, _, err := git.Link("HEAD", "no-such-task", "tester"); err == nil {
		t.Error("linked to a task that is not in the plan")
	}
	if _, _, err := git.Link("--all", "rev4", "tester"); err == nil {
		t.Error("an option was accepted as a commit")
	}
	sugs, _ = git.Suggest(0)
	if len(sugs) != 1 || sugs[0].Commit.Subject != "docs: protocol revisions summary" {
		t.Errorf("after linking: %+v", sugs)
	}
}
