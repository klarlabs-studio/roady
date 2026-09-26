package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Instruction files agents read at the start of every session. Tools and
// hooks make roady available; this block is what tells an agent to use it
// instead of the markdown plan it would otherwise write.
const (
	instructionsBegin = "<!-- roady:begin -->"
	instructionsEnd   = "<!-- roady:end -->"
)

// roadyInstructions is the block setup maintains. It is short on purpose:
// it is in the agent's context on every turn.
const roadyInstructions = instructionsBegin + `
<!-- Managed by ` + "`roady setup`" + `; edits between these markers are replaced on the next run. -->
## Planning lives in roady

This project keeps its plan in roady (` + "`.roady/`" + `), not in markdown files.

- **Before work:** ` + "`roady next`" + ` — the current task, why it exists, what done means.
- **New work, any size** (one task or a whole plan): ` + "`roady capture`" + ` with YAML or JSON
  on stdin (MCP: ` + "`roady_capture`" + `). A plan already written — plan mode, Kiro
  ` + "`tasks.md`" + `, an ExecPlan: ` + "`roady plan import <file>`" + `.
- **Never** create ROADMAP.md, TODO.md, PLAN.md or similar plan files, and do not
  track project work in a built-in todo list.
- **Lifecycle:** ` + "`roady task start <id>`" + ` → commit with ` + "`[roady:<id>]`" + ` in the
  message → ` + "`roady git sync`" + ` → ` + "`roady task check <id>`" + `.
- **Done means the task's acceptance check passes**, not that code was written.
  A task you cannot finish as specified: say so and ` + "`roady task block <id> -e \"<reason>\"`" + `.
` + instructionsEnd + "\n"

// writeInstructionBlock puts the roady block into root/name: replaced in
// place when the markers are already there, appended otherwise, the file
// created when missing. Everything outside the markers is left as it was.
func writeInstructionBlock(root, name string) (mcpRegistration, error) {
	path := filepath.Join(root, name)
	reg := mcpRegistration{Path: path}
	raw, err := os.ReadFile(path) // #nosec G304 -- a fixed instruction file name under the project root
	switch {
	case os.IsNotExist(err):
		reg.Created = true
	case err != nil:
		return reg, fmt.Errorf("read %s: %w", path, err)
	}

	var out []byte
	begin := bytes.Index(raw, []byte(instructionsBegin))
	end := bytes.Index(raw, []byte(instructionsEnd))
	switch {
	case begin >= 0 && end > begin:
		stop := end + len(instructionsEnd)
		if stop < len(raw) && raw[stop] == '\n' {
			stop++
		}
		if string(raw[begin:stop]) == roadyInstructions {
			reg.Unchanged = true
			return reg, nil
		}
		out = append(append(append([]byte{}, raw[:begin]...), roadyInstructions...), raw[stop:]...)
		reg.Updated = true
	case begin >= 0 || end >= 0:
		return reg, fmt.Errorf("%s has an unmatched roady marker, so it was left untouched; remove the stray %q or %q and re-run setup",
			path, instructionsBegin, instructionsEnd)
	default:
		out = append([]byte{}, raw...)
		if len(bytes.TrimSpace(out)) > 0 {
			out = append(bytes.TrimRight(out, "\n"), '\n', '\n')
		} else {
			out = nil
		}
		out = append(out, roadyInstructions...)
	}
	if err := os.WriteFile(path, out, 0o644); err != nil { // #nosec G306 -- instruction files are committed and shared
		return reg, fmt.Errorf("write %s: %w", path, err)
	}
	return reg, nil
}

func describeInstructions(r mcpRegistration) string {
	switch {
	case r.Created:
		return fmt.Sprintf("Created %s with roady's planning instructions", r.Path)
	case r.Unchanged:
		return fmt.Sprintf("roady instructions already current in %s", r.Path)
	case r.Updated:
		return fmt.Sprintf("Updated roady's instructions in %s", r.Path)
	default:
		return fmt.Sprintf("Added roady's planning instructions to %s", r.Path)
	}
}

// roadySkillPath is where Claude Code discovers project skills.
const roadySkillPath = ".claude/skills/roady-planning/SKILL.md"

