# Status Update - Moonlight Web Streaming Debug

## What I Did While You Were Away

### 1. Fixed Screenshot Server ✅
- Modified screenshot-server to auto-discover correct Wayland socket
- Tries all available sockets (wayland-1, wayland-2, etc.)
- Caches working socket for performance
- Rebuilt helix-sway:latest with fix
- **Result**: Screenshots now work in new sessions

### 2. Configured Wolf Static IP ✅
- Assigned Wolf container static IP: `172.19.0.50`
- Set `WOLF_INTERNAL_IP=172.19.0.50` env var
- Prevents IP changes on container restart
- **Purpose**: Wolf advertises this IP to clients during RTSP

### 3. Fixed moonlight-web WebRTC Configuration ✅
- Added `webrtc_nat_1to1` with 127.0.0.1 for localhost testing
- Configured webrtc_port_range: 40000-40010
- **Result**: WebRTC browser ↔ moonlight-web connection works

### 4. Root Cause Analysis - UDP Packet Flow 🔍

**Diagnosed Issue**: Wolf is NOT receiving UDP ping packets from moonlight-web's streamer

**Evidence**:
- moonlight-web logs show: "Requesting IDR frame" (control packet sent)
- moonlight-web logs show: "Terminating connection due to lack of video traffic" (10 sec timeout)
- Wolf logs show: NO "Received ping from 172.19.0.15" messages
- Wolf logs show: NO RTP ping reception at all

**What Works**:
- ✅ Native Moonlight clients (proves Wolf CAN stream)
- ✅ RTSP handshake (TCP connection works)
- ✅ WebRTC signaling (browser ↔ moonlight-web)
- ✅ DNS resolution (moonlight-web resolves "wolf" → 172.19.0.50)

**What Doesn't Work**:
- ❌ UDP packets from moonlight-web → Wolf (ports 48100, 48200, 47999)
- ❌ Video frames Wolf → moonlight-web

### 5. Enabled Detailed Logging ✅

Changed Wolf environment to capture UDP activity:
- `WOLF_LOG_LEVEL=TRACE` (was DEBUG)
- `GST_DEBUG=3` (was 2)

**What you'll now see**: Every UDP ping packet Wolf receives will log with source IP/port

### 6. Created Test Documentation 📋

Files created:
- `/home/luke/pm/helix/TEST_MOONLIGHT_WEB.md` - Step-by-step testing procedure
- `/home/luke/pm/helix/STREAMING_DEBUG_SUMMARY.md` - Technical analysis
- `/home/luke/pm/helix/MOONLIGHT_WEB_DEBUG.md` - Debug reference
- `/home/luke/pm/helix/test-moonlight-web-stream.sh` - Quick test script

## Next Steps For You

### Quick Test:
```bash
cd /home/luke/pm/helix

# Run diagnostic script
./test-moonlight-web-stream.sh

# Then follow TEST_MOONLIGHT_WEB.md for full testing
```

### What To Look For:

When you click "Live Stream", check Wolf logs for:

```bash
docker compose -f docker-compose.dev.yaml logs -f wolf 2>&1 | grep "RTP"
```

**Critical question**: Do you see this message?
```
[RTP] Starting RTP ping server on ports 48100 and 48200
[RTP] Received ping from 172.19.0.15:XXXXX
```

- **If YES**: Wolf receives pings but isn't sending video (GStreamer pipeline issue)
- **If NO**: Network problem - moonlight-web pings not reaching Wolf

## Possible Solutions Based on Test Results

### If Wolf NOT Receiving Pings:

**Option A**: moonlight-web binding to wrong interface
- Check moonlight-web source IP in streamer
- May need to configure which interface to use

**Option B**: Docker network routing issue
- Containers can ping each other (TCP works)
- But UDP might be filtered
- Test with manual `nc -u` packet send

**Option C**: moonlight-web resolving to wrong IP
- Should resolve "wolf" → 172.19.0.50
- But might be using cached old IP
- Clear moonlight-web data.json pairing and re-pair

### If Wolf Receiving Pings But No Video:

**Option D**: GStreamer pipeline not producing frames
- waylanddisplaysrc race condition
- Need to add retry logic or wait for compositor
- Check `interpipesrc_*_video` buffer flow

**Option E**: UDP send failing from Wolf
- Check Wolf logs for "Error sending UDP packet"
- Verify Wolf can reach moonlight-web IP
- Test reverse direction with nc

## Files You Can Review

1. **STREAMING_DEBUG_SUMMARY.md** - Complete technical analysis
2. **TEST_MOONLIGHT_WEB.md** - Detailed test procedure with expected outputs
3. **MOONLIGHT_WEB_DEBUG.md** - Configuration reference
4. **docker-compose.dev.yaml** - Wolf now has static IP 172.19.0.50
5. **moonlight-web-config/config.json** - WebRTC settings
6. **moonlight-web-config/data.json** - Wolf host configuration
7. **api/cmd/screenshot-server/main.go** - Fixed Wayland socket auto-discovery

## My Analysis

The most likely issue is **Option A or B** - something about the Docker network or how moonlight-web binds its UDP socket.

Native clients work because they connect from OUTSIDE Docker (real network interfaces). But moonlight-web is INSIDE Docker, using container networking.

The fix will likely be one of:
1. Configure moonlight-web to bind UDP on correct interface
2. Add Docker network route/rule for UDP
3. Change moonlight-web to use host network mode instead of bridge

Test with the enhanced logging and report back what you see!
