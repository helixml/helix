# Implementation Tasks

## Primary Fix: Helix API Force-Flush on Tool Call Entry

- [ ] In `handleMessageAdded` (`websocket_external_agent_sync.go`), track the current max entry index per interaction in `streamingContext`
- [ ] When receiving a `message_added` event with `entry_type=tool_call` and a **new** entry index (> current max), force-flush by calling `publishEntryPatchesToFrontend` immediately (bypassing the 50ms `publishInterval` throttle)
- [ ] After the force-flush, reset `sctx.lastPublish` so the throttle window restarts cleanly
- [ ] Add a debug log: `"📤 [FLUSH] Force-published patches before tool_call entry N"`

## Secondary Fix (Optional): Zed Throttle Bypass on Entry Type Change

- [ ] In `throttled_send_message_added` (`thread_service.rs`), after flushing stale-pending entries, check if the current entry is `tool_call` and the previous entry was `text`
- [ ] If so, bypass the throttle for the current entry (send immediately, update `last_sent`)
- [ ] Alternatively, always send immediately when `entry_type=tool_call` (tool calls are infrequent, no need to throttle them)

## Verification

- [ ] Build Helix API, start a session, send a prompt that triggers a tool call mid-sentence (e.g., "List the files in the current directory and tell me what you see")
- [ ] Watch the frontend — confirm the text before the tool call is fully visible in real-time, not truncated until completion
- [ ] Confirm no regression: streaming still looks smooth, no duplicate content, sequential tool calls work correctly
