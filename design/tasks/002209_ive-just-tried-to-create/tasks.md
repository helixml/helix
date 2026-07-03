# Implementation Tasks: Org-Wide Project Manager Bot for Cross-Project Spec Tasks

## 0. Verify the existing pieces
- [x] Verify the triggering path end-to-end: confirmed by reading + unit tests — `attentionTopicPublisher` find-or-creates a `KindSpecTask` topic and publishes the event (tests in `spec_task_attention_publisher_test.go`); dispatch → subscribed bot is the existing wired path. Full inner-Helix e2e tracked in section 5.
- [x] Verify each existing spec-task tool makes the correct status transition — covered by `runtime/helix/spectasks_test.go` (create→backlog, start→queued, approve→approved+delegate, request-changes→revision, create-PRs→PR view) and `mcptools` adapter tests.
- [x] Fix `request_spectask_changes` dropping the reviewer `comment`: `RequestChanges` now delivers the comment to the task's agent via a new `SpecTaskWorkflow.RequestChanges` (reuses `BuildRevisionInstructionPrompt` + `sendMessageToSpecTaskAgent`, the exact REST design-review mechanism). Best-effort delivery; the status transition is authoritative.
- [x] Improve `attentionTopicPublisher.PublishAttentionEvent` field coercion: `Title→Subject`, `Description→Body`, `SpecTaskID→ThreadID`, `ID→MessageID`; keep `event_type`/`project_id`/`project_name`/`spec_task_name` in `Extra`. Tests extended.

## 1. Cross-project targeting for spec-task tools
- [x] Add optional `ProjectID` (`json:"project_id,omitempty"`) to each args struct in `mcptools/spec_tasks.go` and pass it through.
- [x] Add a `projectID string` parameter to every method on the `runtime.SpecTasks` interface and `NoopSpecTasks` in `runtime.go`.
- [x] Forward `projectID` through `application/spectasks/spectasks.go` (keep caller identity extraction; worker stays the actor).
- [x] Add a caller org-membership check (`Queries.GetBot(orgID, botID)`) in `application/spectasks` (optional `MemberVerifier`, wired from `Queries` in `builtins.go`). NOTE: already enforced at the MCP mount (`buildMCPServer` → `GetBot`); service-level check is defensive depth, verifier is optional so unit tests need no store. `application/projects` gets the same treatment in section 2.
- [x] In `runtime/helix/spectasks.go`, replace `project()` with `resolveProject(ctx, orgID, workerID, projectID)`: empty → own project (unchanged); non-empty → load project, assert `project.OrganizationID == orgID` (hard cross-org block), acting user = worker's `HiringUserID`.
- [x] Update `ownedTask` to compare against the resolved project id (passes resolved projectID; unchanged helper).
- [x] Update tool descriptions to explain the optional `project_id` (omit = own project).

## 2. Helix project read tools (`list_projects`, `get_project`)
- [x] Add `runtime.Projects` port + `ProjectView` + `NoopProjects`/`ErrProjectsUnsupported` in `runtime.go`.
- [x] Implement `runtime/helix/projects.go` over the store (org-filtered `List`, `Get` with org assertion).
- [x] Add `application/projects` service (mirror `application/spectasks`, incl. optional `MemberVerifier`).
- [x] Add `mcptools/projects.go` with `list_projects` and `get_project` (`project_id`); register as reads in `builtins.go`. NOTE: dropped the `status` filter for now — `list_projects` takes no args (org from caller); a filter can be added later without changing the tool shape.
- [x] Wire the `Projects` service into `mcptools.Deps` + `Config.Build` and the helix impl in `helix_org.go`.

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
