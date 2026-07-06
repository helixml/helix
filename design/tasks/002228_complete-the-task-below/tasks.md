# Implementation Tasks: Unify All Agent Message Sending on the Session-Scoped Prompt Queue

## Make the queue session-scoped
- [~] Make `prompt_history_entries.SpecTaskID` nullable; add `NotifyUserID` column; add `PromptID` to `SpecTaskDesignReviewComment`
- [ ] Add `CreatePromptHistoryEntry` + `GetCommentByPromptID` to `store.Store` + `PostgresStore`; regenerate `store_mocks.go`
- [ ] Extract `processPendingPromptsForSession(ctx, sessionID)` from `processPendingPromptsForIdleSessions`; keep the spec-task entry (listing + canonical filter) calling it
- [ ] Implement `enqueueAgentMessage(ctx, sessionID, message, interrupt, notifyUserID, specTaskID)` (resolve owner/project, insert pending row, nudge `processPendingPromptsForSession`; error on empty session)
- [ ] Add `SpecTaskMessageEnqueuer` callback (`func(ctx, task, message, interrupt bool) error`) in `services/git_http_server.go`; wire `EnqueueMessageToAgent` on `SpecDrivenTaskService` in `server.go`
- [ ] At dispatch (`sendQueuedPromptToSession`) set `requestToCommenterMapping`/`sessionToCommenterMapping` from the row's `NotifyUserID`

## Comment-reply: backfill linkage at dispatch (lower-risk, keeps machinery intact)
- [ ] `sendCommentToAgentNow` enqueues interrupt=true and stores `comment.PromptID`
- [ ] `sendQueuedPromptToSession` backfills `comment.RequestID`/`InteractionID` via `GetCommentByPromptID(prompt.ID)` after creating the interaction
- [ ] No change needed to finalize/streaming/timeout/reconcile (they still use RequestID/InteractionID)

## Migrate every sender onto enqueue
- [ ] CI notifier → enqueue interrupt=true (replace `MessageSenderCINotifier`); no coalescing
- [ ] Push (`spec_task_workflow_handlers.go:213`) → enqueue interrupt=false
- [ ] Rebase (`:314`) → enqueue interrupt=false
- [ ] Approval (`agent_instruction_service.go:673`) → enqueue interrupt=false
- [ ] Comment reply (`spec_task_design_review_handlers.go:1251`) → enqueue interrupt=true
- [ ] Design-review submit (`:403`) → enqueue interrupt=true
- [ ] Org transition (`spec_tasks_org_wiring.go:34`) → enqueue interrupt=true
- [ ] Reviewer revision (`spec_driven_task_service.go:1457`) → enqueue interrupt=true
- [ ] `sendSessionMessage` (`session_handlers.go:2324`) → enqueue (interrupt from body); org bots inherit the fix, no org-runtime change

## Public API contract
- [ ] Change `sendSessionMessage` response to return the queue-entry id (async handle); update swagger, run `./stack update_openapi`, update generated client + CLI

## Delete dead / duplicate code
- [ ] Delete `sendChatMessageToExternalAgent`, `sendMessageToSession`, `sendMessageToSpecTaskAgent`
- [ ] Delete `MessageSenderCINotifier` / `NewMessageSenderCINotifier`
- [ ] Delete `SendImplementationReviewRequest`, `SendRevisionInstruction`, `SendMergeInstruction`, `AgentInstructionService.sendMessage`, `BuildImplementationReviewPrompt`, `BuildMergeInstructionPrompt` (keep `BuildRevisionInstructionPrompt`)
- [ ] Delete the `messageSender` field from `AgentInstructionService`
- [ ] Grep each deleted symbol to prove zero references; `CGO_ENABLED=0 go build ./...` clean

## Testing
- [ ] Unit (gomock): `processPendingPromptsForSession` idle/busy/interrupt/boot-barrier; enqueue row correctness
- [ ] Live E2E (bot seam): long turn + `POST /sessions/{id}/messages` interrupt=false is HELD, delivered after completion, no concurrent empty interaction
- [ ] Live E2E (spec task): CI interrupt cancels+delivers as one turn; push/rebase/approval deferred; comment reply interrupts AND finalizes onto the comment
- [ ] Confirm bot causation (Open Q4): reproduce mid-turn overlap before, gone after
- [ ] Verify boot barrier + PR #2808 desktop-resume reap still hold for general sessions

## Ship
- [ ] Branch off `main`, conventional commits, regular merge (never squash)
- [ ] Push, check Drone CI yourself, fix forward until green
- [ ] PR references https://github.com/helixml/helix/pull/2808 and calls out: session-scoped queue, bot fix, deleted direct path, API contract change
