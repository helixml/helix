# Implementation Tasks

- [x] Add `interrupt bool` parameter to `sendChatMessageToExternalAgent` in `api/pkg/server/websocket_external_agent_sync.go` and include `"interrupt": interrupt` in the `chat_message` command `Data`
- [x] Add `interrupt bool` parameter to `sendMessageToSpecTaskAgent` in `api/pkg/server/spec_task_design_review_handlers.go` and forward it to `sendChatMessageToExternalAgent`
- [x] Update `SpecTaskMessageSender` type in `api/pkg/services/git_http_server.go` to include `interrupt bool` (the binding at `api/pkg/server/server.go:489` works automatically since signatures match)
- [x] Update `sendCommentToAgentNow` to pass `interrupt=true`
- [x] Update the request-changes branch of `respondToDesignReview` to pass `interrupt=true`
- [x] Update `sendApprovalInstructionToAgent` to pass `interrupt=false`
- [x] Update workflow callers in `spec_task_workflow_handlers.go` (post-merge push, rebase) to pass `interrupt=false`
- [x] Update `agent_instruction_service.go` approval-flow caller to pass `interrupt=false`
- [x] Update `spec_driven_task_service.go` auto-revision caller to pass `interrupt=true` (revision instruction is reviewer-driven feedback, equivalent to a comment)
- [x] Update `api/pkg/server/test_helpers.go` `SendChatMessage` shim — defaults to `interrupt=false` (preserves cross-repo e2e test compatibility); add `SendChatMessageWithInterrupt` for tests that exercise the interrupt path
- [x] Update `spec_driven_task_service_test.go` mock sender signature for new bool param
- [x] No existing direct callers of `sendChatMessageToExternalAgent` in `websocket_external_agent_sync_test.go` (verified via grep — tests use `QueueCommand` / `handleMessageCompleted` paths)
- [x] Add unit tests `TestSendChatMessage_InterruptTrue` / `TestSendChatMessage_InterruptFalse` asserting `chat_message` outgoing `Data["interrupt"]` is correctly forwarded
- [x] `go build ./pkg/server/ ./pkg/services/ ./pkg/types/ ./pkg/store/` succeeds
- [x] `go test -run TestWebSocketSyncSuite ./pkg/server/` passes (full suite)
- [x] `go test ./pkg/services/...` passes (full suite, includes spec_driven_task_service_test)
- [ ] Manual end-to-end in inner Helix: start a spec task, let the agent begin a long response, drop a new design-review comment, confirm the prior turn cancels and the comment response starts within a few seconds (WARNING: NOT manually tested yet — unit tests cover the wire-format change; reviewer should validate end-to-end before merge)
- [x] Push branch (`feature/001962-spec-comments-should-be`); user opens PR via the Helix "Open PR" UI
