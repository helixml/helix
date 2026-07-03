# Design: Org-Wide Project Manager Bot for Cross-Project Spec Tasks

## Guiding principles (from `helix/CLAUDE.md` — helix-org philosophy)
- **Prefer data/text over code.** Behaviour lives in the Role prompt, not Go.
- **Keep the MCP surface small.** MCP tools are org-graph primitives (reads +
  mutations of Workers/Topics/Subscriptions) plus the already-accepted spec-task
  primitives. New tools must be justified as primitives.
- **Complete a user action in as few steps as possible**, reusing existing use
  cases (e.g. `create_bot` subscribes at creation via the shared `subscribe`
  use case).
- **Social enforcement first; hard enforcement only when the cost of a violation
  is high.** Cross-**org** access is high cost → hard-enforced. Project
  selection within the org is expressed by the bot's connections + Role prompt.

## What already exists (verify, don't rebuild)
- **Event source:** `AttentionService.EmitEvent` → `AttentionEvent`
  (`api/pkg/types/attention_event.go`; 7 `EventType`s).
- **Helix→org bridge:** `attentionTopicPublisher.PublishAttentionEvent`
  (`api/pkg/server/spec_task_attention_publisher.go`) find-or-creates a
  per-project topic of `transport.KindSpecTask`
  (`api/pkg/org/domain/transport/spectask.go`) and publishes a
  `streaming.Message` whose `Extra` = `{spec_task_id, event_type, project_id}`.
  Wired at `api/pkg/server/helix_org.go:713`.
- **Dispatch/trigger:** publish → `dispatch.Dispatcher` fans out one activation
  per subscribed Bot. Bots read via `read_events` / are activated by the spawner.
- **Subscribe primitive:** `subscribe` / `unsubscribe` MCP tools +
  `application/subscriptions`.
- **Filter/processing system:** `processor.KindFilter` +
  `application/processors` + `application/processing` runner. A filter routes an
  input topic's messages to output topics by a template predicate rendered
  against the message (`.Message.extra` included). This is the connection
  mechanism — no new tools needed.

### 0. Coerce notification fields into first-class Message fields
Today `attentionTopicPublisher.PublishAttentionEvent` maps only `Title→Subject`
and `Description→Body` and drops the rest into `Extra`. Improve the mapping so
predicates and consumers can use natural `streaming.Message` fields:

| `AttentionEvent` | `streaming.Message` | Why |
|---|---|---|
| `Title` | `Subject` | (already) human headline |
| `Description` | `Body` (`text/plain`) | (already) detail |
| `SpecTaskID` | `ThreadID` | all events for one task thread together — enables per-task conversations and `.Message.thread_id` routing |
| `ID` | `MessageID` | stable, unique id (dedupe / `InReplyTo` chaining) |
| `EventType`, `ProjectID` | `Extra` | routing keys with no natural Message field |
| `ProjectName`, `SpecTaskName` | `Extra` | denormalized display, avoids a lookup |

Notes: keep `event_type`/`project_id` in `Extra` **and** thread by `SpecTaskID`
so a predicate can route by type/project while the bot still sees a coherent
per-task thread. Leave `From` empty (the event is system-sourced; `Event.Source`
already records provenance). This is a change to the existing publisher only —
no new event source.

## The gap
The 8 spec-task MCP tools resolve the target project as the **Worker's own
project only**:
`helix/spectasks.go` → `s.project(ctx, orgID, workerID)` reads a single
`ProjectID` from `WorkerRuntimeState` and `ownedTask` rejects any task whose
`ProjectID` differs. There is no way to target another project, no
`list_projects` / `get_project`, and topics are created lazily so a bot cannot
proactively connect to a quiet project.

## Architecture changes

### 1. Cross-project targeting for spec-task tools
Thread an optional `projectID` through the whole spec-task stack. Empty =
current behaviour (own project); non-empty = named project, org-authorized.

- **MCP adapters** (`mcptools/spec_tasks.go`): add optional `ProjectID string`
  (`json:"project_id,omitempty"`) to each args struct; pass through.
- **Application service** (`application/spectasks/spectasks.go`): forward the
  `projectID` to the port. Still extracts caller org/worker identity; the worker
  remains the actor, `projectID` is just the target.
- **Runtime port** (`infrastructure/runtime/runtime.go`): extend the
  `SpecTasks` interface methods with a `projectID string` parameter (update
  `NoopSpecTasks` too). *Append the param* — keep the signatures otherwise
  identical.
- **Helix impl** (`runtime/helix/spectasks.go`): replace `project()` with
  `resolveProject(ctx, orgID, workerID, projectID)`:
  - `projectID == ""` → existing behaviour (own project from `WorkerRuntimeState`,
    plus `HiringUserID` for the acting user).
  - `projectID != ""` → `tasks.GetProject(projectID)`; **assert
    `project.OrganizationID == orgID`** (hard cross-org block, returns a
    permission error); acting user = the Worker's `HiringUserID` from runtime
    state (for `SpecApprovedBy`, PR authorship, `Create.UserID`).
  - `ownedTask` compares against the *resolved* project id.

