# Design: Helix Chat Sidebar Polish

## Architecture Overview

The Helix chat sidebar involves three systems communicating over WebSocket:

```
Zed (Rust) ──WebSocket──▶ Go API Server ──WebSocket/PubSub──▶ React Frontend
   │                          │                                    │
   ├─ AcpThread entries       ├─ streamingContext cache            ├─ streaming.tsx (WebSocket handler)
   ├─ thread_service.rs       ├─ MessageAccumulator               ├─ useLiveInteraction hook
   │  (throttle + flush)      ├─ handleMessageAdded               ├─ InteractionLiveStream
   └─ websocket_sync.rs       ├─ handleMessageCompleted           ├─ Interaction / InteractionInference
                               └─ publishInteractionPatch          └─ EmbeddedSessionView
```

**Data flow for a streaming message:**
1. Zed's `AcpThread` emits `EntryUpdated` events as the LLM streams tokens
2. `thread_service.rs` throttles these (100ms interval) and sends `message_added` events over WebSocket
3. Go API's `handleMessageAdded` accumulates content via `MessageAccumulator`, throttles DB writes (200ms) and frontend publishes (separate interval)
4. Go API publishes `interaction_patch` (delta) or `interaction_update` (full) events via PubSub
5. React `streaming.tsx` receives these, updates `currentResponses` state map
6. `useLiveInteraction` hook merges streaming state with React Query cache
7. `InteractionLiveStream` renders the message with a blinking cursor

## Problem Analysis & Solutions

### Problem 1: Messages Don't Get Finished

**Root Cause Hypothesis:** There's a race between `flush_streaming_throttle` → `message_added` (final content) → `message_completed` on the Zed side, and the Go API's `flushAndClearStreamingContext` + `handleMessageCompleted`. Specifically:

- The Zed side calls `flush_streaming_throttle(&thread_id)` then immediately sends `message_completed`. Both go through `send_websocket_event` which is synchronous to the WebSocket write. But the Go API processes these sequentially on the same goroutine, so ordering is preserved.
- The more likely issue: the Go API's `handleMessageAdded` uses **throttled DB writes** (`dbWriteInterval = 200ms`). The final `message_added` may update the in-memory `streamingContext` but not flush to DB. Then `handleMessageCompleted` calls `flushAndClearStreamingContext` which should flush — but if there's a timing issue where the context was already cleared, the final content is lost.
- Another possibility: the frontend's `interaction_patch` mechanism. If the final patch arrives but `patchContentRef` is cleared before the `requestAnimationFrame` callback fires, the content disappears briefly before the `interaction_update` (completion) arrives.

**Solution:**

1. **Go API (`handleMessageCompleted`):** After `flushAndClearStreamingContext`, reload the interaction from DB and verify `response_message` length. If it's empty or suspiciously short, log a `WARN`. This is already partially done (the function reloads from DB).

2. **Go API (`flushAndClearStreamingContext`):** Ensure the function always writes to DB if `dirty == true`, even if the context is being cleared. Add a guard: if `sctx.dirty` and content exists, force a DB write before clearing. (Review needed — this may already be correct.)

3. **Frontend (`useLiveInteraction`):** The `lastKnownMessage` fallback is the right pattern but may not cover all edge cases. When `isComplete` becomes true, the hook should immediately read from the React Query cache (which `handleMessageCompleted` updates via `interaction_update`) rather than relying on the streaming state. Add explicit priority: completed interaction from cache > lastKnownMessage > streaming state.

4. **Frontend (`streaming.tsx`):** When an `interaction_update` with `state=complete` arrives, clear the `patchContentRef` for that interaction so stale patches don't override the final content.

### Problem 2: Tool Calls Need Collapsing

**Current State:** The Go API's `MessageAccumulator` concatenates all Zed thread entries (assistant messages + tool calls) into a single `response_message` string. Tool calls come through as markdown like:

```
**Tool Call: edit**
Status: Completed

[diff content...]
```

This raw markdown is rendered directly in `InteractionInference.tsx` via the `Markdown` component.

**Solution:** Add a tool call collapsing feature in the frontend markdown rendering:

1. **Create a `CollapsibleToolCall` component** that renders a single summary line with an expand/collapse toggle. When collapsed, show: icon + tool name + status. When expanded, show the full content.

2. **Parse tool call blocks in the markdown** — detect the pattern `**Tool Call: <name>**\nStatus: <status>\n\n<content>` in the response text. This can be done either:
   - **Option A (chosen): Pre-process in `InteractionInference.tsx`** — split the response_message by tool call markers before passing to `<Markdown>`. Wrap each tool call block in a `<CollapsibleToolCall>` component.
   - **Option B: Custom markdown renderer plugin** — add a remark/rehype plugin. Rejected because it couples rendering logic to markdown parsing and is harder to maintain.

