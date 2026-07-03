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

### 3. `connect_project` / `disconnect_project` — the connection primitive
The bot is "connected to" a project == subscribed to that project's
`KindSpecTask` topic. Because topics are created lazily by the attention
publisher, add a primitive that **ensures the topic exists and subscribes in one
call** (mirrors `create_bot`'s create-and-subscribe convenience).

- Extract the find-or-create logic from `attentionTopicPublisher.ensureTopic`
  into a shared helper so the tool and the publisher agree on the topic's
  identity/config (same `SpecTaskConfig{ProjectID}` match rule). Options:
  a small `EnsureSpecTaskTopic(ctx, topics, newID, now, orgID, projectID)`
  function in the server package, or a method on a shared struct. **Do not
  duplicate the ensure logic** — one implementation.
- `connect_project(project_id, botId?)`:
  1. authorize project ∈ caller's org (reuse the `Projects` port `Get`);
  2. ensure the `KindSpecTask` topic for the project;
  3. subscribe the bot (default: the caller) via the existing
     `subscriptions.Subscribe` use case.
- `disconnect_project(project_id, botId?)`: resolve project → topic, then
  `unsubscribe`. (If the topic doesn't exist, it's a no-op success.)

**Wiring note:** `connect_project` needs the topic-ensure seam, which lives in
the server package (it consumes the org `Topics` store + id/clock seams). Pass
an `EnsureProjectTopic` collaborator into `mcptools.Deps` (interface defined in
mcptools, implemented in server) to avoid an import cycle — same pattern as
`EventDispatcher` in `builtins.go`.

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
| Boundary | Enforcement |
|---|---|
| Cross-org project / task access | **Hard** — runtime asserts `project.OrganizationID == caller org`; cross-org `get_project`/task ops fail |
| Which same-org projects the bot manages | **Soft** — expressed by the bot's subscriptions (`connect_project`) + Role prompt |
| Tool availability | A tool is usable iff it's in the Bot's `Tools` (granted per Role) |

## Key decisions & rationale
- **Append `project_id` (optional) rather than a new parallel tool set.** One
  surface, backward compatible; empty arg = today's behaviour.
- **Org boundary is the only hard gate.** Per CLAUDE.md, a PM bot is a trusted
  org-level automation; hard-gating every cross-project call on subscription
  adds friction and Go logic for a low-cost violation. (Noted alternative: also
  require the bot be subscribed to the project before edits — deferred unless the
  cost of a mistaken cross-project edit proves high.)
- **Reuse the attention publisher's topic-ensure logic** for `connect_project`
  so triggering and connecting can never disagree on the topic.
- **Projects behind a runtime port**, mirroring `SpecTasks`, to preserve the
  org↔helix decoupling.

## Testing
- Unit: `resolveProject` (own vs named vs cross-org rejection); `ownedTask`
  against resolved project; `list_projects`/`get_project` org scoping;
  `connect_project` ensure-then-subscribe (incl. topic-not-yet-existing).
- E2E in inner Helix (`localhost:8080`): create two projects in one org, stand up
  a PM bot, `connect_project` to both, trigger an attention event on each
  (e.g. push specs / open PR), confirm the bot is activated and can
  `approve_spectask_spec` / `create_spectask_prs` on the *other* project by
  `project_id`. Confirm a project in a *second* org is not listable/editable.
- Verify `request_spectask_changes` persists the comment.
