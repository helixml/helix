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

**Reuse answer:** Yes — by reusing the **canonical `services` layer** the
Optimus skill itself sits on top of, **without touching the skill code**. The
skill tools (`agent.Tool`, depend on `store.Store`) stay exactly as-is. The new
org MCP tools (`tool.Tool`) reach the helix world through an **infrastructure
port** (the `runtime.ProjectConfig` pattern) whose impl delegates to the
already-tested `services.SpecDrivenTaskService` + `services.GenerateDesignDocPath`
+ `store.Store` — the same code the REST UI drives. On the org side the tools
go through a dedicated **org application service** (front-of-house) over that
port — the same layering the helix project runtime uses today. Nothing in
`api/pkg/agent/skill/project/` or existing `services`/`store` behaviour is
modified, so current functionality cannot regress; the new layers are built
test-first.

## User Stories

The tools model the actions a human project manager performs in the UI, not
generic CRUD (per design review feedback).

1. **As a Helix-Org Worker**, I want to create a spec task in my own project
   and start its planning, so I can turn work I've been asked to do into a
   tracked, spec-driven task.
2. **As a Worker**, I want to list and read spec tasks in my project, so I can
   see what already exists before creating duplicates.
3. **As a reviewing Worker**, I want to review a generated spec, then either
   **approve** it or **request changes**, so I can act as the human-in-the-loop
   on the specification.
4. **As a reviewing Worker**, when I'm happy with the implemented code, I want
   to tell the system to create pull request(s), so the work is opened for
   review on GitHub. (One PR per repo attached to the project; the actual
   merge-approval happens on GitHub, not here.)
5. **As an org owner**, I want these spec-task tools to be opt-in per Role, so
   only Workers I grant them to can mutate or approve project tasks.
6. **As a Worker subscribed to a project**, I want to be **triggered when a
   spec task changes state** (e.g. spec ready for review, changes requested,
   PR opened), so I can react without polling. The spec-task event stream
   should arrive as an org **topic** my subscription feeds off, the same way I
   react to Slack/GitHub topics today.

## Eventing Requirement (new transport + worker trigger)

The event source is the **Helix UI notification system** — the
`AttentionService` (`api/pkg/services/attention_service.go`), which emits an
`AttentionEvent` whenever a spec task reaches a "human action needed" moment.
Its typed event set is exactly the trigger set we want:
`specs_pushed`, `pr_ready`, `spec_failed`, `implementation_failed`,
`ci_passed`, `ci_failed`, `agent_interaction_completed`. The UI reads these via
`/attention-events` (`GlobalNotifications.tsx`).

This is **not** the same as the raw `store.SubscribeForTasks` pubsub that the
Slack *project-updates* trigger consumes (that fires on every field change).
`AttentionService` is a curated, idempotent layer on top; it does separately
also post Slack thread replies, so the two surfaces partly converge there —
but the UI's source of truth is the `AttentionEvent` set. We feed *that* set in
as Worker triggers, so Workers react to the same curated moments a human sees.

Acceptance criteria for the eventing path:

- A new **inbound transport kind** (`transport.KindSpecTask`) exists and is
  registered in the transport strategy map / `kindOrder`.
- A project-scoped **topic** of that kind carries a project's `AttentionEvent`
  stream. The source is `AttentionService` — we hook its emit fan-out (beside
  the existing UI-row + Slack side-effects), not a new event detector.
- Each emitted `AttentionEvent` is published onto the topic as a
  `streaming.Event` (via the `Publishing` service), so the existing
  `dispatch.Dispatcher` fans it out as **one activation per subscribed
  Worker** — the standard trigger path used by Slack/GitHub topics.
- Curation/idempotency is preserved: a deduplicated AttentionEvent does not
  produce a duplicate trigger (mirror the existing idempotency-skip).
- A Worker subscribed to the topic (existing `subscribe` tool /
  `Subscriptions` service) is activated when an event lands, with enough
  payload (task id, event type, new status) to act via the spec-task MCP
  tools.

## Acceptance Criteria

- New org MCP tools exist and are registered in `RegisterBuiltins`, named for
  the reviewer action they perform: `create_spectask`, `list_spectasks`,
  `get_spectask`, `start_spectask_planning`, `review_spectask_spec`,
  `approve_spectask_spec`, `request_spectask_changes`, `create_spectask_prs`.
- `create_spectask_prs` authorizes the system to open pull requests (it does
  not merge/approve on GitHub) and creates one PR per attached repo; its
  response lists every PR created.
- The tools depend on a dedicated org application service
  (`org/application/spectasks`), which depends on an infrastructure port
  (`runtime.SpecTasks`) whose `runtimehelix` impl calls the core helix code —
  matching the existing project-runtime layering. Tools never touch
  `store.Store` or the helix services directly.
- The approve/request-changes verbs delegate to the same workflow code the UI
  uses (`SpecDrivenTaskService.ApproveSpecs`, the `submitDesignReview`
  `request_changes` path, and the `approveImplementation` logic) — they do not
  reimplement the status machine.
- Each tool resolves the project from the **calling Worker's** runtime state
  (`LoadState(...).ProjectID`). A Worker can only touch spec tasks in its own
  project — no `projectID` is accepted from the LLM.
- The approver-identity question is resolved: when a Worker approves, commits
  use a defined identity (hiring user or task creator), not a silent fallback.
- If the calling Worker has no project assigned, tools return a clear error
  (mirroring `ErrProjectConfigUnsupported`).
- `create_spectask` produces a task identical in shape to one created via the
  REST API (task number, design-doc path, status `backlog`), because it calls
  the same `SpecDrivenTaskService.CreateTaskFromPrompt` — reused, not
  reimplemented.
- **No existing code is modified for reuse:** `api/pkg/agent/skill/project/*`
  and existing `api/pkg/services/spec_*` / `api/pkg/store` behaviour are
  untouched, so their tests pass unchanged. The only additive edit to existing
  code is the nil-guarded `Publisher` sink on `AttentionService` (eventing).
- New layers (port impl, application service, tools, eventing hook) are
  developed **test-first (TDD)**.
- The mutating tools are **not** added to `BaseReadTools` (the universal
  baseline); they are granted to a Worker only when listed in its Role.
- Unit tests cover each new tool (fake port) and the in-proc port
  implementation, following the `configure_worker_project_test.go` /
  `project_config_test.go` patterns.

## Out of Scope

- Spec-task attachments, design reviews, work sessions, Zed threads.
- Changes to the MCP gateway routing or the `helix-org` backend transport.
- New UI. This is a backend/MCP-tooling change only.
- **Follow-on (noted, not in scope):** de-duplicating the two existing
  spec-task notification paths — the Slack project-updates trigger
  (`SubscribeForTasks`) and `AttentionService` (UI + its own Slack replies).
  They overlap in event detection and formatting; a future task could make
  AttentionService the single source both consume. See design.md "Follow-on".