3. **Handle streaming gracefully** — during streaming, a tool call block may be incomplete (e.g., we have `**Tool Call: edit**` but no `Status:` line yet). The parser should only collapse complete tool call blocks; incomplete ones render as-is until complete.

### Problem 3: Session/Thread Switching Mess

**Root Cause:** Multiple issues compound:

1. **`streaming.tsx` `currentSessionId` guard is insufficient.** The `handleWebsocketEvent` callback captures `currentSessionId` in its closure (via `useCallback` dependency). But WebSocket events arrive asynchronously — if the user switches from session A to session B, late events for session A can still be processed if the WebSocket hasn't disconnected yet. The guard `if (!currentSessionId) return;` only checks for null, not for "is this event for the current session?"

2. **`useLiveInteraction` `lastKnownMessage` persists across session switches.** The `currentInteractionId` tracking helps, but if the new session's first interaction happens to have the same ID format, stale content could leak. More critically, the `currentResponses` map is keyed by sessionId, so switching sessions with the same sessionId (edge case with session reuse) would show old data.

3. **React Query cache staleness.** `EmbeddedSessionView` uses `useGetSession` with `refetchInterval: 2000`. When switching sessions, the old cache is served immediately while the new fetch is in-flight, causing a flash of old content.

**Solution:**

1. **`streaming.tsx`: Filter events by session ID at the WebSocket level.** Every event from the Go API includes a session identifier (either in `parsedData.session?.id` or derivable from the subscription). Add an explicit check: if the event's session ID doesn't match `currentSessionId`, discard it.

2. **`streaming.tsx`: Full state reset on session switch.** When `setCurrentSessionId` is called with a new value, clear ALL state: `currentResponses`, `stepInfos`, `patchContentRef`, `messageBufferRef`, `messageHistoryRef`. Currently `clearSessionData` only clears some of these. Make it comprehensive.

3. **`useLiveInteraction`: Reset all state on sessionId change.** Add `sessionId` to the dependency that resets `lastKnownMessage` and `currentInteractionId`. Currently it only resets on `initialInteraction?.id` change.

4. **`EmbeddedSessionView`: Invalidate cache on session switch.** When `sessionId` prop changes, call `queryClient.removeQueries` for the old session's query key before fetching the new one. This prevents flash of stale content.

## Key Codebase Patterns Discovered

- **Streaming throttle (Zed):** `thread_service.rs` uses a global `STREAMING_THROTTLE` static with 100ms interval. `flush_streaming_throttle` is called before every `message_completed` to drain pending content.
- **Streaming context (Go):** `streamingContext` in `websocket_external_agent_sync.go` caches DB lookups during streaming. Throttles DB writes at 200ms and frontend publishes at a separate interval. Cleared on `message_completed`.
- **MessageAccumulator (Go):** `wsprotocol/accumulator.go` handles multi-entry responses. Same `message_id` = overwrite (streaming), different `message_id` = append with `\n\n` separator. Persists offset in DB for restart resilience.
- **Frontend streaming:** Three event types drive updates: `session_update` (full session), `interaction_update` (single interaction), `interaction_patch` (delta). Patches are most efficient during streaming; full updates on completion.
- **useLiveInteraction:** Merges SSE streaming data (`currentResponses`) with React Query cache (`initialInteraction`). Uses `lastKnownMessage` to prevent blank flash during the streaming→completed transition.
- **Tool call rendering:** Zed's `ToolCall.to_markdown()` serializes tool calls as markdown with `**Tool Call: <name>**\nStatus: <status>` headers. This arrives at the Go API as part of the accumulated response_message string.
- **Session switching:** `streaming.tsx` manages a single `currentSessionId`. WebSocket subscription is shared (not per-session). The `clearSessionData` function is supposed to reset state but doesn't clear all refs.

## Risks and Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| Tool call markdown format changes in Zed | Collapsing parser breaks | Use a lenient regex that matches the `**Tool Call:` prefix; fall back to raw display if pattern doesn't match |
| Flush timing race on slow connections | Final content lost | The Go API already reloads from DB in `handleMessageCompleted`; ensure the flush writes are synchronous before reload |
| Session switch during active streaming | Stale events processed | Add explicit session ID filtering in WebSocket event handler |
| `requestAnimationFrame` batching delays | Patch content lost on completion | Clear patch ref when `interaction_update` with `state=complete` arrives |