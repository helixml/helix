# Moonlight Streaming Stress Test Suite

**Date:** 2025-11-08
**Author:** Claude
**Status:** Design Proposal

## Problem Statement

**Lobbies mode is now the de facto standard.** Each external agent gets a Wolf lobby in the shared Wolf UI app.

After Wolf/API/moonlight-web restarts, Moonlight streaming connections in lobbies mode become unreliable. Suspected issues:
- Multiple connect/disconnect cycles cause state corruption
- WebRTC peer state gets stuck
- Wolf lobbies don't properly clean up after disconnects
- Race conditions in session lifecycle management
- Moonlight sessions outlive Wolf lobbies after restart

**Architecture (Lobbies Mode):**
- One Wolf UI App shared across all agents
- Each external agent session = One Wolf lobby in that app
- Each browser tab = Fresh CREATE connection (no resume/keepalive)
- Wolf lobby provides persistence, not Moonlight

**Current observability gap:** No automated testing to reproduce these failure modes reliably.

## Objectives

Create an automated stress test suite that:
1. **Randomly exercises the system** - Connects, disconnects, switches sessions until something breaks
2. **Detects failures automatically** - Clear pass/fail criteria
3. **Reproduces issues reliably** - Same seed → same sequence of operations
4. **Reports detailed diagnostics** - What operation failed, what the state was, logs

## Proposed Architecture

### Test Harness Location

```
api/pkg/server/moonlight_stress_test.go  # Go test using testing.T
```

Why Go tests instead of frontend:
- Direct access to backend APIs (no auth friction)
- Can inspect Wolf/moonlight-web internal state via Unix socket
- Can restart services programmatically
- Better for CI/CD integration

### Test Scenarios

#### Scenario 1: Rapid Connect/Disconnect Cycles
```
1. Start N external agent sessions (N=3-5)
2. For each session, rapidly:
   - Connect Moonlight client
   - Wait random 1-10 seconds
   - Disconnect
   - Wait random 1-5 seconds
   - Repeat 20 times
3. Verify all sessions still functional after chaos
```

**Pass criteria:**
- All sessions can reconnect after test
- No stuck sessions in Wolf
- No zombie moonlight-web streamers

#### Scenario 2: Concurrent Multi-Session Streaming
```
1. Start 5 external agent sessions simultaneously
2. Connect Moonlight to all 5 at once
3. Verify all 5 streams working (client_count in Wolf = 5)
4. Randomly disconnect 2 sessions
5. Verify remaining 3 still stream
6. Reconnect the 2
7. Verify all 5 streaming again
```

**Pass criteria:**
- `active_clients` in Wolf matches expected count
- Each session shows unique `client_unique_id` in moonlight-web
- No "PeerDisconnect" errors
- No SCTP chunk errors

#### Scenario 3: Service Restart Chaos
```
1. Start 3 sessions, connect all
2. Restart Wolf (docker restart)
3. Attempt to reconnect all 3
4. Verify behavior (expected: should fail gracefully, then recreate lobbies)
5. Restart moonlight-web
6. Attempt reconnect
7. Restart API
8. Attempt reconnect
```

**Pass criteria:**
- Clear error messages (not silent failures)
- Graceful degradation
- Sessions recover after containers restart
- No permanent stuck state

#### Scenario 4: Multiple Browser Tabs to Same Lobby
```
1. Simulate 3 browser tabs (different FRONTEND_INSTANCE_IDs) connecting to same lobby
2. Each creates fresh Moonlight session (CREATE mode)
3. Verify only one Wolf lobby exists (all connect to same lobby)
4. Verify 3 separate Moonlight sessions (one per tab/client)
5. Close 2 tabs
6. Verify 1 Moonlight session remains active
7. Verify Wolf lobby still exists
```

**Pass criteria:**
- Each tab creates unique client_unique_id
- All sessions use CREATE mode (not join/keepalive)
- Multiple Moonlight sessions can stream same Wolf lobby concurrently
- Wolf lobby persists when individual clients disconnect

### Test Implementation

