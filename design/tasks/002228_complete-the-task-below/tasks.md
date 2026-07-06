# Implementation Tasks: Route Automated Agent Message Senders Through the Prompt Queue

## Enqueue infrastructure
- [ ] Add `CreatePromptHistoryEntry(ctx, *types.PromptHistoryEntry) error` to the `store.Store` interface and implement it in `store_prompt_history.go`
- [ ] Regenerate `store_mocks.go` (mockgen) for the new store method
- [ ] Add `SpecTaskMessageEnqueuer` callback type (`func(ctx, task, message, interrupt bool) error`) in `api/pkg/services/git_http_server.go`
- [ ] Implement `enqueueSpecTaskAgentMessage` on `HelixAPIServer` (target canonical `PlanningSessionID`, user = `CreatedBy`/`Owner`, insert pending row with the given `Interrupt`, nudge `processPendingPromptsForIdleSessions`; error on empty session)
- [ ] Add `EnqueueMessageToAgent` field to `SpecDrivenTaskService` and wire it in `server.go`

## Migrate the four automated senders
- [ ] CI notifier (**interrupt=true**, no coalescing): replace `MessageSenderCINotifier` with an enqueue-based notifier calling the enqueuer with `interrupt=true`; rewire in `server.go:631`
- [ ] Post-merge push instruction (`spec_task_workflow_handlers.go:213`) → enqueue (interrupt=false)
- [ ] Post-merge-failure rebase instruction (`spec_task_workflow_handlers.go:314`) → enqueue (interrupt=false)
- [ ] `SendApprovalInstruction` (`agent_instruction_service.go:673`) → enqueue (interrupt=false); pass enqueuer into `NewAgentInstructionService`

## Delete dead / duplicate code
- [ ] Delete `SendImplementationReviewRequest`, `SendRevisionInstruction`, `SendMergeInstruction`, and `AgentInstructionService.sendMessage`
- [ ] Delete now-unused `BuildImplementationReviewPrompt` / `BuildMergeInstructionPrompt` (+ templates); keep `BuildRevisionInstructionPrompt`
- [ ] Delete `MessageSenderCINotifier` / `NewMessageSenderCINotifier`
- [ ] Delete the `messageSender` field from `AgentInstructionService` once unused
- [ ] (Open Q3) Optionally drop the `interrupt` param from `sendMessageToSpecTaskAgent` (hardcode true); keep it on `sendMessageToSession` / `sendChatMessageToExternalAgent` for the user-send endpoint
- [ ] Confirm the do-not-touch interrupt=true paths (`spec_driven_task_service.go:1457`, `spec_tasks_org_wiring.go:34`, `spec_task_design_review_handlers.go:403,1251`, `session_handlers.go:2324`) are unchanged

## Testing
- [ ] `CGO_ENABLED=0 go build ./...` passes
- [ ] Unit tests (gomock, suite pattern): enqueue creates pending row with correct `Interrupt` + nudges poller; interrupt=false busy → deferred, idle → `processPromptQueue`; interrupt=true busy+thread-established → `processInterruptPrompt`, thread-not-established → deferred (boot barrier)
- [ ] Live E2E in inner Helix: long turn + simulated CI transition cancels the turn and delivers as a single new turn — NOT a concurrent empty interaction (reproduce incident shape; show it's gone)
- [ ] Live E2E: long turn + simulated push/rebase/approval (interrupt=false) is HELD and delivered only after completion
- [ ] Live E2E: verify pre-existing interrupt=true paths (comment reply, org transition) still interrupt correctly
- [ ] Update/remove tests referencing deleted symbols

## Ship
- [ ] Branch off `main`, conventional commits, regular merge (never squash)
- [ ] Regenerate API client if any endpoint changed (`./stack update_openapi`) — none expected
- [ ] Push, check Drone CI yourself, fix forward until green
- [ ] PR references https://github.com/helixml/helix/pull/2808 and calls out the CI-interrupts-not-coalesce decision
