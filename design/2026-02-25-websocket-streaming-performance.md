# Fix O(N^2) WebSocket Streaming Performance

**Date:** 2026-02-25
**Status:** ALL PHASES COMPLETE
**Helix Branch:** `fix/golden-cache-investigation`
**Zed Branch:** `upstream-merge-2026-02-24`

## Problem

The Zed-Helix WebSocket sync protocol has O(N^2) complexity where N = response length in tokens. Every streaming token triggers:

**Zed side:** `content_only(cx)` returns ALL accumulated content (not delta), serializes full content to JSON, sends over WebSocket. No throttling.

**Go side per token (4 synchronous DB queries):**
1. `GetSession(helixSessionID)` -- unchanged during streaming
2. `ListInteractions(perPage=1000)` -- fetches ALL interactions, unchanged during streaming
3. `UpdateInteraction` -- writes full `ResponseMessage` (O(N) bytes)
4. `GetPendingCommentByPlanningSessionID` -- almost always nil

**Plus per token:** goroutine for `linkAgentResponseToComment`, JSON marshal of full interaction, pubsub publish to frontend.

For a 1000-token response (~4KB): 1000 message_added events, each sending 1-4KB, total wire traffic ~2MB (vs 4KB if delta). Plus 4000 DB queries.

## Phase 1: Go-Side Optimizations (COMPLETE)

All changes in `api/pkg/server/websocket_external_agent_sync.go`. 59 tests passing.

### Fix 1: Streaming Context Cache (HIGHEST IMPACT) -- DONE

Cache `GetSession` + `ListInteractions` on first token, reuse for all subsequent. Clear on `message_completed`.

```go
type streamingContext struct {
    session         *types.Session
    interaction     *types.Interaction
    lastDBWrite     time.Time
    lastPublish     time.Time
    dirty           bool
    previousContent string   // Phase 2: delta tracking
    mu              sync.Mutex
}
```

Key methods:
- `getOrCreateStreamingContext()` -- cache miss = DB query, cache hit = return pointer
- `flushAndClearStreamingContext()` -- flush dirty state, delete cache entry

**Saves:** 2 DB round-trips per token.

### Fix 2: Defer Comment Queries to Completion -- DONE

Removed from streaming path entirely. `linkAgentResponseToComment` and `GetPendingCommentByPlanningSessionID` only run in `handleMessageCompleted`.

**Saves:** 2 DB calls + 1 goroutine per token.

### Fix 3: Throttle DB Writes (200ms) -- DONE

`UpdateInteraction` only when `time.Since(lastDBWrite) >= 200ms`. In-memory interaction always has latest content. `flushAndClearStreamingContext()` ensures dirty state is written before `handleMessageCompleted` marks complete.

**Saves:** ~75% of DB writes.

### Fix 4: Throttle Frontend Publishing (50ms) -- DONE

Frontend publishes only when `time.Since(lastPublish) >= 50ms`. `handleMessageCompleted` always publishes (no throttle on completion).

**Saves:** ~75% of JSON marshals + pubsub events.

### Phase 1 Impact

For a 1000-token response streaming at 20 tok/sec:

| Metric | Before | After Phase 1 | Reduction |
|--------|--------|---------------|-----------|
| DB queries | 4000 | ~600 | 85% |
| DB writes | 1000 | ~250 | 75% |
| Goroutines spawned | 1000 | 1 | 99.9% |
| JSON marshals | 1000 | ~250 | 75% |
| Pubsub events | 1000 | ~250 | 75% |

## Phase 2: Patch-Based Protocol Go→Frontend (COMPLETE)

### Goal
Reduce per-update wire traffic from O(N) to O(delta). Each pubsub event was marshalling the full interaction JSON.

### Implementation

**Go side (`api/pkg/server/websocket_external_agent_sync.go`):**

New `interaction_patch` event type sends only the changed portion of ResponseMessage:

