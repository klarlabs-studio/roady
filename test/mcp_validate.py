#!/usr/bin/env python3
"""Drive every registered Roady MCP tool over stdio and report the outcome.

Annotating a tool says how it behaves; this checks it actually answers. Each
tool is called with plausible arguments, and the result is classified as:

  OK        returned content without an error flag
  ENV       failed for an environment reason we deliberately did not set up
            (no AI provider, no plugin binary, no git remote) - not a defect
  FAIL      returned an error we cannot explain away - a real finding
"""
import json
import subprocess
import sys

PROJECT = "."

# Arguments per tool. Anything absent is called with no args beyond defaults.
ARGS = {
    "roady_transition_task": {"task_id": "task-tasks-create", "event": "start",
                              "actor": "validator", "session_id": "val-1", "agent": "validator"},
    "roady_assign_task": {"task_id": "task-tasks-create", "assignee": "bob"},
    "roady_tasks": {"status": "all"},
    "roady_add_feature": {"title": "Validated feature", "description": "added by validator"},
    "roady_update_plan": {"tasks": []},
    "roady_explain_drift": {},
    "roady_query": {"question": "what is left?"},
    "roady_cost_estimate": {"operation": "generate_plan"},
    "roady_task_log_time": {"task_id": "task-auth-signup", "minutes": 15},
    "roady_rate_add": {"id": "val", "name": "Validator", "hourly_rate": 50},
    "roady_rate_remove": {"id": "val"},
    "roady_rate_set_default": {"id": "val"},
    "roady_rate_tax": {"name": "VAT", "percent": 19},
    "roady_team_add": {"name": "carol", "role": "member"},
    "roady_team_remove": {"name": "carol"},
    "roady_debt_trend": {"window_days": 7},
    "roady_drift_recurring": {},
    "roady_sync": {"plugin_path": "/nonexistent-plugin"},
    "roady_init": {"name": "validated"},
    "roady_plan_decompose": {},
    "roady_suggest_priorities": {},
    "roady_review_spec": {},
    "roady_explain_spec": {},
    "roady_generate_plan": {},
}

# Substrings that mean "the environment was not set up for this", not a bug.
ENV_MARKERS = [
    "no ai provider", "ai provider", "provider not configured",
    "failed to load plugin", "plugin", "no such file",
    "not a git repository", "git remote", "no remote",
    "api key", "unauthorized", "connection refused", "dial tcp",
    "ai usage is disabled",
]


def rpc(proc, msg):
    proc.stdin.write(json.dumps(msg) + "\n")
    proc.stdin.flush()
    while True:
        line = proc.stdout.readline()
        if not line:
            return None
        try:
            out = json.loads(line)
        except json.JSONDecodeError:
            continue
        if out.get("id") == msg.get("id"):
            return out


def main():
    proc = subprocess.Popen(
        ["/tmp/roady-test", "mcp", "--transport", "stdio"],
        stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL,
        text=True, bufsize=1,
    )

    rpc(proc, {"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": {
        "protocolVersion": "2024-11-05", "capabilities": {},
        "clientInfo": {"name": "validator", "version": "1"}}})

    listed = rpc(proc, {"jsonrpc": "2.0", "id": 2, "method": "tools/list", "params": {}})
    tools = listed["result"]["tools"]

    results = []
    for i, tool in enumerate(sorted(tools, key=lambda t: t["name"]), start=10):
        name = tool["name"]
        args = dict(ARGS.get(name, {}))
        args.setdefault("project_path", PROJECT)

        resp = rpc(proc, {"jsonrpc": "2.0", "id": i, "method": "tools/call",
                          "params": {"name": name, "arguments": args}})

        if resp is None:
            results.append((name, "FAIL", "no response (server died?)"))
            break

        if "error" in resp:
            detail = str(resp["error"].get("message", ""))[:110]
            kind = "ENV" if any(m in detail.lower() for m in ENV_MARKERS) else "FAIL"
            results.append((name, kind, detail))
            continue

        result = resp.get("result", {})
        text = ""
        for c in result.get("content", []):
            if c.get("type") == "text":
                text += c.get("text", "")

        if result.get("isError"):
            kind = "ENV" if any(m in text.lower() for m in ENV_MARKERS) else "FAIL"
            results.append((name, kind, text[:110].replace("\n", " ")))
            continue

        results.append((name, "OK", text[:60].replace("\n", " ")))

    proc.stdin.close()
    proc.terminate()

    counts = {"OK": 0, "ENV": 0, "FAIL": 0}
    for name, kind, detail in results:
        counts[kind] += 1
        if kind != "OK":
            print(f"{kind:5} {name:32} {detail}")

    print()
    print(f"tools listed: {len(tools)}   called: {len(results)}")
    print(f"OK={counts['OK']}  ENV={counts['ENV']}  FAIL={counts['FAIL']}")
    return 1 if counts["FAIL"] else 0


if __name__ == "__main__":
    sys.exit(main())
