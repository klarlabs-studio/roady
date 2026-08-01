# Audit trails for GRC review

Roady records every state change to a hash-chained event log
(`.roady/events.jsonl`). `roady audit trail` turns that log into a document a
reviewer can read.

## What a trail answers

```bash
roady audit trail task-42                          # everything about one task
roady audit trail --agent claude-code --since 30d  # everything one agent did
roady audit trail --session 9f3c1b2a-...           # everything one run did
roady audit trail task-42 --format json            # machine-readable
```

Each trail reports, in this order:

1. **Chain integrity** — whether the log verifies, and which event failed if not.
2. **Findings** — stated plainly: failed verification, a task marked done with
   no evidence, entries with no agent recorded.
3. **The task** — status, owner, origin, and its `doc:line` citation back to
   the spec that motivated it.
4. **Evidence** — commits, links, and external issue references.
5. **Who acted** — every actor with their agent, session, and action count.
6. **Recorded events** — the raw entries.

`roady audit trail` **exits non-zero when chain verification fails**, so it can
gate a CI job.

## What this attests — read this before quoting a trail

Roady offers **a complete, tamper-evident record of what was asserted.** If any
entry were altered or removed after being written, the hash chain breaks and
the trail says so.

Roady does **not** offer **proof of identity.** Actor, agent, and session values
are supplied by the caller and are never authenticated. Anyone able to run
`roady` can set `ROADY_AGENT` to any string. A trail attests to what was
claimed at the time, not to who acted.

Take that distinction to your auditor honestly. Closing it would require signed
attestations, which Roady does not implement.

## Identity capture

Provenance is stamped onto every event automatically. Precedence:

| Source | Notes |
| --- | --- |
| `ROADY_SESSION_ID` | Explicit session. Otherwise one is generated per process. |
| `ROADY_AGENT` | Explicit agent name. |
| Runtime detection | `CLAUDECODE`, `CURSOR_TRACE_ID`, `CODEX_SANDBOX`, `GEMINI_CLI` |
| Surface | `cli`, `mcp`, or `plugin` — set automatically. |

A session ID is minted **once per process**. One CLI invocation is one session;
one long-lived `roady mcp` server is one session spanning the agent's whole
conversation — which is the granularity a reviewer asks about.

Over MCP, `roady_transition_task` accepts `session_id` and `agent` per call.
A caller-supplied value always wins over the ambient process identity, because
an agent forwarding the run that spawned it knows more than the server does.

## Coverage limits

Events written before this feature shipped carry no agent or session. Trails
count those and report them as a finding rather than implying full coverage —
**this gap is not retroactively fixable**, so the sooner provenance capture is
running, the more history is attributable.
