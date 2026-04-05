# Design: Fix Comment Queue Transition State

## Architecture Pattern

The comment queue uses a single `streamingResponse` React state in `DesignReviewContent.tsx` to track the currently-streaming comment's content. When streaming completes, `setStreamingResponse(null)` is called immediately alongside `queryClient.invalidateQueries()`.

The race: the cache invalidation is async (network round-trip), so child components lose `streamingResponse` before `comment.agent_response` is available in the refreshed cache. Result: comment 1's `displayResponse` is falsy for 1–5 seconds → shows "Waiting for agent response...".

## Solution: Mark Streaming as Complete Instead of Clearing

Add an `isComplete` flag to the `streamingResponse` state shape. When the interaction finishes, set `isComplete: true` instead of clearing to `null`. This keeps comment 1's content visible (without the streaming indicator) until either:
1. Comment 2's first `interaction_patch` event arrives and naturally overwrites `streamingResponse` with comment 2's data, OR
2. The cache refresh confirms `comment.agent_response` is populated (at which point `comment.agent_response` takes over as the display source and `streamingResponse` for that comment is no longer needed)

### State Change

```typescript
// Current state shape
type StreamingState = { commentId: string; content: string; entries: ResponseEntry[] } | null

// New state shape
type StreamingState = {
  commentId: string;
  content: string;
  entries: ResponseEntry[];
  isComplete?: boolean;   // true = done streaming but content still displayed as fallback
} | null
```

### On Completion (lines ~484–497 and ~550–557 in DesignReviewContent.tsx)

```typescript
// Before (clears immediately):
setStreamingResponse(null);

// After (marks complete, keeps content visible):
setStreamingResponse(prev => prev ? { ...prev, isComplete: true } : null);
```

### Natural Cleanup

When the next comment's `interaction_patch` arrives, the handler calls:
```typescript
setStreamingResponse({ commentId: comment2Id, content: ..., entries: ... });
// isComplete is NOT set → comment 2 shows as streaming, comment 1's old state is gone
```
No explicit cleanup of the old comment 1 state is needed.

### Display Components

`InlineCommentBubble` and `CommentLogSidebar` both compute:
```typescript
const isActiveStream = streamingResponse?.commentId === comment.id
const displayResponse = isActiveStream ? streamingResponse!.content : comment.agent_response
const isStreaming = isActiveStream && !comment.agent_response
```

When `isComplete: true`, `isActiveStream` is still `true` for comment 1, so `displayResponse` shows the content. `isStreaming` is `false` once `comment.agent_response` is populated — but here we also want `isStreaming = false` even before the cache updates:

```typescript
const isStreaming = isActiveStream && !comment.agent_response && !streamingResponse?.isComplete
```

This ensures the streaming indicator disappears immediately when the response finishes, while the content stays visible.

## Key Files

| File | Change |
|------|--------|
| `frontend/src/components/spec-tasks/DesignReviewContent.tsx` | Change `setStreamingResponse(null)` to `setStreamingResponse(prev => prev ? { ...prev, isComplete: true } : null)` in both completion handlers |
| `frontend/src/components/spec-tasks/CommentLogSidebar.tsx` | Update `isStreaming` computation to check `!streamingResponse?.isComplete` |
| `frontend/src/components/spec-tasks/InlineCommentBubble.tsx` | Update `isStreaming` prop computation at call site in DesignReviewContent; or update internal `isStreaming` if `isComplete` is passed down |

## Passing isComplete to Child Components

`InlineCommentBubble` receives `streamingResponse?: string` (just the content string) and `streamingEntries`. A new `isStreamingComplete?: boolean` prop is added so the bubble can suppress the streaming indicator while keeping content visible.

`CommentLogSidebar` already receives the full `StreamingResponse` object, so its `isComplete` field is accessed directly.

## Relationship to Prior Backend Fix

Commit `ec845a4aa` ("fix: finalize comment before frontend publish") fixed the backend ordering so `finalizeCommentResponse` runs synchronously **before** the completion event is published. This ensures `agent_response` is in the DB when the frontend refetches.

However, the frontend still has a race window between `setStreamingResponse(null)` clearing the content and React Query's async refetch completing. Our fix closes this remaining gap on the frontend side — both fixes are complementary and neither alone fully eliminates the visual glitch.

## Codebase Pattern Notes

- This project uses React Query with `refetchInterval` for polling and `invalidateQueries` after mutations (per CLAUDE.md — no `setQueryData`).
- WebSocket streaming uses `accumulatedResponse` + `streamEntries` local variables (inside the `useEffect` closure) for real-time state; the `streamingResponse` React state is updated via `setStreamingResponse`.
- Both `session_update` (lines ~414–497) and `interaction_update` (lines ~500–557) completion handlers in `DesignReviewContent.tsx` need the same fix.