```go
// computePatch finds first differing byte between old and new content.
// Fast path: pure append (99% of streaming) → O(len(old)) prefix check.
// Slow path: backwards edit (tool call status changes) → scan for first diff.
func computePatch(previousContent, newContent string) (patchOffset int, patch string, totalLength int)
```

`publishInteractionPatchToFrontend()` sends delta instead of full interaction during streaming. `previousContent` tracked in `streamingContext`, updated after each publish.

**Frontend side (`frontend/src/contexts/streaming.tsx`):**

New `interaction_patch` handler:
1. Applies patch to content stored in `patchContentRef` (a React ref, NOT state)
2. Handles pure appends, backwards edits, and truncations
3. Batches state update via `requestAnimationFrame` — multiple patches in the same frame produce ONE re-render
4. Does NOT update React Query cache during streaming — avoids creating new `interactions` array copies
5. Skips debounced query invalidation for patch events — cache updated only on completion

**React rendering optimization:**
- Patch events flow: `WebSocket → patchContentRef (ref, no re-render) → RAF → setCurrentResponses (one state update per frame)`
- This means: `useLiveInteraction → InteractionLiveStream → Markdown` — only the streaming component tree re-renders
- React Query cache is untouched during streaming, so EmbeddedSessionView and other components don't re-render
- Full interaction_update on completion syncs the React Query cache for final consistency

### Phase 2 Impact

| Metric | After Phase 1 | After Phase 2 | Reduction |
|--------|---------------|---------------|-----------|
| Go→Frontend wire (per event) | O(N) full interaction | O(delta) patch only | ~99% per event |
| JSON marshal size | ~100KB (for 100KB response) | ~200 bytes (new tokens) | 99.8% |
| React Query cache mutations | Every 50ms | Only on completion | ~100% during streaming |
| React re-render scope | Full session interactions | Only streaming component | Targeted |

## Phase 3: Zed-Side Throttle (COMPLETE)

### Goal
Reduce Zed→Go wire traffic. Previously Zed sent full content on every token (~100+ events/sec).

### Implementation

**Rust (`zed/crates/external_websocket_sync/src/thread_service.rs`):**

Added time-based throttling to all three `EntryUpdated` subscription handlers:

```rust
/// Per-entry throttle state for streaming events.
struct StreamingThrottleState {
    last_sent: Instant,
    pending_content: Option<PendingMessage>,
}

const STREAMING_THROTTLE_INTERVAL: Duration = Duration::from_millis(100);
```

`throttled_send_message_added()`: Sends immediately if interval expired, otherwise stores content as pending. `flush_streaming_throttle()`: Called before every `message_completed` send point to ensure final content isn't lost.

**Three subscription handlers updated:**
1. Initial thread creation (line ~696)
2. Follow-up message (line ~842)
3. Loaded thread from agent (line ~1029)

**Three flush points added:**
1. Before `message_completed` in initial thread (line ~751)
2. Before `message_completed` in follow-up (line ~893)
3. In `notify_message_completed()` in `external_websocket_sync.rs`

**Note on Zed→Go delta encoding:** NOT implemented in this pass. The throttle alone gives ~90% reduction. Zed still sends full content per message entry (not delta), but only 10 times/sec instead of 100+. The Go side's `computePatch` handles the Go→Frontend delta. When Zed→Go delta is added later, Go can forward patches directly — `computePatch` becomes a pass-through.

### Phase 3 Impact

| Metric | After Phase 2 | After Phase 3 | Reduction |
|--------|---------------|---------------|-----------|
| Zed→Go events/sec | ~100+ | ~10 | 90% |
| Zed→Go wire traffic | 100× O(N) per second | 10× O(N) per second | 90% |
| Go handleMessageAdded calls | ~100/sec | ~10/sec | 90% |

## Combined Impact (All Phases)

For a 1000-token response streaming at 100 tok/sec (10 seconds):

