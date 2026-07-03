# Implementation Tasks: Org-Wide Project Manager Bot for Cross-Project Spec Tasks

## 0. Verify the existing pieces
- [ ] Verify the triggering path end-to-end: emit an `AttentionEvent`, confirm a `KindSpecTask` topic is created and the event lands on it with `Extra = {spec_task_id, event_type, project_id}`, and a subscribed bot is activated.
- [ ] Verify each existing spec-task tool makes the correct status transition (create/start/review/approve/request-changes/create-PRs).
- [ ] Fix `request_spectask_changes` dropping the reviewer `comment` in `runtime/helix/spectasks.go` (persist it, or document the limitation in the tool description).

## 1. Cross-project targeting for spec-task tools
- [ ] Add optional `ProjectID` (`json:"project_id,omitempty"`) to each args struct in `mcptools/spec_tasks.go` and pass it through.
- [ ] Add a `projectID string` parameter to every method on the `runtime.SpecTasks` interface and `NoopSpecTasks` in `runtime.go`.
- [ ] Forward `projectID` through `application/spectasks/spectasks.go` (keep caller identity extraction; worker stays the actor).
- [ ] In `runtime/helix/spectasks.go`, replace `project()` with `resolveProject(ctx, orgID, workerID, projectID)`: empty → own project (unchanged); non-empty → load project, assert `project.OrganizationID == orgID` (hard cross-org block), acting user = worker's `HiringUserID`.
- [ ] Update `ownedTask` to compare against the resolved project id.
- [ ] Update tool descriptions to explain the optional `project_id` (omit = own project).

## 2. Helix project read tools (`list_projects`, `get_project`)
- [ ] Add `runtime.Projects` port + `ProjectView` + `NoopProjects`/`ErrProjectsUnsupported` in `runtime.go`.
- [ ] Implement `runtime/helix/projects.go` over the store (org-filtered `List`, `Get` with org assertion).
- [ ] Add `application/projects` service (mirror `application/spectasks`).
- [ ] Add `mcptools/projects.go` with `list_projects` (optional `status`) and `get_project` (`project_id`); register as reads in `builtins.go`.
- [ ] Wire the `Projects` service into `mcptools.Deps` + `Config.Build` and the helix impl in `helix_org.go`.

## 3. Connection primitive (`connect_project` / `disconnect_project`)
- [ ] Extract `attentionTopicPublisher.ensureTopic` into a shared `EnsureSpecTaskTopic` helper (no duplication) used by both the publisher and the new tool.
- [ ] Define an `EnsureProjectTopic` collaborator interface in mcptools (implemented in server) and add it to `Deps`/`Config` to avoid an import cycle.
- [ ] Implement `connect_project(project_id, botId?)`: authorize project ∈ org → ensure topic → subscribe via the shared `subscriptions.Subscribe` use case.
- [ ] Implement `disconnect_project(project_id, botId?)`: resolve project → topic → unsubscribe (no-op if topic absent).
- [ ] Register both tools in `builtins.go` (mutations, opt-in per Role — not in `BaseReadTools`).

## 4. PM-bot Role prompt + granting
- [ ] Add a PM-bot Role prompt template under `application/prompts/templates` describing same-org-only scope, discover→connect→filter→manage flow, and `project_id` usage.
- [ ] Confirm the new tools + existing spec-task tools are grantable per Role and that a bot created with them works.

## 5. Tests & build
- [ ] Unit tests: `resolveProject` (own / named / cross-org rejection), `ownedTask`, `list_projects`/`get_project` org scoping, `connect_project` ensure-then-subscribe (topic not-yet-existing).
- [ ] `go build ./api/pkg/org/... ./api/pkg/server/... ./api/pkg/types/` green.
- [ ] E2E in inner Helix: two projects in one org, PM bot connected to both, trigger an event on each, approve/create-PRs on the *other* project by `project_id`; confirm a second-org project is not listable/editable.
