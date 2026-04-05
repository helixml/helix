# Implementation Tasks

## Primary Fix: Helix API Force-Flush on Tool Call Entry

- [x] In `handleMessageAdded` (`websocket_external_agent_sync.go`), detect when a tool_call entry arrives (new message_id + entry_type=tool_call)
- [x] Force-flush by calling `publishEntryPatchesToFrontend` immediately before adding the tool_call to the accumulator
- [x] After the force-flush, reset `sctx.lastPublish` so the throttle window restarts cleanly
- [x] Add a debug log: `"📤 [FLUSH] Force-published patches before tool_call entry"`

## Secondary Fix: Zed Throttle Bypass + NewEntry Flush

- [x] In `throttled_send_message_added` (`thread_service.rs`), bypass the 100ms throttle for `entry_type=tool_call` entries (tool calls are infrequent, no need to throttle them)
- [x] Extract `flush_stale_pending_for_thread()` helper that flushes pending throttled content for other entries in a thread
- [x] In the `NewEntry` handler, call `flush_stale_pending_for_thread()` before sending — ensures preceding text entry's final content is sent before tool_call entry

## Verification

- [x] Build Helix API, start a session, send a prompt that triggers a tool call mid-sentence
- [x] Watch the frontend — confirm the text before the tool call is fully visible in real-time, not truncated until completion
- [x] Confirm no regression: streaming still looks smooth, no duplicate content, sequential tool calls work correctly
