# Wolf Lobby Keepalive Design

## Problem Statement

Wolf lobbies with `stop_when_everyone_leaves=false` (used for persistent external agent sessions) experience a **stale CUDA buffer crash** when:
1. All Moonlight clients disconnect (lobby becomes empty: `connected_sessions.size() == 0`)
2. A new Moonlight client attempts to rejoin the empty lobby

**Root Cause**: Wolf's GStreamer video producer pipeline holds stale GPU buffer references when the lobby becomes empty. On rejoin, these stale buffers cause crashes in the video encoding path.

**Validated Solution**: Keep at least one Moonlight client connected at all times. User testing confirmed: iPad stayed connected → Mac could disconnect/reconnect without any crashes.

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                         Helix API                               │
│                                                                 │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │  WolfExecutor.StartZedAgent()                            │  │
│  │  1. Create Wolf Lobby (container starts)                 │  │
│  │  2. Start keepalive session in goroutine                 │  │
│  │  3. Track keepalive health in ZedSession struct          │  │
│  └──────────────────────────────────────────────────────────┘  │
│                              │                                  │
│                              ▼                                  │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │  startKeepaliveSession(sessionID, lobbyID, PIN)          │  │
│  │  • HTTP POST to moonlight-web API                        │  │
│  │  • Maintain WebSocket connection (or periodic health)    │  │
│  │  • Update keepalive status: starting → active → failed   │  │
│  │  • Handle reconnection if connection drops               │  │
│  └──────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────┘
                              │
                              │ HTTP/WebSocket
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                    moonlight-web Container                      │
│                                                                 │
│  WebSocket Endpoint: /host/stream                              │
│  • Authenticates with MOONLIGHT_INTERNAL_PAIRING_PIN           │
│  • Spawns streamer subprocess                                  │
│  • Streamer connects to Wolf lobby via Moonlight protocol      │
│  • Keeps lobby permanently occupied                            │
└─────────────────────────────────────────────────────────────────┘
                              │
                              │ Moonlight Protocol (RTSP/RTP)
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                         Wolf Container                          │
│                                                                 │
│  Lobby: {id: lobby-xxx, connected_sessions: [keepalive, ...]}  │
│  • Keepalive session NEVER disconnects                         │
│  • Human users can connect/disconnect freely                   │
│  • Lobby never becomes empty → no stale buffer crash           │
└─────────────────────────────────────────────────────────────────┘
```

## Implementation Plan

### Phase 1: Basic Keepalive Session (Completed)

✅ Added keepalive tracking to `ZedSession` struct:
```go
type ZedSession struct {
    // ... existing fields ...

    // Keepalive session tracking (prevents stale buffer crash on rejoin)
    KeepaliveStatus    string     `json:"keepalive_status"`               // "active", "starting", "failed", "disabled"
    KeepaliveStartTime *time.Time `json:"keepalive_start_time,omitempty"` // When keepalive was started
    KeepaliveLastCheck *time.Time `json:"keepalive_last_check,omitempty"` // Last health check time
}
```

✅ Modified `StartZedAgent` to launch keepalive goroutine

⏳ **TODO**: Implement actual moonlight-web connection

### Phase 2: Moonlight-Web Integration

**Approach**: Use moonlight-web's existing WebSocket API

**Connection Flow**:
1. HTTP POST to `http://moonlight-web:9090/host/stream` (WebSocket upgrade)
2. Send `AuthenticateAndInit` message with:
   - `credentials`: From moonlight-web config
   - `host_id`: 0 (local Wolf instance)
   - `app_id`: Lobby ID (Wolf lobbies exposed as apps)
   - Stream settings: minimal quality for keepalive
3. Maintain WebSocket connection
4. Handle server messages: `StageComplete`, `HostNotPaired`, etc.

**Configuration**:
- Use environment variable: `MOONLIGHT_WEB_URL=http://moonlight-web:9090`
- Use minimal stream settings (lowest bitrate/quality for keepalive)
- Auto-authenticate with PIN (already supported via `MOONLIGHT_INTERNAL_PAIRING_PIN`)

**Health Monitoring**:
- WebSocket ping/pong for connection health
- Reconnect on disconnect (with exponential backoff)
- Update `KeepaliveLastCheck` on each successful ping
- Update `KeepaliveStatus`:
  - `starting` → connecting to moonlight-web
  - `active` → WebSocket connected, stream running
  - `reconnecting` → connection dropped, attempting reconnect
  - `failed` → max reconnect attempts exceeded

### Phase 3: Restart Resilience

#### 3.1 moonlight-web Restarts (CRITICAL)

**Problem**: When moonlight-web container restarts, all WebSocket connections are lost. Active Helix sessions lose their keepalive sessions → lobbies become vulnerable to stale buffer crash.

**Solution**: Keepalive Reconciliation Loop

