# Roady – Technical Design Document

## Architectural Style

- Domain Driven Design (DDD)
- Event-oriented
- Deterministic artifacts
- Plugin-first extensibility
- **Domain Events**: Internal triggers for cross-context side effects.
- **Value Object Ubiquity**: Strict typing for domain measurements (Estimates, Priorities).

---

## Core Domains

### Spec Domain
Entities:
- **ProductSpec**: The root of intent.
- **Feature**: Functional blocks.
- **Requirement**: Atomic conditions (structured).

Invariants:
- Feature hashes must be stable.
- Full validation before persistence.

---

### Plan Domain
Entities:
- **Plan**: The logical execution graph.
- **Task**: Units of work derived from Requirements.

Invariants:
- Strictly Directed Acyclic Graph (DAG).
- Each task pins a feature/requirement hash.
- No forward-looking dependencies.

---

### Drift Domain
Entities:
- **Finding**: A single discrepancy.
- **Report**: Aggregated state of alignment.

Rules:
- Drift detection is additive.
- No "auto-fix" without explicit user approval.

---

### Policy Domain
Purpose:
- Constrain behavior (e.g., WIP limits).
- Govern AI usage (routing, costs).
- Prevent prohibited transitions.

Policy never:
- Mutates user intent silently.
- Hides violations from the audit trail.

---

## State Management

Uses **statekit** to model:
- **Spec**: `draft` → `approved` → `locked`
- **Plan**: `proposed` → `approved`
- **Task**: `pending` → `in_progress` → `done` (with `blocked` and `stopped` states)
- **Drift**: `detected` → `acknowledged` → `accepted`

---

## Resilience

Uses **fortify** to handle:
- AI provider transient failures.
- Execution timeouts (30s default).
- Fallback to guided/manual flows.
- Deterministic exponential backoff retries.

---

## AI Routing

- **Provider Abstraction**: Decoupled via `ai.Provider` interface.
- **Factory Pattern**: Centralized instantiation of Ollama, OpenAI, Anthropic, Gemini, or Mock.
- **Policy Control**: `policy.yaml` controls AI usage and limits; `.roady/ai.yaml` sets provider/model defaults.
- **Telemetry**: Usage and token stats emitted to `usage.json`.

---

## CLI Design

Command groups:
- `roady init`: Workspace setup.
- `roady spec *`: `import`, `validate/lint`, `explain`.
- `roady plan *`: `generate`, `approve`, `reject`, `prune`.
- `roady drift *`: `detect`, `explain`.
- `roady status`: High-level summary.
- `roady usage`: Telemetry overview.

Flags:
- `--validate`: Strict check.
- `--explain`: AI analysis.
- `--diff`: Structural comparison.
- `--json`: Machine-readable output.

---

## Plugin System

- **gRPC + MCP**: Primary extension protocols.
- **Optional**: Core works without any plugins.
- **Sandboxed**: Plugins cannot access data outside `.roady/` without permission.
- **Explicit Permissions**: Defined in the workspace configuration.

---

## Audit & Telemetry

### Event Log (`events.jsonl`)
- **Format**: JSON-Lines for high performance and grep-ability.
- **Content**: Every state transition, plan change, and AI operation is recorded.
- **Actor**: Tracks whether an action was performed by a `human` or an `ai`.

### Usage Tracking (`usage.json`)
- **Counters**: Total command counts and model-specific token usage.
- **Aggregation**: The `AuditService` automatically accumulates usage data from event metadata.
- **Purpose**: Provides visibility into AI costs and project velocity.

---

## Guard-based Enforcement

Roady utilizes **statekit guards** to bridge the gap between "State Integrity" and "Business Policy."

1. **State Integrity**: Handled by the FSM (e.g., "Cannot complete a task that hasn't started").
2. **Policy Enforcement**: Injected as a runtime guard function into the FSM.
   - **WIP Limit**: Count `in_progress` tasks in `state.json` before allowing `start`.
   - **Dependencies**: Check that all `depends_on` tasks are marked as `done` before allowing `start`.
   - **Approval**: Verify `plan.json` is in `approved` status before allowing any execution events.

---

## Drift Classification Details

Roady classifies discrepancies into four logical categories:

1. **Structural Drift (Plan vs. Spec)**:
   - Missing tasks for defined requirements.
   - Tasks existing without a corresponding requirement (Orphans).
2. **Implementation Drift (Code vs. State)**:
   - A task is marked `done` but its implementation file is missing or empty.
3. **Policy Drift (State vs. Policy)**:
   - Current execution violates global rules (e.g., WIP limit exceeded by direct manual edit of state).
4. **Intent Drift (Spec vs. Reality)**:
   - The specification no longer reflects the current project North Star (usually detected via AI analysis of recent conversations/docs).

---

## MCP Interface

The Model Context Protocol (MCP) server exposes Roady's deterministic state to AI agents:

- **`roady_init`**: Allow agents to scaffold new projects.
- **`roady_spec_get` / `roady_spec_explain`**: Provide structural and architectural context.
- **`roady_plan_generate` / `roady_plan_approve`**: Orchestrate the planning lifecycle.
- **`roady_task_transition`**: Enable agents to "check out" and "check in" work.
- **`roady_drift_detect` / `roady_drift_explain`**: Empower agents to self-correct and identify misalignments.
- **`roady_usage_get`**: Permit agents to monitor their own resource consumption.

---



## Storage: `.roady/`



Roady uses a local-first, git-friendly storage strategy.



- **`spec.yaml`**: The source of truth for features and requirements.

- **`spec.lock.json`**: The pinned state of the specification (hash-verified).

- **`plan.json`**: The logical task DAG and approval status.

- **`state.json`**: The current execution status and paths for each task.

- **`policy.yaml`**: Project-wide constraints and AI usage limits.
- **`ai.yaml`**: Provider/model defaults for AI requests.

- **`events.jsonl`**: The immutable audit trail of all project changes.

- **`usage.json`**: Accumulated telemetry and AI token consumption.

- **`drift/`**: (Optional) Stored machine-readable drift reports for historical analysis.



---



## Observability



Roady provides high-resolution visibility into the planning lifecycle:



- **Structured Logs**: All operations emit events to the local audit trail.

- **Drift Events**: Discrepancies are identified as first-class signals with severity levels.

- **AI Usage Metrics**: Detailed tracking of token counts and provider performance.

- **TUI Dashboard**: Real-time visualization of plan health and project velocity.
