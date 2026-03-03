# Requirements: Streaming Bug After Patch Optimization

## Problem Statement

After the patch-based streaming optimization was introduced, follow-up messages that involve tool calls cause the previous interaction's response to disappear. The new interaction's response also fails to appear.

Example scenario:
1. User: "say hello" → Agent: "Hello" (streams correctly, displays in UI)
2. User: "list the contents of /tmp" → Agent uses tool call → **"Hello" disappears, new response never appears**

**Additional bug observed:** Interactions are not being marked as complete. Even after the agent finishes responding, the UI shows the interaction as still in progress (e.g., text gets truncated, spinner keeps showing).

## User Stories

### US-1: Follow-up messages preserve previous content
As a user, when I send a follow-up message in a conversation, I expect the previous interaction's response to remain visible while the new response streams.

**Acceptance Criteria:**
- Previous interaction responses remain visible during new interaction streaming
- UI does not flicker or blank out between interactions
- Works for both simple text responses and responses involving tool calls

### US-2: Tool call responses display correctly
As a user, when the agent uses tool calls to answer my question, I expect to see the full response including any tool results.

**Acceptance Criteria:**
- Tool call status/progress is visible during execution
- Final response after tool execution appears in UI
- Response persists after completion (doesn't disappear)

### US-3: Patch-based streaming works across interaction boundaries
As a developer, I expect the patch-based streaming optimization to correctly handle the transition between interactions.

**Acceptance Criteria:**
- `previousContent` on backend is reset for each new interaction
- `patchContentRef` on frontend is properly scoped per interaction
- React Query cache is correctly updated on interaction completion

### US-4: Interactions are marked complete correctly
As a user, when the agent finishes responding, I expect the interaction to be marked as complete so the UI reflects the final state.

**Acceptance Criteria:**
- `message_completed` event is received and processed by Helix backend
- Interaction state is updated to `complete` in the database
- `interaction_update` with `state=complete` is published to frontend
- UI stops showing "in progress" indicators after completion

## Root Cause Analysis

Based on code analysis of both Helix backend and Zed frontend:

### Issue 1: Backend `streamingContext` caches wrong interaction

**Location:** `helix/api/pkg/server/websocket_external_agent_sync.go` - `getOrCreateStreamingContext()`

The streaming context is cached per **session**, not per interaction. When:
1. First interaction completes → `flushAndClearStreamingContext()` clears the cache
2. User sends follow-up → New interaction created
3. First assistant token arrives → `getOrCreateStreamingContext()` creates new context

**Problem:** If `message_completed` for interaction 1 races with `message_added` for interaction 2, the streaming context might still reference interaction 1, causing patches to be computed against the wrong `previousContent`.

### Issue 2: Frontend `patchContentRef` not cleared between interactions

**Location:** `helix/frontend/src/contexts/streaming.tsx`

The `patchContentRef` is keyed by `interactionId`, but when a new interaction starts:
- Patches for the NEW interaction arrive with NEW `interactionId`
- The old interaction's entry in `patchContentRef` is only cleared on `interaction_update` (completion)
- If `interaction_update` for the old interaction arrives AFTER patches for the new interaction start, the `setCurrentResponses` may have stale state

### Issue 3: `currentResponses` keyed by sessionId causes overwrite

**Location:** `helix/frontend/src/contexts/streaming.tsx` and `helix/frontend/src/hooks/useLiveInteraction.ts`

`currentResponses` is a `Map<sessionId, Partial<Interaction>>`. When patches for a new interaction arrive:
- They overwrite the entry for that sessionId
- `useLiveInteraction` checks `currentResponse?.id === initialInteraction?.id`
- If the UI is still rendering the OLD interaction, it correctly ignores mismatched responses
- **BUT**: The Session page decides what to render based on `session.interactions` from React Query cache, which may be stale

### Issue 4: React Query cache update timing

When `interaction_update` or `session_update` arrives:
1. Cache is updated via `queryClient.setQueryData()`
2. `currentResponses` is also updated via `setCurrentResponses()`
3. Components re-render with new data

**Race condition:** If the cache update for the completed interaction 1 arrives AFTER patches for interaction 2 have started updating `currentResponses`, the UI may:
1. See interaction 2's patches in `currentResponses`
2. But still be rendering interaction 1 (which now shows empty because its patches are gone from `patchContentRef`)

### Issue 5: `message_completed` not marking interaction as complete

**Location:** `helix/api/pkg/server/websocket_external_agent_sync.go` - `handleMessageCompleted()`

Several failure modes can cause `message_completed` to not mark the interaction complete:

1. **No session mapping found:** If `contextMappings[acpThreadID]` is empty AND database fallback fails, the function logs a warning and returns early without marking complete.

2. **No waiting interaction found:** If no interaction has `state=waiting`, the function logs a warning and returns early. This can happen if there's a race where the interaction was already marked complete or never created.

3. **Streaming context not flushed:** The `flushAndClearStreamingContext()` should write final content to DB, but if the streaming context doesn't exist (already cleared), content may be lost.

## Protocol Flow (from code analysis)

**Note:** `zed/crates/external_websocket_sync/PROTOCOL_SPEC.md` is out of date - it's missing `simulate_user_input`, `query_ui_state`, and `ui_state_response`. The actual protocol is documented here based on code inspection.

```
Flow 2: Follow-up Message to Existing Thread

Helix -> Zed:  chat_message { message: "...", request_id: "req-2", acp_thread_id: "thread-1" }
Zed -> Helix:  message_added { acp_thread_id: "thread-1", message_id: "msg-2", content: "..." }
Zed -> Helix:  message_completed { acp_thread_id: "thread-1", message_id: "msg-2", request_id: "req-2" }
```

**Important architecture note:** Zed sends **full accumulated content** in each `message_added` event (not deltas). The Helix Go backend then computes patches using `computePatch(previousContent, newContent)` before forwarding to the frontend. This is bandwidth-inefficient on the Zed→Helix link.

**DISCOVERY: Unmerged Zed branch `fix/zed-streaming-speedup`** contains 10 commits not on main, including:
- `9fcab9c009 perf(agent_ui): boundary-based WebSocket updates for streaming` - reduces updates from ~1000/turn to ~7
- `06002c07a5 feat(agent): enable session loading for native zed-agent via WebSocket`
- Several other fixes for WebSocket sync

This branch modifies `crates/agent_ui/src/acp/thread_view.rs` (not external_websocket_sync) to send updates only at boundaries (NewEntry, tool call status change, Stopped) instead of every token. **This should be merged or the approach adopted.**

## Constraints

- Must maintain patch-based streaming efficiency gains (O(delta) not O(N))
- Must not break SSE-based streaming for non-external-agent sessions
- Fix should handle rapid follow-up messages (user sending before completion)
- Must work with tool calls that temporarily modify and then restore content
- E2E test phases 2, 4, and 7 specifically test follow-up scenarios