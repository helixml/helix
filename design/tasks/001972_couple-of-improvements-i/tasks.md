# Implementation Tasks

## Cleanup: rename `PlanningSessionID` → `AgentSessionID` (do this FIRST)

Do this before the MCP-tools work so the new code reads naturally and we only update one set of call sites.

- [ ] Rename `SpecTask.PlanningSessionID` → `SpecTask.AgentSessionID` in `api/pkg/types/simple_spec_task.go` (struct field, JSON tag, GORM column tag → `column:agent_session_id`)
- [ ] Rename `SpecTaskFilters.PlanningSessionID` → `SpecTaskFilters.AgentSessionID` in the same file
- [ ] Delete the unused constants `AgentTypeSpecGeneration` and `AgentTypeImplementation` from `simple_spec_task.go` (verified zero non-definition usages)
- [ ] Add an explicit Postgres column-rename migration: `ALTER TABLE spec_tasks RENAME COLUMN planning_session_id TO agent_session_id;` (idempotent: gate on `IF EXISTS (... column_name = 'planning_session_id' ...)`); run it at startup before `AutoMigrate`
- [ ] Rename `Store.GetPendingCommentByPlanningSessionID` → `Store.GetPendingCommentByAgentSessionID` in `api/pkg/store/store.go`, `store_postgres.go`, `memorystore/memorystore.go`, and regenerate `store_mocks.go`
- [ ] Search-and-replace all Go callers of the old name (the field, the filter, the store method, the parameter name `planningSessionID`/`planningSessionId`)
- [ ] Update the comment-only "planning_session_id -> ..." reference at `api/pkg/server/server.go:112`
- [ ] Update the now-obsolete struct-comment block above the field (the apologising comment that says "single Helix session for entire workflow"); the new name is self-documenting, so remove the apology
- [ ] Run `./stack update_openapi` so swagger emits `agent_session_id`
- [ ] Search-and-replace all frontend references `task.planning_session_id` → `task.agent_session_id` and `planningSessionId` → `agentSessionId` in `frontend/src/`
- [ ] Verify: `grep -rn "PlanningSessionID\|planning_session_id" api/ frontend/src/` returns zero matches outside of the migration script
- [ ] Build verification: `go build ./pkg/server/ ./pkg/store/ ./pkg/types/` and `cd frontend && yarn build`

## Backend — data model

- [ ] Add `SpecTaskProposal` type, `SpecTaskProposalKind`, `SpecTaskProposalStatus` constants in `api/pkg/types/spec_task_proposal.go`
- [ ] Add `AgentMarkedCompleteAt` and `ParentTaskID` fields to `SpecTask` in `api/pkg/types/simple_spec_task.go` (with GORM tags + index on ParentTaskID)
- [ ] Add `SpecTaskProposalStore` interface methods to `api/pkg/store/store.go`: `CreateSpecTaskProposal`, `GetSpecTaskProposal`, `ListSpecTaskProposals(filters)`, `UpdateSpecTaskProposal`
- [ ] Implement Postgres store methods in `api/pkg/store/spec_task_proposal_store.go`
- [ ] Add the new types to `AutoMigrate` in `api/pkg/store/store_postgres.go`
- [ ] Add a memorystore stub in `api/pkg/store/memorystore/` so unit tests using the in-memory store keep working

## Backend — MCP tools

- [ ] Refactor `api/pkg/agent/skill/project/spec_task_create_tool.go:Execute` so the task-creation core is exposed as `services.CreateSpecTaskFromProposal(ctx, store, request)` and the existing tool calls into it
- [ ] Add `api/pkg/server/mcp_backend_helix_proposals.go` defining the three MCP tools (`propose_pull_request`, `propose_spec_task`, `mark_task_complete`) and their handlers
- [ ] Wire `addSpecTaskProposalTools()` into `HelixMCPBackend.addToolsFromAssistant` — invoked only when `session_id` resolves to a spec task session via `store.ListSpecTasks(filters{AgentSessionID: sessionID})` (uses the renamed filter from the cleanup section)
- [ ] Each tool handler: validate inputs, insert `SpecTaskProposal`, publish `spec_task.proposal.created` pubsub event, return `{proposal_id, status: "pending"}`

## Backend — REST API

- [ ] Add `api/pkg/server/spec_task_proposal_handlers.go` with three handlers: list-by-task, list-pending-by-project, decide
- [ ] Register routes in `api/pkg/server/server.go` (or wherever spec task routes live):
  - `GET /api/v1/spec-tasks/{id}/proposals`
  - `GET /api/v1/projects/{id}/proposals?status=pending`
  - `POST /api/v1/proposals/{id}/decide`
