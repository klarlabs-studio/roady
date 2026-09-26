package application

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/felixgeelhaar/roady/pkg/domain"
)

// DefaultAuditBaseline is the revision audit verification compares against
// when the caller names none. HEAD catches a log truncated in the working copy
// since the last commit; CI should pass the protected branch (origin/main), so
// a truncation that was itself committed on a feature branch is caught too.
const DefaultAuditBaseline = "HEAD"

// CommittedFile returns the content of absPath as committed at ref, read with
// `git show`.
//
// ok is false, with a reason, whenever there is no committed copy to compare
// against: the directory is not a git repository, ref does not resolve, or the
// file is not tracked at ref. Those are not findings about the log, so they are
// reported as the absence of a baseline rather than as errors.
func CommittedFile(dir, absPath, ref string) (data []byte, ok bool, reason string) {
	if ref == "" {
		return nil, false, "no baseline requested"
	}
	rel, err := filepath.Rel(dir, absPath)
	if err != nil || strings.HasPrefix(rel, "..") {
		return nil, false, fmt.Sprintf("%s is outside %s", absPath, dir)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	// "<rev>:./<path>" resolves the path relative to -C, so this works from a
	// project nested anywhere inside the repository.
	// #nosec G204 -- ref and path are passed as a single argv element, never through a shell
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "show", ref+":./"+filepath.ToSlash(rel))
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, false, fmt.Sprintf("no committed copy at %s (%s)", ref, firstLine(msg))
	}
	return stdout.Bytes(), true, ""
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// BaselineCheck is the outcome of comparing the log with a committed copy.
type BaselineCheck struct {
	// Ref is the revision compared against.
	Ref string `json:"ref"`
	// Checked is false when there was no committed copy to compare with; Reason
	// then says why. An unchecked baseline is not a finding, but it means
	// truncation could not be ruled out, and callers must not claim it was.
	Checked    bool                    `json:"checked"`
	Reason     string                  `json:"reason,omitempty"`
	Violations []domain.ChainViolation `json:"violations,omitempty"`
}

// VerifyAgainstCommitted compares the working log at eventsPath with the copy
// committed at ref in the repository containing root.
func (s *AuditService) VerifyAgainstCommitted(root, eventsPath, ref string) (BaselineCheck, error) {
	check := BaselineCheck{Ref: ref}
	baseline, ok, reason := CommittedFile(root, eventsPath, ref)
	if !ok {
		check.Reason = reason
		return check, nil
	}
	violations, err := s.VerifyAgainstBaseline(baseline, ref)
	if err != nil {
		return check, err
	}
	check.Checked = true
	check.Violations = violations
	return check, nil
}
