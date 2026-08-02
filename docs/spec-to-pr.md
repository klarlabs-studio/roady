# Gating pull requests on drift

`roady drift detect` exits non-zero when it finds drift, so it works as a CI
gate with no wrapper. What makes it usable in practice is `--fail-on`.

## Why the threshold matters

Without a threshold, any drift at all fails the build — including a
low-severity note that nobody intends to act on this week. A gate that blocks
a merge on noise gets switched off, and then it is not a gate at all.

```bash
roady drift detect                    # fails on any drift
roady drift detect --fail-on high     # fails only on high and critical
roady drift detect --fail-on critical # fails only on critical
```

Everything found is still printed. `--fail-on` changes only the exit code, and
the command says explicitly when it found drift below the threshold rather
than exiting zero silently:

```
Detected 2 drift issues:
- [medium] (spec/MISMATCH) The Specification has changed since the Plan was last updated.
- [high] (plan/MISSING) Requirement 'Daily digest' is missing from Plan.

None at or above critical — not failing.
```

## A pull-request gate

```yaml
name: Drift
on:
  pull_request:
    branches: [main]

jobs:
  drift:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v5
        with:
          # Staleness detection counts commits since the plan changed, so it
          # needs history. A shallow clone reports the plan as current.
          fetch-depth: 0
      - name: Install roady
        run: go install github.com/felixgeelhaar/roady/cmd/roady@latest
      - name: Check drift
        run: roady drift detect --fail-on high
```

Start at `--fail-on critical` on an existing project and tighten once the
backlog is clear. Going straight to failing on everything, on a repository
that has drifted for months, produces one enormous red build that teaches the
team to ignore it.

## Opening a follow-up instead of blocking

On merge to `main`, drift that nobody accepted should become visible work
rather than a build failure nobody sees. The JSON output is designed for this:

```yaml
name: Drift follow-up
on:
  push:
    branches: [main]

jobs:
  drift:
    runs-on: ubuntu-latest
    permissions:
      contents: read
      issues: write
    steps:
      - uses: actions/checkout@v5
        with: { fetch-depth: 0 }
      - run: go install github.com/felixgeelhaar/roady/cmd/roady@latest
      - name: Detect
        id: drift
        run: roady drift detect -o json > drift.json || true
      - name: Open a follow-up
        env:
          GH_TOKEN: ${{ github.token }}
        run: |
          count=$(jq '[.issues[] | select(.severity == "high" or .severity == "critical")] | length' drift.json)
          [ "$count" -eq 0 ] && exit 0
          jq -r '"- [\(.severity)] \(.message)"' <<< "$(jq -c '.issues[]' drift.json)" > body.md
          gh issue create \
            --title "Drift after merge: $count issue(s) need a decision" \
            --body-file body.md \
            --label drift
```

`|| true` is deliberate: the detector exiting non-zero is the signal to open an
issue, not a reason to fail the job.

## Asking for the patch

`roady drift explain --patch` returns a request for a unified diff rather than
prose, with the file each issue points at included.

```bash
roady drift explain --patch          # the prompt, on stdout
roady drift explain --patch --json   # the whole request, for an agent
```

Only **code** drift is offered for patching. Spec drift means intent moved,
plan drift means the task list needs regenerating, and staleness means the
plan was abandoned — none of those are things a diff can settle, and handing
them to a model asking for a patch invites it to rewrite the specification or
the plan to match the code. That is the failure Roady exists to catch, so the
command refuses and says which remedy applies instead. Any such issues are
still listed in the prompt as context.

## Accepting drift

Drift that reflects a deliberate change is not a defect. `roady drift accept`
re-locks the spec snapshot, which clears intent drift and records the decision
in the audit log — so the trail shows the divergence was reviewed rather than
overlooked.
