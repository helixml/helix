# Fix O(N^2) WebSocket Streaming Performance

**Date:** 2026-02-25
**Status:** Phase 1 COMPLETE, Phase 2-3 not started
**Branch:** `fix/golden-cache-investigation`

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

All changes in `api/pkg/server/websocket_external_agent_sync.go`. 52 tests passing.

### Fix 1: Streaming Context Cache (HIGHEST IMPACT) -- DONE

Cache `GetSession` + `ListInteractions` on first token, reuse for all subsequent. Clear on `message_completed`.

```go
type streamingContext struct {
    session     *types.Session
    interaction *types.Interaction
    lastDBWrite time.Time   // Fix 3
    lastPublish time.Time   // Fix 4
    dirty       bool        // Fix 3
    mu          sync.Mutex
}
```

Key methods:
- `getOrCreateStreamingContext()` -- cache miss = DB query, cache hit = return pointer
- `flushAndClearStreamingContext()` -- flush dirty state, delete cache entry

**Saves:** 2 DB round-trips per token.

### Fix 2: Defer Comment Queries to Completion -- DONE

Removed from streaming path entirely (was part of Fix 1 refactor). `linkAgentResponseToComment` and `GetPendingCommentByPlanningSessionID` only run in `handleMessageCompleted`.

**Saves:** 2 DB calls + 1 goroutine per token.

### Fix 3: Throttle DB Writes (200ms) -- DONE

`UpdateInteraction` only when `time.Since(lastDBWrite) >= 200ms`. In-memory interaction always has latest content. `flushAndClearStreamingContext()` ensures dirty state is written before `handleMessageCompleted` marks complete.

Constants: `dbWriteInterval = 200ms`, `publishInterval = 50ms`.

**Saves:** ~75% of DB writes.

### Fix 4: Throttle Frontend Publishing (50ms) -- DONE

`publishInteractionUpdateToFrontend` only when `time.Since(lastPublish) >= 50ms`. `handleMessageCompleted` always publishes (no throttle on completion).

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

## Phase 2: Patch-Based Protocol Go->Frontend (NOT STARTED)

### Goal
Reduce per-update wire traffic from O(N) to O(delta). Currently each pubsub event marshals the full interaction JSON.

### Approach
Track previous content in streamingContext, compute `changed_offset` (first byte that differs), send only from that offset:

```go
type WebsocketEvent struct {
    // ... existing fields ...
    Patch       string `json:"patch,omitempty"`        // content from changed_offset
    PatchOffset int    `json:"patch_offset,omitempty"` // byte position of first change
    TotalLength int    `json:"total_length,omitempty"` // final content length
}
```

Frontend: `content = content[:patch_offset] + patch`. Handles both appends (streaming) and backwards edits (tool call "Running" -> "Finished").

### CRITICAL: React rendering concern
Luke flagged: deltas MUST NOT trigger full session re-renders. Need to audit:
- `frontend/src/contexts/streaming.tsx` -- how `useLiveInteraction` applies updates
- Whether interaction patches can be applied as targeted state updates to just `ResponseMessage`
- React Query cache invalidation -- patch events must NOT invalidate the full session query
- Verify the JS main loop isn't the bottleneck (React re-rendering everything despite receiving deltas)

### Files to modify
- `api/pkg/types/types.go` -- add patch fields to WebsocketEvent
- `api/pkg/server/websocket_external_agent_sync.go` -- compute patch from previous content
- `frontend/src/contexts/streaming.tsx` -- handle patch events

## Phase 3: Zed-Side Throttle + Delta (NOT STARTED)

### Goal
Reduce Zed->Go wire traffic. Currently Zed sends full content on every token (~100+ events/sec).

### Approach
1. **Throttle** in `thread_service.rs`: only send `message_added` every 100ms per message_id. Always send on completion.
2. **Delta protocol**: track `last_sent_content` per message_id, compute `changed_offset`, send patch.

### Files to modify (Rust + Go)
- `zed/crates/external_websocket_sync/src/thread_service.rs` -- throttle + patch computation
- `zed/crates/external_websocket_sync/src/types.rs` -- add patch fields to MessageAdded
- `api/pkg/server/websocket_external_agent_sync.go` -- handle patches in handleMessageAdded
- `api/pkg/server/wsprotocol/accumulator.go` -- add ApplyPatch method

### Note on existing offset tracking
`LastZedMessageOffset` / `LastZedMessageID` in `wsprotocol/accumulator.go` handle **multi-message accumulation** (switching between message_id entries: text -> tool call -> text). This is NOT delta encoding -- Zed still sends full content per message entry. The O(N^2) comes from full-content-per-token.

## CI Fixes

### zed-e2e-test (FIXED)
1. `CGO_ENABLED=0` -> `CGO_ENABLED=1` + install `gcc g++ musl-dev` (go-tree-sitter needs CGo)
2. Add `-ldflags '-extldflags "-static"'` for Alpine->Ubuntu binary portability

### arm64 build-zed (PRE-EXISTING, NOT OUR ISSUE)
`zed-builder:ubuntu25` base image doesn't exist on arm64 runner. Unrelated to streaming work.

## Files Modified (Phase 1)

| File | Change |
|------|--------|
| `api/pkg/server/websocket_external_agent_sync.go` | streamingContext type, getOrCreateStreamingContext, flushAndClearStreamingContext, throttled handleMessageAdded |
| `api/pkg/server/websocket_external_agent_sync_test.go` | 6 new tests for cache + throttle behavior |
| `api/pkg/server/server.go` | `streamingContexts` field + `streamingContextsMu` on HelixAPIServer |
| `api/pkg/server/test_helpers.go` | Initialize `streamingContexts` in test server |
| `.drone.yml` | CGO_ENABLED=1 + static link in zed-e2e-test |

## Test Coverage (52 tests)

New tests:
- `TestStreamingContextCache_SecondTokenSkipsDBQueries` -- cache hit skips GetSession + ListInteractions
- `TestStreamingContextCache_ClearedOnMessageCompleted` -- cache cleared after completion
- `TestStreamingContextCache_UserMessageDoesNotUseCache` -- user messages bypass cache
- `TestStreamingThrottle_DBWriteAfterInterval` -- 200ms throttle + interval-triggered flush
- `TestStreamingThrottle_DirtyFlushOnMessageCompleted` -- dirty state flushed before completion
- `TestStreamingThrottle_MultiMessageAccumulation` -- multi-message_id + tool call status changes

## Commits (on fix/golden-cache-investigation)

1. `f9c6d09` -- fix: enable CGO in zed-e2e-test Drone step + design doc
2. `c39ea3e` -- perf: streaming context cache (Fix 1)
3. `757765c` -- perf: throttle DB writes + frontend publishes (Fixes 3+4)
4. `b72ff09` -- docs: update design doc
5. `3dce9be` -- fix: static link zed-e2e-test binary

## Implementation Log

- 2026-02-25: Created design doc, started implementation
- 2026-02-25: Fix 1 (streaming context cache) implemented and tested
- 2026-02-25: Fix 2 (defer comment queries) done as part of Fix 1
- 2026-02-25: Fixes 3+4 (throttle DB writes + frontend publishes) implemented and tested
- 2026-02-25: CI fix (CGO_ENABLED=1 + static link in zed-e2e-test)
- 2026-02-25: All Phase 1 complete, 52 tests passing, pushed to remote
