# Design: Fix Streaming Bug After Patch Optimization

## Summary

Follow-up messages that involve tool calls cause the previous interaction's response to disappear and the new response to never appear. This is caused by race conditions between the patch-based streaming protocol and interaction lifecycle management.

## Architecture Overview

### Current Flow

```
Zed Agent                    Helix Backend                    Frontend
    |                              |                              |
    |--- message_added (accum) --->|                              |
    |                              |--- interaction_patch ------->|
    |                              |    (delta from previousContent)|
    |--- message_completed ------->|                              |
    |                              |--- interaction_update ------>|
    |                              |    (final state)             |
```

### Key Components

1. **Backend `streamingContext`** (per-session cache)
   - Caches session/interaction data to avoid DB queries per token
   - Tracks `previousContent` for patch computation
   - Cleared on `message_completed`

2. **Frontend `patchContentRef`** (per-interactionId)
   - Reconstructs full content from patches
   - Keyed by interactionId, not sessionId

3. **Frontend `currentResponses`** (per-sessionId)
   - Live streaming state for UI rendering
   - Overwritten when new interaction starts streaming

## Root Cause

When a follow-up message is sent, there's a race between:

1. `message_completed` for interaction 1 → clears streaming context
2. New interaction 2 created in DB
3. First `message_added` for interaction 2 → creates new streaming context

**Bug:** The backend creates a new `streamingContext` with the correct new interaction, but `previousContent` starts at `""`. Meanwhile, the **frontend** may still be processing:
- An `interaction_update` for interaction 1 (which clears `patchContentRef[int1]`)
- But `currentResponses[sessionId]` has already been overwritten by patches for interaction 2

The frontend's `useLiveInteraction` hook correctly checks `currentResponse?.id === initialInteraction?.id`, but the **Session page** may briefly render the old interaction with no content (because `patchContentRef` was cleared but `currentResponses` was overwritten).

## Proposed Fix

### Fix 1: Backend - Reset `previousContent` on new interaction

**File:** `api/pkg/server/websocket_external_agent_sync.go`

In `getOrCreateStreamingContext()`, add detection for interaction change:

```go
// In streamingContext, add:
interactionID string  // Track which interaction we're caching

// In getOrCreateStreamingContext(), after finding targetInteraction:
if sctx.interactionID != "" && sctx.interactionID != targetInteraction.ID {
    // Interaction changed! Reset previousContent for clean delta computation
    sctx.previousContent = ""
    sctx.interactionID = targetInteraction.ID
    log.Info().
        Str("old_interaction", sctx.interactionID).
        Str("new_interaction", targetInteraction.ID).
        Msg("📦 [PERF] Interaction changed, reset previousContent")
}
```

### Fix 2: Frontend - Send interaction ID with first patch

**File:** `api/pkg/server/websocket_external_agent_sync.go`

Include `interaction_id` in every `interaction_patch` event (already done), but also send an explicit "interaction started" signal:

```go
// In publishInteractionPatchToFrontend, when previousContent == "":
if previousContent == "" {
    // First patch for this interaction - include full interaction metadata
    event.InteractionState = interaction.State
    event.IsFirstPatch = true
}
```

### Fix 3: Frontend - Handle interaction transitions explicitly

**File:** `frontend/src/contexts/streaming.tsx`

When receiving a patch with a **different** `interactionId` than `currentResponses[sessionId].id`:

```typescript
// In patch handler:
if (parsedData.type === WEBSOCKET_EVENT_TYPE_INTERACTION_PATCH && parsedData.interaction_id) {
  const interactionId = parsedData.interaction_id;
  const currentSessionResponse = currentResponses.get(currentSessionId);
  
  // Detect interaction transition
  if (currentSessionResponse?.id && currentSessionResponse.id !== interactionId) {
    // New interaction started! Clear old patch content, preserve old response in cache
    patchContentRef.current.delete(currentSessionResponse.id);
    
    // Don't immediately overwrite currentResponses - let the cache update first
    // This prevents the flash of empty content
  }
  
  // ... rest of patch handling
}
```

### Fix 4: Frontend - Preserve previous interaction content during transition

**File:** `frontend/src/hooks/useLiveInteraction.ts`

The `lastKnownMessage` preservation logic is good, but needs to handle the case where `initialInteraction` changes:

```typescript
// When initialInteraction?.id changes AND currentResponse is for a different interaction:
// Keep showing lastKnownMessage until the new interaction has content
const interactionMatchesCurrent = interaction?.id === currentInteractionId;
const hasNewInteractionContent = currentResponse?.id === initialInteraction?.id && 
                                  currentResponse?.response_message;

const message = hasNewInteractionContent 
  ? currentResponse.response_message 
  : (interactionMatchesCurrent ? safeResponseMessage : undefined) 
    || lastKnownMessage 
    || "";
```

## Testing Strategy

### Unit Tests (Backend)

Add test case to `websocket_external_agent_sync_test.go`:

```go
func (s *WebSocketSyncSuite) TestStreamingPatch_InteractionTransition() {
    // 1. Start streaming interaction 1
    // 2. Complete interaction 1 (message_completed)
    // 3. Create interaction 2 (simulates follow-up)
    // 4. Start streaming interaction 2
    // 5. Verify: previousContent reset, patches computed correctly
}
```

### E2E Test

The existing Zed E2E test already covers this scenario in **Phase 2** (follow-up on existing thread) and **Phase 4** (follow-up to non-visible thread). Run:

```bash
cd zed/crates/external_websocket_sync/e2e-test
./run_docker_e2e.sh
```

### Manual Test

1. Start a Zed external agent session
2. Send: "say hello"
3. Wait for "Hello" to appear and complete
4. Send: "list the contents of /tmp" (triggers tool call)
5. Verify:
   - "Hello" remains visible
   - Tool execution status appears
   - Final response appears after tool completes

## Rollback Plan

If the fix causes regressions:

1. Revert to full `interaction_update` events instead of patches during streaming
2. Keep patch-based updates only for pure-append scenarios (no tool calls)

This is a safe fallback because `interaction_update` was the previous behavior and is known to work correctly (just less efficient).

## Discovered Patterns

- **Zed sends accumulated content**: Each `message_added` from Zed contains the **full** accumulated message so far, not a delta. Helix backend computes patches from this.
- **Frontend keying matters**: `patchContentRef` is keyed by interactionId (correct), but `currentResponses` is keyed by sessionId (potential issue during transitions).
- **Streaming throttle in Zed**: Zed throttles `message_added` events and flushes before `message_completed`. Helix must handle the final content correctly.