- [ ] Apply existing `authorizeUserToResource()` RBAC on all three
- [ ] `decide` handler: load proposal → apply `edited_payload` → set status/decided_by/decided_at → dispatch by kind:
  - `pull_request` → call `services.OpenPullRequestFromProposal` (new helper that wraps existing PR open logic, accepting custom `head_branch`/`base_branch`/`title`/`body`); on success store `result_pr_id`/`result_pr_url` and append a `RepoPR` to `task.RepoPullRequests`
  - `spec_task` → call `services.CreateSpecTaskFromProposal` (refactored above); set `ParentTaskID = proposal.SpecTaskID` on new task; store `result_task_id`
  - `mark_complete` → set `task.AgentMarkedCompleteAt = now`; if user clicked Approve (not Send Back), also set `task.Status = Done`, `task.CompletedAt = now`
- [ ] On failure, set `Status = failed`, store `result_error`
- [ ] Add six new prompt templates to `agent_instruction_service.go` next to the existing `revisionPromptTemplate` / `mergePromptTemplate` / `commentPromptTemplate`: `prProposalApprovedPromptTemplate`, `prProposalRejectedPromptTemplate`, `specTaskProposalApprovedPromptTemplate`, `specTaskProposalRejectedPromptTemplate`, `markCompleteConfirmedPromptTemplate`, `markCompleteSentBackPromptTemplate`
- [ ] Add `ProposalDecisionPromptData` struct (mirrors `ApprovalPromptData`) and `BuildProposalDecisionPrompt(task, proposal)` builder that selects the template by `proposal.Kind` + `proposal.Status`
- [ ] Add `SendProposalDecisionInstruction(ctx, task, proposal)` that renders the template and delivers it via the existing user-turn-message path (same call site already used for review comments today)
- [ ] Audit-log via `audit_log_service.go`
- [ ] Add swagger annotations to handlers; run `./stack update_openapi`

## Backend — orchestrator

- [ ] In `api/pkg/services/spec_task_orchestrator.go:handlePullRequest`, gate the `if allMerged && len(task.RepoPullRequests) > 0` auto-transition on `task.AgentMarkedCompleteAt == nil`
- [ ] Add unit test verifying: legacy task with all PRs merged → done; new task with `AgentMarkedCompleteAt` set and all PRs merged → stays in `pull_request` state pending user confirmation

## Backend — prompts

- [ ] Update `api/pkg/services/spec_task_prompts.go` planning template to mention `propose_spec_task` (and clarify `CreateSpecTask` is for Optimus only)
- [ ] Update `api/pkg/services/agent_instruction_service.go` implementation template: replace step 5 with the new `propose_pull_request` + `mark_task_complete` instructions; add `propose_spec_task` callout
- [ ] Verify both prompt builders still produce valid output for cloned tasks (`ClonedTaskPreamble` still injected correctly)

## Frontend — proposals UI

- [ ] Run `./stack update_openapi` after backend handlers land; verify generated client has `v1SpecTasksProposalsList`, `v1ProjectsProposalsPendingList`, `v1ProposalsDecideCreate`
- [ ] Add `frontend/src/hooks/useSpecTaskProposals.ts` (React Query hook); subscribe to `spec_task.proposal.created` pubsub for invalidation
- [ ] Add `frontend/src/components/specTask/PendingProposalsPanel.tsx` rendering all pending proposals for a task; one sub-component per kind:
  - `PRProposalCard` — editable head/base/title/body fields, Approve / Reject buttons
  - `TaskProposalCard` — editable name/description/type/priority, Approve / Reject buttons
  - `MarkCompleteProposalCard` — agent reason, Mark Done / Send Back buttons (Send Back opens a feedback textarea)
- [ ] Mount `PendingProposalsPanel` inside the existing task detail view (find current task detail component near `SpecTaskActionButtons.tsx`)
- [ ] Add a small badge to task cards on the kanban board indicating pending proposal count
- [ ] Add a project-level pending-proposals indicator using `v1ProjectsProposalsPendingList`

## Testing

- [ ] Unit tests for store CRUD on `SpecTaskProposal`
- [ ] Unit tests for the `decide` handler covering: approve PR with edits, approve task, mark complete, reject (with reason delivered to agent), failed dispatch (stays as `failed`)
- [ ] Unit test for the orchestrator gate (covered above under orchestrator)
- [ ] Frontend vitest test for the `useSpecTaskProposals` hook
- [ ] Manual end-to-end in helix-in-helix:
  1. Start a spec task; agent calls `propose_pull_request` with a non-default branch name
  2. Verify proposal surfaces in UI; edit the branch name; approve
  3. Verify push + PR opened on GitHub; `task.RepoPullRequests` updated
  4. Have agent call `propose_spec_task`; approve in UI; verify child task on board with `parent_task_id` set
  5. Have agent call `mark_task_complete`; click Mark Done; verify task transitions to `done`

## Documentation

- [ ] Update `api/pkg/services/spec_task_prompts.go` and `agent_instruction_service.go` docstrings to reflect the new tools (already covered by prompt edits)
- [ ] Add a section to `INTEGRATION_GUIDE.md` (root) describing the proposal lifecycle for integrators
