# Implementation Tasks

## Zed: Trailing-edge flush timer (`zed/crates/external_websocket_sync/src/thread_service.rs`)

- [ ] In `throttled_send_message_added()`, when content is buffered (`sent = false`), spawn a background task that sleeps for `STREAMING_THROTTLE_INTERVAL` then checks if `pending_content` still exists for that key — if so, send it via `send_websocket_event` and clear the buffer
- [ ] Ensure the timer is a no-op if the pending content was already sent or replaced by a subsequent event (check `pending_content` under lock before sending)
- [ ] Verify existing flush paths (`flush_stale_pending_for_thread`, `flush_streaming_throttle`) still work — they clear `pending_content`, so the timer harmlessly finds nothing

## Helix: Trailing-edge publish timer (`helix/api/pkg/server/websocket_external_agent_sync.go`)

- [ ] Add a `flushTimer *time.Timer` field to `streamingContext`
- [ ] In `handleMessageAdded()`, when publish is skipped due to `publishInterval` throttle, start/reset a `time.AfterFunc(publishInterval, ...)` that publishes pending patches
- [ ] In the timer callback: acquire the session lock, check if patches are still pending (compare `lastPublish`), compute and publish entry patches, update `lastPublish`
- [ ] Stop the flush timer on successful publish (in the normal event-driven path) and in `flushAndClearStreamingContext()`

## Testing

- [ ] E2E test: verify that a single text entry with no subsequent events (no tool_call, no new entry) has its final content visible in the frontend within ~150ms
- [ ] Verify no regression: fast streaming (many tokens/sec) still coalesces correctly and doesn't produce duplicate or reordered patches
