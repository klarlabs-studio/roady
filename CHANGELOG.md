# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.22.0] - 2026-08-09

Closes #87 in full: the MCP surface is now groupable, named after the CLI, and
honest about the project's health.

Breaking on the 0.x line, so this is a minor bump — twenty MCP tools are
renamed and no aliases are kept. See the table below before upgrading a client
with hardcoded tool names.

### ⚠ Breaking — MCP tool names now mirror the CLI

The CLI is organised by noun (`roady spec add`, `roady drift detect`,
`roady task assign`); the MCP surface was a flat verb-first list
(`roady_add_feature`, `roady_detect_drift`, `roady_assign_task`). Knowledge of
one did not transfer to the other, and an agent that knew the CLI could not
guess the tool name — or tell whether it existed at all (#87, item 4).

Twenty tools are renamed to `roady_<noun>_<verb>`. **No aliases are kept**:
this repo's own history shows why (`1a9894a` had to drop a previous set of
deprecated aliases, and #87 item 3 was filed because a superseded tool was
still advertised). A deprecation that is documented but still registered is
the thing being fixed, so shipping one here would be self-defeating.

| before | after |
| --- | --- |
| `roady_add_feature` | `roady_spec_add` |
| `roady_explain_spec` | `roady_spec_explain` |
| `roady_review_spec` | `roady_spec_review` |
| `roady_get_spec` | `roady_spec_get` |
| `roady_generate_plan` | `roady_plan_generate` |
| `roady_approve_plan` | `roady_plan_approve` |
| `roady_update_plan` | `roady_plan_update` |
| `roady_get_plan` | `roady_plan_get` |
| `roady_suggest_priorities` | `roady_plan_prioritize` |
| `roady_detect_drift` | `roady_drift_detect` |
| `roady_accept_drift` | `roady_drift_accept` |
| `roady_explain_drift` | `roady_drift_explain` |
| `roady_record_semantic_drift` | `roady_drift_record_semantic` |
| `roady_transition_task` | `roady_task_transition` |
| `roady_assign_task` | `roady_task_assign` |
| `roady_dispatch_task` | `roady_task_dispatch` |
| `roady_get_state` | `roady_state_get` |
| `roady_get_snapshot` | `roady_snapshot_get` |
| `roady_get_usage` | `roady_usage_get` |
| `roady_check_policy` | `roady_policy_check` |

Every new name matches an existing CLI verb where one exists —
`roady_plan_prioritize` from `roady plan prioritize`, not the more literal
`roady_priorities_suggest`.

### Added