```go
// In wolf_executor.go
func (w *WolfExecutor) ReconcileKeepaliveSessions(ctx context.Context) {
    w.mutex.RLock()
    defer w.mutex.RUnlock()

    for sessionID, session := range w.sessions {
        // Check if keepalive is missing or failed
        if session.KeepaliveStatus == "failed" || session.KeepaliveStatus == "" {
            log.Warn().
                Str("session_id", sessionID).
                Str("lobby_id", session.WolfLobbyID).
                Msg("Keepalive session missing or failed, restarting")

            // Restart keepalive
            go w.startKeepaliveSession(ctx, sessionID, session.WolfLobbyID, lobbyPIN)
        }
    }
}
```

**Trigger Mechanisms**:
1. **Periodic reconciliation**: Run every 30 seconds
2. **Event-driven**: Listen for moonlight-web health check failures
3. **On-demand**: API endpoint to force reconciliation

**Implementation**:
- Add to API startup: `go api.wolfExecutor.ReconcileKeepaliveLoop(ctx)`
- Runs continuously in background
- Checks all active sessions for keepalive health
- Restarts failed keepalive sessions automatically

#### 3.2 Wolf Restarts (Future - Currently Out of Scope)

**Current Behavior**: Wolf restart wipes all lobbies → all sessions lost

**Future Design** (not implemented now):
- Helix would need to persist lobby state in database
- On Wolf restart, Helix reconciles lobbies:
  1. List all active Helix sessions from DB
  2. Recreate lobbies in Wolf
  3. Restart containers
  4. Re-establish keepalive sessions

**Design Note**: This requires lobby persistence, which is a major architectural change. For now, Wolf restarts are considered catastrophic failures requiring manual intervention.

#### 3.3 Wolf Crash Detection and Auto-Restart (CRITICAL)

**Problem**: If keepalive mechanism fails and Wolf crashes due to stale buffer error, the system needs to recover automatically.

**Solution**: Wolf Health Monitoring + Auto-Restart

**Detection Strategy**:
1. **Stderr monitoring**: Watch for specific crash patterns in Wolf logs:
   - `gst_mini_object_unref: assertion failed`
   - `CUDA buffer error`
   - `Segmentation fault in video producer`

2. **Heartbeat monitoring**: Wolf responds to `/api/v1/health` endpoint
   - Helix pings every 5 seconds
   - If 3 consecutive failures → Wolf considered crashed

3. **Exit code monitoring**: Docker captures Wolf exit codes
   - Exit code 139 = Segmentation fault
   - Exit code 134 = Abort signal

**Auto-Restart Mechanism**:

**Option A: Docker Restart Policy** (Simplest)
```yaml
# docker-compose.dev.yaml
services:
  wolf:
    restart: unless-stopped  # Already configured
    # Docker automatically restarts on crash
```

**Option B: Helix-Controlled Restart** (More control)
```go
// In api/pkg/wolf/health.go
func (w *WolfHealthMonitor) MonitorLoop(ctx context.Context) {
    ticker := time.NewTicker(5 * time.Second)
    defer ticker.Stop()

    for {
        select {
        case <-ticker.C:
            if err := w.checkWolfHealth(); err != nil {
                w.consecutiveFailures++

                if w.consecutiveFailures >= 3 {
                    log.Error().Msg("Wolf health check failed 3 times, restarting container")

                    // Restart Wolf container
                    if err := w.restartWolfContainer(); err != nil {
                        log.Error().Err(err).Msg("Failed to restart Wolf container")
                    }

                    // After restart, reconcile sessions
                    w.executor.ReconcileKeepaliveSessions(ctx)

                    w.consecutiveFailures = 0
                }
            } else {
                w.consecutiveFailures = 0
            }
        case <-ctx.Done():
            return
        }
    }
}

func (w *WolfHealthMonitor) restartWolfContainer() error {
    // Use Docker API to restart Wolf container
    cmd := exec.Command("docker", "compose", "-f", "docker-compose.dev.yaml", "restart", "wolf")
    return cmd.Run()
}
```

**Post-Restart Reconciliation**:
1. Wait for Wolf to be healthy (max 30 seconds)
2. Reconcile all Helix sessions:
   - For each active session, check if lobby exists in Wolf
   - If missing, recreate lobby
   - Restart keepalive session
3. Log reconciliation results

**Crash Prevention Strategy** (Defense in Depth):
- **Primary**: Keepalive sessions prevent lobbies from becoming empty
- **Secondary**: Health monitoring detects Wolf crashes
- **Tertiary**: Auto-restart recovers from crashes
- **Quaternary**: Post-restart reconciliation restores session state

## API Endpoints

### GET /api/v1/sessions/{sessionID}/keepalive
Returns keepalive session health status

**Response**:
```json
{
  "session_id": "ses_123",
  "lobby_id": "lobby-external-agent-ses_123",
  "keepalive_status": "active",
  "keepalive_start_time": "2025-10-10T12:34:56Z",
  "keepalive_last_check": "2025-10-10T12:35:10Z",
  "connection_uptime_seconds": 14
}
```

