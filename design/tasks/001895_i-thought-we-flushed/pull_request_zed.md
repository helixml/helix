# Add trailing-edge flush timer for streaming throttle

## Summary
Fix partial text updates staying buffered until the next LLM token arrives. The 100ms streaming throttle in `throttled_send_message_added` was purely event-driven — if no new `EntryUpdated` event arrived after content was buffered, `pending_content` would sit indefinitely until the next event or `Stopped`.

## Changes
- Add `flush_scheduled: bool` field to `StreamingThrottleState` to prevent duplicate timer spawns
- When content is buffered (throttle window not yet elapsed), spawn a `smol::spawn` async task that sleeps for `STREAMING_THROTTLE_INTERVAL` (100ms) then drains `pending_content` if still present
- Reset `flush_scheduled` in all existing flush paths (`flush_stale_pending_for_thread`, `flush_streaming_throttle`, and the send path in `throttled_send_message_added`)
- Timer is a no-op if content was already sent or the entry was removed from the throttle map

## Test plan
- [ ] E2E: verify text updates are sent within ~100ms even when no subsequent tokens arrive
- [ ] Verify no regression: fast streaming still coalesces correctly
