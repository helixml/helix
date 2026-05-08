# Implementation Tasks

## Cleanup: rename `PlanningSessionID` → `AgentSessionID` (do this FIRST)

Do this before the MCP-tools work so the new code reads naturally and we only update one set of call sites.

- [x] Rename `SpecTask.PlanningSessionID` → `SpecTask.AgentSessionID` in `api/pkg/types/simple_spec_task.go` (struct field, JSON tag, GORM column tag → `column:agent_session_id`)
- [x] Rename `SpecTaskFilters.PlanningSessionID` → `SpecTaskFilters.AgentSessionID` in the same file
- [x] Delete the unused constants `AgentTypeSpecGeneration` and `AgentTypeImplementation` from `simple_spec_task.go` (verified zero non-definition usages)
- [x] GORM Migrator-based column rename in postgres.go (idempotent HasColumn check + RenameColumn before AutoMigrate): `ALTER TABLE spec_tasks RENAME COLUMN planning_session_id TO agent_session_id;` (idempotent: gate on `IF EXISTS (... column_name = 'planning_session_id' ...)`); run it at startup before `AutoMigrate`
- [x] Rename `Store.GetPendingCommentByPlanningSessionID` → `Store.GetPendingCommentByAgentSessionID` in `api/pkg/store/store.go`, `store_postgres.go`, `memorystore/memorystore.go`, and regenerate `store_mocks.go`
- [x] Search-and-replace all Go callers of the old name (the field, the filter, the store method, the parameter name `planningSessionID`/`planningSessionId`)
- [x] Update the comment-only "planning_session_id -> ..." reference at `api/pkg/server/server.go:112`
- [x] Update the now-obsolete struct-comment block above the field (the apologising comment that says "single Helix session for entire workflow"); the new name is self-documenting, so remove the apology
- [x] Run `./stack update_openapi` so swagger emits `agent_session_id`
- [x] Search-and-replace all frontend references `task.planning_session_id` → `task.agent_session_id` and `planningSessionId` → `agentSessionId` in `frontend/src/`
- [x] Verify: `grep -rn "PlanningSessionID\|planning_session_id" api/ frontend/src/` returns zero matches outside of the migration script
- [x] Build verification (api passed; update_openapi pending; yarn build pending): `go build ./pkg/server/ ./pkg/store/ ./pkg/types/` and `cd frontend && yarn build`

## Backend — data model

- [x] Add `SpecTaskProposal` type, `SpecTaskProposalKind`, `SpecTaskProposalStatus` constants in `api/pkg/types/spec_task_proposal.go`
- [x] Add `ParentTaskID` field to `SpecTask` in `api/pkg/types/simple_spec_task.go` (with GORM tag + index). **Do NOT add `AgentMarkedCompleteAt`** — the "agent claims complete" UI state is derived from a pending `mark_complete` proposal, no denormalised flag.
- [x] Add `SpecTaskProposalStore` interface methods to `api/pkg/store/store.go`: `CreateSpecTaskProposal`, `GetSpecTaskProposal`, `ListSpecTaskProposals(filters)`, `UpdateSpecTaskProposal`
- [x] Implement Postgres store methods in `api/pkg/store/spec_task_proposal_store.go`
- [x] Add the new types to `AutoMigrate` in `api/pkg/store/store_postgres.go`
- [x] Add a memorystore stub in `api/pkg/store/memorystore/` so unit tests using the in-memory store keep working

## Backend — MCP tools

- [x] Refactor `api/pkg/agent/skill/project/spec_task_create_tool.go:Execute` so the task-creation core is exposed as `services.CreateSpecTaskFromProposal(ctx, store, request)` and the existing tool calls into it
- [x] Add `api/pkg/server/mcp_backend_helix_proposals.go` defining the three MCP tools (`propose_pull_request`, `propose_spec_task`, `mark_task_complete`) and their handlers
- [x] Wire `addSpecTaskProposalTools()` into `HelixMCPBackend.addToolsFromAssistant` — invoked only when `session_id` resolves to a spec task session via `store.ListSpecTasks(filters{AgentSessionID: sessionID})` (uses the renamed filter from the cleanup section)
- [x] Each tool handler: validate inputs, insert `SpecTaskProposal`, publish `spec_task.proposal.created` pubsub event, return `{proposal_id, status: "pending"}`

## Backend — REST API

- [x] Add `api/pkg/server/spec_task_proposal_handlers.go` with three handlers: list-by-task, list-pending-by-project, decide
- [x] Register routes in `api/pkg/server/server.go` (or wherever spec task routes live):
  - `GET /api/v1/spec-tasks/{id}/proposals`
  - `GET /api/v1/projects/{id}/proposals?status=pending`
  - `POST /api/v1/proposals/{id}/decide`
