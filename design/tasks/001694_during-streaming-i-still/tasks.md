# Implementation Tasks

## Primary Fix: Helix API Force-Flush on Tool Call Entry

- [x] In `handleMessageAdded` (`websocket_external_agent_sync.go`), detect when a tool_call entry arrives (new message_id + entry_type=tool_call)
- [x] Force-flush by calling `publishEntryPatchesToFrontend` immediately before adding the tool_call to the accumulator
- [x] After the force-flush, reset `sctx.lastPublish` so the throttle window restarts cleanly
- [x] Add a debug log: `"📤 [FLUSH] Force-published patches before tool_call entry"`

## Secondary Fix (Optional): Zed Throttle Bypass on Entry Type Change

- [ ] In `throttled_send_message_added` (`thread_service.rs`), after flushing stale-pending entries, check if the current entry is `tool_call` and the previous entry was `text`
- [ ] If so, bypass the throttle for the current entry (send immediately, update `last_sent`)
- [ ] Alternatively, always send immediately when `entry_type=tool_call` (tool calls are infrequent, no need to throttle them)

## Verification

- [x] Build Helix API, start a session, send a prompt that triggers a tool call mid-sentence
- [x] Watch the frontend — confirm the text before the tool call is fully visible in real-time, not truncated until completion
- [x] Confirm no regression: streaming still looks smooth, no duplicate content, sequential tool calls work correctly
