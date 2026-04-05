# Re-read current entry content in NewEntry handler to prevent truncated streaming

## Summary
When tool calls arrived during streaming, the preceding text appeared truncated because the throttle buffer's `pending_content` was stale — captured before `flush_streaming_text()` ran but never updated (no `EntryUpdated` is emitted after the flush).

## Changes
- **NewEntry handler**: Instead of flushing stale `pending_content` from the throttle buffer, re-read ALL preceding entries' current content directly from the thread model. Since `flush_streaming_text()` has already run by the time `NewEntry` fires, `content_only(cx)` returns complete text.
- **Throttle bypass**: Tool call entries (`entry_type == "tool_call"`) bypass the 100ms throttle in `throttled_send_message_added` — tool calls are infrequent and their status updates should arrive promptly.

## Root Cause (Iteration 2 discovery)
`AcpThread::push_entry()` calls `flush_streaming_text()` before emitting `NewEntry`. This flushes all pending text into the Markdown entity, BUT it does NOT emit `EntryUpdated`. So the throttle's `pending_content` snapshot — captured at the last `EntryUpdated` before `flush_streaming_text` — is missing the final tokens that were in the `StreamingTextBuffer`.

The first fix (`flush_stale_pending_for_thread`) only sent this stale snapshot, which was incomplete. The real fix reads directly from the thread model which always has the current content.

## Testing
- Built Zed with fix, deployed to inner Helix
- Created task that triggers multiple tool calls (Find, ls, Write, git commands)
- API logs confirm force-flush triggered: `📤 [FLUSH] Force-published patches before tool_call entry`
- Chat panel shows complete text before every tool call — no truncation

Release Notes:

- N/A
