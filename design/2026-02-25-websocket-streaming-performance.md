# Fix O(N^2) WebSocket Streaming Performance

**Date:** 2026-02-25
**Status:** In Progress
**PR:** TBD

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

## Phase 1 Fixes (This PR)

### Fix 1: Streaming Context Cache (HIGHEST IMPACT)

Cache `GetSession` result and interaction pointer on first `message_added` for a given helix session. Reuse for all subsequent tokens. Clear on `message_completed`.

```go
type streamingContext struct {
    session       *types.Session
    interaction   *types.Interaction
    lastDBWrite   time.Time
    lastPublish   time.Time
    dirty         bool
    mu            sync.Mutex
}
```

**Saves:** 2 DB round-trips per token. For 1000 tokens -> 2000 fewer DB queries.

**Status:** [ ] Implemented [ ] Tested

### Fix 2: Defer Comment Queries to Completion (HIGH IMPACT)

Move `linkAgentResponseToComment` goroutine and `GetPendingCommentByPlanningSessionID` query to only run on `message_completed`. The comment only needs the final content.

Cache the pending comment lookup once (first token) in the streamingContext -- don't query DB every token.

**Saves:** 2 DB calls + 1 goroutine per token.

**Status:** [ ] Implemented [ ] Tested

### Fix 3: Throttle DB Writes (HIGH IMPACT)

Don't call `UpdateInteraction` on every token. Buffer the latest state in the streamingContext and flush to DB on a timer (every 200ms) or on `message_completed`.

Risk: if API crashes mid-stream, up to 200ms of content is lost. Acceptable -- response is in Zed anyway and `message_completed` will write the final state.

**Saves:** ~80% of UpdateInteraction calls. For 1000 tokens at 10 tok/sec -> ~100 DB writes instead of 1000.

**Status:** [ ] Implemented [ ] Tested

### Fix 4: Throttle Frontend Publishing (MEDIUM IMPACT)

Don't publish to frontend on every token. Coalesce: only publish if last publish was >50ms ago or on `message_completed`.

Frontend already batches to `requestAnimationFrame` (~16ms), so publishing faster than 50ms is wasted work. Each publish currently JSON-marshals the full interaction (O(N) bytes).

**Saves:** ~80% of JSON marshaling + pubsub overhead.

**Status:** [ ] Implemented [ ] Tested

## Impact Estimate (Phase 1)

For a 1000-token response streaming at 20 tok/sec (50 seconds):

| Metric | Before | After Phase 1 | Reduction |
|--------|--------|---------------|-----------|
| DB queries | 4000 | ~600 | 85% |
| DB writes | 1000 | ~250 | 75% |
| Goroutines spawned | 1000 | 1 | 99.9% |
| JSON marshals | 1000 | ~250 | 75% |
| Pubsub events | 1000 | ~250 | 75% |

## Files Modified

| File | Change |
|------|--------|
| `api/pkg/server/websocket_external_agent_sync.go` | Add streamingContext, cache session/interaction, throttle DB writes + pubsub, defer comment queries |
| `api/pkg/server/websocket_external_agent_sync_test.go` | Add tests for all throttle behavior |
| `api/pkg/server/server.go` | Add streamingContexts field to HelixAPIServer |
| `api/pkg/server/test_helpers.go` | Initialize streamingContexts in test server |

## Future Phases

### Phase 2: Patch-Based Protocol Go->Frontend
Compute `changed_offset`, send only from that offset. Handles backwards edits (tool call status changes).

### Phase 3: Zed-Side Throttle + Delta
Throttle Zed events to 100ms. Patch-based Zed->Go protocol. Eliminates O(N^2) entirely.

## CI Fix (Prerequisite)

Drone build `zed-e2e-test` step: `CGO_ENABLED=0` doesn't work with `go-tree-sitter` (CGo dependency). Fix: use `CGO_ENABLED=1` and install `gcc` in the go-builder stage.

## Verification

```bash
# Unit tests
cd api && go test -v -run TestWebSocketSyncSuite ./pkg/server/ -count=1

# Build check
cd api && go build ./pkg/server/

# Manual test: start spectask, monitor API logs for reduced DB calls
```

## Implementation Log

- 2026-02-25: Created design doc, started implementation