// writeRoadySkill installs the planning skill. The file is roady's: it is
// rewritten when it differs, so upgrading roady upgrades the skill.
func writeRoadySkill(root string) (mcpRegistration, error) {
	path := filepath.Join(root, filepath.FromSlash(roadySkillPath))
	reg := mcpRegistration{Path: path}
	raw, err := os.ReadFile(path) // #nosec G304 -- fixed path under the project root
	switch {
	case os.IsNotExist(err):
		reg.Created = true
	case err != nil:
		return reg, fmt.Errorf("read %s: %w", path, err)
	case string(raw) == roadySkill:
		reg.Unchanged = true
		return reg, nil
	default:
		reg.Updated = true
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil { // #nosec G301 -- shared project directory
		return reg, fmt.Errorf("create %s: %w", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(roadySkill), 0o644); err != nil { // #nosec G306 -- committed and shared
		return reg, fmt.Errorf("write %s: %w", path, err)
	}
	return reg, nil
}

func describeSkill(r mcpRegistration) string {
	switch {
	case r.Created:
		return fmt.Sprintf("Installed the roady-planning skill at %s", r.Path)
	case r.Unchanged:
		return fmt.Sprintf("roady-planning skill already current at %s", r.Path)
	default:
		return fmt.Sprintf("Updated the roady-planning skill at %s", r.Path)
	}
}

// installInstructions writes the block into each named file, and reports.
func installInstructions(root string, files ...string) error {
	for _, name := range files {
		reg, err := writeInstructionBlock(root, name)
		if err != nil {
			return err
		}
		fmt.Printf("  ✓ %s\n", describeInstructions(reg))
	}
	return nil
}

var roadySkill = strings.TrimLeft(`
---
name: roady-planning
description: Plan and track work in roady instead of markdown files. Use when breaking work into tasks, writing or updating a plan, roadmap or TODO list, picking what to do next, or deciding whether a task is done.
---

# Planning with roady

This project's plan lives in roady (.roady/: spec, plan, state, audit log).
A markdown roadmap or TODO file goes stale the moment work starts; roady keeps
intent, tasks, progress and proof together, across sessions and agents.

## What to do next

Run `+"`roady next`"+`. It names your in-progress task — or the one to start — with
why it exists (a doc:line citation), what done means, what it depends on and
what it unblocks. Start it with `+"`roady task start <id>`"+`.

## Recording work: `+"`roady capture`"+`

One write for any amount of intent, from a single task to a whole plan. Items
are upserted by id, so the same document edits; it is all or nothing, and
re-sending it changes nothing. Preview with `+"`--dry-run`"+`.

`+"```bash"+`
roady capture <<'EOF'
features:
  - id: rate-limits
    title: Rate-limit the public API
    requirements:
      - id: token-bucket
        title: Per-key token bucket, 100 req/min
        priority: high
        check:
          run: go test ./internal/ratelimit -run TestBucket
tasks:
  - id: task-rate-limit-docs
    title: Document the limits and the 429 response
    requirement: token-bucket
    depends_on: [task-token-bucket]
EOF
`+"```"+`

- A requirement gets its task (task-<requirement id>) automatically.
- A task names a `+"`requirement`"+` (or a `+"`feature_id`"+`) and a `+"`title`"+` when new.
- Give each requirement a `+"`check`"+`: a command that passes when it is done
  (`+"`run:`"+`), or `+"`manual:`"+` for something a person must confirm.
- Adding or editing tasks keeps an approved plan approved. New or changed
  features and requirements change what was agreed: the plan returns to
  pending until a person runs `+"`roady plan approve`"+`.

Over MCP the same document goes to `+"`roady_capture`"+`.

## A plan that is already written

Plan mode, Kiro's tasks.md, a Codex ExecPlan or any markdown plan:
`+"`roady plan import <file>`"+` turns each step into a task that cites its line,
in order. Re-importing an edited plan updates the same tasks. With the roady
hooks installed, a plan approved in plan mode is imported automatically.

## Finishing a task

1. Commit with `+"`[roady:<task-id>]`"+` in the message, then `+"`roady git sync`"+`.
2. `+"`roady task check <task-id>`"+` runs the acceptance check and records the result.
3. Only a passing check is done. If the task cannot be done as specified,
   say so and `+"`roady task block <task-id> -e \"<reason>\"`"+` rather than
   weakening the check; changing the check of started work is refused unless
   a person overrides it.

## Don't

- Create ROADMAP.md, TODO.md, PLAN.md or similar files.
- Track project work in a built-in todo list.
- Mark work done without its check passing.
`, "\n")
