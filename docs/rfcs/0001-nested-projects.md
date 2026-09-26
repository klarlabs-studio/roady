# RFC 0001: Nested Sub-Projects Under `.roady/projects/<name>/`

Status: Implemented (branch `feat/nested-projects`)

## Summary

Allow a single repository to host multiple Roady projects side-by-side under one `.roady/` directory by placing each named sub-project at `.roady/projects/<name>/`. Existing flat repositories continue to work unchanged — their `.roady/` contents form an implicit "root project".

One coding agent can now manage many feature- or product-scoped projects in the same workspace by switching `--project <name>` (CLI) or `project` (MCP) without changing directories.

## Motivation

Today, every Roady project owns its own directory. To track work for two related features in the same repository you either:

- Spin up two unrelated workspaces with two unrelated `.roady/` dirs, or
- Cram everything into one plan that mixes contexts.

For an AI coding agent operating across several features at once, neither option is ideal. Agents want a stable on-disk namespace that mirrors the way humans think about features (and that survives context resets).

## Layout

```
repo/
  .roady/                          # root project (unchanged)
    spec.yaml
    plan.json
    state.json
    events.jsonl
    policy.yaml
    rates.yaml
    time_entries.yaml
    projects/                      # NEW: named sub-projects
      feature-auth/
        spec.yaml
        plan.json
        state.json
        events.jsonl
        policy.yaml
        ...
      feature-payments/
        spec.yaml
        plan.json
        ...
```

Sub-project names must match `^[a-z0-9][a-z0-9._-]{0,63}$`. The literal name `projects` is reserved (collision with the parent directory).

## CLI surface

A new global flag `--project / -P` selects a sub-project; `ROADY_PROJECT` is the env-var fallback. With no flag and no env, commands target the root project.

```bash
# Initialise the repo's root project (unchanged).
roady init --template minimal

# Initialise a feature sub-project.
roady -P feature-auth init --template minimal

# Work in the sub-project context.
roady -P feature-auth spec show
roady -P feature-auth task ready
roady -P feature-auth task start <id>

# Or via environment.
export ROADY_PROJECT=feature-auth
roady status
```

## MCP surface

Every tool request struct that already takes `project_path` (path to the repo root) now also takes an optional `project` string (sub-project name).

```jsonc
{
  "name": "roady_get_ready_tasks",
  "arguments": {
    "project_path": "/path/to/repo",   // workspace root
    "project": "feature-auth"           // sub-project under .roady/projects/<name>/
  }
}
```

Behaviour:
- `project_path` empty + `project` empty → server's default services (root project at the server root).
- `project_path` set + `project` empty → root project at the given path.
- `project_path` set + `project` set → sub-project at `<path>/.roady/projects/<name>/`.
- `project_path` empty + `project` set → sub-project at `<server-root>/.roady/projects/<name>/`.

Service instances are cached per `(path, project)` key.

## Storage layer

`storage.FilesystemRepository` gained an optional sub-project field. Two constructors:

```go
storage.NewFilesystemRepository(root)                          // root project
storage.NewFilesystemRepositoryForProject(root, projectName)   // sub-project
```

Internal API:
- `ProjectBase() string` — returns the on-disk directory for this scope (`<root>/.roady` or `<root>/.roady/projects/<name>`).
- `SubProject() string`, `IsSubProject() bool` — introspection.
- `ResolvePath(filename)` — same contract as before; resolves under `ProjectBase()`.

All higher-level repository methods (SaveSpec, LoadPlan, SaveState, etc.) are unchanged — they delegate to `ResolvePath` which now honours the project base.

## Discovery

> Removed: `roady discover`, `roady org`, and `OrgService` were later deleted. Sub-projects are addressed only via `--project` / `project`.

`OrgService.DiscoverProjectsWithSub() ([]DiscoveredProject, error)` walks the tree, returns every `.roady/` it finds AND every sub-project under `.roady/projects/<name>/`. The legacy `DiscoverProjects() ([]string, error)` is kept and now returns just the root-project paths.

`roady discover` and `roady org status` surface sub-projects with a `(name)` suffix and the on-disk path of the sub-project directory.

## Cache key

The MCP server caches `AppServices` per `(repo-root, sub-project)` pair, so switching sub-projects in flight does not rebuild the full stack for the root project.

## Migration

None required. Existing flat `.roady/` repositories continue to function exactly as before — they are treated as the root project of that repo. A sub-project can be added at any time with:

```bash
roady -P <new-name> init --template minimal
```

No data movement, no config flips.

## Out of scope

- Cross-project task dependencies (`@project:task-id` syntax). Tasks remain project-scoped. A follow-up RFC can introduce cross-project references via a new `Dependency` field on `planning.Task`.
- A single org-level dashboard collating sub-project Kanban columns. Deferred.
- Renaming or moving sub-projects via CLI (today: edit the directory name on disk).

## Backwards compatibility

| Surface | Compatibility |
|---|---|
| Existing flat `.roady/` repos | Unchanged. |
| `storage.NewFilesystemRepository(root)` | Unchanged signature; returns root-project repo. |
| `wiring.NewWorkspace(root)` | Unchanged signature; returns root-project workspace. |
| `wiring.BuildAppServices(root)` | Unchanged signature; targets root project. |
| MCP request structs | `project_path` unchanged; `project` is new and optional. |

No deprecations.

## Tests

- New package tests in `pkg/storage/filesystem_subproject_test.go` cover name validation, path scoping, isolation between root and sub-projects, and traversal rejection.
- Updated `pkg/storage/filesystem_test.go` to clarify that nested filenames are still rejected on a root-project repo (sub-projects are addressed by constructor, not by passing nested filenames).
- The full test suite remains green.