| Metric | Before (all phases) | After (all phases) | Reduction |
|--------|--------------------|--------------------|-----------|
| Zed→Go events | 1000 | ~100 | 90% |
| Go DB queries | 4000 | ~60 | 98.5% |
| Go DB writes | 1000 | ~25 | 97.5% |
| Goroutines spawned | 1000 | 1 | 99.9% |
| Go→Frontend events | 1000 | ~25 | 97.5% |
| Go→Frontend wire per event | O(N) bytes | O(delta) bytes | ~99% |
| React re-render scope | Full session | Streaming component only | Targeted |
| React Query cache mutations | Every 50ms | On completion only | ~100% during streaming |

## CI Fixes

### zed-e2e-test (FIXED)
1. `CGO_ENABLED=0` → `CGO_ENABLED=1` + install `gcc g++ musl-dev` (go-tree-sitter needs CGo)
2. Add `-ldflags '-extldflags "-static"'` for Alpine→Ubuntu binary portability

### arm64 build-zed (PRE-EXISTING, NOT OUR ISSUE)
`zed-builder:ubuntu25` base image doesn't exist on arm64 runner. Unrelated to streaming work.

## Files Modified

### Phase 1
| File | Change |
|------|--------|
| `api/pkg/server/websocket_external_agent_sync.go` | streamingContext, cache, throttle |
| `api/pkg/server/websocket_external_agent_sync_test.go` | 6 tests for cache + throttle |
| `api/pkg/server/server.go` | `streamingContexts` field |
| `api/pkg/server/test_helpers.go` | Initialize `streamingContexts` |
| `.drone.yml` | CGO_ENABLED=1 + static link |

### Phase 2
| File | Change |
|------|--------|
| `api/pkg/types/types.go` | Patch fields on WebsocketEvent |
| `api/pkg/types/enums.go` | `WebsocketEventInteractionPatch` constant |
| `api/pkg/server/websocket_external_agent_sync.go` | `computePatch`, `publishInteractionPatchToFrontend`, `previousContent` tracking |
| `api/pkg/server/websocket_external_agent_sync_test.go` | 6 tests: computePatch (5 cases) + previousContent tracking |
| `frontend/src/types.ts` | `interaction_patch` type + patch fields on IWebsocketEvent |
| `frontend/src/contexts/streaming.tsx` | Patch handler with patchContentRef, RAF batching, skip cache invalidation |

### Phase 3
| File | Change |
|------|--------|
| `zed/crates/external_websocket_sync/src/thread_service.rs` | `STREAMING_THROTTLE`, `throttled_send_message_added`, `flush_streaming_throttle`, 3 handlers updated |
| `zed/crates/external_websocket_sync/src/external_websocket_sync.rs` | Flush in `notify_message_completed` |

## Test Coverage (59 tests)

Phase 1 tests:
- `TestStreamingContextCache_SecondTokenSkipsDBQueries`
- `TestStreamingContextCache_ClearedOnMessageCompleted`
- `TestStreamingContextCache_UserMessageDoesNotUseCache`
- `TestStreamingThrottle_DBWriteAfterInterval`
- `TestStreamingThrottle_DirtyFlushOnMessageCompleted`
- `TestStreamingThrottle_MultiMessageAccumulation`

Phase 2 tests:
- `TestComputePatch_Append` -- fast path: pure append
- `TestComputePatch_BackwardsEdit` -- slow path: tool call status change
- `TestComputePatch_EmptyPrevious` -- first token
- `TestComputePatch_Identical` -- no change
- `TestComputePatch_Truncation` -- content got shorter
- `TestStreamingPatch_PreviousContentTracked` -- previousContent updated through streaming lifecycle

## Implementation Log

- 2026-02-25: Created design doc, started implementation
- 2026-02-25: Phase 1 (Fixes 1-4) implemented and tested
- 2026-02-25: CI fix (CGO_ENABLED=1 + static link in zed-e2e-test)
- 2026-02-25: Phase 2 (patch-based Go→Frontend protocol) implemented and tested
- 2026-02-25: Frontend optimized: patch events skip React Query cache, batch via RAF
- 2026-02-25: Phase 3 (Zed-side throttle, 100ms) implemented across all 3 handlers
- 2026-02-25: All phases complete, 59 tests passing
