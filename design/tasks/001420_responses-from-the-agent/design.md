# Design: Fix Design Review Comment Streaming

## Overview

Restore streaming of agent responses to design review comments by passing `requestID` through the patch-based streaming path so commenters receive real-time updates.

## Architecture Context

### WebSocket Pub/Sub Model

The system uses **per-user queues**, not broadcast. Each user subscribes to their own queue:
- Queue name: `session-updates.<userID>.<sessionID>`
- Users only receive messages explicitly published to their queue
- This is NOT a broadcast to all users with read permission on the project

When streaming, the server must explicitly publish to each user who should receive updates:
1. **Session owner**: Always receives updates (primary recipient)
2. **Commenter**: Only receives updates if we explicitly publish to their queue via `requestToCommenterMapping`

### Current Data Flow (Broken)
```
Agent (Zed) → message_added → handleMessageAdded → streamingContext
    → publishInteractionPatchToFrontend(sessionID, owner, interaction, previousContent)
    → pubsub.Publish(owner's queue only)
    ✗ Commenter never receives patches
```

### Fixed Data Flow
```
Agent (Zed) → message_added → handleMessageAdded → streamingContext (with requestID)
    → publishInteractionPatchToFrontend(sessionID, owner, interaction, previousContent, requestID)
    → pubsub.Publish(owner's queue)
    → pubsub.Publish(commenter's queue via requestToCommenterMapping lookup)
    ✓ Both owner and commenter receive patches
```

## Key Components

| File | Change |
|------|--------|
| `api/pkg/server/websocket_external_agent_sync.go` | Add `requestID` to `streamingContext`, pass to publish function |
| `api/pkg/server/websocket_external_agent_sync.go` | Update `publishInteractionPatchToFrontend` to accept optional `requestID` and publish to commenter |

## Implementation Details

### 1. Extend streamingContext struct

Add `requestID` field to cache the request ID when streaming context is created:

```go
type streamingContext struct {
    session         *types.Session
    interaction     *types.Interaction
    requestID       string  // NEW: cached for commenter notification
    // ... existing fields
}
```

### 2. Capture requestID in handleMessageAdded

Extract `requestID` from `syncMsg.Data["request_id"]` and store in streaming context on first message_added.

### 3. Update publishInteractionPatchToFrontend

Add optional `requestID` parameter (variadic to maintain backward compatibility):

```go
func (apiServer *HelixAPIServer) publishInteractionPatchToFrontend(
    sessionID, owner string,
    interaction *types.Interaction,
    previousContent string,
    requestID ...string,  // NEW: optional
) error
```

If `requestID` is provided, look up commenter from `requestToCommenterMapping` and publish patch to their queue as well.

## Risks & Mitigations

| Risk | Mitigation |
|------|------------|
| Performance regression from extra pubsub publish | Negligible - one extra publish per throttled interval (50ms), same pattern as `publishInteractionUpdateToFrontend` |
| Frontend not handling patches for comments | Frontend already handles `session_update` which has full interaction - patches are reconstructed to same format |

## Decision: Why Patch to Commenter (Not Full Update)

Sending patches to commenters (rather than falling back to full `session_update`) maintains the performance benefits of the optimization. The commenter's browser reconstructs the full response from patches just like the owner's browser does.

## Testing

1. Submit a comment on a design review as a non-owner user
2. Verify streaming response appears in real-time in the comment bubble
3. Verify chat streaming in main session view still works
4. Verify multiple comments in queue still process sequentially

## Implementation Notes

### Changes Made

Single file modified: `api/pkg/server/websocket_external_agent_sync.go`

1. Added `requestID string` field to `streamingContext` struct (line 29)
2. In `handleMessageAdded`, extract `request_id` from `syncMsg.Data` when role is "assistant" (line 959)
3. Store `requestID` in streaming context on first message_added (lines 975-982)
4. Updated `publishInteractionPatchToFrontend` signature to accept `requestID ...string` (variadic for backward compat)
5. Added commenter publishing logic using existing `requestToCommenterMapping` pattern (lines 2601-2618)

### Key Discovery

The `requestToCommenterMapping` already existed and was used by `publishInteractionUpdateToFrontend` and `publishSessionUpdateToFrontend` - the patch function just wasn't using it because it didn't have access to the requestID.

### Build Note

Local Go build fails due to pre-existing tree-sitter dependency issue on main branch (not related to this change). Verified no syntax errors via diagnostics tool.

### Frontend Changes Were Needed (Discovery)

Initial assumption was wrong - the frontend `DesignReviewContent.tsx` only handled `session_update` events, NOT `interaction_patch` events. The main streaming context (`streaming.tsx`) handles patches, but the design review component had its own WebSocket handler that didn't.

**Second file modified:** `frontend/src/components/spec-tasks/DesignReviewContent.tsx`

Added handling for:
1. `interaction_patch` events - reconstruct full content from delta patches (same algorithm as streaming.tsx)
2. `interaction_update` events - handle completion state and invalidate queries
3. Reset `patchContent` on completion for next streaming session

### Critical Fix: sessionToCommenterMapping (Second Discovery)

The backend fix alone still didn't work because **Zed's `message_added` events don't include `request_id`**. The original code tried to extract `request_id` from `syncMsg.Data` but it wasn't there.

**Root cause:** `requestToCommenterMapping` is keyed by `request_id`, but we only have `request_id` when we *send* the command to Zed, not when Zed streams back responses.

**Solution:** Added `sessionToCommenterMapping` (keyed by `sessionID` instead of `requestID`):

1. **server.go**: Added `sessionToCommenterMapping map[string]string` field
2. **spec_task_design_review_handlers.go**: Set `sessionToCommenterMapping[sessionID] = notifyUserID` when sending comment
3. **websocket_external_agent_sync.go**: Look up commenter by `helixSessionID` in `handleMessageAdded`
4. **spec_task_design_review_handlers.go**: Clean up mapping in `finalizeCommentResponse`

Changed `streamingContext.requestID` to `streamingContext.commenterID` since we now store the looked-up commenter directly.