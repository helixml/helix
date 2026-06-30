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

### 2. New runtime port `SpecTasks` — modelled on a human reviewer's actions
Per review feedback, the port verbs mirror what a human project manager
actually does in the UI (review a spec, approve it, request changes, approve
PR creation) rather than generic CRUD. Add to `runtime.go`:
```go
type SpecTasks interface {
    // Authoring
    Create(ctx, caller, CreateSpecTaskInput) (SpecTaskView, error)
    List(ctx, caller, ListSpecTasksFilter) ([]SpecTaskView, error)
    Get(ctx, caller, taskID) (SpecTaskView, error)
    StartPlanning(ctx, caller, taskID) (SpecTaskView, error) // begin spec generation

    // Reviewing — the human-in-the-loop actions
    ReviewSpec(ctx, caller, taskID) (SpecReviewView, error)            // read generated requirements/design/tasks
    ApproveSpec(ctx, caller, taskID) (SpecTaskView, error)             // spec_review -> spec_approved -> implementation
    RequestChanges(ctx, caller, taskID, comment string) (SpecTaskView, error) // -> spec_revision
    ApprovePullRequest(ctx, caller, taskID) (SpecTaskView, error)      // approve implementation / PR creation
}
```
`caller` carries the orgID + workerID taken from `inv.Caller`; the port
resolves the project internally from worker state, so the tool layer never
handles a `projectID`. Plus `NoopSpecTasks` (returns `ErrSpecTasksUnsupported`)
and the view/input structs.

These verbs map 1:1 onto the existing human REST handlers and the canonical
`SpecDrivenTaskService` the UI already drives — so each is a thin delegation,
not a reimplemented state machine:

| Port method | Reuses (existing code) |
|-------------|------------------------|
| `Create` | shared create helper / `SpecDrivenTaskService.CreateTaskFromPrompt` |
| `StartPlanning` | `SpecDrivenTaskService.StartSpecGeneration` / `StartJustDoItMode` (`startPlanning` handler) |
| `ReviewSpec` | `getTaskSpecs` read logic (`/spec-tasks/{id}/specs`) + `listDesignReviews` |
| `ApproveSpec` | `SpecDrivenTaskService.ApproveSpecs` (`approveSpecs` handler) |
| `RequestChanges` | `submitDesignReview` with `Decision: "request_changes"` → `spec_revision` |
| `ApprovePullRequest` | `approveImplementation` handler logic |

### 3. In-proc impl `runtimehelix.SpecTasks`
New file `api/pkg/org/infrastructure/runtime/helix/spectasks.go`. Mirrors
`project_config.go`: holds `*store.Store` + `*services.SpecDrivenTaskService`,
loads worker state to get `ProjectID` (error if empty), enforces that any
referenced task belongs to that project, then delegates each verb to the
service / handler-equivalent listed above. Wire it in `helix_org.go` next to
`NewProjectConfig` (~line 427) and set `deps.SpecTasks` before
`RegisterBuiltins` (~line 590).

### 4. Org MCP tools
One file per verb in `api/pkg/org/interfaces/mcptools/`, each implementing
`tool.Tool` (`Name`, `Description`, `InputSchema`, `Invoke`), following
`get_worker_project.go`. They read the caller from `inv.Caller`
(orgID + workerID), unmarshal args, call `deps.SpecTasks`, and JSON-marshal
the result. Add `SpecTasks runtime.SpecTasks` to `mcptools.Deps` and the
corresponding wiring in `Config.Build()` / `DefaultDeps` (default
`runtime.NoopSpecTasks{}`). Register all of them in `RegisterBuiltins`.

### 5. Authorization & scoping
The caller worker *is* the subject — there is no `workerId` argument. Project
is resolved from `inv.Caller`'s own state, so a Worker can only manage tasks
in the project it is assigned to. Mutating tools stay out of `BaseReadTools`;
owners grant them per-Role via `create_role` / `update_role`.

## Tool Surface (summary)

Named to read like the actions a human reviewer takes:

| Tool | Args | Effect |
|------|------|--------|
| `create_spectask` | name, description, type?, priority?, original_prompt?, skip_planning?, depends_on? | Create task in caller's project (status `backlog`) |
| `list_spectasks` | status?, priority?, type? | List tasks in caller's project |
| `get_spectask` | task_id | Read one task (must belong to project) |
| `start_spectask_planning` | task_id | Begin spec generation (or queue implementation if `skip_planning`) |
| `review_spectask_spec` | task_id | Read the generated requirements/design/tasks for review |
| `approve_spectask_spec` | task_id | Approve the spec → advances to implementation |
| `request_spectask_changes` | task_id, comment | Send the spec back for revision (`spec_revision`) |
| `approve_spectask_pr` | task_id | Approve implementation / PR creation |

## Files Touched

- New: `api/pkg/spectask/service.go` (+ test) — shared authoring core
  (create/list/get + design-doc-path/task-number), reused by the Optimus
  skill tools and the org port. Workflow verbs delegate to the existing
  `services.SpecDrivenTaskService` + design-review path rather than
  duplicating them.
- New: `api/pkg/org/infrastructure/runtime/helix/spectasks.go` (+ test).
- New: one tool file per verb in `api/pkg/org/interfaces/mcptools/` (+ tests).
- Edit: `runtime.go` (port + Noop + structs), `builtins.go` (Deps field +
  RegisterBuiltins), `helix_org.go` (composition wiring).
- Edit: `api/pkg/agent/skill/project/*_tool.go` — refactor the
  create/list/get tools to call the shared authoring service
  (behaviour-preserving).

## Risks / Gotchas

- **Behaviour parity:** keep `generateDesignDocPath` and task-number logic
  byte-for-byte when extracting, or the design-doc directory naming (which
  feeds branch names) changes. Cover with the existing skill tests.
- **Typed-nil ports:** `Config.Build()` must default `SpecTasks` to
  `runtime.NoopSpecTasks{}` to avoid nil-interface panics (same care taken
  for `ProjectConfig`/`Dispatcher`).
- **Cross-project leakage:** never read a `projectID` from tool args; always
  derive from worker state and re-check task ownership on every verb.
- **Approver identity / GitHub OAuth (open decision):** `approveSpecs`,
  `submitDesignReview`, and `approveImplementation` validate the *human*
  approver's GitHub OAuth (`ValidateUserGitHubOAuth`) so their credentials
  drive commits and push during implementation. A Worker has no GitHub OAuth
  identity. The port must decide whose credentials are used when a Worker
  approves — most likely the Worker's hiring user (already persisted on the
  Worker's runtime state via `SaveHiringUser`) or the task creator. Resolve
  this before implementing `ApproveSpec` / `ApprovePullRequest`; do not let an
  approval silently fall back to the wrong identity.
