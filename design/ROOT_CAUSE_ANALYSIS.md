# Root Cause Analysis - moonlight-web Streaming

## Discovery: UDP Socket Binding Logic

Found in `/home/luke/pm/moonlight-web-stream/moonlight-common-sys/moonlight-common-c/src/PlatformSockets.c` lines 300-308:

```c
// Use localAddr to bind if it was provided
if (localAddr && localAddr->ss_family != 0) {
    memcpy(&bindAddr, localAddr, addrLen);
    SET_PORT(&bindAddr, 0);  // Kernel picks random port
}
else {
    // Wildcard bind to 0.0.0.0
    memset(&bindAddr, 0, sizeof(bindAddr));
    SET_FAMILY(&bindAddr, addressFamily);
}
```

The `LocalAddr` is populated from `getsockname()` on the RTSP TCP connection (RtspConnection.c:481-490).

## Network Mode Comparison

### Bridge Mode (SHOULD WORK):
1. Streamer resolves "wolf" → `172.19.0.50`
2. TCP RTSP connects to `172.19.0.50:48010`
3. `getsockname()` → `172.19.0.15:random` (moonlight-web's container IP)
4. UDP binds to `172.19.0.15:0`
5. `sendto(172.19.0.50:48100)` from `172.19.0.15:random`
6. ✅ Both containers on same Docker network - routing works!
7. ✅ Manual test confirmed: `nc -u 172.19.0.50 48100` from moonlight-web → Wolf receives from 172.19.0.15

**Problem**: Streamer never actually sends pings (we've never seen Wolf receive them in bridge mode)

### Host Mode (DOESN'T WORK):
1. Streamer resolves "localhost" → `127.0.0.1`
2. TCP RTSP connects to `localhost:48010`
3. Goes through Docker port forwarding
4. `getsockname()` → `127.0.0.1:random` or host IP
5. UDP binds to that address
6. `sendto(localhost:48100)`
7. Goes through Docker port forwarding
8. Docker NAT rewrites source to `172.19.0.1` (gateway)
9. Wolf receives pings from `172.19.0.1:random`
10. Wolf sends video back to `172.19.0.1`
11. ❌ Packets never reach streamer on host!

**Problem**: NAT return path broken - Wolf can't send back to streamer through port forwarding

## Key Question

**Why doesn't the streamer send pings in bridge mode?**

Possible causes:
1. Streamer crashes/exits before ping loop starts
2. `bindUdpSocket()` fails silently
3. `sendto()` fails with no logging
4. Thread that sends pings never starts
5. Some other initialization failure

## Evidence Needed

Check moonlight-web logs for:
- `[Stream]: failed to spawn streamer process`
- `[Stream]: streamer process didn't include stdin/stdout`
- Any stderr output from streamer binary
- Streamer lifecycle messages

## Debugging Steps for User

### Test 1: Verify Streamer Spawns in Bridge Mode
```bash
# Create external agent session
# Click "Live Stream"
# Immediately check:
docker exec helix-moonlight-web-1 sh -c 'ls -la /proc/*/exe 2>/dev/null | grep streamer'
```

If streamer process exists → check its logs
If no process → spawning failed

### Test 2: Check Streamer stderr Output
Moonlight-common uses `Limelog()` which writes to stderr. Check for:
```bash
docker compose -f docker-compose.dev.yaml logs moonlight-web | grep "Limelog\|bind.*failed\|sendto.*failed"
```

### Test 3: Add Debug Logging to web-server
The IPC between web-server and streamer might show errors. Check:
```bash
docker compose -f docker-compose.dev.yaml logs moonlight-web | grep "\[Ipc\]\|\[Stream\]"
```

## Current Build Status

Docker build in progress with `RUST_LOG=moonlight_common=trace` added to Dockerfile.
This will show detailed logging from:
- Host resolution
- Socket binding
- UDP ping sending
- Any errors in the process

Once build completes, the logs will reveal exactly why pings aren't being sent.

## Hypothesis

Most likely: The streamer process starts, does RTSP successfully, but then encounters an error during UDP socket initialization that isn't being logged properly. The trace logging will reveal this.

Alternatively: There's a compatibility issue between moonlight-common (designed for Sunshine) and Wolf's implementation of the Moonlight protocol.
