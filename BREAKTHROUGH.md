# BREAKTHROUGH - UDP Routing Works!

## Key Finding

**Manual UDP test SUCCEEDED!**

Wolf received a manual UDP packet from moonlight-web:

```
[RTP] Received ping from 172.19.0.15:56173 (21 bytes)
[RTP] video from 172.19.0.15:56173
```

## What This Means

✅ **Docker network is fine** - UDP packets CAN flow between containers
✅ **Wolf RTP server is working** - It receives and processes UDP pings
✅ **IP resolution is correct** - moonlight-web knows Wolf is at 172.19.0.50

❌ **moonlight-web's streamer is NOT sending pings** during actual streaming

## The Real Problem

The `streamer` binary exists (`/app/streamer`, 23MB) but when you click "Live Stream":
- Streamer process spawns (we see logs from it)
- RTSP handshake completes successfully
- Streamer enters "Waiting for IDR frame" loop
- But **NO UDP ping packets are sent to Wolf**

Compare:
- **Manual nc command**: Wolf receives ping immediately
- **Streamer during stream**: Wolf receives NOTHING

## Root Cause

The moonlight-web streamer is likely:

### Hypothesis A: Binding to Wrong Local IP
The `bindUdpSocket()` call in VideoStream.c might bind to:
- 127.0.0.1 (loopback) instead of 172.19.0.15
- Wrong interface from WebRTC/STUN activity
- Then `sendto()` fails silently (no route)

### Hypothesis B: Using Wrong RemoteAddr
The Rust wrapper might pass wrong address to moonlight-common:
- Localhost instead of "wolf"
- Cached old IP
- WebRTC peer IP instead of Wolf IP

### Hypothesis C: Streamer Crashes Before Sending Pings
Process starts, does RTSP, but crashes/hangs before ping thread starts

## Recommended Actions

### 1. Add Debug Logging to Streamer

Edit `/home/luke/pm/moonlight-web-stream/Dockerfile`:
```dockerfile
ENV RUST_LOG=moonlight_common=trace
```

Rebuild and check logs for:
```
Resolved host: <address>
Binding UDP socket to: <local_ip>
Sending ping to: <remote_ip>:<port>
```

### 2. Check Streamer Process State

During streaming, check if streamer is running:
```bash
docker exec helix-moonlight-web-1 ls -la /proc/*/exe 2>/dev/null | grep streamer
```

If it's running, get its PID and check:
```bash
docker exec helix-moonlight-web-1 ls -la /proc/<PID>/fd | grep socket
```

Should show UDP sockets open.

### 3. Compare Working (nc) vs Broken (streamer)

**Working nc command that Wolf receives**:
```bash
nc -u 172.19.0.50 48100
```

**Streamer code (VideoStream.c:74)**:
```c
sendto(rtpSocket, pingData, sizeof(pingData), 0, (struct sockaddr*)&saddr, AddrLen);
```

The difference might be in how the socket is bound initially.

### 4. Quick Fix: Use Host Networking (Workaround)

If debugging takes too long, use host networking as a workaround:

```yaml
moonlight-web:
  network_mode: "host"
  # Remove port mappings (not needed with host mode)
```

This makes moonlight-web behave exactly like a native client.

## Summary for User

**Good news**: The Docker network works fine! Manual UDP test succeeded.

**Bad news**: The streamer binary isn't sending UDP pings when it should.

**Next step**: Enable RUST_LOG=trace and rebuild moonlight-web to see why streamer isn't sending pings.

**Quick workaround**: Use `network_mode: "host"` for moonlight-web to bypass the issue entirely.

---

See `DIAGNOSIS_COMPLETE.md` for full technical analysis.
