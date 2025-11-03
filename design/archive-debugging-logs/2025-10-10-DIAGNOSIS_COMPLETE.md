# Moonlight Web Streaming - Complete Diagnosis

## TL;DR

**CONFIRMED ROOT CAUSE**: Wolf's RTP ping server starts successfully, but **UDP ping packets from moonlight-web's streamer never arrive at Wolf**.

## Proof

Wolf logs show (timestamp 17:14:15.952):
```
[RTP] Starting RTP ping server on ports 48100 and 48200
```

But in the 10 seconds after, there are:
- ✅ NO "Received ping from 172.19.0.15" messages
- ✅ NO RTP activity in Wolf logs
- ✅ moonlight-web logs show "Waiting for IDR frame" then timeout

## Why Native Clients Work But moonlight-web Doesn't

**Native Moonlight clients** (iPad, desktop):
- Connect from OUTSIDE Docker network
- Use real network interfaces (WiFi, Ethernet)
- UDP packets route through host OS networking stack
- Reach Wolf via port forwarding (0.0.0.0:48100 → 172.19.0.50:48100)

**moonlight-web streamer**:
- Runs INSIDE Docker network (container helix-moonlight-web-1)
- Container IP: 172.19.0.15
- Should send UDP directly to Wolf at 172.19.0.50
- But packets never arrive!

## Network Configuration

### Working (Verified):
- ✅ DNS: `wolf` resolves to `172.19.0.50` inside moonlight-web container
- ✅ TCP: moonlight-web can reach Wolf on TCP port 47989 (RTSP works)
- ✅ Network: Both containers on `helix_default` bridge (172.19.0.0/16)
- ✅ Ping: Containers can ping each other (ICMP works)

### Failing:
- ❌ UDP: Packets from moonlight-web:* → Wolf:48100/48200/47999 don't arrive

## Code Analysis - Where Pings Are Sent

**File**: `/home/luke/pm/moonlight-web-stream/moonlight-common-sys/moonlight-common-c/src/VideoStream.c`

**Lines 61-78**:
```c
memcpy(&saddr, &RemoteAddr, sizeof(saddr));  // RemoteAddr = resolved "wolf" address
SET_PORT(&saddr, VideoPortNumber);            // VideoPortNumber = 48100 from RTSP

while (!interrupted) {
    sendto(rtpSocket, pingData, sizeof(pingData), 0,
           (struct sockaddr*)&saddr, AddrLen);  // Send to Wolf
    sleep(500ms);
}
```

**Line 274**:
```c
rtpSocket = bindUdpSocket(RemoteAddr.ss_family, &LocalAddr, AddrLen, ...);
```

The streamer binds a UDP socket and sends pings every 500ms to the resolved Wolf address.

## Possible Causes

### 1. Source IP Binding Issue (MOST LIKELY)
moonlight-web container might have multiple IPs or interfaces:
- Container IP: 172.19.0.15 (Docker bridge)
- Loopback: 127.0.0.1
- Maybe others from WebRTC/STUN

The `bindUdpSocket()` call uses `LocalAddr` which might be wrong interface.

**Test**: Check what IP moonlight-web binds its UDP socket to
**Fix**: Force moonlight-web to bind to 172.19.0.15 explicitly

### 2. Docker Bridge UDP Filtering
Docker bridge networks sometimes filter UDP differently than TCP:
- TCP works (RTSP handshake succeeds)
- But UDP might be blocked by br-netfilter or iptables

**Test**: Send manual UDP packet from moonlight-web → Wolf
```bash
docker exec helix-moonlight-web-1 bash -c 'echo "TEST" | nc -u 172.19.0.50 48100'
```

**Fix**: Enable UDP forwarding in Docker network or use host networking

### 3. moonlight-common Using Wrong RemoteAddr
Maybe the Rust wrapper around moonlight-common passes wrong address?

**Test**: Add debug logging to see what address streamer resolves
**Fix**: Patch moonlight-web to log RemoteAddr before sending pings

## Recommended Fix Attempts (In Order)

### Fix #1: Enable UDP Debug Logging in moonlight-web

Modify moonlight-web Dockerfile to set:
```dockerfile
ENV RUST_LOG=debug
```

Rebuild and check streamer logs for:
- What address it resolves "wolf" to
- What local IP it binds UDP socket to
- Any UDP send errors

### Fix #2: Manual UDP Packet Test

Install netcat in both containers:
```bash
docker exec helix-moonlight-web-1 apt update && apt install -y netcat-openbsd
docker exec helix-wolf-1 apt update && apt install -y netcat-openbsd tcpdump
```

Test UDP send:
```bash
# From moonlight-web to Wolf
docker exec helix-moonlight-web-1 nc -u 172.19.0.50 48100 <<< "PING"

# Monitor Wolf
docker exec helix-wolf-1 tcpdump -i any -n 'udp port 48100'
```

If this works → streamer code issue
If this fails → Docker network issue

### Fix #3: Use Host Network Mode for moonlight-web

Change docker-compose.dev.yaml:
```yaml
moonlight-web:
  network_mode: "host"  # Use host networking instead of bridge
```

This would make moonlight-web behave like a native client.

**Downside**: Loses container isolation

### Fix #4: Add Explicit UDP Route

If Docker bridge is filtering, add explicit route/rule:
```bash
docker network inspect helix_default
# Check for any UDP-blocking rules
```

## Files Ready For User

1. `WHEN_YOU_RETURN.md` - Quick status summary
2. `TEST_MOONLIGHT_WEB.md` - Step-by-step test procedure
3. `STREAMING_DEBUG_SUMMARY.md` - Technical deep dive
4. `test-moonlight-web-stream.sh` - Quick diagnostic script
5. This file - Complete diagnosis

## Current Environment State

- Wolf: Running with `WOLF_LOG_LEVEL=TRACE` on 172.19.0.50
- moonlight-web: Running on 172.19.0.15
- helix-sway:latest: Rebuilt with fixed screenshot server
- External agent sessions: Using new screenshot server (working)
- Bidirectional Zed messaging: Working perfectly

## Next Debug Step

The fastest way to confirm the diagnosis is Fix #2 (manual UDP test). If manual UDP packets reach Wolf, then it's a moonlight-web streamer code issue. If they don't reach Wolf, it's a Docker networking issue.
