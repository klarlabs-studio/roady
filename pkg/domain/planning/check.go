package planning

import (
	"fmt"
	"strings"
	"time"
)

// Check is how a task shows it is done: a command whose exit status decides,
// or a named manual check that a person confirms.
//
// It exists because "done" used to be whatever the caller said. Agents report
// work as finished when it is not — the long-running-agent harness literature
// names premature completion as the main failure — and roady recorded the
// claim as fact. A check turns the claim into something that can be run again.
type Check struct {
	// Run is a shell command executed in the project root. Exit status 0 passes.
	Run string `json:"run,omitempty" yaml:"run,omitempty"`
	// Manual describes a check a person performs and confirms. It is never
	// satisfied by an agent on its own.
	Manual string `json:"manual,omitempty" yaml:"manual,omitempty"`
}

// IsZero reports whether no check is defined.
func (c *Check) IsZero() bool {
	return c == nil || (strings.TrimSpace(c.Run) == "" && strings.TrimSpace(c.Manual) == "")
}

// Validate rejects a check that is ambiguous about how it passes.
func (c *Check) Validate() error {
	if c == nil {
		return nil
	}
	run, manual := strings.TrimSpace(c.Run) != "", strings.TrimSpace(c.Manual) != ""
	switch {
	case run && manual:
		return fmt.Errorf("a check sets either run or manual, not both")
	case !run && !manual:
		return fmt.Errorf("a check needs run (a command) or manual (a description)")
	}
	return nil
}

// Kind names the check type for display and results.
func (c *Check) Kind() string {
	if c == nil {
		return ""
	}
	if strings.TrimSpace(c.Run) != "" {
		return CheckKindRun
	}
	return CheckKindManual
}

const (
	CheckKindRun    = "run"
	CheckKindManual = "manual"
)

// CheckResult is one execution of a task's check, kept as evidence.
type CheckResult struct {
	Kind     string    `json:"kind"`
	Command  string    `json:"command,omitempty"` // the command run, or the manual check's description
	Passed   bool      `json:"passed"`
	ExitCode int       `json:"exit_code"`
	Commit   string    `json:"commit,omitempty"` // HEAD when the check ran
	Dirty    bool      `json:"dirty,omitempty"`  // uncommitted changes were present
	By       string    `json:"by"`               // who ran or confirmed it
	At       time.Time `json:"at"`
	Duration string    `json:"duration,omitempty"`
	// Output is the tail of combined stdout and stderr, bounded so state.json
	// stays readable. Enough to see why a check failed, not a build log.
	Output string `json:"output,omitempty"`
}

// MaxCheckOutput bounds CheckResult.Output.
const MaxCheckOutput = 2048

// TailOutput keeps the last MaxCheckOutput bytes of out.
func TailOutput(out []byte) string {
	if len(out) <= MaxCheckOutput {
		return string(out)
	}
	return "…" + string(out[len(out)-MaxCheckOutput:])
}

// LastCheck returns the most recent check result, if any.
func (r TaskResult) LastCheck() (CheckResult, bool) {
	if len(r.Checks) == 0 {
		return CheckResult{}, false
	}
	return r.Checks[len(r.Checks)-1], true
}
