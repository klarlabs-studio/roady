# Model-assisted work

Roady does not call language models.

It used to: `pkg/ai` held clients for Anthropic, OpenAI, Gemini, and Ollama,
and commands like `roady plan generate --ai` ran inference themselves. That
made sense before agents were the primary caller. It stopped making sense
once they were — an agent invoking Roady already has a model, a key, a
budget, and far more context about the task than Roady can reconstruct from
files. Roady calling a *second* model meant a second credential to configure,
a second bill, and a worse answer.

## What happens instead

Roady assembles the context and hands the job back:

```bash
roady plan generate --ai          # prompt on stdout, framing on stderr
roady plan generate --ai --json   # the whole request as JSON
```

```json
{
  "operation": "decompose_spec",
  "system": "You are a senior engineer breaking a specification into an executable plan.",
  "prompt": "Decompose this specification into concrete, implementable tasks...",
  "expected_format": "{\"tasks\": [{\"id\": \"task-...\", ...}]}",
  "write_back": "roady_capture",
  "guidance": "Produce the tasks yourself, then record them with roady_capture (or `roady capture` on the CLI): the same JSON, validated all or nothing."
}
```

The prompt goes to **stdout** and everything else to **stderr**, so it pipes
into a model without contamination:

```bash
roady query "what is left to do?" | llm
```

## The round trip

Requests that produce data Roady stores name the tool that accepts it:

| Operation | Write back with |
| --- | --- |
| `decompose_spec` | `roady capture` / `roady_capture` |
| `semantic_drift` | `roady drift` / `roady_drift` action `record` |
| `explain_spec`, `review_spec`, `query_project`, `explain_drift` | nothing — for the reader |
| `suggest_priorities` | nothing — applying them is a plan edit |

Over MCP every builder is an action: `roady_query`, `roady_drift` (`semantic`,
`explain`), `roady_spec` (`explain`, `review`) and `roady_plan` (`prioritize`,
`decompose`) return requests the same way the CLI prints them.

## Policy still applies

`allow_ai: false` in `.roady/policy.yaml` still refuses these operations. A
team that set it meant "do not use a model on this project", and that intent
does not change because the inference moved to the caller.

## What went away

- `pkg/ai` and the four provider clients
- `.roady/ai.yaml`, `ROADY_AI_PROVIDER`, `ROADY_AI_MODEL`, and every API key
- `roady_cost_estimate` — Roady spends no tokens, so it cannot project a bill
- `roady spec parse` — its whole job was having a model structure raw text
- `roady spec analyze --reconcile` — same
