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

Over MCP, `roady_task_transition` accepts `session_id` and `agent` per call.
A caller-supplied value always wins over the ambient process identity, because
an agent forwarding the run that spawned it knows more than the server does.

## When verification reports a problem

`roady audit verify` distinguishes four things that all used to read as
"possible tampering":

| Finding | Means |
| --- | --- |
| `content hash does not reproduce under <algo>` | The entry names an algorithm this build knows, and its hash still does not reproduce. **This one means the entry was altered.** |
| `predates the hash_algo field` | The entry names no algorithm, so the function that wrote its hash is unknown. It can be neither confirmed nor convicted. See below. |
| `outside the chain` | The entry has no hash at all, so it was never chained. Something appended to `events.jsonl` directly instead of going through Roady. |
| `cannot verify` | The entry was written by a Roady version using a hash algorithm this build does not know. Not an attack. |

The command closes with a breakdown by cause, and — when no entry failed
under a known algorithm — says so explicitly. That line is the one to read
first: it is the difference between a log full of unverifiable history and a
log somebody edited. Every finding is still listed and the command still
exits non-zero either way, so nothing is suppressed by the distinction.

### Historical entries may be unverifiable

The hash algorithm changed once, in commit `fe1c290`, which folded
`canonicalJSON` into the digest. **Every event written before that commit
whose metadata is non-trivial no longer reproduces its recorded hash.** They
were not altered; the algorithm moved under them.

This went unnoticed for months because a mismatch was reported as possible
tampering with no way to tell the two apart. Roady's own repository carries
73 such entries.

Events are now stamped with `hash_algo`, so a future change is recognised
rather than mistaken for an attack. Entries written before versioning carry
no stamp and are checked against the current algorithm on a best-effort
basis.

**What this means for a trail you show an auditor:** entries from before the
algorithm change cannot be cryptographically verified. Say so, rather than
presenting the trail as fully verified.

## Coverage limits

Events written before this feature shipped carry no agent or session. Trails
count those and report them as a finding rather than implying full coverage —
**this gap is not retroactively fixable**, so the sooner provenance capture is
running, the more history is attributable.

## Working in parallel

`events.jsonl` is append-only and marked `merge=union` in `.gitattributes`,
so two collaborators appending concurrently merge without conflict.
Verification treats the log as a hash-linked **graph** rather than a strict
sequence: branches that forked from a shared parent and merged still verify.

What that still proves, unchanged:

- **Content tampering** is caught, because each event's hash covers its own
  content *and* its parent reference. Altering a field or reparenting an
  event breaks that event's hash.
- **Deletion** is caught, because a removed event leaves its child's parent
  reference dangling.
- **Duplication** is reported, because verification reads the raw log while
  everything else reads a deduplicated view.

What relaxing strict order gives up: a total ordering across concurrent
writers. Roady never claimed one, and requiring it made parallel work
impossible.

`state.json` is a whole-file document and *will* still conflict. It is
derived data, so resolve it by replaying the log:

```bash
git checkout --ours .roady/state.json
roady state rebuild --dry-run    # see what the log says
roady state rebuild
```

The replay is idempotent and order-independent, so both sides of a merge
produce the same result.
