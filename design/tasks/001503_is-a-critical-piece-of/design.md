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

## Implementation Notes

### Root Cause Analysis

**Problem 1 (Messages don't get finished):** The Go API side was actually correct — `flushAndClearStreamingContext` already forces a DB write when `sctx.dirty == true`, and `handleExternalAgentReceiver` processes WebSocket messages sequentially (no reordering). The real bug was on the frontend: `interaction_patch` events use `requestAnimationFrame` to batch updates. When `interaction_update` with `state=complete` arrived, a stale RAF callback from a previous patch could fire AFTER the completion update, overwriting the final `response_message` with truncated streaming content.

**Problem 2 (Tool calls need collapsing):** Zed's `ToolCall.to_markdown()` outputs `**Tool Call: <name>**\nStatus: <status>\n\n<body>`. The Go API's `MessageAccumulator` concatenates these into a single `response_message` string (separated by `\n\n` for distinct message IDs). The frontend rendered this raw markdown directly.

**Problem 3 (Session switching mess):** `clearSessionData` in `streaming.tsx` only cleared `stepInfos` for the new session — it didn't clear `currentResponses`, `patchContentRef`, `patchPendingRef`, `messageBufferRef`, or `messageHistoryRef` for the old session. Additionally, `useLiveInteraction` only reset `lastKnownMessage` when `interactionId` changed, not when `sessionId` changed, causing stale content to leak across sessions.

### Files Modified

| File | Change |
|------|--------|
| `api/pkg/server/websocket_external_agent_sync.go` | Added WARN log when `message_completed` finds empty `response_message` |
| `frontend/src/components/session/CollapsibleToolCall.tsx` | **NEW** — `parseToolCallBlocks()` parser + `CollapsibleToolCall` component |
| `frontend/src/components/session/InteractionInference.tsx` | Added `MessageWithToolCalls` wrapper that splits response by tool call blocks |
| `frontend/src/components/session/InteractionLiveStream.tsx` | Same `MessageWithToolCalls` treatment for live streaming messages |
| `frontend/src/contexts/streaming.tsx` | Enhanced `clearSessionData` to clear all refs; session ID filtering on `session_update`; synchronous `currentResponses` update on completion; clear `patchPendingRef` on completion |
| `frontend/src/hooks/useLiveInteraction.ts` | Reset all state on `sessionId` change via `prevSessionIdRef`; prioritize `initialInteraction.response_message` over `lastKnownMessage` when `isComplete` |
| `frontend/src/components/session/EmbeddedSessionView.tsx` | Reset `hasInitiallyScrolled`, scroll state, and remove old session cache on `sessionId` change |

### Key Design Decisions

1. **Synchronous update on completion:** When `interaction_update` with `state=complete` arrives in `streaming.tsx`, we update `currentResponses` synchronously (not via `requestAnimationFrame`) to prevent the RAF race condition. This is safe because completion events are infrequent (once per message turn).

2. **Tool call parser uses regex, not AST:** The `parseToolCallBlocks` function uses a regex `^\*\*Tool Call: (.+?)\*\*\s*\nStatus: (\S+)` to detect complete tool call blocks. Incomplete blocks (missing `Status:` line, e.g. during streaming) pass through as raw markdown. This is deliberately lenient — if the format changes, it gracefully degrades to showing raw text.

3. **Old session cache removal:** `EmbeddedSessionView` calls `queryClient.removeQueries()` for the old session when `sessionId` changes. This prevents flash of stale content but means navigating back to a session requires a fresh fetch. This is acceptable because session data is small and the refetch is fast.

4. **Priority chain for message resolution in `useLiveInteraction`:** `completedMessage` (from initialInteraction when isComplete) > `safeResponseMessage` (from streaming currentResponses) > `lastKnownMessage` (preserved fallback) > `""`. The `completedMessage` path ensures the final DB-sourced content always wins over throttled streaming data.

### Gotchas

- The Go API build has a pre-existing `go-tree-sitter` dependency issue that causes `go build ./pkg/server/` to fail. This is not from our changes — use `go vet` on specific files or check CI for real build status.
- `Interaction.tsx` uses `React.memo` with a custom `areEqual` function that checks `response_message` and `state` — this correctly triggers re-renders when the completion event updates the React Query cache.
- The `CollapsibleToolCall` component is duplicated between `InteractionInference.tsx` and `InteractionLiveStream.tsx` as a local `MessageWithToolCalls` wrapper. This avoids creating a separate file for a thin wrapper and keeps the rendering logic co-located with where it's used.

## Risks and Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| Tool call markdown format changes in Zed | Collapsing parser breaks | Use a lenient regex that matches the `**Tool Call:` prefix; fall back to raw display if pattern doesn't match |
| Flush timing race on slow connections | Final content lost | The Go API already reloads from DB in `handleMessageCompleted`; ensure the flush writes are synchronous before reload |
| Session switch during active streaming | Stale events processed | Add explicit session ID filtering in WebSocket event handler |
| `requestAnimationFrame` batching delays | Patch content lost on completion | Clear patch ref when `interaction_update` with `state=complete` arrives |