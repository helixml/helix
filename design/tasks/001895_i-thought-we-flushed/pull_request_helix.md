# Add trailing-edge flush timer for frontend publish throttle

## Summary
Fix partial text updates staying visible until the next Zed event arrives. The 50ms frontend publish throttle in `handleMessageAdded` was purely event-driven — if no new `message_added` event arrived after a throttled skip, pending patches would sit in the buffer indefinitely.

## Changes
- Add `flushTimer *time.Timer` field to `streamingContext`
- When `handleMessageAdded` skips a publish due to the 50ms throttle, start a `time.AfterFunc` that fires after `publishInterval` to drain pending patches
- Stop the timer on successful publishes and in `flushAndClearStreamingContext` / streaming context resets
- Timer callback acquires `sctx.mu` and checks state before publishing, making it safe against races

## Test plan
- [x] All 46 existing Go unit tests pass (`TestWebSocketSyncSuite`)
- [ ] E2E: verify text updates resolve within ~150ms during LLM pauses (no more "stuck" partial text)
