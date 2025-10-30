# Next Steps: Moonlight-Web Mode Switching Implementation

## Context

We have TWO working moonlight-web architectures, each on a different branch:

1. **"single" mode** (feature/session-persistence)
   - Keepalive/Join architecture
   - One WebRTC peer swaps in/out per session
   - Simpler, more reliable for single viewers
   - Uses WebSocket: `GET /host/stream` with SessionMode (Create/Keepalive/Join)

2. **"multi" mode** (feat/multi-webrtc)
   - Streamers API with broadcasting support
   - Multiple browsers view same stream simultaneously
   - More complex, designed for concurrent viewers
   - Uses REST: `POST /api/streamers` then `GET /api/streamers/{id}/peer`

Both branches now have **unique certificate generation** applied, fixing the Wolf client_id collision issue where multiple sessions couldn't run simultaneously.

## Environment Variable Added

`MOONLIGHT_WEB_MODE` - defaults to "single"
- Location: `docker-compose.dev.yaml` line 98
- Values: "single" or "multi"

## TODO: Implement Mode Switching in Helix Backend

### File: `api/pkg/external-agent/wolf_executor_apps.go`

Current state: Line 178 always calls `connectKeepaliveWebSocketForApp()` (multi mode REST API)

**Change needed:**

```go
// Around line 170-180, replace:
err = w.connectKeepaliveWebSocketForApp(ctx, wolfAppID, agent.SessionID, displayWidth, displayHeight, displayRefreshRate)

// With:
moonlightMode := os.Getenv("MOONLIGHT_WEB_MODE")
if moonlightMode == "" {
    moonlightMode = "single" // Default
}

if moonlightMode == "multi" {
    // Use REST API approach (current implementation)
    err = w.connectKeepaliveWebSocketForApp(ctx, wolfAppID, agent.SessionID, displayWidth, displayHeight, displayRefreshRate)
} else {
    // Use WebSocket approach (session-persistence)
    err = w.connectKeepaliveWebSocketSingleMode(ctx, wolfAppID, agent.SessionID, displayWidth, displayHeight, displayRefreshRate)
}
```

**New method needed:** `connectKeepaliveWebSocketSingleMode()`

This should:
1. Establish WebSocket to `ws://moonlight-web:8080/host/stream`
2. Send `AuthenticateAndInit` message with:
   - `credentials`: MOONLIGHT_CREDENTIALS
   - `session_id`: sessionID (for session persistence)
   - `mode`: `SessionMode::Keepalive` (headless, no WebRTC)
   - `client_unique_id`: unique per session
   - `host_id`, `app_id`, stream settings, etc.
3. Handle incoming messages (stages, errors)
4. Keep WebSocket alive for session lifetime

**Reference:** Look at the old WebSocket implementation that was replaced by REST API

## TODO: Implement Mode Switching in Dashboard

### File: `api/pkg/server/agent_sandboxes_handlers.go`

Current state: Line 339 queries `/api/streamers` (multi mode)

**Change needed in `fetchMoonlightWebSessions()`:**

```go
// Around line 338-340, replace:
url := fmt.Sprintf("%s/api/streamers", moonlightWebURL)

// With mode checking:
moonlightMode := os.Getenv("MOONLIGHT_WEB_MODE")
if moonlightMode == "" {
    moonlightMode = "single"
}

var url string
if moonlightMode == "multi" {
    url = fmt.Sprintf("%s/api/streamers", moonlightWebURL)
} else {
    url = fmt.Sprintf("%s/api/sessions", moonlightWebURL)
}
```

**Then update response parsing:**

```go
if moonlightMode == "multi" {
    // Parse streamers response (current code)
    var streamersResponse struct { ... }
} else {
    // Parse sessions response (session-persistence format)
    var sessionsResponse struct {
        Sessions []struct {
            SessionID       string  `json:"session_id"`
            ClientUniqueID  *string `json:"client_unique_id"`
            Mode            string  `json:"mode"`
            HasWebsocket    bool    `json:"has_websocket"`
        } `json:"sessions"`
    }
}
```

## TODO: Build and Test

### To test "single" mode (default):

1. Rebuild moonlight-web with session-persistence branch:
```bash
cd ~/pm/moonlight-web-stream
git checkout feature/session-persistence
cd ~/pm/helix
./stack build-moonlight-web
```

2. Restart API to pick up mode switching:
```bash
docker compose -f docker-compose.dev.yaml restart api
```

3. Create external agent sessions - should use WebSocket approach

### To test "multi" mode:

1. Rebuild moonlight-web with multi-webrtc branch:
```bash
cd ~/pm/moonlight-web-stream
git checkout feat/multi-webrtc
cd ~/pm/helix
./stack build-moonlight-web
```

2. Set mode and restart:
```bash
export MOONLIGHT_WEB_MODE=multi
docker compose -f docker-compose.dev.yaml down api
docker compose -f docker-compose.dev.yaml up -d api
```

3. Create external agent sessions - should use REST API approach

## Critical Discovery: Unique Certificates Fix

**Problem:** Wolf identifies clients by `hash(client_cert)`. When all sessions shared moonlight-web's single pairing cert, they all got the same `client_id`, causing session conflicts.

**Solution:** Each session now:
- Generates fresh certificate with `generate_new_client()`
- Auto-pairs with Wolf using `MOONLIGHT_INTERNAL_PAIRING_PIN`
- Gets unique Wolf `client_id` (different cert hash)
- Can run simultaneously with other sessions

This fix is applied to BOTH branches.

## Known Issues

### Multi Mode (feat/multi-webrtc):
- Streamers appear then disappear from registry
- `/api/streamers` returns empty even though Wolf shows active sessions
- IPC receiver loop may be ending prematurely
- Need to investigate why `recv()` returns None unexpectedly

### Single Mode (feature/session-persistence):
- Not yet integrated with Helix backend (TODO above)
- Needs testing with unique certificates
- Dashboard integration pending

## Session Evidence

From testing "multi" mode before switching:
- 4 Wolf apps created successfully
- 3 active Wolf streaming sessions
- Each with unique client_id (16478842112980170722, 7369857017285278554, 4397935152187044858)
- All at 2560x1600@60fps
- Wolf containers launching and sending initial frames
- Some sessions timeout with "lack of video traffic" after IDR frame issues

The unique certificate fix IS working - multiple sessions CAN coexist with different Wolf client IDs!
