# DDD Refactor Phase 3: Coverage, Governance & Integration

Extend Roady's coverage around documentation, AI validation, MCP transports, and release automation so CLI/agents remain aligned.

## Documentation & Guidance Updates
Keep guides accurate as the config surface evolves.
- Audit docs (`README`, `docs/integrations`, `docs/core`, `docs/tdd`, etc.) for legacy `policy.yaml` provider/model references.
- Document `.roady/ai.yaml` and how it interacts with env overrides and MCP capabilities.
- Publish guidance for using MCP HTTP/WS transports plus upcoming release automation features.

## AI Response Validation & Resilience
Ensure AI-plan parsing never accepts broken JSON.
- Embed a JSON schema for tasks and reject responses that violate it; log the failure clearly.
- Retry once with a stricter prompt if the schema still fails, then surface the raw response for debugging.
- Emit telemetry when retries occur so teams can track unreliable models.

## MCP & CLI Feature Parity
Raise MCP to full feature parity with the CLI so they can be used interchangeably.
- Ensure MCP tools call the same `application` services as the CLI (already done) and add coverage for additional transports (stdio/http/ws) in E2E tests.
- Document `pkg/mcp` vs `internal/infrastructure/mcp` so integrators pick the stable entry point.
- Provide a tiny helper SDK (or example) that reuses CLI commands via the shared wiring, enabling plugin authors to embed CLI behavior.

## Workflows & Automation
Reduce manual steps for releases and governance.
- Add a script (`scripts/release.sh`) that composes `coverctl`, `gorun ./cmd/roady plan generate`, and `relicta release` to standardize releases.
- Log governance events (`.roady/events.jsonl`) whenever a plan is approved/released for audit trail.
- ~~Measure AI token usage vs policy limits~~ — dropped: `roady usage` and token limits were removed.
