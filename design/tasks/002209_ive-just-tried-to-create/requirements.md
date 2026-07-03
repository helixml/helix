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

### US-1: Route a project's events to the PM bot via the existing filter system
As an org owner, I want my org-wide PM bot to receive a project's spec-task
notification events using the org's existing topic + processing/filter system —
**not** via any new connect/disconnect tool — so I can wire it to several
projects at once for multi-project workflows.

**Acceptance Criteria**
- **No new "connect_project"/"disconnect_project" MCP tools.** Connection is
  expressed with the primitives that already exist: the per-project
  `KindSpecTask` topic (auto-created by `attentionTopicPublisher`), a **filter
  processor** (`processor.KindFilter`) whose predicate selects the wanted
  messages, and the existing `subscribe` use case.
- The filter predicate can select on the event payload the topic already
  carries — `.Message.extra` holds `{spec_task_id, event_type, project_id, ...}`
  (e.g. route only `pr_ready` / `ci_failed`, or a specific `project_id`).
- Relevant Helix notification fields are **coerced into the most appropriate
  first-class `streaming.Message` fields** (not only stuffed into `Extra`) so
  predicates and downstream consumers can use natural fields: the attention
  event's title → `Subject`, description → `Body`, spec-task id → `ThreadID`
  (so all events for one task thread together), attention-event id →
  `MessageID`. Structured routing keys that have no natural Message field
  (`event_type`, `project_id`) and denormalized display fields (`project_name`,
  `spec_task_name`) stay in `Extra`.
- Wiring one bot to N projects works and the bot is triggered by events from all
  wired projects (multiple filter routes / subscriptions, same machinery the
  Slack auto-router already uses via `Output.ManagedFor`).
- Cross-org routing is impossible: topics/events are already org-scoped, and the
  project-discovery tools (US-2) only return same-org projects.

### US-1b: Connection is configured at bot-creation time, driven by discovery
As an org owner, when I create the PM bot I want to be asked which projects to
connect it to, using the discovered project list (US-2), and have the wiring
(filter processor route + subscription) set up as part of creation using the
existing use cases.

**Acceptance Criteria**
- The bot-creation flow uses `list_projects` to present selectable projects.
- The selected projects are wired using the existing topic/processor/subscribe
  use cases (reused, not reimplemented), consistent with the helix-org
  "complete the action in as few steps as possible, reuse existing use cases"
  principle.
- No dedicated per-project connect tool is introduced.

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
  org, it receives events through the topics/filter routes it was wired to at
  creation, it can inspect the `event_type`/`spec_task_id`/`project_id` keys on
  each event, and it drives spec tasks by passing the target `project_id`.

## Out of Scope
- New notification/event *sources* beyond the existing `AttentionEvent` set
  (`specs_pushed`, `agent_interaction_completed`, `spec_failed`,
  `implementation_failed`, `pr_ready`, `ci_passed`, `ci_failed`).
- The parallel agent-skill project tools in `api/pkg/agent/skill/project/`
  (OpenAI function-calling, single-project). This work targets the org-graph
  MCP surface (`api/pkg/org/`).
- Merging PRs on GitHub (kept on GitHub, as today).
- Frontend UI for configuring the PM bot beyond what MCP tools already expose.
