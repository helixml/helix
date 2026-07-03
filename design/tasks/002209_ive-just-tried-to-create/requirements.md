# Requirements: Org-Wide Project Manager Bot for Cross-Project Spec Tasks

## Background

We want an org-wide **project manager (PM) bot** — a helix-org Bot/Worker that
can be connected to one or more Helix **projects within its own organization**
(never another org) and can drive the state of those projects' spec tasks in
response to Helix notification events.

A previous change added a limited set of org MCP spec-task tools
(`create_spectask`, `list_spectasks`, `get_spectask`, `start_spectask_planning`,
`review_spectask_spec`, `approve_spectask_spec`, `request_spectask_changes`,
`create_spectask_prs` — see `api/pkg/org/interfaces/mcptools/spec_tasks.go`).
That implementation is **incomplete for a PM bot** because every tool is
hard-scoped to the calling Worker's *own* project
(`api/pkg/org/infrastructure/runtime/helix/spectasks.go` → `s.project()` +
`ownedTask()`). A PM bot needs to act on *other* projects it manages.

The **triggering side already exists** and works: the `AttentionService`
(`api/pkg/services/attention_service.go`) emits `AttentionEvent`s (the same
events that back the UI "Needs Attention" notifications panel), and
`attentionTopicPublisher` (`api/pkg/server/spec_task_attention_publisher.go`,
wired at `api/pkg/server/helix_org.go:713`) republishes each one onto a
per-project org Topic of `transport.KindSpecTask`. Every event carries an
`Extra` payload `{spec_task_id, event_type, project_id}` — the "key" a bot can
filter on. A Bot subscribed to that topic is triggered via the normal dispatch
path. This side needs verification and a small ergonomics fix (topics are
created lazily, so a bot cannot connect to a project that has never emitted an
event yet).

## User Stories

### US-1: Connect the PM bot to a project (multi-project, org-scoped)
As an org owner, I want to connect my org-wide PM bot to a specific project in
my org so that the bot starts receiving that project's spec-task notification
events and can manage its tasks — and I want to connect it to several projects
at once for multi-project workflows.

**Acceptance Criteria**
- A `connect_project` MCP tool ensures the project's `KindSpecTask` topic exists
  (creating it on demand, even if no attention event has fired yet) and
  subscribes the target bot to it, in one call.
- The tool rejects a `project_id` that belongs to a **different organization**
  than the caller (hard cross-org block).
- Connecting the same bot to N projects works; the bot receives events from all
  connected projects.
- A `disconnect_project` tool (or documented use of `unsubscribe`) removes the
  connection.

### US-2: Discover projects in the org
As the PM bot, I want to list the projects in my org and read a single
project's details so I can decide which ones to manage and reference their ids.

**Acceptance Criteria**
- A `list_projects` MCP tool returns the projects in the caller's org
  (id, name, description, status; optional status filter).
- A `get_project` MCP tool returns one project by id, scoped to the caller's org
  (cross-org id returns not-found / access error).

### US-3: Manage spec tasks on a connected project
As the PM bot, I want the existing spec-task tools to operate on a project I
name (not only my own runtime project) so I can list, start, review, approve,
request changes on, and open PRs for tasks in the projects I manage.

**Acceptance Criteria**
- Each spec-task tool accepts an optional `project_id` argument.
- When `project_id` is supplied, the tool acts on that project, provided it
  belongs to the caller's org (hard cross-org block enforced in the runtime).
- When `project_id` is omitted, behaviour is unchanged (acts on the Worker's own
  project) — existing single-project Workers keep working.
- Every referenced `task_id` is verified to belong to the resolved project
  (an operation on a task outside the resolved project is rejected).

### US-4: Existing spec-task tools verified correct
As a developer, I want the existing spec-task tools double-checked so the PM bot
can rely on them.

**Acceptance Criteria**
- Each tool is exercised end-to-end and confirmed to make the correct status
  transition.
- Known gap fixed: `request_spectask_changes` currently only flips status and
  bumps the revision count, dropping the reviewer's `comment`
  (`helix/spectasks.go` RequestChanges). The comment must be persisted where the
  agent/human can see it (matching, as far as practical, the REST/UI review
  path), or the limitation must be explicitly documented in the tool
  description.

### US-5: PM bot is grantable and documented
As an org owner, I want to stand up a PM bot with the right tools and a role
prompt describing its cross-project, org-scoped responsibilities.

**Acceptance Criteria**
- The new tools plus the existing spec-task tools can be granted to a bot's Role
  (they are not part of `BaseReadTools`; they are opt-in per Role).
- A PM-bot Role prompt exists that explains: it only manages projects in its own
  org, it must `connect_project` to receive events, and it filters events by the
  `event_type`/`spec_task_id` key on the topic.

## Out of Scope
- New notification/event *sources* beyond the existing `AttentionEvent` set
  (`specs_pushed`, `agent_interaction_completed`, `spec_failed`,
  `implementation_failed`, `pr_ready`, `ci_passed`, `ci_failed`).
- The parallel agent-skill project tools in `api/pkg/agent/skill/project/`
  (OpenAI function-calling, single-project). This work targets the org-graph
  MCP surface (`api/pkg/org/`).
- Merging PRs on GitHub (kept on GitHub, as today).
- Frontend UI for configuring the PM bot beyond what MCP tools already expose.
