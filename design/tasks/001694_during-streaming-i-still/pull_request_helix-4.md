# Fix truncated sentences before tool calls during streaming

## Summary
When tool calls arrived during streaming, the text preceding them appeared truncated until the interaction completed. This happened because the tool call entry was published to the frontend while the preceding text entry's final content was still waiting in the 50ms publish throttle buffer.

## Changes
- Add force-flush logic in `handleMessageAdded` that publishes pending patches immediately before processing a tool_call entry
- When a new message_id arrives with `entry_type=tool_call`, force-publish all pending entry patches before adding the tool call to the accumulator
- Add debug log `"📤 [FLUSH] Force-published patches before tool_call entry"` for observability

## Root Cause
Zed sends entries with a unique message_id per logical entry. When a tool call begins:
1. Zed creates a new entry (message_id=2), sends it immediately
2. The preceding text entry (message_id=1) may still have pending content in the 100ms throttle buffer
3. Zed's stale-pending flush sends the text content, but Helix's 50ms publish throttle may hold it
4. Frontend sees entry_count=2 before entry 1's content is complete

The fix force-flushes pending patches when a tool_call entry arrives, ensuring the frontend sees complete text before entry_count increases.

## Testing
- Triggered multiple tool calls during streaming
- Verified force-flush logs appear: `📤 [FLUSH] Force-published patches before tool_call entry`
- Confirmed text before tool calls is fully visible in real-time
- No regression: streaming still smooth, no duplicate content
