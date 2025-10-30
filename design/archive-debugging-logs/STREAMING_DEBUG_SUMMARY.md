# Moonlight Web Streaming - Root Cause Analysis

## Executive Summary

**moonlight-web streamer CAN'T receive video frames from Wolf despite successful RTSP negotiation.**

## Confirmed Working Components

1. ✅ **WebRTC (Browser ↔ moonlight-web)**: ICE negotiation succeeds, peer connection "connected"
2. ✅ **RTSP Handshake (moonlight-web ↔ Wolf)**: Completes successfully, ports negotiated
3. ✅ **DNS Resolution**: moonlight-web resolves "wolf" → 172.19.0.50 correctly
4. ✅ **UDP Ping Sending**: moonlight-common C code sends pings every 500ms (lines 74-80 in VideoStream.c)
5. ✅ **Native Moonlight Clients**: Work perfectly with same Wolf instance
6. ✅ **Docker Network**: Both containers on helix_default (172.19.0.0/16)

## The Problem

**moonlight-web logs**: `"Terminating connection due to lack of video traffic"` (after 10 seconds)

This comes from VideoStream.c:92-94 - the streamer waits 10 seconds for video packets, receives NONE, then terminates.

## UDP Packet Flow Analysis

### What SHOULD Happen:
1. RTSP negotiation → Wolf tells streamer "send to port 48100 for video"
2. moonlight-web streamer binds local UDP socket
3. Streamer sends UDP pings to `172.19.0.50:48100` every 500ms
4. Wolf receives pings, learns client IP/port from source address
5. Wolf sends video RTP packets to streamer's IP/port
6. Streamer receives packets, decodes, sends to browser via WebRTC

### What's Actually Happening:
1. ✅ RTSP negotiation completes
2. ✅ Streamer binds local UDP socket
3. ✅ Streamer sends pings (code confirmed in VideoStream.c:74)
4. ❓ Wolf receives pings? **NO EVIDENCE IN LOGS**
5. ❌ Wolf never sends video
6. ❌ Streamer times out after 10 seconds

## Key Evidence

### Wolf Logs:
- **NO** `[RTP] Received ping from 172.19.0.15:*` messages
- **NO** `client_ip` logs with 172.19.0.15
- **NO** UDP traffic visible in Wolf logs
- GStreamer pipeline created successfully
- waylanddisplaysrc connects to wayland-2 compositor
- interpipe sinks/sources set up correctly

### moonlight-web Logs:
- Streamer process spawns successfully
- RTSP completes: "Video port: 48100, Audio: 48200, Control: 47999"
- WebRTC peer connection: "connected"
- Continuous "Requesting IDR frame" messages
- After 10 sec: "Terminating connection due to lack of video traffic"

### Network Configuration:
- Wolf static IP: `172.19.0.50` (via docker-compose.dev.yaml)
- moonlight-web IP: `172.19.0.15` (dynamic)
- `WOLF_INTERNAL_IP=172.19.0.50` (for RTSP advertisement)
- Both on same network: `helix_default`
- Port exposure: Wolf UDP ports 47999/48100/48200 exposed to HOST (not needed for container-to-container)

## Root Cause Hypotheses

### Hypothesis 1: Wolf RTP ping server not listening yet (MOST LIKELY)
**Evidence**:
- `docker exec helix-wolf-1 ss -ulnp` shows NO listeners on 48100/48200
- Wolf only binds UDP ports when it fires `RTPVideoPingEvent`
- That event fires when RTSP SETUP is received

**Test**: Check Wolf logs for "Starting RTP ping server on ports" message

### Hypothesis 2: moonlight-web using wrong source IP for UDP
**Evidence**: moonlight-web has multiple network interfaces
**Test**: Capture tcpdump on Wolf to see if packets arrive from unexpected source

### Hypothesis 3: Docker network filtering UDP between containers
**Evidence**: None (TCP works fine)
**Test**: Send manual UDP packet from moonlight-web to Wolf

### Hypothesis 4: Wolf binds to 127.0.0.1 instead of 0.0.0.0
**Evidence**: Code shows `udp::endpoint(udp::v4(), video_port)` which should be 0.0.0.0
**Test**: Check actual socket binding with ss/netstat during active stream

## Debug Commands

### Monitor Wolf RTP Activity:
```bash
docker compose -f docker-compose.dev.yaml logs -f wolf 2>&1 | grep -E "RTP|client_ip|Starting.*ping|Received ping"
```

### Monitor Streamer:
```bash
docker compose -f docker-compose.dev.yaml logs -f moonlight-web | grep -E "Stream|video|IDR"
```

### Capture UDP Traffic:
```bash
# On host (requires tcpdump installed in containers or host capture)
docker exec helix-wolf-1 tcpdump -i any -n 'udp and (port 48100 or port 48200 or port 47999)'
```

### Test Manual UDP Packet:
```bash
# From moonlight-web to Wolf
docker exec helix-moonlight-web-1 bash -c 'echo "PING" | nc -u 172.19.0.50 48100'

# Check Wolf received it
docker compose -f docker-compose.dev.yaml logs wolf | tail -20
```

## Next Steps

1. **Enable verbose logging**:
   - Set `GST_DEBUG=4` in Wolf environment
   - Add trace logging for UDP socket binding

2. **Create fresh streaming session** and capture:
   - Wolf logs showing RTP ping server startup
   - tcpdump of UDP packets between containers
   - Streamer logs showing ping attempts

3. **Verify UDP reachability**:
   - Send manual UDP packet from moonlight-web → Wolf
   - Confirm Wolf receives it

4. **Compare with native client**:
   - Connect native Moonlight client
   - Capture what Wolf logs for RTP ping reception
   - Compare with moonlight-web behavior

## GStreamer Refcount Errors (Secondary Issue)

13,477 `gst_mini_object_unref` errors in 5 minutes indicates memory corruption in Wolf's GStreamer code. However:
- This doesn't prevent native Moonlight clients from working
- Likely unrelated to moonlight-web specific issue
- Possibly from PauseStreamEvent modifications (needs separate investigation)