**Why the org check in the runtime (not the app layer):** the app layer never
sees a project row; the runtime is where projects and org ids are available and
where the existing `ownedTask` guard lives. This keeps org-boundary enforcement
in one place.

### 2. New Helix project read tools + `Projects` port
Projects are a Helix concept, so (like `SpecTasks`) they go behind a runtime
port so `api/pkg/org/` stays decoupled from the Helix store.

- New port `runtime.Projects` (in `runtime.go`) with `NoopProjects`
  (`ErrProjectsUnsupported`) as the default:
  - `List(ctx, orgID) ([]ProjectView, error)`
  - `Get(ctx, orgID, projectID) (ProjectView, error)` — org-scoped;
    cross-org id → not-found/permission error.
- `ProjectView`: `id, name, description, status, default_repo_id,
  default_helix_app_id` (append-only projection; do not leak the whole model).
- Helix impl in `runtime/helix/projects.go` over the store
  (`ListProjects` filtered by org, `GetProject` + org assertion). Wire in
  `helix_org.go` next to `NewSpecTasks`.
- Two MCP tools in `mcptools/projects.go`: `list_projects` (optional `status`
  filter), `get_project` (`project_id`). Register in `builtins.go` as reads.
- Add `Projects *projects.Service` (new thin `application/projects` service,
  mirroring `application/spectasks`) to `mcptools.Deps` + `Config`.

### 3. Connection via the existing filter-processor system (NO new tools)
**Decision (review feedback): do not add `connect_project`/`disconnect_project`
tools.** The org already has a flexible topic + processing/filter system; a bot
is connected to a project's events using those primitives, and project selection
is driven by discovery (§2) at bot-creation time.

How the pieces already fit:
- Each project's spec-task events already stream on its `KindSpecTask` topic,
  and each `streaming.Message` carries `Extra = {spec_task_id, event_type,
  project_id}`.
- A **filter processor** (`processor.KindFilter`,
  `api/pkg/org/domain/processor/filter.go`) reads an input topic, renders a Go
  `text/template` **predicate against the message context** — which exposes
  `.Message.extra` (`api/pkg/org/domain/processor/template.go` `templateData`) —
  and republishes matching messages to its output topic(s). So a predicate like
  one keying on `.Message.extra.event_type` / `.Message.extra.project_id`
  "filters messages for a bot" with zero new code.
- The bot subscribes to the processor's output topic (or directly to the
  project topic) via the existing `subscribe` use case; the dispatcher then
  activates it. The Slack auto-router already demonstrates reconciler-owned
  filter routes per Worker via `Output.ManagedFor` — the same pattern applies if
  we later want managed PM-bot routes.

What this task must provide for connection:
- **Nothing new on the MCP tool surface.** Verify a filter processor can be
  created (existing `application/processors` use case) with an input of a
  project's `KindSpecTask` topic and a predicate over `.Message.extra`, and that
  a subscribed bot is triggered.
- **Deterministic input topic at wiring time.** Today the `KindSpecTask` topic
  is created lazily on the first attention event, so wiring before any event has
  fired has nothing to point at. Fix by extracting the find-or-create logic in
  `attentionTopicPublisher.ensureTopic` into a shared helper
  (`EnsureSpecTaskTopic(...)`) and calling it from the bot-creation/wiring path
  (reused, **not** a new MCP tool) so the input topic exists deterministically.
  One implementation of the ensure logic, shared with the publisher.
- **Bot-creation UX** uses `list_projects` (§2) to offer selectable projects and
  wires the chosen ones with the existing topic/processor/subscribe use cases.
  No dedicated per-project connect verb.

### 4. Granting + Role prompt (data, not code)
- The new tools are **not** added to `BaseReadTools`. They are opt-in, granted
  per Role (the spec-task tools already follow this — `builtins.go:296`).
- Provide a PM-bot Role prompt (a template under
  `application/prompts/templates`, following the existing prompt-template
  pattern) that states: manage only same-org projects; `list_projects` to
  discover; `connect_project` to start receiving events; filter events by the
  `event_type` / `spec_task_id` keys on `read_events`; then drive tasks with the
  spec-task tools passing `project_id`.

## Authorization model (summary)
Enforced as a fixed pipeline every discovery/spec-task tool runs, in shared
layers so no tool can skip a step:

1. **Trusted caller only.** `orgID` and `botID` come from the authenticated MCP
   invocation (`inv.Caller`), never from tool JSON args. A tool that reads an org
   id from its args as the auth basis is a bug.