```go
// api/pkg/server/moonlight_stress_test.go
package server_test

import (
    "context"
    "fmt"
    "math/rand"
    "sync"
    "testing"
    "time"

    "github.com/helixml/helix/api/pkg/external-agent"
    "github.com/stretchr/testify/require"
)

// Test requires running dev environment (docker-compose.dev.yaml)
func TestMoonlightStressRapidConnectDisconnect(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping stress test in short mode")
    }

    ctx := context.Background()

    // Setup: Create 3 external agent sessions
    sessions := createTestSessions(t, ctx, 3)
    defer cleanupTestSessions(t, ctx, sessions)

    // Stress test: Rapid connect/disconnect cycles
    var wg sync.WaitGroup
    errors := make(chan error, 100)

    for _, session := range sessions {
        wg.Add(1)
        go func(sessionID string) {
            defer wg.Done()

            for cycle := 0; cycle < 20; cycle++ {
                // Connect
                clientID := fmt.Sprintf("stress-test-%s-%d", sessionID, cycle)
                err := connectMoonlightClient(ctx, sessionID, clientID)
                if err != nil {
                    errors <- fmt.Errorf("connect failed (cycle %d): %w", cycle, err)
                    return
                }

                // Random active time (1-10s)
                time.Sleep(time.Duration(rand.Intn(9)+1) * time.Second)

                // Disconnect
                err = disconnectMoonlightClient(ctx, sessionID, clientID)
                if err != nil {
                    errors <- fmt.Errorf("disconnect failed (cycle %d): %w", cycle, err)
                    return
                }

                // Random idle time (1-5s)
                time.Sleep(time.Duration(rand.Intn(4)+1) * time.Second)
            }
        }(session)
    }

    wg.Wait()
    close(errors)

    // Collect all errors
    var allErrors []error
    for err := range errors {
        allErrors = append(allErrors, err)
    }

    // Verify final state
    finalState, err := getMoonlightWebState(ctx)
    require.NoError(t, err, "Failed to fetch final state")

    // Assert no errors during chaos
    require.Empty(t, allErrors, "Errors occurred during stress test: %v", allErrors)

    // Assert clean final state
    require.Equal(t, 0, len(finalState.Sessions), "Expected 0 active sessions after cleanup")
}

// Helper functions (to be implemented)
func createTestSessions(t *testing.T, ctx context.Context, count int) []string {
    // Create external agent sessions via API
    // Return session IDs
    panic("TODO: Implement")
}

func cleanupTestSessions(t *testing.T, ctx context.Context, sessions []string) {
    // Stop and remove external agent containers
    panic("TODO: Implement")
}

func connectMoonlightClient(ctx context.Context, sessionID, clientID string) error {
    // Simulate browser connecting to moonlight stream
    // Use WebSocket + WebRTC like the frontend does
    panic("TODO: Implement")
}

func disconnectMoonlightClient(ctx context.Context, sessionID, clientID string) error {
    // Close WebSocket cleanly
    panic("TODO: Implement")
}

func getMoonlightWebState(ctx context.Context) (*MoonlightState, error) {
    // Query http://moonlight-web:8080/api/sessions
    panic("TODO: Implement")
}
```

### Observability Enhancements

**Already implemented:**
- ✅ `/api/v1/moonlight/status` endpoint exposes moonlight-web sessions
- ✅ Frontend dashboard in Agent Sandboxes tab shows real-time state

**Additional enhancements needed:**
1. **Moonlight-web metrics endpoint** - Add `/api/metrics` with:
   - Total sessions created (lifetime counter)
   - Failed connection attempts
   - Average session duration
   - WebRTC negotiation failures
   - SCTP packet errors count

2. **Wolf session lifecycle events** - Log when:
   - Lobby created/destroyed
   - Client connected/disconnected
   - Streamer started/stopped
   - Session cancelled vs cleanly closed

3. **Frontend real-time alerts** - Show toast notifications for:
   - Moonlight connection failures
   - SCTP packet errors (indicates corruption)
   - Session stuck in "connecting" for >10s

### Failure Detection

**Clear failure modes to detect:**

1. **Stuck sessions** - Session exists in moonlight-web but has_websocket=false for >60s
2. **Zombie streamers** - Streamer process running but no active WebSocket
3. **Wolf lobby mismatch** - Lobby exists in Wolf but no matching moonlight-web session
4. **SCTP errors** - Repeated "chunk too short" warnings indicate data corruption
5. **Ice negotiation timeouts** - RTCPeerConnection stuck in "checking" state

**Automated health check:**
```bash
# Run every 5 seconds
curl http://moonlight-web:8080/api/sessions | jq '
  .sessions[] |
  select(.has_websocket == false) |
  "WARNING: Session \(.session_id) has no WebSocket"
'
```

### Implementation Phases

**Phase 1 (MVP):**
- ✅ Basic `/moonlight/status` endpoint
- ✅ Frontend dashboard showing active sessions
- 🔄 Simple Go stress test (Scenario 1: rapid connect/disconnect)

**Phase 2 (Enhanced observability):**
- Moonlight-web `/metrics` endpoint
- Frontend alerts for failures
- Wolf lifecycle event logging

**Phase 3 (Comprehensive testing):**
- All 4 test scenarios implemented
- CI/CD integration (run on every PR)
- Automated issue reporting (GitHub issue created on failure)

## Success Metrics

Test suite is successful when:
1. **Reliably reproduces issues** - If run 10 times, catches the bug at least 8 times
2. **Clear failure reports** - Test output shows exactly what operation failed and system state
3. **Fast feedback** - Completes in <5 minutes for rapid iteration
4. **CI-friendly** - Can run in GitHub Actions without manual intervention

## Open Questions

1. **WebSocket connection limits** - Is there a browser limit on concurrent WebSockets to same origin?
2. **WebRTC peer limits** - Does browser cap concurrent RTCPeerConnections?
3. **Wolf app vs lobbies** - Should concurrent streams use separate Wolf apps or lobbies?
4. **Moonlight-web session persistence** - How long should sessions persist without WebSocket?

## Next Steps

1. Implement simple stress test for Scenario 1
2. Run against current codebase to baseline failure rate
3. Fix identified issues
4. Add remaining test scenarios
5. Integrate into CI/CD pipeline

## References

- Moonlight-web source: `~/pm/moonlight-web-stream/`
- Wolf executor: `api/pkg/external-agent/wolf_executor.go`
- Frontend stream viewer: `frontend/src/components/external-agent/MoonlightStreamViewer.tsx`
