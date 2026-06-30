# Requirements: Add Helix-Org MCP Tools for Workers to Manage Spec Tasks

## Background

Helix-org AI Workers connect to an MCP server at `/workers/{id}/mcp`
(`api/pkg/org/interfaces/server/mcp.go`). The advertised tools are derived
live from the Worker's `Role.Tools`, drawn from the `mcptools` registry
(`api/pkg/org/interfaces/mcptools/`).

Today a Worker can read its org graph, hire peers, publish events, and
read/patch its own helix project config (`get_worker_project` /
`configure_worker_project`) — but it **cannot create or manage spec tasks**
in the project it is assigned to.

Separately, Helix agents (e.g. the Optimus agent) already manage spec tasks
through the "HelixProjects" skill at `api/pkg/agent/skill/project/`
(`CreateSpecTaskTool`, `ListSpecTasksTool`, `GetSpecTaskTool`,
`UpdateSpecTaskTool`, `StartSpecTaskTool`). This logic wraps the helix
`store.Store` spec-task methods plus design-doc-path generation, task-number
assignment, and status transitions.

**Reuse answer:** Yes — but not by importing the skill tools verbatim. They
implement the `agent.Tool` interface and depend directly on `store.Store`,
while org MCP tools implement `tool.Tool` and reach the helix world only
through **runtime ports** (the `runtime.ProjectConfig` pattern). We reuse the
code by extracting the shared spec-task logic into one helper that both the
existing skill tools and the new org port call, so behaviour cannot drift.

## User Stories

1. **As a Helix-Org Worker**, I want to create a spec task in my own project
   via MCP, so I can turn work I've been asked to do into a tracked task.
2. **As a Worker**, I want to list and read spec tasks in my project, so I can
   see what already exists before creating duplicates.
3. **As a Worker**, I want to update a task (status, priority, name,
   description) and start a task, so I can drive it through the workflow.
4. **As an org owner**, I want these spec-task tools to be opt-in per Role, so
   only Workers I grant them to can mutate project tasks.

## Acceptance Criteria

- New org MCP tools exist and are registered in `RegisterBuiltins`:
  `create_spectask`, `list_spectasks`, `get_spectask`, `update_spectask`,
  `start_spectask`.
- Each tool resolves the project from the **calling Worker's** runtime state
  (`LoadState(...).ProjectID`). A Worker can only touch spec tasks in its own
  project — no `projectID` is accepted from the LLM.
- If the calling Worker has no project assigned, tools return a clear error
  (mirroring `ErrProjectConfigUnsupported`).
- `create_spectask` produces a task identical in shape to one created by the
  Optimus skill (task number, design-doc path, status `backlog`), proving the
  shared logic is reused, not reimplemented.
- The five existing Optimus skill tools continue to behave identically after
  the shared logic is extracted (existing tests in
  `api/pkg/agent/skill/project/` still pass).
- The mutating tools are **not** added to `BaseReadTools` (the universal
  baseline); they are granted to a Worker only when listed in its Role.
- Unit tests cover each new tool (fake port) and the in-proc port
  implementation, following the `configure_worker_project_test.go` /
  `project_config_test.go` patterns.

## Out of Scope

- Spec-task attachments, design reviews, work sessions, Zed threads.
- Changes to the MCP gateway routing or the `helix-org` backend transport.
- New UI. This is a backend/MCP-tooling change only.
