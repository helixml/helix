# Agent Handoff: Kickoff Approach for External Agents

## Current Status & Context

### What Works Right Now
- ✅ External agent sessions create successfully
- ✅ Zed connects to Helix WebSocket and works autonomously
- ✅ Mode switching infrastructure complete (MOONLIGHT_WEB_MODE)
- ✅ First stream connection works (video displays)
- ❌ After page refresh, streaming shows black screen

### The Problem We Were Solving
When browser joins keepalive session:
- Old approach: ICE restart on existing peer → track bindings become paused → no video
- Attempted fix: Recreate peer when browser joins → complex Rust refactoring → stop/restart Moonlight creates new Wolf session

### The Simpler Kickoff Approach (NEW)

**Key Insight:** We don't need persistent Moonlight streaming. We just need Wolf to START the container.

**New Flow:**
1. **Backend creates external agent** → Creates Wolf app
2. **Backend "kickoff" connection** → Opens Moonlight in keepalive mode for 10 seconds
   - This triggers Wolf to start the container (Zed begins working)
   - After 10 seconds: Disconnect cleanly
   - App becomes "resumable" for that Moonlight client in Wolf
3. **Browser connects later** → Frontend does regular Moonlight RESUME
   - Uses same client certificate (same client_id in Wolf)
   - Wolf resumes the app (no new session)
   - Fresh WebRTC negotiation, clean track bindings
   - Video flows!

## Current Codebase State

### Helix (feature/external-agents-hyprland-working)
**Commits:**
- 00a234b8b: Mode switching in wolf_executor_apps.go
- 8daaca705: Mode-aware frontend Stream class
- 10d581b9c: Regenerated OpenAPI with moonlight_web_mode
- bd0d538ca: MODE_SWITCHING_SUMMARY.md

**Key Files:**
- `api/pkg/external-agent/wolf_executor_apps.go`:
  - `connectKeepaliveWebSocketForAppSingle()`: WebSocket to /api/host/stream
  - `connectKeepaliveWebSocketForAppMulti()`: REST POST to /api/streamers
- `api/pkg/server/agent_sandboxes_handlers.go`:
  - `fetchMoonlightWebSessions()`: Mode-aware endpoint query
- `frontend/src/components/external-agent/MoonlightStreamViewer.tsx`:
  - Fetches mode from /api/v1/config
  - Creates Stream with correct mode (join vs peer)

### Moonlight-Web (feature/session-persistence - branch feature/kickoff)
**Current commit:** b3f73d3 - Clean slate with:
- ✅ Dockerfile optimizations (system OpenSSL, debug mode, cache improvements)
- ✅ Unique certificate generation per session
- ✅ /api/sessions endpoint
- ✅ Keepalive mode support

**Working branches:**
- `feature/session-persistence`: Session persistence + keepalive
- `feature/kickoff`: Clean slate for new approach (b3f73d3)
- `feature/recreate-peer-on-join`: Complex refactor attempt (parked)

## Implementation Plan for Kickoff Approach

### Changes Needed in Helix Only

**File:** `api/pkg/external-agent/wolf_executor_apps.go`

**Function:** `connectKeepaliveWebSocketForAppSingle()` (currently lines ~776-930)

**Current behavior:**
- Opens WebSocket to `/api/host/stream`
- Sends AuthenticateAndInit with `mode: keepalive`
- Waits for ConnectionComplete or timeout
- WebSocket stays open until session ends

**New behavior (10-second kickoff):**
```go
func (w *AppWolfExecutor) connectKeepaliveWebSocketForAppSingle(...) error {
    // 1. Connect WebSocket
    conn, _, err := dialer.DialContext(ctx, wsURL, nil)
    
    // 2. Send AuthenticateAndInit (mode: keepalive)
    // Same as current code
    
    // 3. Wait for Wolf container to start (10 seconds)
    time.Sleep(10 * time.Second)
    
    // 4. Disconnect cleanly
    conn.WriteMessage(websocket.CloseMessage, ...)
    conn.Close()
    
    log.Info("Kickoff complete - app is now resumable for this client")
    
    return nil
}
```

**Frontend** (likely NO changes needed):
- MoonlightStreamViewer already uses Stream class in "join" mode
- Stream class already sends AuthenticateAndInit
- Moonlight protocol should automatically RESUME if app is running for that client
- Verify: Check browser console logs for "create" vs "join" vs actual resume logic

### Testing the Approach

**Step 1:** Create external agent session
- Backend creates Wolf app
- Backend does 10-second kickoff
- Container starts, Zed begins working
- Kickoff disconnects

**Step 2:** Toggle "Live Stream" in browser  
- Frontend connects with same session_id ("agent-{sessionId}")
- Should RESUME the app (Wolf already has it running for this client)
- Fresh WebRTC tracks created
- Video should flow!

**Step 3:** Page refresh
- Frontend disconnects, reconnects
- Should RESUME again (app still running)
- Fresh tracks each time
- No black screen!

### Key Questions to Investigate

