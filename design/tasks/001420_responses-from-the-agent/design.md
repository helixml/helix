# Design: Fix Design Review Comment Streaming

## Overview

Restore streaming of agent responses to design review comments by passing `requestID` through the patch-based streaming path so commenters receive real-time updates.

## Architecture Context

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
    → pubsub.Publish(commenter's queue via requestToCommenterMapping)
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