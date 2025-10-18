# Moonlight Web Streaming Debug Status

## Current Diagnosis

**Problem**: moonlight-web streamer connects successfully but gets no video frames from Wolf

**What Works**:
- ✅ WebRTC (browser ↔ moonlight-web): Connected
- ✅ RTSP handshake (moonlight-web ↔ Wolf): Successful
- ✅ Native Moonlight clients: Work perfectly
- ✅ Screenshots: Working (using wayland-2 socket)

**What Doesn't Work**:
- ❌ Video frames: moonlight-web receives no IDR frames from Wolf
- ❌ Control channel: Wolf never receives IDR requests from moonlight-web
- ❌ UDP packets: No traffic flowing between containers

## Root Cause Investigation

The issue is in the **UDP packet flow** between Wolf and moonlight-web:

1. **RTSP negotiation (TCP)**: Works fine
2. **UDP RTP pings**: moonlight-web should send pings to Wolf ports 48100/48200
3. **Wolf learns client IP**: From the source IP of those ping packets
4. **Wolf sends video/audio**: To the learned client IP/port
5. **Control channel (UDP 47999)**: For IDR frame requests

**Current Status**:
- Wolf container: `172.19.0.50` (static IP)
- moonlight-web container: `172.19.0.15` (dynamic)
- Both on `helix_default` Docker network
- Ports exposed: Wolf has 47989, 48010, 47999/48100/48200 UDP forwarded to host
- moonlight-web has 8081 (web) and 40000-40010/udp (WebRTC) forwarded

## Test To Run

Run this while attempting to stream:

```bash
# Terminal 1: Monitor Wolf RTP/UDP activity
docker compose -f docker-compose.dev.yaml logs -f wolf 2>&1 | grep -E "RTP|client_ip|UDP|Forcing IDR"

# Terminal 2: Monitor moonlight-web streamer
docker compose -f docker-compose.dev.yaml logs -f moonlight-web | grep -E "Stream|IDR|ping|Moonlight"

# Terminal 3: Monitor UDP packets between containers
docker exec helix-wolf-1 tcpdump -i any -n 'udp and (port 48100 or port 48200 or port 47999)' 2>&1
```

## Hypothesis

The moonlight-web streamer might be:
1. Not sending UDP pings to Wolf (connection never established)
2. Sending pings but to wrong IP (using 127.0.0.1 instead of wolf hostname)
3. Sending pings but Wolf doesn't process them correctly

## Potential Fixes to Try

### Fix 1: Verify moonlight-web can resolve "wolf" hostname
```bash
docker exec helix-moonlight-web-1 getent hosts wolf
# Should return: 172.19.0.50 wolf
```

### Fix 2: Check if streamer is actually sending UDP pings
Look in moonlight-common Rust code for where it sends initial ping packets

### Fix 3: Enable more verbose logging
Set `GST_DEBUG=4` in Wolf environment to see GStreamer UDP socket activity

### Fix 4: Check if WOLF_INTERNAL_IP affects anything
Currently set to `172.19.0.50` - try removing it to see if Wolf auto-detects better

## Configuration Changes Made

1. **docker-compose.dev.yaml**:
   - Wolf static IP: `172.19.0.50`
   - `WOLF_INTERNAL_IP=172.19.0.50`
   - Network subnet: `172.19.0.0/16`

2. **moonlight-web-config/config.json**:
   - `webrtc_port_range`: 40000-40010
   - `webrtc_nat_1to1`: `{ips: ["127.0.0.1"], ice_candidate_type: "host"}`
   - `webrtc_network_types`: ["udp4", "udp6"]

3. **moonlight-web-config/data.json**:
   - `address`: "wolf" (Docker hostname)
   - `http_port`: 47989
   - Pairing cleared (re-pair needed)

## Next Debugging Steps

1. Create new external agent session
2. Click "Live Stream"
3. Capture tcpdump of UDP traffic
4. Check if moonlight-web streamer is sending pings to 172.19.0.50:48100/48200
5. If not, check moonlight-common source for how it resolves host address
6. If yes, check why Wolf isn't receiving them (firewall/routing)
