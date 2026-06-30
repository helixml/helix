# Design: Add Helix-Org MCP Tools for Workers to Manage Spec Tasks

## Architecture Overview

The helix-org MCP server (`api/pkg/org/interfaces/server/mcp.go`) builds a
per-Worker `*mcp.Server` whose tools come from `Role.Tools` resolved against
`s.registry` (`mcptools.Registry`). Org tools never touch `store.Store`
directly — they delegate to **application services** or **runtime ports**
defined in `api/pkg/org/infrastructure/runtime/runtime.go` and implemented by
the in-proc adapter `api/pkg/org/infrastructure/runtime/helix/`.

The exact precedent is `ProjectConfig`: a port
(`runtime.ProjectConfig`) + an in-proc impl (`runtimehelix.ProjectConfig`,
`project_config.go`) that resolves `workerID → projectID` via the Worker's
runtime state (`LoadState`) and then reads/writes the helix project through
the in-proc client. We follow this shape exactly.

## Key Decisions

### 1. Reuse via a shared spec-task helper (not direct import)
Extract the core logic currently inside `api/pkg/agent/skill/project/`
(create/list/get/update/start, `generateDesignDocPath`, task-number
assignment, status transitions, project-ownership check) into a small shared
service — e.g. `api/pkg/spectask/service.go` operating on `store.Store` and
`types.SpecTask`. The existing skill tools become thin adapters over it, and
the new org port calls the same service. **Why:** the two MCP surfaces (agent
skill vs org tool) have different interfaces and dependency rules; a shared
service is the only way to genuinely reuse the logic without it drifting.

### 2. New runtime port `SpecTasks`
Add to `runtime.go`:
```go
type SpecTasks interface {
    Create(ctx, orgID, workerID, CreateSpecTaskInput) (SpecTaskView, error)
    List(ctx, orgID, workerID, ListSpecTasksFilter) ([]SpecTaskView, error)
    Get(ctx, orgID, workerID, taskID) (SpecTaskView, error)
    Update(ctx, orgID, workerID, taskID, UpdateSpecTaskPatch) (SpecTaskView, error)
    Start(ctx, orgID, workerID, taskID) (SpecTaskView, error)
}
```
Plus `NoopSpecTasks` (returns `ErrSpecTasksUnsupported`) and the view/input
structs. The port resolves the project internally from worker state, so the
tool layer never handles a `projectID`.

### 3. In-proc impl `runtimehelix.SpecTasks`
New file `api/pkg/org/infrastructure/runtime/helix/spectasks.go`. Mirrors
`project_config.go`: holds `*store.Store` + the shared spec-task service,
loads worker state to get `ProjectID` (error if empty), enforces that any
referenced task belongs to that project, and delegates to the service. Wire
it in `helix_org.go` next to `NewProjectConfig` (~line 427) and set
`deps.SpecTasks` before `RegisterBuiltins` (~line 590).

### 4. Org MCP tools
Five files in `api/pkg/org/interfaces/mcptools/`, each implementing
`tool.Tool` (`Name`, `Description`, `InputSchema`, `Invoke`), following
`get_worker_project.go`. They read the caller from `inv.Caller`
(orgID + workerID), unmarshal args, call `deps.SpecTasks`, and JSON-marshal
the result. Add `SpecTasks runtime.SpecTasks` to `mcptools.Deps` and the
corresponding wiring in `Config.Build()` / `DefaultDeps` (default
`runtime.NoopSpecTasks{}`). Register all five in `RegisterBuiltins`.

### 5. Authorization & scoping
The caller worker *is* the subject — there is no `workerId` argument. Project
is resolved from `inv.Caller`'s own state, so a Worker can only manage tasks
in the project it is assigned to. Mutating tools stay out of `BaseReadTools`;
owners grant them per-Role via `create_role` / `update_role`.

## Tool Surface (summary)

| Tool | Args | Effect |
|------|------|--------|
| `create_spectask` | name, description, type?, priority?, original_prompt?, skip_planning?, depends_on? | Create task in caller's project (status `backlog`) |
| `list_spectasks` | status?, priority?, type? | List tasks in caller's project |
| `get_spectask` | task_id | Read one task (must belong to project) |
| `update_spectask` | task_id, name?, description?, status?, priority? | Patch task |
| `start_spectask` | task_id | Queue for spec generation or implementation (`skip_planning`) |

## Files Touched

- New: `api/pkg/spectask/service.go` (+ test) — shared core logic.
- New: `api/pkg/org/infrastructure/runtime/helix/spectasks.go` (+ test).
- New: 5 tool files in `api/pkg/org/interfaces/mcptools/` (+ tests).
- Edit: `runtime.go` (port + Noop + structs), `builtins.go` (Deps field +
  RegisterBuiltins), `helix_org.go` (composition wiring).
- Edit: `api/pkg/agent/skill/project/*_tool.go` — refactor to call the shared
  service (behaviour-preserving).

## Risks / Gotchas

- **Behaviour parity:** keep `generateDesignDocPath` and task-number logic
  byte-for-byte when extracting, or the design-doc directory naming (which
  feeds branch names) changes. Cover with the existing skill tests.
- **Typed-nil ports:** `Config.Build()` must default `SpecTasks` to
  `runtime.NoopSpecTasks{}` to avoid nil-interface panics (same care taken
  for `ProjectConfig`/`Dispatcher`).
- **Cross-project leakage:** never read a `projectID` from tool args; always
  derive from worker state and re-check task ownership on get/update/start.
