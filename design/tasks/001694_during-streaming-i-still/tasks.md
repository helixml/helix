# Implementation Tasks

## Helix API (Primary Fix)

- [ ] In `handleMessageAdded` (`websocket_external_agent_sync.go`), detect when the incoming `message_added` event starts a new tool call entry (entry type changes from "text" to "tool_call")
- [ ] Before processing that tool call event, call `publishEntryPatchesToFrontend` immediately (bypassing the 50ms `publishInterval` throttle) to flush any pending text patches
- [ ] Update `sctx.lastPublish` after this forced flush so the throttle window resets correctly
- [ ] Add a test or log to verify the forced flush fires at tool call boundaries

## Zed Extension (Secondary Fix)

- [ ] In `acp_thread.rs`, find where a new tool call entry is added after `flush_streaming_text()`
- [ ] Ensure the final `message_added` WebSocket send for the text entry is completed (awaited/synchronous) before sending the `message_added` for the new tool call entry
- [ ] Verify ordering in integration test or by inspecting WebSocket frame order in logs

## Verification

- [ ] Manually test a prompt that triggers a tool call mid-sentence and confirm the text before the tool call appears complete in real time
- [ ] Confirm no duplicate content or regression in streaming smoothness
- [ ] Check that sequential tool calls (tool → text → tool) also work correctly
