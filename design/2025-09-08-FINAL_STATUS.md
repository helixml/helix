# Final Status - Moonlight Web Streaming Investigation

## What's Working

1. ✅ **Screenshots**: Auto-discover Wayland sockets, working perfectly
2. ✅ **Zed Bidirectional Messaging**: Complete, tested, working
3. ✅ **UDP Routing in Bridge Mode**: Manual test confirmed packets reach Wolf
4. ✅ **Native Moonlight Clients**: macOS, iPad, iOS, Linux all work perfectly
5. ✅ **Wolf Static IP**: 172.19.0.50 configured and stable

## The Problem

**moonlight-web has NEVER successfully streamed from Wolf**, despite:
- RTSP handshake completing
- WebRTC connection establishing
- All network routes being verified

## Root Cause Discovery

### Network Mode Analysis

**Bridge Mode** (Both containers on Docker network):
- ✅ UDP routing works (manual `nc -u` test successful)
- ✅ Wolf received test ping from `172.19.0.15:56173`
- ❌ Streamer binary never sends pings during actual streaming
- **Conclusion**: Network is fine, issue is in streamer code/lifecycle

**Host Mode** (moonlight-web on host network):
- ✅ UDP pings reach Wolf (from `172.19.0.1` - gateway)
- ✅ Wolf starts video pipeline
- ❌ Return path broken - Wolf sends to `172.19.0.1`, never reaches streamer
- **Conclusion**: Fundamental NAT routing issue, not viable

### Socket Binding Analysis

Found critical code in `PlatformSockets.c:300-308`:

```c
if (localAddr && localAddr->ss_family != 0) {
    memcpy(&bindAddr, localAddr, addrLen);  // Bind to specific IP
    SET_PORT(&bindAddr, 0);                  // Random port
}
```

The `LocalAddr` comes from `getsockname()` on the RTSP TCP connection (RtspConnection.c:481-490).

**In bridge mode**: `getsockname()` should return `172.19.0.15:random` → Perfect!
**In host mode**: `getsockname()` returns host IP → NAT problems

## Key Insight

**Bridge mode networking is correct**. The issue is the streamer binary fails to send UDP pings for an unknown reason.

Possibilities:
1. Streamer crashes after RTSP before ping loop
2. UDP bind fails silently
3. sendto() fails without logging
4. Moonlight-common incompatibility with Wolf's protocol implementation

## Files Modified for Investigation

1. **docker-compose.dev.yaml**:
   - Wolf: static IP `172.19.0.50`, `WOLF_LOG_LEVEL=TRACE`
   - moonlight-web: Back to bridge mode

2. **moonlight-web-stream/Dockerfile**:
   - Added `ENV RUST_LOG=moonlight_common=trace,moonlight_web=trace`
   - Needs rebuild to take effect

3. **api/cmd/screenshot-server/main.go**:
   - Auto-discovers Wayland sockets
   - Caches working socket

4. **helix-sway:latest**: Rebuilt with fixed screenshot server

## Next Steps for User Testing

### Option 1: Test With Trace Logging (Bridge Mode)

After I rebuild moonlight-web with RUST_LOG:

```bash
# Rebuild moonlight-web
cd /home/luke/pm/moonlight-web-stream
docker build -t helix-moonlight-web -f Dockerfile .

# Restart
cd /home/luke/pm/helix
docker compose -f docker-compose.dev.yaml down moonlight-web
docker compose -f docker-compose.dev.yaml up -d moonlight-web

# Test streaming
# Create session, click "Live Stream"

# Watch for trace logs:
docker compose -f docker-compose.dev.yaml logs -f moonlight-web 2>&1 | grep -E "trace|TRACE|bind|sendto|RemoteAddr|LocalAddr"
```

Expected logs will show:
- What IP "wolf" resolves to
- What local IP UDP socket binds to
- Whether sendto() succeeds or fails
- Any errors in between

### Option 2: Test From External Machine (Recommended!)

This bypasses all Docker complexities:

```bash
# On your laptop/iPad:
# 1. Open browser to http://node01.lukemarsden.net:8080
# 2. Create external agent session
# 3. Click "Live Stream"
```

This tests if moonlight-web → Wolf works when NOT complicated by Docker networking.

If this works → Docker networking issue confirmed
If this fails → moonlight-web/Wolf incompatibility

### Option 3: Use Native Moonlight Client (Already Working)

You mentioned macOS/iPad/iOS/Linux native clients all work. These prove Wolf streaming is solid. The web client is just for convenience.

## My Hypothesis

I believe the streamer binary in bridge mode either:
1. **Encounters a bind() error** when trying to bind UDP socket to the container IP
2. **Crashes silently** after RTSP before starting ping thread
3. **Has a bug in moonlight-common** that only manifests in container environments

The trace logging will reveal which one.

## Quick Test Commands

```bash
# Check if streamer process is running during streaming
docker exec helix-moonlight-web-1 sh -c 'ls -la /proc/*/cmdline 2>/dev/null | xargs -I {} sh -c "cat {} 2>/dev/null | tr \"\\0\" \" \"; echo"' | grep streamer

# Check streamer stderr for errors
docker compose -f docker-compose.dev.yaml logs moonlight-web | grep -A5 "failed\|error\|Error"

# Verify UDP manual test still works
docker exec helix-moonlight-web-1 bash -c 'echo "TEST" | nc -u 172.19.0.50 48100'
docker compose -f docker-compose.dev.yaml logs wolf --tail 10 | grep "Received ping"
```

## Files Created for Reference

1. `FINAL_STATUS.md` - This file
2. `ROOT_CAUSE_ANALYSIS.md` - Socket binding analysis
3. `STREAMING_DEBUG_SUMMARY.md` - Complete technical deep dive
4. `BREAKTHROUGH.md` - Manual UDP test success
5. `TEST_MOONLIGHT_WEB.md` - Testing procedure
6. `WHEN_YOU_RETURN.md` - Quick status overview

## Current Environment

- Wolf: `172.19.0.50`, TRACE logging, ready
- moonlight-web: Bridge mode, needs rebuild with RUST_LOG
- Both: helix_default Docker network
- Screenshots: Working
- Zed messaging: Working

Test in the morning with either:
- External machine (easiest to validate)
- Bridge mode with trace logging (will show exact failure point)