**Status Values**:
- `starting`: Connecting to moonlight-web
- `active`: Keepalive session running normally
- `reconnecting`: Connection lost, attempting to reconnect
- `failed`: Max reconnect attempts exceeded
- `disabled`: Keepalive not configured for this session

### POST /api/v1/sessions/reconcile-keepalive
Force reconciliation of all keepalive sessions (admin endpoint)

**Response**:
```json
{
  "reconciled_count": 3,
  "failed_count": 0,
  "sessions": [
    {"session_id": "ses_123", "action": "restarted", "success": true},
    {"session_id": "ses_456", "action": "already_active", "success": true}
  ]
}
```

## Frontend UI

### Session Card Enhancement

Add keepalive indicator to session card:

```tsx
<Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
  <Typography variant="body2">Keepalive:</Typography>
  {keepaliveStatus === 'active' && (
    <Chip
      icon={<CheckCircleIcon />}
      label="Active"
      color="success"
      size="small"
    />
  )}
  {keepaliveStatus === 'starting' && (
    <Chip
      icon={<CircularProgress size={16} />}
      label="Starting"
      color="info"
      size="small"
    />
  )}
  {keepaliveStatus === 'failed' && (
    <Chip
      icon={<ErrorIcon />}
      label="Failed"
      color="error"
      size="small"
    />
  )}
</Box>
```

### Admin Dashboard

Add system health panel showing:
- Total active sessions
- Keepalive sessions count
- Failed keepalive count
- Last reconciliation time
- Force reconcile button

## Testing Plan

### Manual Testing

1. **Basic Flow**:
   - Create external agent session
   - Verify keepalive starts automatically
   - Check keepalive status via API
   - Connect with Moonlight client
   - Disconnect Moonlight client
   - Reconnect Moonlight client (should not crash)

2. **Restart Scenarios**:
   - Create session with keepalive active
   - Restart moonlight-web container
   - Verify keepalive auto-recovers
   - Test session still works (no crash on rejoin)

3. **Failure Scenarios**:
   - Kill Wolf container (simulate crash)
   - Verify Docker restarts Wolf automatically
   - Verify Helix reconciles sessions
   - Verify keepalive sessions restored
   - Test all sessions still functional

### Automated Tests

```go
// In api/pkg/external-agent/wolf_executor_test.go

func TestKeepaliveSession(t *testing.T) {
    // Test keepalive starts automatically
    // Test keepalive survives moonlight-web restart
    // Test keepalive handles connection failures
}

func TestKeepaliveReconciliation(t *testing.T) {
    // Test reconciliation detects failed keepalives
    // Test reconciliation restarts failed sessions
}
```

## Rollout Plan

### Phase 1: Core Implementation (This PR)
- ✅ Add keepalive tracking to ZedSession
- ✅ Implement startKeepaliveSession with placeholder
- ⏳ Implement moonlight-web WebSocket connection
- ⏳ Add keepalive status API endpoint

### Phase 2: Resilience (Next PR)
- Implement keepalive reconciliation loop
- Add moonlight-web restart detection
- Add Wolf health monitoring
- Implement auto-restart on crash

### Phase 3: Observability (Next PR)
- Add frontend keepalive indicator
- Add admin dashboard for system health
- Add metrics/logging for debugging

### Phase 4: Future Improvements
- Wolf lobby persistence (for Wolf restart recovery)
- Graceful keepalive shutdown
- Configurable keepalive stream quality
- Multi-region keepalive failover

## Success Criteria

✅ **Primary**: No stale buffer crashes when Moonlight clients disconnect/reconnect
✅ **Secondary**: System auto-recovers from moonlight-web restarts
✅ **Tertiary**: System auto-recovers from Wolf crashes
✅ **Quaternary**: Keepalive status visible in UI

## Known Limitations

1. **Wolf Restart**: Currently catastrophic (lobbies wiped). Future work needed for lobby persistence.

2. **Keepalive Overhead**: Each session has an additional Moonlight connection. Impact:
   - CPU: ~5% per keepalive (low bitrate stream)
   - Memory: ~50MB per keepalive
   - Network: ~100 Kbps per keepalive (minimal quality)

3. **Reconnection Gap**: Brief window (5-10 seconds) during moonlight-web restart where lobbies are unprotected. Mitigation: Fast reconciliation loop (30 second interval).

4. **Single Point of Failure**: If both moonlight-web AND Wolf crash simultaneously, recovery requires manual intervention.

## Future Enhancements

1. **Lobby Persistence**: Store lobby state in database for Wolf restart recovery
2. **Multi-Region Keepalive**: Distribute keepalive sessions across multiple moonlight-web instances for redundancy
3. **Adaptive Quality**: Dynamically adjust keepalive stream quality based on system load
4. **Graceful Shutdown**: Clean keepalive session termination when session ends
5. **Keepalive Pooling**: Reuse keepalive connections across multiple sessions (optimization)

---

**Status**: Phase 1 in progress
**Last Updated**: 2025-10-10
**Author**: Claude Code
**Reviewers**: Luke Marsden
