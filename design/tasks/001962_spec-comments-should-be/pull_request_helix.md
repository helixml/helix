# Spec design-review comments interrupt the agent

## Summary

When a reviewer leaves a comment on a spec-task design document (or submits "request changes" overall feedback), the comment is now sent to the agent with `interrupt=true`. The agent cancels its current turn and processes the comment immediately, instead of letting the comment wait behind in-flight work that may already be stale.

System-driven messages (approval-phase kickoff, post-merge push/rebase instructions) keep `interrupt=false` — they should respect the agent's queue rather than preempt it.

This piggybacks on the existing `interrupt` field that the prompt-history queue already passes in the same `chat_message` WebSocket command (`websocket_external_agent_sync.go:2845`), so no agent-side (Zed/Claude/Qwen) change is needed.

## Changes

- `sendChatMessageToExternalAgent` gains an `interrupt bool` parameter; the value is forwarded into the `chat_message` `Data["interrupt"]` field.
- `sendMessageToSpecTaskAgent` (the unified spec-task message helper) gains the same parameter and passes it through.
- `SpecTaskMessageSender` function type updated to match.
- Comment / request-changes / auto-revision callers pass `interrupt=true`. Approval kickoff and post-merge workflow callers pass `interrupt=false`. Each call site has a one-line comment explaining the choice.
- New unit tests `TestSendChatMessage_InterruptTrue` / `TestSendChatMessage_InterruptFalse` capture the outgoing command and assert `Data["interrupt"]` is correct.
- Test helper `SendChatMessage` keeps its old signature (defaults `interrupt=false`) for the Zed e2e test server's compatibility; added `SendChatMessageWithInterrupt` for tests that need to exercise the flag.

## Test plan

- [x] `go build ./pkg/server/ ./pkg/services/ ./pkg/types/ ./pkg/store/`
- [x] `CGO_ENABLED=1 go test -run TestWebSocketSyncSuite ./pkg/server/ -count=1`
- [x] `CGO_ENABLED=1 go test ./pkg/services/... -count=1`
- [ ] Manual: start a spec task in inner Helix, wait for the agent to begin a long response, drop a new design-review comment, observe the prior turn cancel and the comment response start within a few seconds.