2. **Caller is an org member.** Verify the bot exists in that org via
   `Queries.GetBot(orgID, botID)` before any work (this is what `read_events`
   already does). No org id / bot-not-in-org → reject. Since this needs store
   access (today's `callerIdentity` is pure), thread the `Queries` facade into
   the `application/spectasks` + `application/projects` services and run the
   `GetBot` check there, so both surfaces enforce it uniformly rather than each
   tool re-implementing it.
3. **Every project_id is org-owned.** `list_projects` filters by `orgID`;
   `get_project` and any spec-task tool with a `project_id` assert
   `project.OrganizationID == orgID` in the runtime `resolve*` (not-found /
   permission error otherwise).
4. **Every task_id is project-owned.** `ownedTask` asserts `task.ProjectID ==`
   the resolved (already org-verified) project — so a task is transitively
   org-verified, and a cross-org task id is rejected.

| Boundary | Enforcement |
|---|---|
| Caller is a member of the org | **Hard** — `GetBot(orgID, botID)` in the shared `callerIdentity`; org/identity taken only from `inv.Caller` |
| Cross-org project / task access | **Hard** — runtime asserts `project.OrganizationID == caller org`; `ownedTask` chains task→project→org; cross-org ids fail |
| Which same-org projects the bot manages | **Soft** — expressed by the bot's filter routes / subscriptions + Role prompt |
| Tool availability | A tool is usable iff it's in the Bot's `Tools` (granted per Role) |

## Key decisions & rationale
- **Append `project_id` (optional) rather than a new parallel tool set.** One
  surface, backward compatible; empty arg = today's behaviour.
- **Org boundary is the only hard gate.** Per CLAUDE.md, a PM bot is a trusted
  org-level automation; hard-gating every cross-project call on subscription
  adds friction and Go logic for a low-cost violation. (Noted alternative: also
  require the bot be subscribed to the project before edits — deferred unless the
  cost of a mistaken cross-project edit proves high.)
- **No dedicated connect/disconnect tools** (review feedback). Reuse the org's
  topic + filter-processor + subscribe primitives; drive project selection from
  `list_projects` at bot-creation time. Share the attention publisher's
  topic-ensure logic (refactor, not a new tool) so wiring has a deterministic
  input topic.
- **Projects behind a runtime port**, mirroring `SpecTasks`, to preserve the
  org↔helix decoupling.

## Testing
- Unit: `resolveProject` (own vs named vs cross-org rejection); `ownedTask`
  against resolved project; `list_projects`/`get_project` org scoping;
  `EnsureSpecTaskTopic` find-or-create idempotency; a filter predicate over
  `.Message.extra` (project_id / event_type) selecting/dropping correctly.
- E2E in inner Helix (`localhost:8080`): create two projects in one org, stand up
  a PM bot, wire it to both via the existing topic/filter-processor/subscribe
  path, trigger an attention event on each (e.g. push specs / open PR), confirm
  the bot is activated and can `approve_spectask_spec` / `create_spectask_prs` on
  the *other* project by `project_id`. Confirm a project in a *second* org is not
  listable/editable.
- Verify `request_spectask_changes` persists the comment.

## Implementation Notes (as-built)

- **Membership was already enforced at the MCP mount.** `buildMCPServer`
  (`api/pkg/org/interfaces/server/mcp.go`) calls `GetBot(orgID, botID)` before
  building the caller and derives the caller's org from the *persisted*
  `bot.OrganizationID` (not spoofable). The service-level `MemberVerifier` we
  added (`application/spectasks` + `application/projects`) is defensive depth; it
  is wired from `Queries` in `builtins.go` and is optional (nil → skipped) so
  unit tests need no store. Because `DefaultDeps` wires `Queries`, two existing
  deps tests had to seed a real bot row.
- **Cross-project + org boundary** live in `runtime/helix/spectasks.go`
  `resolveProject`: empty projectID → own project (unchanged); non-empty → load
  project + assert `OrganizationID == caller org`. `ownedTask` then chains
  task→project. Interface change: every `runtime.SpecTasks` verb took a new
  `projectID string` param (threaded through the service + tool adapters).
- **`request_spectask_changes` comment fix**: added `SpecTaskWorkflow.RequestChanges`
  to the port; the server wrapper (`spec_tasks_org_wiring.go`) reuses
  `services.BuildRevisionInstructionPrompt` + `sendMessageToSpecTaskAgent`
  (the exact REST design-review mechanism). Delivery is best-effort; the status
  transition is authoritative.
- **Projects port** (`runtime/helix/projects.go`) imports the helix store
  (`pkg/store`) for `ListProjectsQuery` — no import cycle (`pkg/store` doesn't
  import `pkg/org`). `helixStore` (the big `store.Store` interface) satisfies the
  structural `ProjectStore`.
- **Filter routing gotcha**: `.Message.extra` is raw JSON bytes; predicates read
  it via built-in `printf "%s"` then `contains`. The coercion of `SpecTaskID →
  ThreadID` means per-task routing uses the first-class `.Message.thread_id`,
  which is cleaner than digging into `extra`.
- **No new connect/disconnect tools** (per review). `EnsureSpecTaskTopic`
  (extracted from the attention publisher) lets a wiring path pre-create a
  project's topic; connection = existing `create_bot`/`subscribe` + optional
  filter processor, guided by the new `/pm-bot` prompt.
- **Prompt count**: prompt tests use ad-hoc registries, so adding the `/pm-bot`
  builtin didn't break any count assertion.