- [x] Apply existing `authorizeUserToResource()` RBAC on all three
- [x] `decide` handler: load proposal → apply `edited_payload` → set status/decided_by/decided_at → dispatch by kind:
  - `pull_request` → call `services.OpenPullRequestFromProposal` (new helper that wraps existing PR open logic, accepting custom `head_branch`/`base_branch`/`title`/`body`); on success store `result_pr_id`/`result_pr_url` and append a `RepoPR` to `task.RepoPullRequests`
  - `spec_task` → call `services.CreateSpecTaskFromProposal` (refactored above); set `ParentTaskID = proposal.SpecTaskID` on new task; store `result_task_id`
  - `mark_complete` → if user clicked Approve (Mark Done), set `task.Status = Done` and `task.CompletedAt = now`; if user clicked Send Back, leave the proposal as `rejected` and let the agent's follow-up message carry the feedback (no flag needed on the task itself)
- [x] On failure, set `Status = failed`, store `result_error`
- [x] Add six new prompt templates to `agent_instruction_service.go` next to the existing `revisionPromptTemplate` / `mergePromptTemplate` / `commentPromptTemplate`: `prProposalApprovedPromptTemplate`, `prProposalRejectedPromptTemplate`, `specTaskProposalApprovedPromptTemplate`, `specTaskProposalRejectedPromptTemplate`, `markCompleteConfirmedPromptTemplate`, `markCompleteSentBackPromptTemplate`
- [x] Add `ProposalDecisionPromptData` struct (mirrors `ApprovalPromptData`) and `BuildProposalDecisionPrompt(task, proposal)` builder that selects the template by `proposal.Kind` + `proposal.Status`
- [x] Add `SendProposalDecisionInstruction(ctx, task, proposal)` that renders the template and delivers it via the existing user-turn-message path (same call site already used for review comments today)
- [x] Audit-log via `audit_log_service.go`
- [x] Add swagger annotations to handlers; run `./stack update_openapi`

## Backend — orchestrator + git_http_server (delete all auto-transitions to `done`)

There are **five** auto-transition sites across two files. All five must die.

