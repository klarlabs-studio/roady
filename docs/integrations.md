# Roady Integrations: Jira & Linear Architecture

## Overview

Moving beyond `roady-plugin-mock`, this document outlines the architecture for "Real World" syncers. The goal is to allow Roady to remain the **Source of Truth for Planning** while allowing **Execution** to happen in tools like Linear or Jira.

## 1. The "Link" Concept

Roady currently tracks task execution in `.roady/state.json`. To support integrations, we must enrich the `TaskResult` model to store references to external systems.

### Updated `state.json` Schema Proposal

```json
{
  "task_states": {
    "task-core-foundation": {
      "status": "in_progress",
      "owner": "felix",
      "external_refs": {
        "linear": {
          "id": "e84910-...",
          "identifier": "LIN-123",
          "url": "https://linear.app/roady/issue/LIN-123",
          "last_synced_at": "2024-01-13T10:00:00Z"
        }
      }
    }
  }
}
```

## 2. Plugin Interface Upgrade

The current `Syncer` interface is too simple (`Sync(plan, state) -> updates`). We need a richer protocol to handle the initial creation of tickets and bi-directional linking.

### Proposed Interface

```go
type Syncer interface {
    // Init ensures the plugin can connect (auth check)
    Init(config map[string]string) error

    // Sync performs the bi-directional synchronization
    // 1. Pushes new Roady tasks to External System (if missing)
    // 2. Pulls status updates from External System to Roady
    Sync(plan *planning.Plan, state *planning.ExecutionState) (*SyncResult, error)
}

type SyncResult struct {
    // StatusUpdates: TaskID -> NewStatus (e.g., "done")
    StatusUpdates map[string]planning.TaskStatus
    
    // LinkUpdates: TaskID -> ExternalRef (e.g., newly created Linear IDs)
    LinkUpdates   map[string]ExternalRef
    
    // Errors: Non-fatal errors encountered during sync
    Errors        []string
}
```

## 2. AI Configuration

Roady runs no inference and needs no model configuration. See `docs/prompts.md`.


Use `roady ai configure` to update this file alongside `.roady/policy.yaml`, and keep
`policy.yaml` focused on limits (`max_wip`, `allow_ai`, `token_limit`). All MCP transports
and CLI commands read from the same wiring layer to stay in sync.

## 3. Implementation Strategy: Linear (The Pathfinder)

We will implement **Linear** first because its API is modern, fast, and strict, making it a better model for Roady's DAG than Jira's complex workflows.

### Configuration (`.roady/policy.yaml` or Env)

```yaml
integration:
  provider: "linear"
  config:
    team_id: "e849..."
    project_id: "b491..." # Optional: Sync to specific Linear Project
```

### Mapping Logic

| Roady State | Linear State | Action |
| :--- | :--- | :--- |
| `Pending` | (New) | **Create Issue** in Triage/Backlog |
| `In Progress` | `In Progress` | No Change |
| `In Progress` | `Done` | **Update Roady** to `Done` |
| `Done` | `In Progress` | **Update Linear** to `Done` (Roady is truth) |

## 4. Implementation Strategy: Jira (The Enterprise)

Jira requires more configuration due to custom workflows.

### Configuration

```yaml
integration:
  provider: "jira"
  config:
    domain: "acme.atlassian.net"
    project_key: "PROJ"
    # User must map generic Roady states to specific Jira transitions ID
    status_map:
      in_progress: "31" # Jira Transition ID for "Start Progress"
      done: "41"        # Jira Transition ID for "Done"
```

## 5. Drift Detection Impact

The `DriftService` must be updated to check **Synchronization Drift**:
*   *Drift:* Task is `Done` in Roady but `In Progress` in Linear.
*   *Drift:* Task exists in Roady Plan but was deleted in Jira.

## 6. Plugin Configuration

### Interactive TUI Configuration (Recommended)

Roady provides an interactive TUI wizard for configuring sync plugins. This is the easiest way to set up integrations:

```bash
# Add a new plugin configuration
roady sync add

# Edit an existing configuration
roady sync edit my-jira

# Remove a configuration
roady sync remove my-jira

# List configured plugins
roady sync list

# Show configuration details
roady sync show my-jira

# Install a specific plugin
roady sync install jira
```

The `roady sync add` wizard:
1. **Selects a plugin** from available integrations (Jira, Linear, GitHub, Notion, Trello, Asana)
2. **Automatically installs** the plugin binary if not present (via `go install`)
3. **Prompts for credentials** with appropriate field masking for sensitive values
4. **Saves configuration** to `.roady/plugins.yaml`

### Configuration File Format

Configurations are stored in `.roady/plugins.yaml`:

```yaml
plugins:
  jira-prod:
    binary: ./roady-plugin-jira
    config:
      domain: https://company.atlassian.net
      project_key: ROAD
      email: team@company.com
      api_token: your-api-token

  linear-dev:
    binary: ./roady-plugin-linear
    config:
      api_key: lin_api_xxxxx
      team_id: TEAM-UUID
```

### Using Named Configurations

```bash
# Sync using a named configuration
roady sync --name jira-prod
roady sync -n linear-dev
```

> **Security Note:** Add `.roady/plugins.yaml` to `.gitignore` to avoid committing credentials.

## 7. Execution Plan

1.  **Refactor Domain:** Update `TaskResult` struct in `internal/domain/planning/state.go` to support `ExternalRefs`.
2.  **Refactor Plugin:** Update `Syncer` interface in `internal/domain/plugin/interface.go`.
3.  **Implement `roady-plugin-linear`:**
    *   Use `machinebox/graphql` for Linear API.
    *   Implement creation and status polling.
4.  **Implement `roady-plugin-jira`:**
    *   Use `andygrunwald/go-jira`.
    *   Implement transition logic.
