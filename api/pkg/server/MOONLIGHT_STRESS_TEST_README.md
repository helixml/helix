# Moonlight Streaming Stress Test Suite

## Overview

Automated stress tests to detect Moonlight streaming reliability issues including:
- Session resume bugs
- Multiple connect/disconnect cycle failures
- WebRTC peer state corruption
- Wolf lobby cleanup issues
- Race conditions in session lifecycle

## Prerequisites

1. **Running dev environment:**
   ```bash
   docker compose -f docker-compose.dev.yaml up -d
   ```

2. **Admin auth token:**
   ```bash
   export ADMIN_TOKEN=$(curl -s http://localhost:8080/api/v1/auth/login \
     -H "Content-Type: application/json" \
     -d '{"username":"luke.marsden@gmail.com","password":"'${ADMIN_USER_PASSWORD}'"}' \
     | jq -r '.token')
   ```

3. **Verify Moonlight monitoring works:**
   ```bash
   curl -s http://localhost:8080/api/v1/moonlight/status \
     -H "Authorization: Bearer $ADMIN_TOKEN" | jq '.'
   ```

## Running Tests

### Quick health check:
```bash
cd api/pkg/server
go test -run TestMoonlightHealthCheck -v
```

### All stress tests (takes ~5 minutes):
```bash
cd api/pkg/server
go test -run TestMoonlight -v -timeout 10m
```

### Individual scenarios:

**Scenario 1: Rapid connect/disconnect**
```bash
go test -run TestMoonlightStressScenario1 -v
```

**Scenario 2: Concurrent multi-session**
```bash
go test -run TestMoonlightStressScenario2 -v
```

**Scenario 3: Service restart chaos** (requires manual Wolf restart)
```bash
go test -run TestMoonlightStressScenario3 -v
```

**Scenario 4: Browser tab simulation**
```bash
go test -run TestMoonlightStressScenario4 -v
```

**Memory leak detection:**
```bash
go test -run TestMoonlightMemoryLeak -v
```

**Concurrent disconnect stress:**
```bash
go test -run TestMoonlightConcurrentDisconnects -v
```

## Interpreting Results

### Success Criteria

**Scenario 1 (Rapid connect/disconnect):**
- ✅ Session count stays < 10 (no unbounded growth)
- ✅ Active WebSockets >= 0 (no negative values)
- ✅ No panics or crashes

**Scenario 2 (Concurrent multi-session):**
- ✅ All 5 sessions start successfully
- ✅ Total sessions >= 5 in moonlight-web
- ✅ Sessions remain stable for 30 seconds
- ✅ No sessions disappear during test

**Scenario 3 (Service restart):**
- ✅ Clear error messages (not silent failures)
- ✅ Graceful degradation
- ✅ Sessions can recover after restart

**Scenario 4 (Browser tabs):**
- ✅ Multiple clients can be cached for same session
- ✅ Only one active WebSocket at a time (kickoff behavior)
- ✅ Can query state without errors

**Memory leak test:**
- ✅ Session growth <= 3 after 5 cycles
- ⚠️ Client count growth > 10 indicates certificate cache leak

**Concurrent disconnect test:**
- ✅ Active WebSockets = 0 after mass disconnect
- ✅ No stuck sessions

### Common Failure Modes

**"PeerDisconnect" errors:**
- Cause: WebRTC negotiation failed
- Check: Moonlight-web logs for SCTP errors
- Fix: May indicate Wolf session not created or networking issue

**"SCTP chunk too short" warnings:**
- Cause: Data corruption in WebRTC data channel
- Impact: Usually non-fatal but indicates instability
- Action: Monitor if frequency increases over time

**Sessions stuck without WebSocket:**
- Cause: Session created but client never connected, or disconnected but not cleaned up
- Check: `has_websocket: false` persisting for >60s
- Fix: Implement session timeout/garbage collection

**Client certificate accumulation:**
- Cause: Every browser tab creates new certificate, never cleaned
- Impact: Unbounded memory growth over days/weeks
- Fix: Add certificate cache expiry (e.g., prune certs unused for 7 days)

## Monitoring During Tests

### Watch Moonlight state in real-time:
```bash
watch -n 2 "curl -s http://localhost:8080/api/v1/moonlight/status \
  -H 'Authorization: Bearer \$ADMIN_TOKEN' | jq '.'"
```

### Watch Wolf lobbies:
```bash
watch -n 2 "docker compose -f docker-compose.dev.yaml exec api \
  curl --unix-socket /var/run/wolf/wolf.sock http://localhost/api/v1/lobbies 2>/dev/null | jq '.data | length'"
```

### Watch moonlight-web logs:
```bash
docker compose -f docker-compose.dev.yaml logs -f moonlight-web | grep -v "H264 Stats"
```

### Watch for SCTP errors:
```bash
docker compose -f docker-compose.dev.yaml logs -f moonlight-web 2>&1 | grep SCTP
```

## Dashboard Monitoring

Navigate to: **Dashboard → Agent Sandboxes tab**

You'll see two monitoring sections:
1. **Wolf Streaming Status** - Wolf apps, lobbies, GPU stats
2. **Moonlight Web Streaming Status** - All clients, sessions, WebSocket state

The Moonlight section shows:
- **Total Clients** - All cached certificates (lifetime accumulation)
- **Total Sessions** - Active + idle streamers
- **Active WebSockets** - Currently connected clients
- **Idle Sessions** - Keepalive/resumable sessions

## Known Issues

1. **Wolf restart clears all apps** - Containers don't auto-re-register
   - Workaround: Restart external agent containers to trigger re-registration
   - Fix needed: Persistent Wolf state or auto-registration on startup

2. **Moonlight-web certificate cache grows unbounded**
   - Each browser tab creates unique client_unique_id
   - Certificates never expire
   - Long-running instances accumulate hundreds of cached certs
   - Fix needed: Add certificate cache expiry policy

3. **"PeerDisconnect" after Wolf restart**
   - Wolf lobbies cleared but moonlight-web sessions persist
   - Mismatch causes streamer to disconnect peer
   - Fix needed: Moonlight-web should detect Wolf state and recreate sessions

## Adding New Test Scenarios

Example template:
```go
func TestMoonlightStressYourScenario(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping stress test in short mode")
    }

    h := newTestHelpers(t)
    ctx := context.Background()

    // 1. Setup: Create sessions
    sessionID, err := h.createExternalAgentSession(ctx, "your-test")
    require.NoError(t, err)
    defer h.stopExternalAgent(ctx, sessionID)

    // 2. Exercise: Do chaotic things
    // ...

    // 3. Verify: Check expected state
    status, err := h.getMoonlightStatus(ctx)
    require.NoError(t, err)
    require.Equal(t, expectedValue, status.SomeField)
}
```

## Future Enhancements

1. **Add WebSocket/WebRTC client simulation** - Actually connect like browser does
2. **Integrate into CI/CD** - Run on every PR
3. **Add metrics collection** - Track failure rates over time
4. **Automated issue creation** - File GitHub issue when test fails
5. **Performance benchmarks** - Measure connection establishment time
6. **Load testing** - Test with 50+ concurrent sessions
