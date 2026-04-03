# Design: Fix Truncated Sentences Before Tool Calls During Streaming

## Architecture Overview

The streaming pipeline has three stages:

```
Zed (16ms reveal buffer) → Helix API (50ms publish throttle) → Frontend (patch apply)
```

### Stage 1 — Zed (`zed-4/crates/acp_thread/src/acp_thread.rs`)

Zed maintains a `StreamingTextBuffer` that reveals LLM tokens to the UI on a 16ms tick (`TASK_UPDATE_MS = 16`). On each tick it:
1. Drains pending tokens into the Markdown entity (local UI update)
2. Sends a `message_added` WebSocket event to the Helix API with the current content

When a tool call entry is created, `flush_streaming_text()` is called to immediately reveal any buffered text before the tool call entry is added. However, the issue is ordering: the flush sends the final text to Zed's UI, but the corresponding `message_added` WebSocket event to Helix may be sent *after* the `message_added` for the new tool_call entry. The tool call entry creation triggers its own `message_added` immediately (not on the 16ms tick), so it races with the trailing text flush.

### Stage 2 — Helix API (`helix-4/api/pkg/server/websocket_external_agent_sync.go`)

The API receives `message_added` events and applies two throttles:
- `publishInterval = 50ms` — minimum time between frontend WebSocket publishes
- `dbWriteInterval = 200ms` — minimum time between database writes

When a tool call entry arrives, if the 50ms window hasn't elapsed since the last publish, the patch for the final text entry is **not sent immediately**. The tool call patch may be batched into the *same* publish as the (now-complete) text entry, or into the *next* publish — but if the frontend is tracking entry count, the tool call's arrival can cause the frontend to display the new entry before the text entry's final patch arrives.

At interaction completion, `flushAndClearStreamingContext` fires an unthrottled publish — this is why content is always correct at the end.

### Stage 3 — Frontend (`helix-4/frontend/src/contexts/streaming.tsx`)

The frontend applies patches as they arrive. Patches include `entry_count` (the total number of entries). When `entry_count` grows (a new tool call entry appears), the frontend starts rendering the new entry. If the patch for the *previous* text entry hasn't arrived yet, that entry displays truncated content momentarily.

## Root Cause

Two layered issues compound each other:

**Issue A (Helix API):** When a tool call entry is received, the 50ms publish throttle may delay the final patch for the preceding text entry. The tool call entry can arrive in the same or next publish window, causing the frontend to render a new entry before the prior one is complete.

**Fix A:** When the Helix API receives a `message_added` event whose `entry_type` is `tool_call` (i.e., a new entry type that differs from the current entry), force an immediate unthrottled publish of all pending patches before processing the tool call entry.

**Issue B (Zed):** The `flush_streaming_text()` call on tool call creation sends the final text to Zed's local UI but sends its `message_added` asynchronously — potentially after the tool call's `message_added`. If Helix receives the tool call event first, the final text update is processed after and the publish order may be wrong.

**Fix B:** In Zed, after `flush_streaming_text()`, explicitly await or synchronously send the final `message_added` for the text entry *before* sending the `message_added` for the new tool call entry. This guarantees ordering on the wire.

## Key Files

| File | Relevance |
|------|-----------|
| `zed-4/crates/acp_thread/src/acp_thread.rs` | `flush_streaming_text()`, `TASK_UPDATE_MS`, tool call entry creation |
| `helix-4/api/pkg/server/websocket_external_agent_sync.go` | `handleMessageAdded`, `publishInterval`, `flushAndClearStreamingContext` |
| `helix-4/api/pkg/server/wsprotocol/accumulator.go` | `MessageAccumulator` — per-entry type tracking |
| `helix-4/frontend/src/contexts/streaming.tsx` | Patch application, `entry_count` handling |
| `helix-4/frontend/src/utils/patchUtils.ts` | `applyPatch` utility |

## Decision: Fix Both Layers

Fixing only the Helix API (Issue A) is the safer and more immediately impactful change — it doesn't require changes to the Zed extension and handles cases where ordering is already wrong on the wire. Fix B in Zed is a belt-and-suspenders improvement for correctness.

Recommended implementation order:
1. Helix API: force-flush before processing a tool call entry type transition
2. Zed: ensure synchronous/ordered send of final text before tool call message