1. **Does Moonlight protocol auto-resume?**
   - Check: When frontend connects in "join" mode, does start_stream() in moonlight-common use resume if app is running?
   - Location: `moonlight-common/src/high.rs` line ~615
   - Logic: `if current_game == 0 || current_game != app_id { LAUNCH } else { RESUME }`

2. **Does "join" mode trigger resume?**
   - Check: moonlight-web Stream class "join" mode behavior
   - Does it reuse existing session or create new one?

3. **Certificate reuse:**
   - Same session_id = same generated certificate
   - Same certificate = same client_id in Wolf
   - Same client_id = can RESUME apps for that client

### Files to Reference

**Moonlight protocol:**
- `/home/luke/pm/moonlight-web-stream/moonlight-common/src/network/launch.rs`
  - `host_launch()`: Start new app
  - `host_resume()`: Resume existing app
- `/home/luke/pm/moonlight-web-stream/moonlight-common/src/high.rs`
  - `start_stream()`: Auto-selects launch vs resume

**Current keepalive implementation:**
- `/home/luke/pm/helix/api/pkg/external-agent/wolf_executor_apps.go:776-930`
  - WebSocket connection logic
  - AuthenticateAndInit message format

**Frontend streaming:**
- `/home/luke/pm/helix/frontend/src/components/external-agent/MoonlightStreamViewer.tsx`
  - Stream class usage in "join" mode
- `/home/luke/pm/helix/frontend/src/lib/moonlight-web-ts/stream/index.ts`
  - Mode-aware WebSocket endpoint selection
  - AuthenticateAndInit message construction

### Success Criteria

✅ Create external agent → container starts
✅ Wait 10 seconds → kickoff disconnects  
✅ Toggle "Live Stream" → video appears
✅ Page refresh → video still works (no black screen)
✅ Multiple sessions → each has own client_id, independent

### Rollback Plan

If kickoff approach doesn't work:
- `feature/recreate-peer-on-join` has working first-connect
- Can continue that refactor (needs 2-3 more hours)
- Or use multi-mode (set MOONLIGHT_WEB_MODE=multi)

## Environment Configuration

**Current mode:** Single (session-persistence)
```bash
# In docker-compose.dev.yaml line 98:
MOONLIGHT_WEB_MODE=single  # Default

# Moonlight-web branch:
feature/kickoff  # Clean slate for new approach
```

**To test multi-mode:**
```bash
export MOONLIGHT_WEB_MODE=multi
cd ~/pm/moonlight-web-stream && git checkout feat/multi-webrtc
./stack build-moonlight-web
docker compose -f docker-compose.dev.yaml restart api
```

## Quick Start for New Agent

```bash
cd /home/luke/pm/helix

# Current branches:
# - Helix: feature/external-agents-hyprland-working (mode switching done)
# - Moonlight-web: feature/kickoff (clean slate)

# Mode switching already works - test it:
curl http://localhost:8080/api/v1/config | jq '.moonlight_web_mode'  # Returns "single"

# Moonlight-web is on feature/kickoff (b3f73d3):
cd /home/luke/pm/moonlight-web-stream
git branch  # Should show: * feature/kickoff

# Implementation: Just modify one function in Helix
vim api/pkg/external-agent/wolf_executor_apps.go
# Edit connectKeepaliveWebSocketForAppSingle() to disconnect after 10 seconds

# Test:
# 1. Create external agent session
# 2. Wait 10+ seconds (see kickoff disconnect in logs)
# 3. Toggle "Live Stream" in browser
# 4. Verify video appears
# 5. Refresh page
# 6. Verify video still works
```

## Critical Lessons Learned

1. **Mode switching infrastructure works perfectly** - don't touch it
2. **Wolf keeps containers alive** even when Moonlight disconnects
3. **Moonlight RESUME reconnects to existing app** without new session
4. **Track binding issues only happen with ICE restart on same peer**
5. **Fresh peer = fresh tracks = clean bindings**
6. **Certificate determines client_id in Wolf** - reuse cert to resume
7. **First connect works** - problem is ONLY page refresh/reconnect

## Debug Commands

```bash
# Check Wolf sessions:
docker compose -f docker-compose.dev.yaml exec api curl -s --unix-socket /var/run/wolf/wolf.sock http://localhost/api/v1/sessions | jq '.'

# Check moonlight-web sessions:
curl -s 'http://localhost:8081/api/sessions' -H 'Authorization: Bearer helix' | jq '.'

# Check containers still running:
docker ps | grep zed-external

# Watch moonlight-web logs:
docker compose -f docker-compose.dev.yaml logs -f moonlight-web | grep -E "Creating|Keepalive|Resume|Launch"

# Check mode:
curl http://localhost:8080/api/v1/config | jq '.moonlight_web_mode'
```

## Next Steps

1. Implement 10-second kickoff disconnect in `connectKeepaliveWebSocketForAppSingle()`
2. Test that container starts and stays running after disconnect
3. Test that browser can resume the app
4. Test page refresh works
5. Commit and celebrate! 🎉
