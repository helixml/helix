# Design: Agent Comment Response Display & Copy Bug

## Bug 1: "[Agent did not respond]" fix

### Key Files
- `api/pkg/server/spec_task_design_review_handlers.go` — `updateCommentWithStreamingResponse` (line ~920), `finalizeCommentResponse` (line ~954), timeout logic (line ~700)
- `api/pkg/server/websocket_external_agent_sync.go` — `handleMessageCompleted`, `messageRequestID` extraction (line 1994)

### What the Code Does Today

**During streaming** (`message_added` events):
- `updateCommentWithStreamingResponse(ctx, requestID, content)` is called
- Calls `s.Store.GetCommentByRequestID(ctx, requestID)` — if this fails, the error is returned but ignored at the call site; no content is persisted
- If found, calls `UpdateCommentAgentResponse()` to persist the partial response

**On completion** (`message_completed` event):
- `messageRequestID, _ := syncMsg.Data["request_id"].(string)` — if the agent omits `request_id`, this is `""`
- If `messageRequestID != ""`: calls `finalizeCommentResponse(ctx, requestID)` (primary path)
- If `messageRequestID == ""`: falls back to session-based `GetPendingCommentByPlanningSessionID()` — this can also fail
- If both paths fail: `finalizeCommentResponse` is never called, `RequestID` is never cleared

**Timeout (2 minutes after `sendCommentToAgentNow`):**
- Re-fetches comment from DB
- If `RequestID != "" && AgentResponse == ""`: sets error message
- Condition: both must be true — so if `updateCommentWithStreamingResponse` WAS saving content, the timeout won't fire

### Diagnosis Strategy for Implementer

Add targeted logging to understand which failure mode is occurring:

1. In `updateCommentWithStreamingResponse`: log when `GetCommentByRequestID` fails (currently the error is returned but likely swallowed at call site — verify this)
2. In `handleMessageCompleted`: log whether `messageRequestID` is empty or populated
3. In `finalizeCommentResponse`: log the call and outcome

This will clarify whether it's Mode A (streaming not persisting), Mode B (finalize not called), or both.

### Fix Approach

**If Mode A (streaming not persisting):**
- Investigate why `GetCommentByRequestID` fails to find the comment during streaming
- Ensure the `requestID` passed to streaming update matches what's stored on the comment

**If Mode B (finalize not called):**
- Check whether the agent actually includes `request_id` in the `message_completed` payload
- If not, ensure the fallback session-based lookup works reliably, OR ensure the agent always includes `request_id`

**Defensive fix (regardless of root cause):**
In `finalizeCommentResponse`, if `AgentResponse` is still empty after all lookup attempts, also try reading from the DB's latest streaming-saved content (the `UpdateCommentAgentResponse` updates it incrementally — if those saves work, the content is already there when finalize runs).

The timeout guard (`AgentResponse == ""`) is correct logic — but the precondition (streaming updates saving properly) must be reliable.

### Frontend Note
The frontend uses `streamingResponse` state during streaming (correct), then clears it and refetches on `state === "complete"`. The display after refetch is entirely driven by `comment.agent_response` from the DB. No frontend-only fix is needed — the DB content must be correct.

---

## Bug 2: Cmd+C / Ctrl+C Copy Fix

### Key File
`frontend/src/components/spec-tasks/DesignReviewContent.tsx` — `handleKeyPress` around line 544

### Fix
Guard the `"c"` shortcut so it only fires when no modifier key is held:

```typescript
// Before:
case "c":
  setShowCommentForm((prev) => !prev);
  e.preventDefault();
  break;

// After:
case "c":
  if (!e.ctrlKey && !e.metaKey) {
    setShowCommentForm((prev) => !prev);
    e.preventDefault();
  }
  break;
```

This is a one-line guard change. No other copy-related issues were found in the codebase (no `user-select: none` or copy event blocking in the spec-tasks components).

---

## Patterns Learned (for future agents)

- Comment response flow: `sendCommentToAgentNow` → WebSocket streaming (`message_added` → `updateCommentWithStreamingResponse`) → completion (`message_completed` → `finalizeCommentResponse`) → timeout fallback
- The `RequestID` field on a comment is the lynchpin: present = "processing", absent = "done"
- The 2-minute timeout is keyed per session in `s.sessionCommentTimeout[sessionID]`
- Frontend streaming is driven by WebSocket events, not polling; after completion it invalidates React Query
- `handleKeyPress` is registered on `window` — modifier key guards are essential for keyboard shortcuts that share a key with copy/paste
