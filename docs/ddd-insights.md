# DDD Review & Strategic Ideas

> **Historical.** This review predates v0.15.0, which removed Roady's embedded
> provider clients entirely. The suggestions below about `.roady/ai.yaml`,
> `ROADY_AI_*` overrides and `roady init --ai-provider` describe a design that
> no longer exists: Roady runs no inference and needs no model configuration.
> Kept for the structural observations, which still hold.

## Domain-Centric Structure
- `pkg/domain` defines bounded contexts: `spec`, `planning`, `drift`, `policy`, and `plugin`. Each contains only domain logic plus interfaces that describe intent (e.g., `WorkspaceRepository`, `Policy`, `Task`).
- `pkg/application` orchestrates domain workflows (init, spec import, drift check, AI planning) and keeps infrastructure access through repository interfaces, which keeps domain layers stable regardless of CLI/MCP wiring.
- Infrastructure (`internal/infrastructure`) stays outside the domain/external surface: `config/` reads project-specific policy, `wiring/` composes repositories/providers/services, `cli/` and `mcp/` expose user-facing commands and verbs. This mirrors the classic DDD layering of UI → application → domain → infrastructure.

## Gaps & Workarounds
1. **Coverage Blind Spots** – `coverctl` reports application infra coverage below the 80% policy threshold and domain coverage skewed by untested heuristics. Targeted table-driven tests for `pkg/application/*` and `internal/infrastructure/*` helpers would pay back quickly.
2. **AI Configuration Surface** – `.roady/ai.yaml` currently only records provider/model. There is no documented flow for storing credentials or enabling per-environment overrides via the CLI/init experience; the policy load only gates usage/limits. Without a flag/cmd to tune AI defaults, adopters rely on env vars scattered across docs.
3. **Governance Event Logging** – `scripts/release.sh` serializes `coverctl`/`relicta` steps, but `.roady/events.jsonl` still only reflects task transitions/plan generations. Plan approvals/releases should append structured governance events so audits can detect when we accepted drift.
4. **MCP/CLI Parity & Wiring** – `pkg/mcp` merely forwards to `internal/infrastructure/mcp`; tests for transports and CLI flags do not assert feature parity. Without exhaustively exercising `StartHTTP`, `StartStdio`, and `StartWebSocket`, regressions can slip in as transports diverge.
5. **Doc Surface Drift** – Several docs still describe `policy.yaml` as the single source of AI truth. After moving provider/model defaults to `.roady/ai.yaml`, the doc and CLI messaging should highlight both this file and `ROADY_AI_*` env overrides so new contributors know how to configure models (Ollama vs OpenAI vs Gemini).

## Technical Debt & Improvements
1. **Coverage Debt** – Write targeted tests for `application.PlanService`, `Application.InitService`, and wiring helpers (`internal/infrastructure/wiring/*`). Mock `domain` repositories to avoid I/O while exercising policy checks, plan reconciliation, and drift detection heuristics.
2. **Policy-Driven AI Defaults** – The domain currently embeds `AllowAI`/`TokenLimit` in policy. Introduce an infra-level config (e.g., `internal/infrastructure/config/ai.yaml`) plus CLI flags (or `roady init --ai-provider`) so the policy only gates usage, while the provider selection can be updated without rewiring domain code.
3. **Plan Traceability** – Capture `plan.approve` and `plan.release` events in `.roady/events.jsonl` (maybe via `PlanService` instrumentation) so audit trails show when drift was acknowledged.
4. **MCP SDK Example** – Provide a thin helper that instantiates the shared wiring (repo + audits + provider) and exposes `application` services for plugin authors. This would demonstrate how MCP and CLI stay in sync and give integrators a starting point for extension.
5. **AI Validation Metrics** – The AI service already enforces JSON schema validation and a single retry, but instrumentation is sparse. Emit structured telemetry (e.g., log entries or usage counters) every time retries fire so teams can highlight flaky vendors/models in governance reviews.

## Ideas for Extension
- **Drift Acceptance Command** – Add `roady drift accept` to note that plan/spec drift is intentional and embed that acknowledgement in `.roady/events.jsonl`, helping teams differentiate between unresolved mismatch and intentional evolution.
- **Wire Governance Hooks** – Extend `scripts/release.sh` to append a `relicta note` or a `plan.release` event to `.roady/events.jsonl` automatically, ensuring compliance logs exist without manual steps.
- **AI Model Matrix** – Add a subcommand (e.g., `roady ai matrix`) that outputs the available providers/models and their configured default, allowing teams to compare capacities (Ollama, OpenAI, Gemini, etc.) before planning.
- **MCP/CLI Smoke Tests** – Automate cross-transport validation (stdio, HTTP, WebSocket) via existing plan/spec fixtures so we quickly detect regressions when wiring deepens.

## Next Steps
1. Prioritize the coverage gaps flagged by `coverctl` (especially `pkg/application` and `internal/infrastructure`) with mocks and table-driven tests.  
2. Expand AI configuration docs and add CLI/Init helpers so the `policy` file only enforces limits while `ai.yaml`/env controls provider/model selection (including keys).  
3. Capture governance events (`plan.approve`, `plan.release`, AI retries) in `.roady/events.jsonl` and prove the release script logs them via Relicta hooks.
