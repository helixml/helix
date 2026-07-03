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
- [x] Confirm a `processor.KindFilter` routes a project's spec-task events to a bot — test `TestFilterRoutesSpecTaskEventsToBot` in `processor/filter_test.go` routes on both `.Message.thread_id` (spec task, now a first-class field) and `.Message.extra` keys (`event_type`/`project_id` via `printf "%s"`).
- [x] Extract `attentionTopicPublisher.ensureTopic` into a shared `EnsureSpecTaskTopic` helper (single implementation, idempotent) so a wiring path can pre-create the input topic deterministically. Refactor, not a new MCP tool.
- [x] Bot→project wiring uses existing primitives only: `list_projects` (discovery) → `EnsureSpecTaskTopic` (deterministic input topic) → existing `create_bot`/`subscribe` (subscribe the bot to the topic) → optional `create_topic`+filter `processor` for event-type filtering. Driven by the PM-bot Role prompt (section 4); no dedicated UI or connect verb. NOTE: no separate frontend form — helix-org bots are created via the `create_bot` MCP tool in owner-chat, so "ask which projects" is prompt-driven.
- [x] Did NOT add `connect_project`/`disconnect_project` tools.

## 4. PM-bot Role prompt + granting
- [x] Added the `/pm-bot` prompt (`application/prompts/pm_bot.go` + `templates/pm_bot.md`, registered in prompts `builtins.go`): drafts an org-wide PM bot — discover projects via `list_projects`, same-org-only scope, how events arrive (`subject`/`thread_id`/`extra` keys), grant the spec-task + discovery + topic tools, subscribe to the projects' `Spec tasks: <projectId>` topics, drive tasks with `project_id`.
- [x] Confirmed the project + spec-task tools are grantable per Role and NOT in `BaseReadTools` (`projects_registration_test.go`, `spec_tasks_registration_test.go`).

## 5. Tests & build
- [ ] Unit tests: caller org-membership rejection (bot not in org), `resolveProject` (own / named / cross-org rejection), `ownedTask` (cross-org task id rejected), `list_projects`/`get_project` org scoping, `EnsureSpecTaskTopic` idempotency, filter predicate over `.Message.extra`.
- [ ] `go build ./api/pkg/org/... ./api/pkg/server/... ./api/pkg/types/` green.
- [ ] E2E in inner Helix: two projects in one org, PM bot connected to both, trigger an event on each, approve/create-PRs on the *other* project by `project_id`; confirm a second-org project is not listable/editable.
