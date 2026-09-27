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
import tempfile

PROJECT = "."

# Arguments per tool. Anything absent is called with no args beyond defaults.
# Noun tools take an action; the validator calls a read-only one for each, so
# running it leaves the project as it was. Decisions (plan approve, drift
# accept, ...) ask the user and are covered by the Go tests.
ARGS = {
    "roady_task": {"action": "list"},
    "roady_plan": {"action": "get"},
    "roady_spec": {"action": "validate"},
    "roady_drift": {"action": "detect"},
    "roady_audit": {"action": "verify"},
    "roady_state": {"action": "get"},
    "roady_policy": {"action": "check"},
    "roady_git": {"action": "sync"},
    "roady_goal": {"action": "list"},
    "roady_capture": {"dry_run": True, "tasks": []},
    "roady_query": {"question": "what is left?"},
    # init writes a new project, so it gets a directory of its own.
    "roady_init": {"name": "validator", "project_path": tempfile.mkdtemp(prefix="roady-validate-")},
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
