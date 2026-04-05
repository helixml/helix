# Flush pending text content before sending new entries to prevent truncated streaming

## Summary
When tool calls arrived during streaming, the preceding text appeared truncated because the `NewEntry` handler sent the tool_call entry without flushing the text entry's pending throttled content first. The stale-pending flush only existed in `throttled_send_message_added`, which `NewEntry` bypasses.

## Changes
- Extract `flush_stale_pending_for_thread()` helper from the inline stale-pending flush in `throttled_send_message_added`
- Call `flush_stale_pending_for_thread()` in the `NewEntry` handler before sending, ensuring preceding text entry content is fully sent before a tool_call entry
- Bypass the 100ms throttle for `entry_type=tool_call` in `throttled_send_message_added` — tool calls are infrequent, and their status updates ("In Progress" → "Completed") should arrive promptly

## Root Cause
The `NewEntry` handler sends new entries directly via `send_websocket_event` (no throttle). When a tool_call entry is created mid-streaming, the preceding text entry may still have unsent content sitting in the 100ms throttle buffer. The tool_call arrives at the API before the text entry's final content, causing truncation visible to the user.

## Testing
- Built Zed with fix, deployed to inner Helix
- Triggered prompts that cause multiple tool calls mid-response
- Verified text before tool calls is fully visible during streaming, not truncated

Release Notes:

- N/A
