# DDD Refactor Phase 4: Policy Decoupling & Governance

> **Historical.** Written before v0.15.0 removed Roady's embedded provider
> clients. `roady ai configure`, `.roady/ai.yaml`, `BuildAppServicesWithProvider`
> and `docs/ai-configuration.md` describe a design that no longer exists —
> Roady runs no inference. Kept for the layering discussion.

With the new wiring helper in `internal/infrastructure/wiring/services.go`, both CLI and MCP now share a single composition root. That means we can safely continue the roadmap without worrying about divergent adapters or configuration paths.

## 1. Policy vs Provider Separation
- Keep policy files focused on governance (`max_wip`, `allow_ai`, `token_limit`). Strip AI provider/model metadata out of `.roady/policy.yaml` and move all such defaults into `.roady/ai.yaml` (already the canonical source).
- Update `roady ai configure` and the interactive prompt to edit `.roady/ai.yaml` only for provider/model changes while keeping policy limited to allowance/budgets. Mention this explicitly in the CLI output so users know where each setting lives.
- Expose a helper (`internal/infrastructure/wiring.BuildAppServicesWithProvider`) that lets tests or external tools inject a mock provider without touching policy, ensuring policy never drives provider selection.
- Document the split and governance guidance in `docs/ai-configuration.md` so contributors can reference the precise location for provider defaults, environment overrides, and drift logging.

## 2. Governance Event Logging
- Capture every plan lifecycle transition in `.roady/events.jsonl` with structured metadata. The application services already log `plan.generate`, `plan.update_smart`, `plan.approve`, etc.; confirm the audit entries include `actor`, `plan_id`, and relevant context (e.g., `token_limit`, `model`).
- Emit a new `drift.accepted` event whenever we regenerate/approve a plan after spec edits so audits can trace accepted drift. This can be wired through the shared services or a small `governance` helper that writes to the audit log.
- Add tests that validate governance events are recorded when plan approval and drift explanation flows run under both CLI and MCP (use the wiring helper to build services so tests mirror production wiring).

## 3. MCP/CLI Parity
- Use `wiring.BuildAppServices` across the codebase (CLI commands, MCP handlers, smoke tests) to guarantee the same services and AI provider are used regardless of transport.
- Expand MCP transport tests (stdio/http/ws) as needed to cover new governance helpers, ensuring parity with CLI commands (plan generate/prune/approve, drift accept, etc.).
- Document this parity guarantee in `docs/ddd-insights.md` so maintainers understand future evolutions must pass through the shared wiring layer.

## Next Steps
1. Adjust the CLI `ai configure` command output and documentation so provider/model settings live only in `.roady/ai.yaml`.
2. Implement a governance helper that logs `drift.accepted` and wired plan transitions, plus add targeted tests using `BuildAppServices`.
3. Re-run the `scripts/release.sh` flow (or equivalent) once the `coverctl` tooling issue is resolved so the 80 % infrastructure policy is enforced.