- **`ROADY_MCP_TOOLS` selects which tool groups the MCP server advertises.**
  All seventy tools were registered on every session, and a client pays for
  each one in its prompt whether or not the project has a rate card or a debt
  ledger (#87, item 2). `ROADY_MCP_TOOLS=core` advertises 30 instead of 70;
  groups are `core`, `cost`, `team`, `org`, `debt`, `deps`, `plugin`, `sync`,
  `analytics`, `audit`. Unset (or `all`) registers everything, so trimming is
  opt-in and no existing client loses a tool it already calls. An unknown
  group name fails startup rather than quietly starting a smaller server.

### Fixed

- **An MCP handler's diagnosis reached the client as a bare `-32603 internal
  error`.** The cause was computed and then discarded, so a caller was told
  something failed but never what — which is how an invalid `spec.yaml`
  presented as a broken server (#84, #86).
- **`roady_status` answered normally while `spec.yaml` was unparseable.** It
  reads `plan.json` and `state.json` and never unmarshals the spec, so the
  server looked healthy while exactly one tool looked flaky, and the reporter
  concluded roady was broken rather than their file. Status now carries the
  spec's health and names `roady_spec_validate` as the remedy (#87 item 5,
  #89).

### Security / CI

- **The nox scanner was running degraded and reporting success.** nox runs
  only the plugins named in `plugins.required`; the list was absent, so the
  taint-analysis and reachability plugins never ran and their findings were
  absent rather than clean. Required plugins are now declared, a missing one
  fails the run, and the findings the working scanner surfaced are cleared
  (#78, #79).
- **The security gate disarmed itself.** A repository that waived every
  finding cleanly ended up with nothing to compare against, so the gate
  stopped firing at exactly the point it was meant to start protecting (#81).
- **`secrets: inherit` passed nothing across the owner boundary**, so the
  remediation workflow ran without `NOX_TOKEN` and could not push. Passed
  explicitly (#80).
- Added a changed-files gate on pull requests (#82); restored scanning of
  `stderr.go`, excluded only because taint fingerprints were unstable before
  nox 0.7.3 (#83); pinned the scanner to a SHA-pinned action release (#88).

### Changed

- `go.klarlabs.de/statekit` v1.8.0 → v1.13.2 (#90).

### Documentation

- CLAUDE.md corrected where it had drifted from the code: the MCP tool list
  named sixteen tools when the server had grown to seventy — every one of the
  sixteen still correct, and the list four-fifths incomplete, which reads the
  same as complete. Replaced with the command that asks the code (#85).

## [0.21.0] - 2026-08-03

Adopting Roady in an existing project left its derived state describing the
template it was initialised from, and every check said fine.

A minor release rather than a patch: it adds `roady spec lock`, `roady_spec_lock`,
and MCP schema 3.5.0.

### Fixed — derived state was never reconciled when the spec was replaced (#77)

`roady init` writes `spec.yaml`, `spec.lock.json` and `state.json` together, so
they agree by construction. The ordinary way to adopt Roady in an existing
project is then to replace the generated spec — and nothing re-derived the
other two. The lock kept the template's identity while being the baseline every
drift check compares against, so drift was measured against a spec the project
never had, and `spec validate` answered "Spec is valid and correctly formatted."

- **New: `roady spec lock`** and `roady_spec_lock`. Re-captures the baseline
  from the current spec and reconciles `state.json`'s `project_id`. A no-op
  when they already agree, so it is safe in a script or an agent loop. There
  was previously no supported way to do either — the reporter regenerated the
  lock by hand with a Python one-liner.
- **`spec validate` now says what it did not check.** It validates shape, which
  is a defensible split, but a bare success read as "everything here is fine".
  It now warns when the lock or the execution state disagrees with the spec,
  and names the remedy. The command still succeeds: disagreement is drift's
  verdict to render, and `roady drift detect` reports it as `spec/MISMATCH`.

MCP schema 3.5.0.

## [0.20.0] - 2026-08-03

One audit-chain verifier instead of two that disagreed, and the surfaces
telling the truth about what Roady does.

Nothing here is a new capability. It is the set of places where Roady said one
thing and did another: two verifiers giving opposite verdicts on the same log,
a tool reading a different source than the command it mirrors, a metric that
could not distinguish "none" from "not measured", and help text describing a
model integration removed two releases ago.

### Fixed — the CLI and docs advertised a model integration that was removed

Roady stopped calling language models in v0.15.0 and lost its web dashboard in
v0.14.0. The code changed; the things that tell people what Roady does did not.

In the binary:

- `--ai` read "Use AI to decompose the spec into tasks". It prints a prompt for
  you to run. `plan prioritize`, `plan smart-decompose` and `spec review` all
  claimed to be "AI-powered", as did three MCP tool descriptions — so agents
  were told the same thing.
- `--reconcile` advertised "Use AI to semanticly deduplicate and reconcile the
  spec". That flag now only returns an error explaining it was removed, so its
  help described a capability that cannot run.

In the docs:

- `installation.md` documented `ROADY_AI_PROVIDER`, `OPENAI_API_KEY`,
  `ANTHROPIC_API_KEY` and an Ollama setup. Following it configured nothing —
  none of those variables has a reader.
- `mcp-guide.md` told MCP users to export provider settings and pointed at
  `docs/ai-configuration.md`, which does not exist. It now lists the variables
  that are actually read, each checked against the source.
- `advanced.md` and `governance-checklist.md` described provider overrides and
  a `roady ai configure` command that never existed.
- `ddd-insights.md` and `ddd-refactor-phase4.md` propose that removed design at
  length; marked historical rather than rewritten, since they are records of a
  decision and their structural observations still hold.

### Fixed — roady_timeline read a different source than the CLI

`roady status timeline` reads the raw event log; `roady_timeline` read the
event-sourced projection, which carries different fields. The two surfaces
described the same history differently — introduced by the change that claimed
to close the parity gap. Both now read the same source.

### Removed — dead AI telemetry

`GetAITelemetry` existed twice, on both audit services, and was called from
nowhere. It aggregates token counts from event types Roady stopped producing in
0.15.0, so on any project since it returns zeros regardless of what happened —
a metric that cannot distinguish "no usage" from "not measured". Removed rather
than reconciled.

### Fixed — one audit-chain verifier, not two

Two services write to `events.jsonl` and both answer "is this log intact?".
They disagreed. `AuditService` verified it as a hash-linked graph;
`EventSourcedAuditService` still required each entry to follow the previous
line and so reported tampering for the branch-and-merge shape concurrent
appends legitimately produce — the case `AuditService` was fixed for in 0.14.0
and this copy never received. On the same file at the same moment the CLI said
"intact and verified" while MCP reported a violation.

An audit chain whose verdict depends on which code path asked is not evidence
of anything.

- The graph semantics now live once, in `domain.VerifyChain`. Both services map
  their event type into `domain.ChainEntry` and delegate; hashing stays with
  each type, since they hash different fields.
- `events.BaseEvent` gains `HashMatches` and `Verifiable`, so the event-sourced
  side can also tell an unknown hash algorithm from tampering rather than
  calling both "possible tampering".
- The old copy also built finding indices with `string(rune('0'+i))`, which
  emits punctuation past index 9.
- Pinned by tests that a branching and an out-of-order log verify clean, and
  that a removed parent still does not — mutation-checked by restoring the
  linear requirement, which fails them.

## [0.19.0] - 2026-08-02

Semantic drift, and the surfaces reaching parity.

Roady could tell you a task was missing or an id orphaned; it could not tell
you whether the code that exists still does what the requirement asked for.
That question needs a reader, so Roady now frames it and the caller's model
answers it. Alongside that, an enumeration of every CLI command against the
tool surface closed the operations an agent could not reach — it could read a
project but not maintain one.

Three of the fixes below came from comparing the CLI and MCP against each
other rather than from either one failing.

### Added — semantic drift

The structural checks decide by comparing artifacts: a task is missing, an id
is orphaned, a file does not exist. None of them can tell whether code that
exists still does what the requirement asked for. "Sessions expire after 30
minutes" is structurally satisfied by an implementation that expires them after
thirty days.

Answering that needs a reader, so Roady frames the question and hands it over —
the requirement's own words, where the work landed, the doc:line to check
against — and the caller's model answers it. Roady runs no inference; the
caller has the working tree in view and Roady does not.

- `roady drift semantic` and `roady_semantic_drift` build the request and
  return the requirements it covers.
- `roady_record_semantic_drift` takes the judgements back. Divergences become
  drift issues under the new `SEMANTIC` category; agreement records nothing,
  because this reports drift rather than a tally of what was checked.
- A judgement naming a requirement that was never asked about is discarded — a
  model returning ids it invented is a known failure — and a divergence
  reported without an explanation is refused, since it cannot be acted on.
- Only requirements something claims to implement are asked about. A
  requirement with no task is structural drift the other checks already report.
- The issue hint discloses that a language model reached the verdict, so nobody
  reads it as mechanically established.

### Added — MCP parity with the CLI

An enumeration of all 105 CLI commands against the tool surface found 12
operations an agent could not reach. Eleven are now tools; the rest are
inherently local (shell completion, the TUI, the server itself). An agent could
read a project but not maintain one: approve a plan and never reject it, prune
nothing, validate nothing, recover from nothing.

- `roady_report` — the stakeholder progress report as markdown, self-contained
  html, or json. This is the answer to "keep leadership informed", and an agent
  could not produce one.
- `roady_spec_analyze` — build the spec from a directory of documents. An agent
  could read, amend and explain a spec but never create one, so the entry point
  to the whole workflow was unreachable.
- `roady_plan_prune`, `roady_plan_reject`, `roady_audit_verify`,
  `roady_spec_validate`, `roady_spec_import`, `roady_state_rebuild`,
  `roady_timeline`, `roady_debt_history`, `roady_debt_score`.
- Every registered tool is asserted to carry behaviour annotations, so nothing
  ships without telling a client whether it writes.

MCP schema 3.4.0; 69 tools.

### Fixed — the CLI and MCP gave different answers

Three places where one question had two implementations, found by comparing the
surfaces rather than by either one failing.

- **Audit chain verification.** `roady_audit_verify` first used
  `EventSourcedAuditService`, which still checks the log as a strict linear
  sequence and so reports tampering for the branch-and-merge shape concurrent
  appends legitimately produce — the case `AuditService` was fixed for in
  0.14.0. On the same project at the same moment the CLI said "intact and
  verified" while MCP reported a violation. The handler now uses the verifier
  the CLI uses, and a test asserts the two agree. (`EventSourcedAuditService`
  still carries the stale copy; that is recorded, not yet fixed.)
- **The `since` window.** The CLI parsed with `fmt.Sscanf`, which stops at the
  first non-digit and reports no error, so `--since 7xd` silently meant seven
  days there while the identical string was rejected over MCP. Both now use
  `application.ParseSince`, which refuses malformed input rather than guessing.
- **Report titles.** A relative `project_path` titled the report `.`.

### Fixed — plan/STALE read a timestamp nothing maintained (#76)

`plan.json` carries an `updated_at` that drift reads to decide whether the
repository has left the plan behind. Every writer had to set it by hand and
approving a plan did not, so a plan could be rewritten while its timestamp
stayed months old. A field only some writers maintain is a field that lies.

- Stamped in `SavePlan`, the one funnel every plan mutation passes through, so
  no present or future writer can forget. `CreatedAt` is untouched; reads do
  not disturb it.
- The message reported what it could not know. "The plan has not changed in N
  days" was disprovable by looking at a file you had just edited; it now says
  what it measured — last updated N days ago, M commits since.
- The hint offered one remedy, regenerating, which is the one action a curated
  plan must not take: the reconciler keeps tasks it did not propose but
  replaces any task whose id matches a spec requirement, so hand-written titles
  and descriptions on those are lost. The non-destructive routes come first
  now, and regenerating is stated with its cost.
- Not taken: the reported suggestion to fall back to the file's mtime. A fresh
  clone rewrites mtimes, so that would read as "updated now" on every checkout
  and suppress the detector exactly where it matters most.

### Fixed — .gitignore hid the workspace Roady says you should commit

Roady's own `.gitignore` carried a blanket `.roady/` rule. The state files were
tracked only because they predated it; anything written afterwards —
`policy.yaml`, `rates.yaml`, a sub-project under `projects/` — would have been
swallowed silently, in the one repository where that should be caught first.
Only genuinely machine-local files are ignored now.

### Added — workspace member repositories

`.roady/org.yaml` has carried a `repos:` list since the type was introduced
and nothing ever read it. A workspace could declare its members and Roady
would quietly walk the tree instead — missing any repository outside the root,
silently including any scratch checkout inside it, and answering a different
question from the one asked.

- A declared list is now authoritative, and can name repositories outside the
  workspace directory, which is the reason to declare them at all. Relative
  and absolute paths both work.
- Discovery remains the behaviour when nothing is declared, so a workspace
  that never needed the distinction does not have to start.
- `roady org status` and `roady org drift` enumerate from the membership,
  covering each member's root project and its sub-projects.
- A declared member that cannot be reached — missing, not a directory, or
  holding no Roady project — is reported with the reason and how to fix it,
  and carried on the reports as a warning. A cross-repo view that silently
  omits a repository looks like coverage, which is the failure declaring
  members exists to prevent.
- New: `roady org members`, and `roady_org_members` over MCP.

## [0.18.0] - 2026-08-02

Five bugs found by running Roady on a real 118-feature project, all reported
against 0.13.2. Four of them shared a shape: Roady knew enough to get the
answer right, and stored or reported something else without saying so.

### Fixed — generated ids are safe to use as ids (#74)

Ids were derived by lowercasing a title and replacing spaces, which passed
through everything else — including `/`, `+`, `(`, `)` and the em-dash. 84 of
118 feature ids in the reported spec were affected. A feature id becomes a
path component under `.roady/projects/<name>/`, a URL segment, and an argument
typed into a shell, so `/` was a latent path-traversal shape and
`phase-a-—-pilot-finalization-(2-weeks)` could not be pasted into
`roady task start` without quoting.

- Letters and digits survive; everything else becomes a separator. Latin
  diacritics fold to ASCII so `Prüfung` and `Prufung` are one id rather than
  two, scripts with no ASCII equivalent are kept, and length is bounded
  because ids reach filenames.
- A title of pure punctuation falls back to a hash of the title rather than an
  empty id colliding with every other such feature.
- **Existing ids are preserved.** Because ids derive from titles, fixing the
  derivation would have renamed every affected feature on the next
  `spec analyze` and orphaned its tasks — trading the bug for a worse one.
  Re-analysis matches features by title and keeps the id a feature already
  has; only new features take the current form.

### Fixed — plan writes resolve the feature they name (#73)

Drift matches tasks to features by id, so a task whose `feature_id` held a
title was reported as an orphan the moment it was stored: the plan was born
drifted and nothing said so. 31 of 49 tasks in the reported plan were affected.

- Plan writes now resolve each link against the spec, accepting a feature's
  id, its title, or either one slugified — which also repairs ids written
  before the slugifier was fixed.
- A link matching nothing is left exactly as written, because guessing would
  attach the task to the wrong feature and that is harder to notice than an
  orphan. It is returned to the caller instead, since the agent that wrote it
  is the one able to correct it and is about to move on.

### Fixed — add_feature writes to the project, not the working directory (#71)

`roady_add_feature` resolved `docs/backlog.md` relative to the process working
directory while honouring `project_path` for everything else. The MCP server
runs from wherever it was started, so a call naming one repository wrote that
repository's spec correctly and its feature documentation into an unrelated
working tree.

- The backlog path resolves against the project root.
- The call no longer reports success unconditionally. The sync failure went to
  stderr and was dropped; `AddFeature` now returns what it actually did, and
  both the CLI and the MCP tool report the sync only when it happened.

### Changed — roady_tasks pages and projects its results (#75)

A 96-task plan serialised to 84,258 characters and exceeded the MCP client's
tool-result limit — poor behaviour from the tool most likely to be called at
the start of a session, in a system whose claim is surviving context resets.
The same plan now answers in 5,742.

- Descriptions are omitted from listings unless `detail=true`. They were the
  bulk of the payload, and a caller listing tasks wants to identify one and
  then read that one.
- `limit` and `offset`, defaulting to 50 and capped at 200.
- The response is a page rather than a bare array: a truncated array tells the
  caller it has seen the whole plan, which is worse than the oversized
  response it replaces. It carries the total, what was returned, whether more
  exists, and a sentence saying how to get it.
- **Breaking:** `status=all` returns one flat list rather than `ready` /
  `in_progress` / `blocked` buckets. Each task carries its own status, so
  nothing is lost and one set of counts describes the whole answer.

### Fixed — MCP transition errors reach the agent (#72)

Already shipped in 0.15.0 and recorded here for completeness: domain errors
from `roady_transition_task` reached the client as a bare
`MCP error -32603: internal error` rather than the CLI's actionable message.

### Added — per-task subagent dispatch

`roady task dispatch <id> --agent <name> [--session <id>]`, and
`roady_dispatch_task` over MCP.

An agent told to "work on task-42" has to reconstruct why the task exists,
what counts as finished, and how to report back. Roady already holds all
three, so a dispatch hands them over: the originating feature and requirement,
the `doc:line` citation, and the exact call that closes the task.

- The completion contract carries the agent and session, because work a
  subagent does that never lands as a recorded transition is invisible — the
  task stays in progress and nothing attributes it.
- The claim is recorded under the subagent's identity too, so one piece of
  work does not split across two sessions in the audit trail.
- Only ready tasks dispatch, and a refusal says which case applies: already
  done, already claimed and by whom, blocked, or waiting on a named dependency.
- `--dry-run` builds the brief without claiming; `--json` emits it for an
  agent to consume.
- Gaps are surfaced rather than papered over: a task with no citation, no
  acceptance criteria, or nothing but a title warns on stderr.

MCP schema 3.2.0.

### Added — drift as a CI gate

- `roady drift detect --fail-on <severity>` controls what makes the command
  exit non-zero. Without it any drift fails the build, including a
  low-severity note nobody intends to act on this week — and a gate that
  blocks a merge on noise gets switched off, which makes it no gate at all.
- Everything found is still printed; the threshold changes only the exit code.
  When drift is found below the threshold the command says so, rather than
  exiting zero silently and leaving the operator unsure the threshold applied.
- An unrecognised severity string still counts at the lowest threshold, so a
  typo cannot quietly remove an issue from every gate.
- `docs/spec-to-pr.md` documents a pull-request gate and a merge-time job that
  opens a follow-up issue instead of failing a build nobody sees.

### Added — cross-project task dependencies

An org-level plan can express that a task waits on work in another
sub-project: `@feature-auth:task-signup` in `DependsOn`.

- `planning.ParseDependencyRef` handles the canonical `@project:task` form and
  the sigil-less `project:task` that predates it, since rejecting the latter
  would silently reinterpret a real cross-project edge as a dangling local one.
- Readiness resolves external edges against the named sub-project's state.
  Previously an external reference was passed to the *local* state lookup,
  which reported "pending" because no such local task existed — pinning the
  task as blocked forever with no way to resolve it.
- Unresolvable and incomplete external dependencies both block, and the
  resolver distinguishes them: a reference that cannot be found is a broken
  plan, not work in progress.
- DAG cycle detection now skips external edges deliberately rather than by
  accident — the previous code tolerated them only because it silently ignores
  any dependency it cannot resolve.

## [0.17.0] - 2026-08-01

Audit trails become answerable by the agents they were built for.

### Added

- MCP tool `roady_audit_trail`. The evidence trail was CLI-only, so the agents
  the GRC work was built for could not ask for one without shelling out.
  Accepts `task_id`, `agent`, and `session_id` (combinable) plus a `since`
  window. MCP schema is now 3.1.0.
- `sdk.Client.AuditTrail` exposes it to SDK consumers.
- `AuditTrailService` is wired into `AppServices` rather than constructed
  inline by the CLI, so both surfaces share one instance.

### Fixed

- `roady status` printed `(v)` for a project whose spec carries no version,
  and would render a non-numeric version as `vplanned`. The parenthetical is
  now omitted when there is no version to show.

## [0.16.0] - 2026-08-01

Found by running Roady against itself. Every item here was invisible to a
green build and a passing test suite.

**Behaviour change worth reading before upgrading:** `roady drift detect` can
now report drift — and exit non-zero — on projects that previously reported
clean, because it detects plans the repository has left behind. If you gate CI
on its exit code, expect stale-plan projects to start failing. That is the
feature working; refresh the plan or archive it.

### Fixed — `plan prune` left orphaned execution state

`roady plan prune` removed tasks from `plan.json` and left their entries in
`state.json`, so the two files disagreed about what the project consisted of
and nothing reconciled them. Roady's own repository carried 113 such entries.
Prune now drops state for tasks it removed — the history lives in
`events.jsonl` and is untouched — and writes nothing when there is nothing to
drop, so a no-op prune does not bump the state version and provoke a spurious
locking conflict. The audit event records how many tasks were pruned and
retained.

Both writers to `events.jsonl` now stamp `hash_algo`. Only `AuditService` did
initially, which left half the log unversioned and defeated the point of
versioning it. A test pins the two constants together across packages.

### Fixed — the audit chain could not verify its own history

Dogfooding `roady audit verify` on Roady's own repository found 105 integrity
violations in a log that had never been reported as broken. None were
tampering. Three separate causes:

- **Two hash functions wrote to one log.** `events.BaseEvent` hashes Type and
  AggregateID; `domain.Event` hashes Action. Any event carrying an aggregate
  ID verified under one and failed under the other. Verification now accepts
  either scheme.
- **A shell script appended raw JSON.** `scripts/release.sh` wrote entries
  with no hash and no `prev_hash` straight into the chained log — 12 of them
  over several months. It no longer does, and unhashed entries are reported
  as *outside the chain* rather than as tampering.
- **The hash algorithm changed under existing events.** Commit `fe1c290`
  folded `canonicalJSON` into the digest, so every earlier event with
  non-trivial metadata stopped reproducing its hash. 73 entries are affected
  and cannot be cryptographically verified.

Events now carry `hash_algo`, so the next algorithm change is recognised
rather than mistaken for an attack, and verification reports *cannot verify*
for an unknown algorithm. `docs/audit-grc.md` states plainly that pre-change
history is unverifiable, instead of implying the whole trail is sound.

### Added — staleness drift

`roady drift detect` now reports a plan the repository has left behind.

Every other check compares Roady's own artifacts against each other, so a
plan nobody edits stays internally consistent forever while the code moves
on. Roady's own repository demonstrated the blind spot: 55 commits and seven
releases past a plan marked 113/113 done, spec still titled "v0.11.0", and
drift detection reported *"No drift detected. Project is in a healthy
state."*

- New `drift.CategoryStale` and `DriftDetector.DetectStalenessDrift`.
- Judged by commits rather than file timestamps, since a fresh clone
  rewrites mtimes and would report every checkout as drift. Commits touching
  `.roady/` are excluded, so a project cannot stay "fresh" by only editing
  its own bookkeeping.
- Severity scales with divergence; a quiet repository is never stale, and
  when git is unavailable the check stays silent rather than guessing.

## [0.15.0] - 2026-08-01

Roady stops calling language models, and MCP tool failures become readable
to the agent that triggered them. Both are breaking.

MCP schema version is now **3.0.0** (see `docs/mcp-schema-changelog.md`);
`pkg/sdk` requires major 3. An SDK pinned to major 2 will refuse to connect,
by design.

No API key is needed for anything any more.

MCP schema version is now **3.0.0** (see `docs/mcp-schema-changelog.md`);
`pkg/sdk` moves to `SupportedSchemaMajor = "3"` to match.

### Removed — leftover AI configuration

End-to-end evaluation found `roady ai configure` and the AI half of
`roady config wizard` still writing `.roady/ai.yaml` — configuring a provider
that nothing reads any more. A user could set one up and never learn why it
had no effect. Both are gone, along with `internal/infrastructure/config`.

`CLAUDE.md` and `docs/integrations.md` still documented `pkg/ai/`, `ai.yaml`,
and `ROADY_AI_PROVIDER`. `CLAUDE.md` is the file agents read to work on this
repo, so it was actively misdirecting them.

### Fixed — MCP tool errors reach the agent

Every tool reported failures as JSON-RPC protocol faults, so the library
replaced Roady's message with `-32603 "internal error"` and logged the real
text to stderr where no agent could see it. An agent calling a tool without a
default rate configured was told "internal error" and had nothing to act on.

Tool-execution failures are now returned the way the MCP spec intends — a
normal result carrying `isError` with the message as readable content.
Protocol errors are reserved for malformed requests. 112 call sites, and 18
handlers had their return type widened to carry a result.

Validated by calling all 53 tools over stdio: 53/53 function, and every
failure message now arrives intact.

### Changed — BREAKING: Roady no longer calls language models

Roady embedded provider clients and ran inference itself. An agent invoking
Roady already has a model, a key, a budget, and more context than Roady can
reconstruct from files — so a second model meant a second credential, a
second bill, and a worse answer.

The model-assisted operations survive as prompt builders: Roady assembles
the context and returns it, the caller runs inference, and results come back
through the named write-back tool. See `docs/prompts.md`.

- **Removed** `pkg/ai` and the Anthropic / OpenAI / Gemini / Ollama clients,
  `pkg/domain/ai`, `AIPlanningService`, `.roady/ai.yaml`, `ROADY_AI_PROVIDER`
  / `ROADY_AI_MODEL`, and all API-key handling. No key is needed for anything.
- **Changed** `roady_plan_decompose`, `roady_explain_spec`, `roady_review_spec`,
  `roady_query`, `roady_suggest_priorities`, and `roady_explain_drift` to
  return a prompt request (`operation`, `system`, `prompt`, `expected_format`,
  `write_back`, `guidance`) instead of model output. These now work with no
  configuration at all.
- **Changed** the matching CLI commands to print the prompt on stdout and the
  framing on stderr, so it pipes cleanly. `--json` emits the whole request.
- **Removed** `roady_cost_estimate` — Roady spends no tokens and cannot
  project a bill for a model it does not call.
- **Removed** `roady spec parse` and `roady spec analyze --reconcile`, whose
  only purpose was having a model structure text.
- **Changed** `roady watch --auto-sync` to regenerate with the deterministic
  planner. An unattended file watcher is the last place that should silently
  spend tokens.
- `allow_ai: false` still refuses these operations; the intent behind it does
  not change because inference moved to the caller.

## [0.14.1] - 2026-08-01

Patch release so the published tag sits on a commit with green CI. No
behaviour change for anyone using the CLI or MCP server.

### Fixed

- `AuditTrailService.BuildTrail` dereferenced the event-sourced audit
  service unconditionally, while `NewAuditTrailService` documents every
  collaborator except plan as optional — passing nil panicked. Both audit
  implementations now reduce to a normalised entry shape, so a nil
  event-sourced service falls back to the plain `AuditService`, which holds
  the same events. The CLI always passed a non-nil service, so this was
  unreachable from the shipped binaries.
- `.gitignore`'s bare `dist/` rule matched
  `internal/infrastructure/mcp/dist/`, silently dropping new MCP App
  artifacts from the index. This is why `ui://roady/billing` shipped
  registered but missing.

### Added

- Test coverage for the service entry points and the four new command
  runners, restoring the coverage policy gate (application 83.2%,
  infrastructure 77.2%).

## [0.14.0] - 2026-08-01

Coordination and stakeholder reporting without a UI, agent-traceable audit
trails, and bidirectional tracker sync. Three breaking changes, all listed
below.

MCP schema version is now **2.0.0** (see `docs/mcp-schema-changelog.md`).
`pkg/sdk` moves to `SupportedSchemaMajor = "2"` to match; an SDK build
pinned to major 1 will refuse to talk to a v0.13 server, by design.

### Added — coordination

- `roady task mine`, `roady task assigned <name>`, and `roady task unassigned`
  make assignment readable. Previously `roady task assign` wrote an owner that
  nothing could query back, so "who is working on what" had no answer.
- MCP `roady_tasks` gains an `assignee` filter and an `unassigned` status.
- `policy.max_wip_per_owner` caps in-progress work per person. The existing
  project-wide `max_wip` let one person hold the entire allowance.
- `policy.enforce_team_roles` (default `false`) makes `.roady/team.yaml` a
  guard rather than documentation — a listed viewer can no longer transition
  tasks. Only actors present in the roster are checked, so unlisted actors and
  existing projects are unaffected.

### Added — stakeholder reporting

- `roady report` renders progress, forecast, risks, ownership, and a change
  log as Markdown, self-contained HTML (~5KB, no scripts or external
  requests, light/dark aware, printable), or JSON. `--since` accepts `7d`,
  `2w`, or an absolute date.
- `roady notify digest` sends one chat-sized progress summary through
  configured adapters instead of a message per domain event. Supports
  `--dry-run`, `--adapter`, and `--since`.
- Completion estimates are withheld below three velocity data points rather
  than printing a date nobody should plan around.

### Added — audit trails for GRC

- `roady audit trail [task-id] [--agent X] [--session Y] [--since 30d]`
  produces an evidence trail: chain-integrity status, findings, the task's
  evidence and spec citation, who acted, and every recorded event. Markdown
  or JSON. Exits non-zero when chain verification fails, so it can gate CI.
- Every event now records the agent and session behind it. Resolution:
  `ROADY_SESSION_ID` / `ROADY_AGENT`, then runtime detection (Claude Code,
  Cursor, Codex, Gemini CLI), plus the surface (`cli` / `mcp` / `plugin`).
  A session is minted once per process, so one MCP server run groups an
  agent's whole conversation. Previously every agent recorded as the literal
  string `ai-agent`, making "which agent did this?" unanswerable.
- MCP `roady_transition_task` accepts `session_id` and `agent`; a
  caller-supplied identity overrides the ambient process one.
- `docs/audit-grc.md` documents what a trail attests — **a tamper-evident
  record of what was asserted, not proof of identity**, since actor and agent
  are caller-supplied and unauthenticated.

### Removed — BREAKING

- Stale duplicate docs: `docs/roadmap.md` (superseded by `ROADMAP.md`,
  and whitespace-corrupted — 178 of 232 lines were blank) and
  `docs/small.md` (a truncated copy of `docs/vision.md`).
- The web dashboard is gone: `roady dashboard serve`, `roady dashboard
  open`, the `/kanban` and `/org/kanban` boards, `/api/*` endpoints, the
  SSE stream, action endpoints, and the shared-token auth. The whole
  `pkg/infrastructure/dashboard` package and `docs/dashboard.md` are
  removed.

  Rationale: an agent can render and update a human-readable view from
  the same data through Roady's MCP Apps, and people who are not in an
  agent client are better served by a document than by a server they
  have to reach. Replacements:

  - `roady report --format html -o status.html` — shareable progress
  - `roady notify digest` — push a summary to a channel
  - `roady dashboard` — the interactive TUI, unchanged

### Added — parallel collaboration

- `roady audit verify` treats the event log as a hash-linked graph instead
  of a strict sequence. Two people appending concurrently produced branches
  that union-merge cleanly but failed verification, which made parallel work
  impossible. Nothing is given up: each event's hash already covers its own
  content and its parent reference, so tampering and reparenting are still
  caught, and a removed event still leaves a dangling parent.
- `.gitattributes` marks `events.jsonl` `merge=union` so git merges it
  instead of conflicting.
- `LoadEvents` deduplicates by event ID so a line reproduced by a union
  merge cannot double-count in velocity, cost, or task projections.
  Verification reads the raw log via `LoadEventsRaw` and still reports
  duplicates.
- `roady state rebuild [--dry-run]` reconstructs `state.json` by replaying
  the log. `state.json` is a whole-file document and still conflicts in git;
  it is derived data, so the resolution is to keep either side and replay.
  The replay is idempotent and order-independent.

### Changed — MCP agent experience

- Every MCP tool now carries behaviour annotations (`readOnlyHint`,
  `destructiveHint`, `idempotentHint`, `openWorldHint`). None did before,
  and the spec's defaults are pessimistic — an unannotated tool is assumed
  not read-only and potentially destructive, so ~30 pure read tools were
  telling every client that reading a plan might destroy something.
  Classification is accurate rather than convenient: the AI tools are
  deliberately *not* marked read-only because they record token usage to
  the audit log, and plan generation / drift acceptance are marked
  destructive because they overwrite state that cannot be recovered.
- Tests fail the build if a tool is registered without a classification,
  if a classification outlives its tool, or if the judgement calls above
  are reversed.

### Removed — BREAKING (MCP)

- The five deprecated tool aliases from v0.10.0 are gone:
  `roady_get_ready_tasks`, `roady_get_blocked_tasks`,
  `roady_get_in_progress_tasks` (use `roady_tasks` with `status`),
  `roady_sticky_drift` (use `roady_drift_recurring`), and
  `roady_smart_decompose` (use `roady_plan_decompose`).
  Tool definitions cost context in every agent session: the surface was
  ~11,000 tokens, of which these five were ~900. Duplicate tools also
  degrade tool-selection accuracy. Now 54 tools, ~10,200 tokens.
  `pkg/sdk` is repointed at the canonical tools; its method signatures are
  unchanged, so SDK consumers need no edit.

### Added — bidirectional tracker sync

- `roady sync` now writes Roady's status back to the external tracker.
  `Syncer.Push` was implemented by all seven plugins but called from
  nothing, so sync was read-only despite the docs claiming otherwise.
  `--no-push` restores pull-only behaviour.
- Inbound sync honours all five statuses. Previously only `done` and
  `in_progress` were mapped and `blocked`/`pending`/`verified` were
  silently dropped, so a task blocked in Jira stayed pending in Roady.
- `planning.EventForTransition` / `planning.PathToStatus` reverse-map the
  FSM, so a status the tracker reports is reached by walking real
  transitions rather than being written past the state machine.

### Fixed

- All 14 committed MCP App artifacts were stale, built against older
  dependencies. Rebuilt, and CI now rebuilds them on every run and fails
  on drift — nothing previously verified that the committed HTML matched
  `app/src`, which is how they went stale unnoticed.
- The website still advertised the removed web dashboard (`roady
  dashboard serve`, a "Live Kanban" feature card, and a `/org/kanban`
  reference). Replaced with `roady report` and `roady audit trail`.
- `ui://roady/billing` was registered as an MCP App but its built file was
  never committed, so every request for it failed at runtime. The app is
  now built and shipped, with a test asserting every registered app
  resource is readable.
- Plugin provider inference matched only github/jira/linear, so trello,
  asana, and notion all filed task links under `external` and collided in
  `ExternalRefs`. All six are recognised, and custom plugins derive a
  stable name from their binary.

- Service-load warnings printed to stdout, corrupting `--json` output on every
  command that offers it. They now go to stderr.
- `roady task start` resolved the actor from `USER` while `roady task mine`
  used `ROADY_USER` then `git user.name`; ownership and the new per-owner
  guards disagreed about identity. Both now share one resolver.

## [0.12.0] - 2026-05-16

Four polish features on top of v0.11.3's Kanban. All backward-compatible.

### Added — Reopen action + DnD transition

- New `TaskService.ReopenTask` + `POST /actions/task/reopen` move a Done or Verified task back to Pending.
- Done cards on `/kanban` get a **↺ Reopen** button.
- DnD adds Done → Backlog and Done → Ready transitions (both call reopen).

### Added — Live updates over Server-Sent Events

- New `GET /events` SSE stream emits a `task-changed` event after every successful task transition. The board reloads within ~200 ms of the change instead of waiting on the meta-refresh.
- Live indicator in the header reflects connection state.
- Meta-refresh kept as fallback (now 60 s) for browsers without `EventSource`.
- 25 s heartbeat keeps the stream alive through proxies (Cloudflare, nginx).

### Added — Cross-project Kanban actions (DnD on `/org/kanban`)

- Cards on `/org/kanban` are now draggable. Drops route the mutation to the right sub-project's `TaskService` via the new `OrgTaskActions` resolver.
- Action endpoints accept optional `project_path` and `project` form fields; absent → defaults to the server-default `TaskActions`, present → routes through `OrgTaskActions.ResolveTaskActions`.
- Wired automatically by `roady dashboard serve`.

### Added — Dashboard auth token

- New `--auth-token <value>` flag on `roady dashboard serve|open` (env: `ROADY_DASHBOARD_TOKEN`) protects every dashboard request with a shared bearer token.
- Token accepted three ways: `Authorization: Bearer <t>`, `Cookie: roady_token=<t>`, or `?token=<t>` (one-time handshake: sets the cookie, redirects to strip the secret from the URL bar).
- Constant-time comparison; secure cookie on TLS / X-Forwarded-Proto.
- Empty token = public (backward-compatible).

## [0.11.3] - 2026-05-16

### Added — Drag-and-drop on the Kanban board

- Cards on `/kanban` are now draggable between columns. Drop a card on the target column to transition the task — no need to hunt for the right button.
- Valid transitions (mapped to existing POST endpoints): Ready → In Progress (start), In Progress → Done (complete), In Progress → Blocked (block), Blocked → In Progress (unblock).
- Visual feedback: target column outlines green for allowed drops, red for disallowed transitions (e.g. dragging a Done card onto Blocked is a no-op).
- Buttons remain available — drag-and-drop is additive, not a replacement. Server reloads on drop so the board reflects the new authoritative state.
- Vanilla HTML5 DnD; no client library. Only renders when task actions are wired (`Server.EnableTaskActions`); read-only boards stay read-only.

## [0.11.2] - 2026-05-16

### Added — Interactive Kanban (task action buttons)

- The `/kanban` board is no longer read-only. Cards now render contextual buttons by column: **Ready → Start**, **In Progress → Complete / Block**, **Blocked → Unblock**.
- New POST endpoints: `/actions/task/start`, `/actions/task/complete`, `/actions/task/block`, `/actions/task/unblock`. Form-encoded, sender redirected back to the referring page.
- Wired automatically by `roady dashboard serve` via the existing `TaskService`. No extra flags.
- Backward-compatible: if a custom server is constructed without `EnableTaskActions`, the action routes stay unregistered and the board is read-only.

## [0.11.1] - 2026-05-16

Adds the cross-project Kanban view that ties together v0.11.0's nested
sub-projects + per-project Kanban features. One agent, many feature
streams, one live board.

### Added — Cross-project Kanban (`/org/kanban`)

- New `/org/kanban` route on the dashboard renders every project under the workspace root — the root project plus every sub-project under `.roady/projects/<name>/` — merged into one five-column board.
- Cards are tagged with their origin project label so it's clear which row each task belongs to.
- New `/api/org/kanban` JSON endpoint exposes the same board for external tools.
- Header strip lists every discovered project with task and done counts.
- Auto-refreshes every 30s.
- Wired automatically by `roady dashboard serve`; no extra flags.

## [0.11.0] - 2026-05-16

Adds two user-facing surfaces that lay the groundwork for an agent
managing many parallel feature streams in one repo: nested sub-projects
and a live Kanban view. Both are fully backward-compatible.

### Added — Nested sub-projects

- A single repository can now host multiple Roady projects side-by-side under one `.roady/` directory by placing each named sub-project at `.roady/projects/<name>/`. See `docs/rfcs/0001-nested-projects.md`.
- New global CLI flag `--project / -P <name>` (env: `ROADY_PROJECT`) scopes every command to a named sub-project. With no flag, commands target the repo's root project, exactly as before.
- New MCP request field `project` (optional, alongside the existing `project_path`) routes tool calls to a sub-project. `AppServices` are cached per `(path, project)` key.
- `roady discover` and `roady org status` now surface sub-projects in addition to root projects.
- New storage constructor `storage.NewFilesystemRepositoryForProject(root, name)` plus helpers `ProjectBase()`, `SubProject()`, `IsSubProject()`. The legacy `NewFilesystemRepository(root)` continues to return a root-project repository unchanged.
- New `OrgService.DiscoverProjectsWithSub()` returns both root projects and sub-projects; legacy `DiscoverProjects()` is kept and unchanged in shape.
- Backward-compatible: existing flat `.roady/` repos continue to work unchanged. No data migration required.

### Added — Kanban dashboard view

- New `/kanban` route on the web dashboard (`roady dashboard serve`) renders the project's tasks across five status columns: **Backlog · Ready · In Progress · Blocked · Done**. Auto-refreshes every 30s.
- "Ready" computes dependency satisfaction so unblocked pending tasks surface separately from backlog items waiting on upstream work. "Done" rolls Verified into Done.
- New JSON endpoint `/api/kanban` returns the same board for external tools / IDE plugins / CI.
- Nav bar across dashboard pages gains a Kanban link.

## [0.10.1] - 2026-05-03

Patch release. No user-facing feature changes. Closes audit gaps flagged before the public launch wave.

- Plugin builds (`asana`, `github`, `mock`, `notion`, `trello`) added to GoReleaser. Every plugin now ships in the bundled release archive (previously only `linear` and `jira` shipped).
- CI gains a `golangci-lint` job. Full repo on main pushes; `--new-from-rev=origin/main` on PRs to avoid drowning new contributions in pre-existing warnings.
- New `release-smoke-test` CI job downloads the freshly published Linux/amd64 archive, runs `roady --version` and `roady demo` against it, fails the workflow on any error.
- Cloud waitlist form replaced its hidden-localStorage backend with a transparent `mailto:` fallback. No more form-without-a-server pattern.
- New GitHub Pages workflow (`.github/workflows/website.yml`) auto-deploys the Astro site on every main push affecting `website/`, `README.md`, or `docs/`.
- This `CHANGELOG.md` extended through v0.10.
- `fix(ci)`: website deploy uses Node 22 for Astro 5+ compatibility.
- `fix(ci)`: pin golangci-lint to `@latest` for go 1.26 compatibility.

## [0.10.0] - 2026-05-03

Bundles the v0.9 + v0.10 cycles. v0.9 was cut as a soft release and never tagged.

### Added — v0.9.0 Activation & Clarity

- `roady --help` grouped by user intent (Get Started / Track & Report / Integrate / Admin) via cobra command groups.
- `roady init` defaults to interactive wizard in a TTY (proper isatty check); `--non-interactive` flag for CI; next-step CTA on completion.
- `roady demo` command — scaffolds a pre-seeded sample with intentional spec/lock divergence, runs drift detect. Sub-second aha for first-time visitors.
- `roady status` empty-state ladder (`uninitialised` / `no-spec` / `no-plan`) with actionable next-step hints.

### Added — v0.10.0 AI Quality & Telemetry

- Eval harness (`evals/`) over the planning pipeline: golden fixtures (cli-tool / web-api / multi-feature), AI planner contract via programmable mock, drift precision/recall corpus. Opt-in real-provider matrix via `-tags evals_ai`.
- `Task.Origin` provenance (`heuristic | ai | human`) with source-doc citations propagated end-to-end (`from doc:line` shown in `roady status`).
- Native streaming via `OnToken` callback on `ai.CompletionRequest`. Real SSE / NDJSON wiring for Anthropic, OpenAI, Gemini, Ollama. AI service auto-routes via `ai.WithOnToken` context helper. CLI prints streamed tokens live; MCP forwards as progress notifications.
- `Confidence` and `Sources` on `CompletionResponse`. Real providers populate `Confidence` from natural stop signal.
- MCP tool consolidation: parameterised `roady_tasks` (status enum) supersedes the three legacy `roady_get_*_tasks` tools, kept as deprecation aliases. Canonical `roady_plan_decompose` and `roady_drift_recurring` aliases for off-pattern names.
- `roady_cost_estimate` MCP tool — pre-flight token + USD projection with pricing table for Anthropic / OpenAI / Gemini.
- Unified `roady notify` namespace (add/list/test/remove). `roady messaging` and `roady webhook notif` retained as deprecation aliases.
- AI command progress + clean cancellation via shared `withAIProgress` wrapper across the five AI CLI surfaces.

### Changed — v0.11 Positioning

- New positioning: "planning memory for AI coding agents" (treated as hypothesis pending real ICP validation; see `docs/positioning.md`).
- README rewritten around a single 5-step workflow.
- `docs/positioning.md`, `docs/vs.md`, `docs/advanced.md`, `ROADMAP.md` shipped.
- Astro website hero / definition / features / commands / MCP / integrations sections realigned with the new positioning.

### Fixed

- TTY detection used `os.ModeCharDevice` which returned `true` for `/dev/null` on Linux, breaking the e2e test on CI by silently triggering the interactive wizard. Switched to `go-isatty` for proper termios check.
- Pre-existing `readStdin` bug in `cli/spec.go` that always returned `""`.
- 40 `errcheck` lint warnings on writer outputs across v0.10 additions.

### Security

- All open dependabot advisories closed (postcss XSS, astro XSS, astro allowlist bypass).

### Breaking

- None. Every deprecated MCP tool name continues to work.

## [0.9.2] - 2026-03-24

Patch release. See [GitHub release notes](https://github.com/felixgeelhaar/roady/releases/tag/v0.9.2).

## [0.9.1] - 2026-03-24

Patch release. See [GitHub release notes](https://github.com/felixgeelhaar/roady/releases/tag/v0.9.1).

## [0.9.0] - 2026-03-21

See [GitHub release notes](https://github.com/felixgeelhaar/roady/releases/tag/v0.9.0).

## [0.8.0] - 2026-03

See [GitHub release notes](https://github.com/felixgeelhaar/roady/releases/tag/v0.8.0).

## [0.7.3] - 2026-02-17

See [GitHub release notes](https://github.com/felixgeelhaar/roady/releases/tag/v0.7.3).

## [0.7.0] - 2026-02

See [GitHub release notes](https://github.com/felixgeelhaar/roady/releases/tag/v0.7.0).

## [0.6.3] - 2026-02-08

### Fixed

- Propagate actual error details in `roady_transition_task` and `roady_assign_task` MCP handlers instead of returning generic messages ([#3](https://github.com/felixgeelhaar/roady/issues/3))
- Initialize task status to `pending` when `SetTaskOwner`, `AddEvidence`, or `SetExternalRef` creates a new entry in the task states map, preventing broken transitions on tasks assigned before their first status write

## [0.6.2] - 2026-02-03

### Fixed

- Fix undefined label in spec MCP app visualization (title vs name mismatch)
- Fix undefined `has_drift` field in drift MCP app (derive from issues array length)

## [0.6.1] - 2026-02-01

### Fixed

- Fix burndown chart dipping below zero on completed projects
- Enable Vue runtime compiler and remove TypeScript syntax from templates

## [0.6.0] - 2026-02-01

### Added

- Org-level dashboard aggregating status across multiple Roady projects
- fsnotify-based file watching replacing polling
- Plugin contract testing for Syncer interface
- Reliable webhook delivery with retry
- Org policy inheritance with child project overrides
- Cross-project drift detection
- Plugin registry with versioning and health monitoring
- Pluggable messaging adapters (Slack support)
- Realtime event streaming via SSE
- Selective watch patterns with include/exclude globs
- Auto-sync workflows on file changes
- Shell completions for bash/zsh/fish/powershell
- Structured CLI error types with actionable hints
- Interactive config wizard for `ai.yaml` and `policy.yaml`
- Guided onboarding with starter templates (minimal, web-api, cli-tool, library)
- Versioned MCP tool schema with backward-compatible evolution
- Public Go SDK client package (`pkg/sdk/`)
- OpenAPI 3.0 spec generation from MCP tools
- Typed SDK request/response helpers
- Task assignment with owner field
- Role-based access control (`team.yaml`)
- Optimistic locking for concurrent state modifications
- Git-based workspace sync (push/pull)
- AI spec quality review with completeness scoring
- AI priority suggestions from dependency analysis
- Context-aware task decomposition with codebase scanning
- Natural language query interface for project status
- Interactive D3.js MCP app visualizations (org, sync, forecast, git-sync, and 10 others)

### Fixed

- Move roady-sync example out of workflows to prevent CI execution

## [0.5.0] - 2026-01-30

### Added

- Event-sourced audit in production with `FileEventStore` and `InMemoryEventPublisher`
- Domain event dispatcher with `LoggingHandler`, `DriftWarningHandler`, and `TaskTransitionHandler`
- Live velocity projection subscribing to event publisher for real-time updates
- D3.js interactive visualizations across 10 MCP apps (donut, force-directed graph, arc gauge, horizontal bars, line chart, collapsible tree, swimlane)
- Vue 3 + D3.js app source with Vite build pipeline and `apps.go` embed directive

### Changed

- `InitService` and `DebtService` now accept `domain.AuditLogger` interface instead of concrete `*AuditService`
- `BaseEvent` includes `Action` field for backward-compatible JSON serialization
- `FileEventStore` defers directory creation to first write, avoiding interference with `IsInitialized()` checks
- Deduplicated `BuildAppServices` into shared `buildServicesWithProvider` helper

### Fixed

- User-friendly MCP errors replacing raw Go error strings
- FlexBool/FlexInt types accepting both native and string JSON values
- Embedded MCP app dist files committed for CI `go:embed` compatibility

## [0.4.1] - 2026-01-19

### Fixed

- Patch release with minor fixes (see [v0.4.0...v0.4.1](https://github.com/felixgeelhaar/roady/compare/v0.4.0...v0.4.1))

## [0.4.0] - 2026-01-16

### Added

- GitHub Actions CI integration
- Web dashboard for plan visualization
- Event sourcing for audit trail
- gRPC transport for plugin communication and MCP
- Push method on Syncer interface for bidirectional sync
- Notion, Asana, and Trello plugins
- Push support for Linear, GitHub, Jira, and mock plugins
- Interactive TUI for plugin configuration with auto-install
- Per-plugin configuration file support
- HTTP webhook server for real-time sync
- Marketing website migrated to Astro with Vue

### Changed

- MCP wiring refactored with split AI config
- Phase 3 and Phase 4 code quality improvements

### Fixed

- CI builds binary to `dist/` for e2e tests
- Website `.nojekyll` for GitHub Pages compatibility

## [0.3.0] - 2026-01-13

### Added

- Drift accept command for acknowledging intentional spec divergence

## [0.2.1] - 2026-01-13

### Changed

- Expose core packages in `pkg/` to enable library usage
- Inject version flags in goreleaser config

## [0.2.0] - 2026-01-13

### Added

- Jira plugin for bidirectional sync
- Linear plugin with external task linking
- External refs on task state for plugin integration
- Interactive dashboard (`roady dashboard`) TUI for visualizing plans and drift
- Dynamic policy engine with configurable `.roady/policy.yaml`
- Plugin architecture with gRPC-based Syncer interface
- Smart plan injection via `roady_update_plan` MCP tool
- GoReleaser for multi-platform builds and Homebrew distribution
- Release automation for GitHub and Homebrew

## [0.1.0] - 2026-01-13

### Added

- Core domain models: Spec, Plan, Task, Drift
- Spec ingestion from Markdown (`roady spec import`)
- Spec locking (`.roady/spec.lock.json`) for immutable planning boundaries
- Plan reconciliation merging new specs without destroying existing task state
- Drift detection for Spec vs Plan and Plan vs Code
- Audit trail with structured logging to `.roady/events.jsonl`
- MCP server (`roady mcp`) exposing core tools to AI agents
- Resilience via `fortify` integration for filesystem retries
- State management via `statekit` FSM for task transitions

[Unreleased]: https://github.com/felixgeelhaar/roady/compare/v0.10.0...HEAD
[0.10.0]: https://github.com/felixgeelhaar/roady/compare/v0.9.2...v0.10.0
[0.9.2]: https://github.com/felixgeelhaar/roady/compare/v0.9.1...v0.9.2
[0.9.1]: https://github.com/felixgeelhaar/roady/compare/v0.9.0...v0.9.1
[0.9.0]: https://github.com/felixgeelhaar/roady/compare/v0.8.0...v0.9.0
[0.8.0]: https://github.com/felixgeelhaar/roady/compare/v0.7.3...v0.8.0
[0.7.3]: https://github.com/felixgeelhaar/roady/compare/v0.7.0...v0.7.3
[0.7.0]: https://github.com/felixgeelhaar/roady/compare/v0.6.3...v0.7.0
[0.6.3]: https://github.com/felixgeelhaar/roady/compare/v0.6.2...v0.6.3
[0.6.2]: https://github.com/felixgeelhaar/roady/compare/v0.6.1...v0.6.2
[0.6.1]: https://github.com/felixgeelhaar/roady/compare/v0.6.0...v0.6.1
[0.6.0]: https://github.com/felixgeelhaar/roady/compare/v0.5.0...v0.6.0
[0.5.0]: https://github.com/felixgeelhaar/roady/compare/v0.4.1...v0.5.0
[0.4.1]: https://github.com/felixgeelhaar/roady/compare/v0.4.0...v0.4.1
[0.4.0]: https://github.com/felixgeelhaar/roady/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/felixgeelhaar/roady/compare/v0.2.1...v0.3.0
[0.2.1]: https://github.com/felixgeelhaar/roady/compare/v0.2.0...v0.2.1
[0.2.0]: https://github.com/felixgeelhaar/roady/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/felixgeelhaar/roady/releases/tag/v0.1.0