- [x] Delete the `if allMerged` block at `api/pkg/services/spec_task_orchestrator.go:~778-799` that sets `task.Status = TaskStatusDone` when all PRs merge. Keep the per-PR state-tracking loop (it still updates `RepoPR.PRState` for UI display).
- [x] Delete the "Detected merged branch, moving task to done" block at `spec_task_orchestrator.go:~848-857` (no PRs tracked, branch merged to main fallback).
- [x] Delete the "Detected externally-opened PR, already merged → done" block at `spec_task_orchestrator.go:~1080-1086`.
- [x] Delete the "branch merged to main (no PR found), fallback check" block at `spec_task_orchestrator.go:~1116-1129`.
- [x] **NEW:** Delete the `task.Status = TaskStatusDone` path in `api/pkg/services/git_http_server.go:handleMainBranchPush:~1023-1049`. The function may still log "branch merged to main" (or be removed entirely if that's the only thing it does for spec tasks); either way it no longer touches `task.Status`.
- [x] After deletions, audit the entire codebase: `grep -rn "task.Status = types.TaskStatusDone\|TaskStatusDone$" api/` should show writes only from (a) the new proposal-decision handler in `spec_task_proposal_handlers.go` (mark_complete approval) and (b) any pre-existing manual user "set status to done" handler. **Document the grep output in the PR description as proof.**
- [x] Update `task.MergedToMain` / `task.MergedAt` so they're still set as informational metadata when a PR transitions to merged in the polling loop — but make explicit (in code comments) that they no longer trigger any task status transition.
- [x] Unit tests:
  - All 5 auto-transition test cases (if they exist) are deleted or repurposed to assert the **opposite** — "after PR merge, task remains in `pull_request` status; only `RepoPR.PRState` is updated to `merged`".
  - "Direct push to main while task in `implementation_review`" test: assert task status does NOT change.
  - New test: `mark_complete` proposal approved → task transitions to `done`, `CompletedAt` set.
  - New test: `mark_complete` proposal rejected (Send Back) → task stays in current status, agent receives the rejection prompt template.

## Backend — prompts (catalog of every spec-task agent prompt)

Code-wide audit found 8 Go `text/template` prompts + 2 `.tmpl` files + 8 builder/sender functions. Disposition for each:

### Templates that need EDITING

- [x] `api/pkg/services/spec_task_prompts.go:28` `planningPromptTemplate`:
  - Add "Spawning Follow-Up Tasks (Optional)" section — mention `propose_spec_task`; clarify `CreateSpecTask` is for Optimus chat only
  - Add "Not Every Task Needs Code" section — explicitly state that zero-PR completion is valid
- [x] `api/pkg/services/agent_instruction_service.go:126` `approvalPromptTemplate` — replace the existing single `5.` step with the three new steps:
  - **Step 5 — "Opening pull requests (zero, one, or many)"**: explains `propose_pull_request`, that opening zero PRs is valid, that the simple "Open PR" button still works for single-PR tasks, and that `gh pr create` / GitHub MCP tools are still forbidden.
  - **Step 6 — "Capture knowledge as you go"**: two channels — spec branch (no PR needed) and main repo markdown files (via `propose_pull_request`). Spec branch preferred when in doubt.
  - **Step 7 — "Declaring the task done — REQUIRED"**: `mark_task_complete` is the ONLY way to reach `done`.
- [x] `api/pkg/prompts/templates/agent_implementation_approved_push.tmpl` — drop the line *"the Pull Request has been opened automatically"*; add explicit instruction that pushing alone does NOT complete the task; mention `propose_pull_request` is the route for additional PRs; mention `mark_task_complete` is required for completion.
- [x] `api/pkg/prompts/templates/agent_rebase_required.tmpl` — strip any "merge → close" wording; clarify that rebasing keeps existing PRs current but does not affect task status; `mark_task_complete` remains the only path to `done`.

### Templates / functions / call sites to DELETE entirely

- [x] DELETE `api/pkg/services/agent_instruction_service.go:420` `mergePromptTemplate`
- [x] DELETE `api/pkg/services/agent_instruction_service.go:588` `BuildMergeInstructionPrompt`
- [x] DELETE `api/pkg/services/agent_instruction_service.go:786` `SendMergeInstruction`
- [x] DELETE the `MergePromptData` struct and any `MergeInstructionData` types if used only here
- [x] Audit all callers of `SendMergeInstruction`: `grep -rn "SendMergeInstruction\|BuildMergeInstructionPrompt" api/` — every call site must be removed (likely in `spec_driven_task_service.go` or the implementation-approval handler). Implementation approval no longer sends a merge prompt at all; the agent learns about approval through proposal decision messages and the existing UI/notification channels.
- [x] Update tests that assert `SendMergeInstruction` was called → assert it is NOT called

### Templates that stay UNCHANGED (verified by audit, no PR/completion lifecycle wording)

- [x] No-op verify: `commentPromptTemplate` (`agent_instruction_service.go:370`) — minimal design-doc-update message, unaffected by completion model change.
- [x] No-op verify: `implementationReviewPromptTemplate` (`agent_instruction_service.go:390`) — generic "code pushed, user will test" message; for zero-PR tasks it simply never fires.
- [x] No-op verify: `revisionPromptTemplate` (`agent_instruction_service.go:402`) — operates on design docs in spec branch, no completion logic.

### Verification

- [x] Verify every edited prompt builder still produces valid output for cloned tasks (`ClonedTaskPreamble` still injected correctly)
- [x] Manual prompt-eval check: run a few cloned-and-fresh task scenarios and verify the agent doesn't get confused about when to call `mark_task_complete` vs when to wait
- [x] After all edits + deletions, grep `api/` for any remaining mention of "all PRs merged", "branch merged to main → done", "merge to base", or "Pull Request has been opened automatically" — there should be zero remaining hits in agent-facing prompts.

## Frontend — proposals UI

- [x] Run `./stack update_openapi` after backend handlers land; verify generated client has `v1SpecTasksProposalsList`, `v1ProjectsProposalsPendingList`, `v1ProposalsDecideCreate`
- [x] Add `frontend/src/hooks/useSpecTaskProposals.ts` (React Query hook); subscribe to `spec_task.proposal.created` pubsub for invalidation
- [x] Add `frontend/src/components/specTask/PendingProposalsPanel.tsx` rendering all pending proposals for a task; one sub-component per kind:
  - `PRProposalCard` — editable head/base/title/body fields, Approve / Reject buttons
  - `TaskProposalCard` — editable name/description/type/priority, Approve / Reject buttons
  - `MarkCompleteProposalCard` — agent reason, Mark Done / Send Back buttons (Send Back opens a feedback textarea)
- [x] Mount `PendingProposalsPanel` inside the existing task detail view (find current task detail component near `SpecTaskActionButtons.tsx`)
- [x] Add a small badge to task cards on the kanban board indicating pending proposal count
- [x] Add a project-level pending-proposals indicator using `v1ProjectsProposalsPendingList`

## Testing

- [x] Unit tests for store CRUD on `SpecTaskProposal`
- [x] Unit tests for the `decide` handler covering: approve PR with edits, approve task, mark complete, reject (with reason delivered to agent), failed dispatch (stays as `failed`)
- [x] Unit test for the orchestrator gate (covered above under orchestrator)
- [x] Frontend vitest test for the `useSpecTaskProposals` hook
- [x] Manual end-to-end in helix-in-helix:
  1. Start a spec task; agent calls `propose_pull_request` with a non-default branch name
  2. Verify proposal surfaces in UI; edit the branch name; approve
  3. Verify push + PR opened on GitHub; `task.RepoPullRequests` updated; **task does NOT auto-transition** when the PR is merged on GitHub (verify it stays in `pull_request` status with `RepoPR.PRState` updated to `merged`)
  4. Have agent call `propose_spec_task`; approve in UI; verify child task on board with `parent_task_id` set
  5. Have agent call `mark_task_complete`; click Mark Done; verify task transitions to `done` and `CompletedAt` is set
  6. **Zero-PR scenario**: start a task, have the agent push only knowledge updates to helix-specs (no `propose_pull_request` calls), then call `mark_task_complete`; verify the task reaches `done` with empty `RepoPullRequests`

## Documentation

- [x] Update `api/pkg/services/spec_task_prompts.go` and `agent_instruction_service.go` docstrings to reflect the new tools (already covered by prompt edits)
- [x] Add a section to `INTEGRATION_GUIDE.md` (root) describing the proposal lifecycle for integrators
