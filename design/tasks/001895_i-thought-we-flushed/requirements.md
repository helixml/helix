# Requirements: Timer-Based Flush for Streaming Throttles

## Problem

Partial text updates are visible in the Helix frontend until the next event arrives from Zed. The two throttle layers (Zed→Helix and Helix→frontend) buffer content but only flush it on the **next incoming event**. There is no independent timer to drain pending content when events pause.

## Root Cause

Both throttle layers are purely **event-driven**:

1. **Zed** (`thread_service.rs:241` `throttled_send_message_added`): Buffers tokens in `pending_content` with a 100ms throttle. Flushed only when the next `EntryUpdated` event arrives, a new entry starts, or `Stopped` fires.
2. **Helix** (`websocket_external_agent_sync.go:1229`): Buffers patches with a 50ms `publishInterval`. Published only when the next `message_added` WebSocket message arrives, a `tool_call` force-flush triggers, or `message_completed` fires.

If the LLM pauses between tokens (thinking, network jitter, end of a text block before a tool call), pending content sits in both buffers indefinitely until the next event arrives.

## User Stories

- As a user watching an AI response stream in Helix, I want to see text appear smoothly and completely, not freeze mid-word until the next update arrives.

## Acceptance Criteria

- [ ] When no new events arrive for >100ms after the last throttled Zed message, pending content is automatically flushed to the WebSocket
- [ ] When no new `message_added` events arrive for >50ms after the last Helix throttle, pending patches are automatically published to the frontend
- [ ] Partial text no longer appears "stuck" during LLM pauses — content resolves within ~100-150ms of being available
- [ ] No regression in throughput: during fast streaming, the timer should not add overhead (it resets on each event, only fires during gaps)
- [ ] Existing force-flush paths (tool_call boundaries, Stopped/message_completed) continue to work unchanged
