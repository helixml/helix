# Implementation Tasks

- [x] Add `interrupt bool` parameter to `sendChatMessageToExternalAgent` in `api/pkg/server/websocket_external_agent_sync.go` and include `"interrupt": interrupt` in the `chat_message` command `Data`
- [~] Add `interrupt bool` parameter to `sendMessageToSpecTaskAgent` in `api/pkg/server/spec_task_design_review_handlers.go` and forward it to `sendChatMessageToExternalAgent`
- [ ] Update `SendMessageToAgent` field type on `specDrivenTaskService` (and any interface it satisfies) to match the new signature; fix the binding at `api/pkg/server/server.go:489`
- [ ] Update `sendCommentToAgentNow` (around `spec_task_design_review_handlers.go:1021`) to pass `interrupt=true`
- [ ] Update the request-changes branch of `respondToDesignReview` (around `spec_task_design_review_handlers.go:378`) to pass `interrupt=true`
- [ ] Update `sendApprovalInstructionToAgent` (around `spec_task_design_review_handlers.go:1572`) to pass `interrupt=false`
- [ ] Update workflow callers in `spec_task_workflow_handlers.go:211` and `:294` to pass `interrupt=false`
- [ ] Update `api/pkg/server/test_helpers.go:74` and any other test/internal callers of `sendChatMessageToExternalAgent` for the new signature
- [ ] Update existing tests in `api/pkg/server/websocket_external_agent_sync_test.go` that call `sendChatMessageToExternalAgent` directly
- [ ] Add a unit test asserting `chat_message` outgoing `Data["interrupt"] == true` when the spec-comment path is used, and `false` for approval/workflow paths
- [ ] `go build ./api/pkg/server/ ./api/pkg/services/` succeeds
- [ ] Manual end-to-end in inner Helix: start a spec task, let the agent begin a long response, drop a new design-review comment, confirm the prior turn cancels and the comment response starts within a few seconds
- [ ] Push branch, open PR with link to this design doc, monitor Drone CI, fix any failures
