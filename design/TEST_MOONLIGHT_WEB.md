# Testing Instructions for Moonlight Web Streaming

## Current State

I've enabled trace logging in Wolf to capture UDP/RTP activity:
- `WOLF_LOG_LEVEL=TRACE` (shows all RTP ping reception)
- `GST_DEBUG=3` (shows GStreamer pipeline activity)

Wolf has been restarted and is ready for testing.

## Test Procedure

### Step 1: Create a Fresh Session
1. Open Helix UI at http://localhost:8080
2. Navigate to "External Agents" or create a PDE
3. Click "Create New Session"
4. Wait for session to fully initialize (~30 seconds)

### Step 2: Monitor Logs (Open 3 Terminal Windows)

**Terminal 1 - Wolf RTP Activity**:
```bash
cd /home/luke/pm/helix
docker compose -f docker-compose.dev.yaml logs -f wolf 2>&1 | grep -E "RTP|client_ip|Received ping|Starting.*ping"
```

**Terminal 2 - moonlight-web Streamer**:
```bash
cd /home/luke/pm/helix
docker compose -f docker-compose.dev.yaml logs -f moonlight-web | grep -E "Stream|IDR|video|ping"
```

**Terminal 3 - UDP Packet Capture** (if available):
```bash
# This requires tcpdump - may not be available
docker exec helix-wolf-1 tcpdump -i any -n -v 'udp and (port 48100 or port 48200 or port 47999)' 2>&1
```

### Step 3: Attempt Streaming
1. In Helix UI, click "Live Stream" button
2. Watch the 3 terminal windows
3. Look for these specific messages:

**In Terminal 1 (Wolf logs), you SHOULD see**:
```
[RTP] Starting RTP ping server on ports 48100 and 48200
[RTP] Received ping from 172.19.0.15:XXXXX
```

**In Terminal 2 (moonlight-web logs), you WILL see**:
```
[Moonlight Stream]: Video port: 48100
[Moonlight Stream]: Requesting IDR frame
```

**What to Check**:
- Does Wolf show "Received ping from 172.19.0.15"?
  - YES → Wolf is receiving pings but not sending video (GStreamer issue)
  - NO → Network/routing problem between containers

### Step 4: Manual UDP Test

If Wolf isn't receiving pings, test UDP manually:

```bash
# Send manual UDP packet from moonlight-web to Wolf video port
docker exec helix-moonlight-web-1 bash -c 'echo "TEST" > /dev/udp/172.19.0.50/48100'

# Check Wolf logs
docker compose -f docker-compose.dev.yaml logs wolf | tail -30
```

## Expected Results

### If Working Correctly:
- Wolf logs: `[RTP] Received ping from 172.19.0.15:XXXXX`
- Wolf logs: Video pipeline starts sending buffers
- moonlight-web logs: "Received first video packet"
- Browser: Video appears

### If Still Broken - Scenario A (No pings received):
- Wolf logs: NO "Received ping" messages
- Indicates network/routing issue
- Fix: Check Docker network, firewall rules, source IP binding

### If Still Broken - Scenario B (Pings received, no video):
- Wolf logs: Shows "Received ping" but no video buffers sent
- Indicates GStreamer pipeline issue
- Fix: Check waylanddisplaysrc connection, pipeline state, refcount errors

## Files Changed

1. **docker-compose.dev.yaml**:
   - Wolf static IP: `172.19.0.50`
   - Enabled trace logging

2. **moonlight-web-config/config.json**:
   - webrtc_nat_1to1: 127.0.0.1 for localhost testing
   - webrtc_port_range: 40000-40010

3. **moonlight-web-config/data.json**:
   - address: "wolf" (uses Docker DNS)
   - Pairing certificates present

4. **api/cmd/screenshot-server/main.go**:
   - Auto-discovers correct Wayland socket (tries all)
   - Caches working socket for performance

5. **helix-sway:latest**: Rebuilt with new screenshot server

## Quick Test Without UI

```bash
# Test Wolf HTTP reachability from moonlight-web
docker exec helix-api-1 curl -s http://172.19.0.50:47989/serverinfo | head -5

# Should return XML with <hostname>Wolf</hostname>
```

## Log File Locations

- Wolf logs: `docker compose -f docker-compose.dev.yaml logs wolf`
- moonlight-web logs: `docker compose -f docker-compose.dev.yaml logs moonlight-web`
- API logs: `docker compose -f docker-compose.dev.yaml logs api`

## After Testing

Report back with:
1. Did Wolf show "Received ping from 172.19.0.15"? (YES/NO)
2. Did video appear in browser? (YES/NO)
3. Error messages from any of the 3 terminals
4. tcpdump output if available
