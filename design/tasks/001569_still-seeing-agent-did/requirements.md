# Requirements: Agent Comment Response Display & Copy Bug

## Bug 1: "[Agent did not respond]" shown after streaming completes

### User Story
As a user reviewing agent responses to design comments, I want to see the complete response the agent gave, even after streaming has ended — without getting a false "did not respond" error.

### Current Behavior
1. Agent responds to a comment; response streams in correctly via WebSocket.
2. Streaming ends; frontend invalidates React Query cache and refetches the comment.
3. The refetched comment shows "[Agent did not respond - try sending your comment again]" (set by a 2-minute backend timeout), even though the agent clearly responded.

### Root Cause (from code analysis)
The backend has a 2-minute timeout (`sessionCommentTimeout`) that fires if the comment still has a non-empty `RequestID` and empty `AgentResponse`. Two failure modes:

**Mode A – streaming updates not saving to DB:**
`updateCommentWithStreamingResponse` calls `GetCommentByRequestID`, which could fail to find the comment if the request_id mapping is missing/wrong. The error is logged but not fatal — streaming appears to work in the UI (via in-memory WebSocket state) but nothing is persisted. When the timeout fires, `AgentResponse == ""`, so the error message is set.

**Mode B – `finalizeCommentResponse` not called:**
`handleMessageCompleted` extracts `request_id` from `syncMsg.Data["request_id"]` (line 1994 of `websocket_external_agent_sync.go`). If the agent doesn't include `request_id` in the completion message, `messageRequestID` is empty string, the fallback session-based lookup runs, and if that also fails, `finalizeCommentResponse` is never called. The `RequestID` is never cleared, so the 2-min timeout eventually fires. If `updateCommentWithStreamingResponse` also didn't save content (Mode A), `AgentResponse` is still empty and the error is set.

### Acceptance Criteria
- After streaming completes, the comment displays the full agent response — not "[Agent did not respond - try sending your comment again]".
- The stored `agent_response` in the DB matches what was streamed.
- The response shown is specifically from this comment's reply (not from a later follow-up interaction).
- The 2-minute timeout error message is only shown when the agent genuinely did not respond.

---

## Bug 2: Cmd+C / Ctrl+C Does Not Copy Text

### User Story
As a user reading agent responses in the Helix UI, I want to be able to copy text with Cmd+C (macOS) or Ctrl+C (Linux/Windows) as expected.

### Current Behavior
Pressing Cmd+C or Ctrl+C does nothing (or toggles the comment form instead of copying).

### Root Cause
`DesignReviewContent.tsx` registers a global `keydown` handler that intercepts the `"c"` key to toggle the comment form (`setShowCommentForm`). The handler calls `e.preventDefault()` without checking whether Ctrl or Cmd is held. Since `e.key` for Ctrl+C and Cmd+C is also `"c"`, the handler intercepts and cancels those keyboard shortcuts too.

```typescript
// Current (buggy):
case "c":
  setShowCommentForm((prev) => !prev);
  e.preventDefault();  // blocks Ctrl+C and Cmd+C
  break;
```

### Acceptance Criteria
- Cmd+C and Ctrl+C work normally to copy selected text anywhere in the Helix UI.
- Pressing bare `c` (without modifier keys) still toggles the comment form as intended.
