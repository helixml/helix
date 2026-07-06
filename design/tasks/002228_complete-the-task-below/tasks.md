# Implementation Tasks: Unify All Agent Message Sending on the Session-Scoped Prompt Queue

## Make the queue session-scoped
- [x] Make `prompt_history_entries.SpecTaskID` nullable; add `NotifyUserID` column; add `PromptID` to `SpecTaskDesignReviewComment`
- [x] Add `CreatePromptHistoryEntry` + `GetCommentByPromptID` to `store.Store` + `PostgresStore`; regenerate `store_mocks.go`
- [x] Extract `processPendingPromptsForSession(ctx, sessionID)` from `processPendingPromptsForIdleSessions`; keep the spec-task entry (listing + canonical filter) calling it
- [x] Implement `enqueueAgentMessage(ctx, sessionID, message, interrupt, notifyUserID, specTaskID)` (resolve owner/project, insert pending row, nudge `processPendingPromptsForSession`; error on empty session)
- [x] Add `SpecTaskMessageEnqueuer` callback (`func(ctx, task, message, interrupt bool) error`) in `services/git_http_server.go`; wire `EnqueueMessageToAgent` on `SpecDrivenTaskService` in `server.go`
- [x] At dispatch (`sendQueuedPromptToSession`) set `requestToCommenterMapping`/`sessionToCommenterMapping` from the row's `NotifyUserID`

## Comment-reply: backfill linkage at dispatch (lower-risk, keeps machinery intact)
- [x] `sendCommentToAgentNow` enqueues interrupt=true and stores `comment.PromptID`
- [x] `sendQueuedPromptToSession` backfills `comment.RequestID`/`InteractionID` via `GetCommentByPromptID(prompt.ID)` after creating the interaction
- [x] No change needed to finalize/streaming/timeout/reconcile (they still use RequestID/InteractionID)

## Migrate every sender onto enqueue
- [x] CI notifier → enqueue interrupt=true (replace `MessageSenderCINotifier`); no coalescing
- [x] Push (`spec_task_workflow_handlers.go:213`) → enqueue interrupt=false
- [x] Rebase (`:314`) → enqueue interrupt=false
- [x] Approval (`agent_instruction_service.go:673`) → enqueue interrupt=false
- [x] Comment reply (`spec_task_design_review_handlers.go:1251`) → enqueue interrupt=true
- [x] Design-review submit (`:403`) → enqueue interrupt=true
- [x] Org transition (`spec_tasks_org_wiring.go:34`) → enqueue interrupt=true
- [x] Reviewer revision (`spec_driven_task_service.go:1457`) → enqueue interrupt=true
- [x] `sendSessionMessage` (`session_handlers.go:2324`) → enqueue (interrupt from body); org bots inherit the fix, no org-runtime change

## Public API contract
- [x] Change `sendSessionMessage` response to return the queue-entry id (async handle); update swagger, run `./stack update_openapi`, update generated client + CLI

## Delete dead / duplicate code
- [x] Delete production direct wrappers `sendMessageToSession`, `sendMessageToSpecTaskAgent` (no prod callers). KEPT `sendChatMessageToExternalAgent` — it is production-unreachable now but is the low-level primitive the cross-repo Zed WS-sync e2e harness drives via `test_helpers.SendChatMessage` (passes its own request_id and asserts routing); deleting it would require rewriting the pinned zed e2e server. Documented as test-harness-only.
- [x] Delete `MessageSenderCINotifier` / `NewMessageSenderCINotifier`
- [x] Delete `SendImplementationReviewRequest`, `SendRevisionInstruction`, `SendMergeInstruction`, `AgentInstructionService.sendMessage`, `BuildImplementationReviewPrompt`, `BuildMergeInstructionPrompt` (keep `BuildRevisionInstructionPrompt`)
- [x] Delete the `messageSender` field from `AgentInstructionService`
- [x] Grep each deleted symbol to prove zero references; `CGO_ENABLED=0 go build ./...` clean

## Testing
- [x] Unit (gomock): `processPendingPromptsForSession` idle/busy/interrupt/boot-barrier; enqueue row correctness
- [x] Live E2E (bot seam): long turn + `POST /sessions/{id}/messages` interrupt=false is HELD, delivered after completion, no concurrent empty interaction
- [~] CI interrupt / comment-reply: covered by unit tests + shared queue path proven live via bot seam; not independently live-driven (CI poll loop / full review flow needed)
- [x] Bot causation: proved at the exact bot seam — mid-turn interrupt=false is now HELD (no concurrent empty interaction), delivered when idle
- [x] Verify boot barrier + PR #2808 reap preserved (unit-tested; shared code path proven live)

## Ship
- [x] Branch off `main`, conventional commits (regular merge at PR time — never squash)
- [~] Pushed to feature branch. Drone CI runs when the PR is opened by the platform (branch not on GitHub until then); local build + unit tests + live E2E all green
- [x] PR description (pull_request_helix.md) references https://github.com/helixml/helix/pull/2808 and calls out session-scoped queue, bot fix, deleted direct path, API contract change
