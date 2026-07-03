# Implementation Tasks: Org-Wide Project Manager Bot for Cross-Project Spec Tasks

## 0. Verify the existing pieces
- [ ] Verify the triggering path end-to-end: emit an `AttentionEvent`, confirm a `KindSpecTask` topic is created and the event lands on it with `Extra = {spec_task_id, event_type, project_id}`, and a subscribed bot is activated.
- [ ] Verify each existing spec-task tool makes the correct status transition (create/start/review/approve/request-changes/create-PRs).
- [ ] Fix `request_spectask_changes` dropping the reviewer `comment` in `runtime/helix/spectasks.go` (persist it, or document the limitation in the tool description).
- [ ] Improve `attentionTopicPublisher.PublishAttentionEvent` field coercion: `Title→Subject`, `Description→Body`, `SpecTaskID→ThreadID`, `ID→MessageID`; keep `event_type`/`project_id`/`project_name`/`spec_task_name` in `Extra`. Update/extend its tests.

## 1. Cross-project targeting for spec-task tools
- [ ] Add optional `ProjectID` (`json:"project_id,omitempty"`) to each args struct in `mcptools/spec_tasks.go` and pass it through.
- [ ] Add a `projectID string` parameter to every method on the `runtime.SpecTasks` interface and `NoopSpecTasks` in `runtime.go`.
- [ ] Forward `projectID` through `application/spectasks/spectasks.go` (keep caller identity extraction; worker stays the actor).
- [ ] Add a caller org-membership check (`Queries.GetBot(orgID, botID)`) in the `application/spectasks` + `application/projects` services (thread in the `Queries` facade) so every tool verifies the bot belongs to the org; take org/identity only from `inv.Caller`, never from tool args.
- [ ] In `runtime/helix/spectasks.go`, replace `project()` with `resolveProject(ctx, orgID, workerID, projectID)`: empty → own project (unchanged); non-empty → load project, assert `project.OrganizationID == orgID` (hard cross-org block), acting user = worker's `HiringUserID`.
- [ ] Update `ownedTask` to compare against the resolved project id.
- [ ] Update tool descriptions to explain the optional `project_id` (omit = own project).

## 2. Helix project read tools (`list_projects`, `get_project`)
- [ ] Add `runtime.Projects` port + `ProjectView` + `NoopProjects`/`ErrProjectsUnsupported` in `runtime.go`.
- [ ] Implement `runtime/helix/projects.go` over the store (org-filtered `List`, `Get` with org assertion).
- [ ] Add `application/projects` service (mirror `application/spectasks`).
- [ ] Add `mcptools/projects.go` with `list_projects` (optional `status`) and `get_project` (`project_id`); register as reads in `builtins.go`.
- [ ] Wire the `Projects` service into `mcptools.Deps` + `Config.Build` and the helix impl in `helix_org.go`.

## 3. Connection via existing filter/processor system (NO new connect tools)
- [ ] Confirm a `processor.KindFilter` can be created (existing `application/processors` use case) with a project's `KindSpecTask` topic as input and a predicate over `.Message.extra` (`event_type` / `project_id`), routing to an output topic a bot subscribes to.
- [ ] Extract `attentionTopicPublisher.ensureTopic` into a shared `EnsureSpecTaskTopic` helper (single implementation) so the bot-creation/wiring path can pre-create the input topic deterministically — this is a refactor, **not** a new MCP tool.
- [ ] Wire the bot-creation flow to offer projects from `list_projects` and set up the chosen projects' filter route + subscription using the existing topic/processor/subscribe use cases (reused, not reimplemented).
- [ ] Do NOT add `connect_project`/`disconnect_project` tools.

## 4. PM-bot Role prompt + granting
- [ ] Add a PM-bot Role prompt template under `application/prompts/templates` describing same-org-only scope, how events arrive via wired topics/filter routes, inspecting the `event_type`/`project_id` keys, and driving tasks with `project_id`.
- [ ] Confirm the new tools + existing spec-task tools are grantable per Role and that a bot created with them works.

## 5. Tests & build
- [ ] Unit tests: caller org-membership rejection (bot not in org), `resolveProject` (own / named / cross-org rejection), `ownedTask` (cross-org task id rejected), `list_projects`/`get_project` org scoping, `EnsureSpecTaskTopic` idempotency, filter predicate over `.Message.extra`.
- [ ] `go build ./api/pkg/org/... ./api/pkg/server/... ./api/pkg/types/` green.
- [ ] E2E in inner Helix: two projects in one org, PM bot connected to both, trigger an event on each, approve/create-PRs on the *other* project by `project_id`; confirm a second-org project is not listable/editable.
