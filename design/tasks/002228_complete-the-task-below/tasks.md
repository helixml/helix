# Implementation Tasks: Route Automated interrupt=false Agent Messages Through the Prompt Queue

## Enqueue infrastructure
- [ ] Add `CreatePromptHistoryEntry(ctx, *types.PromptHistoryEntry) error` to the `store.Store` interface and implement it in `store_prompt_history.go`
- [ ] Regenerate `store_mocks.go` (mockgen) for the new store method(s)
- [ ] Add `SpecTaskMessageEnqueuer` callback type in `api/pkg/services/git_http_server.go`
- [ ] Implement `enqueueSpecTaskAgentMessage` on `HelixAPIServer` (target canonical `PlanningSessionID`, user = `CreatedBy`/`Owner`, insert pending interrupt=false row, nudge `processPendingPromptsForIdleSessions`; error on empty session)
- [ ] Add `EnqueueMessageToAgent` field to `SpecDrivenTaskService` and wire it in `server.go`

## Migrate the four automated senders
- [ ] CI notifier: replace `MessageSenderCINotifier` with an enqueue-based notifier; rewire in `server.go:631`
- [ ] Post-merge push instruction (`spec_task_workflow_handlers.go:213`) → enqueue
- [ ] Post-merge-failure rebase instruction (`spec_task_workflow_handlers.go:314`) → enqueue
- [ ] `SendApprovalInstruction` (`agent_instruction_service.go:673`) → enqueue; pass enqueuer into `NewAgentInstructionService`

## CI coalescing (pending Open Q1/Q2 answer — recommended)
- [ ] Add `CoalescePendingCINotification` store method + mark CI entries via `Tags` sentinel
- [ ] CI enqueue path coalesces consecutive pending CI entries for a session; push/rebase/approval stay distinct

## Delete dead / duplicate code
- [ ] Delete `SendImplementationReviewRequest`, `SendRevisionInstruction`, `SendMergeInstruction`, and `AgentInstructionService.sendMessage`
- [ ] Delete now-unused `BuildImplementationReviewPrompt` / `BuildMergeInstructionPrompt` (+ templates); keep `BuildRevisionInstructionPrompt`
- [ ] Delete `MessageSenderCINotifier` / `NewMessageSenderCINotifier`
- [ ] Delete the `messageSender` field from `AgentInstructionService` once unused
- [ ] (Open Q5) Optionally drop the `interrupt` param from `sendMessageToSpecTaskAgent` (hardcode true); keep it on `sendMessageToSession` / `sendChatMessageToExternalAgent` for the user-send endpoint
- [ ] Confirm the do-not-touch interrupt=true paths (`spec_driven_task_service.go:1457`, `spec_tasks_org_wiring.go:34`, `spec_task_design_review_handlers.go:403,1251`, `session_handlers.go:2324`) are unchanged

## Testing
- [ ] `CGO_ENABLED=0 go build ./...` passes
- [ ] Unit tests (gomock, suite pattern): enqueue creates pending row + nudges poller; busy → deferred (no interaction); idle → dispatched via `processPromptQueue`
- [ ] Unit tests for CI coalescing (if built): second CI enqueue coalesces; push/rebase do not
- [ ] Live E2E in inner Helix: long turn + simulated CI transition is HELD and delivered only after completion (reproduce incident shape; show it's gone)
- [ ] Live E2E: verify the four interrupt=true paths still interrupt correctly
- [ ] Update/remove tests referencing deleted symbols

## Ship
- [ ] Branch off `main`, conventional commits, regular merge (never squash)
- [ ] Regenerate API client if any endpoint changed (`./stack update_openapi`) — none expected
- [ ] Push, check Drone CI yourself, fix forward until green
- [ ] PR references https://github.com/helixml/helix/pull/2808 and calls out the coalescing decision
