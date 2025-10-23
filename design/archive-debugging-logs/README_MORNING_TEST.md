# Morning Test Plan - Moonlight Web Streaming

## TL;DR

moonlight-web streaming has never worked with Wolf. I've narrowed it down to: **the streamer binary doesn't send UDP pings to Wolf in bridge mode**, even though manual UDP test proves the network route works.

## Quick Win: Test From External Machine

**Easiest test** - bypasses all Docker networking complexity:

1. On your laptop/iPad, open: `http://node01.lukemarsden.net:8080`
2. Create an external agent session
3. Click "Live Stream"
4. If video appears → Problem is Docker-specific
5. If blank screen → moonlight-web/Wolf incompatibility

## Detailed Test: Bridge Mode With Trace Logging

I've prepared moonlight-web with trace logging. To use it:

###Step 1: Finish the rebuild

```bash
cd /home/luke/pm/helix

# Check if build finished
docker images | grep helix-moonlight-web

# If build still running, wait for it or rebuild:
cd /home/luke/pm/moonlight-web-stream
docker build -t helix-moonlight-web -f Dockerfile .
```

The Dockerfile now has `ENV RUST_LOG=moonlight_common=trace` which will log every UDP operation.

### Step 2: Restart moonlight-web

```bash
cd /home/luke/pm/helix
docker compose -f docker-compose.dev.yaml down moonlight-web
docker compose -f docker-compose.dev.yaml up -d moonlight-web

# Verify RUST_LOG is set
docker exec helix-moonlight-web-1 env | grep RUST_LOG
# Should show: RUST_LOG=moonlight_common=trace,moonlight_web=trace
```

### Step 3: Monitor During Streaming

Open 2 terminals:

**Terminal 1** - Wolf RTP activity:
```bash
cd /home/luke/pm/helix
docker compose -f docker-compose.dev.yaml logs -f wolf 2>&1 | grep "RTP"
```

**Terminal 2** - Streamer trace logs:
```bash
cd /home/luke/pm/helix
docker compose -f docker-compose.dev.yaml logs -f moonlight-web
```

Then:
1. Create external agent session in Helix UI
2. Click "Live Stream"
3. Watch both terminals

**What to look for**:
- Terminal 1: Does Wolf show `[RTP] Received ping from 172.19.0.X`?
- Terminal 2: Does streamer show bind/sendto trace logs?

### Step 4: Interpret Results

**If Wolf receives pings**:
- Bridge mode works!
- Video should appear
- Problem solved

**If Wolf doesn't receive pings**:
- Check Terminal 2 for streamer errors:
  - `bind() failed`
  - `sendto failed`
  - `getaddrinfo() failed`
  - Any crash/panic messages
- This will show exactly where streamer fails

## What I Discovered

### The Network Works
Manual test from moonlight-web container:
```bash
echo "TEST" | nc -u 172.19.0.50 48100
```
Wolf received it from `172.19.0.15:56173` ✅

### The Streamer Doesn't Send Pings
Despite network working, during actual streaming:
- RTSP completes successfully
- WebRTC connects
- But Wolf never receives UDP pings from streamer
- Streamer times out after 10 seconds: "No video traffic"

### The Code Says It Should Work

moonlight-common C code (VideoStream.c:74):
```c
while (!interrupted) {
    sendto(rtpSocket, pingData, size, 0, (struct sockaddr*)&Wolf_Address, addrLen);
    sleep(500ms);
}
```

This loop should send pings every 500ms. But it doesn't happen.

**Why?** → Trace logging will tell us

## Configuration State

- Wolf: `172.19.0.50` (static), TRACE logging enabled
- moonlight-web: Bridge mode, RUST_LOG=trace (after rebuild)
- data.json: `address: "wolf"` (Docker DNS)
- config.json: `bind_address: "0.0.0.0:8080"`

## Files for Reference

- `FINAL_STATUS.md` - Complete investigation summary
- `ROOT_CAUSE_ANALYSIS.md` - Socket binding code analysis
- `STREAMING_DEBUG_SUMMARY.md` - Technical details
- `BREAKTHROUGH.md` - Manual UDP test success

## My Recommendation

**Start with the external machine test** - it's the fastest way to know if this is Docker-specific or a fundamental moonlight-web/Wolf incompatibility.

If external works → Focus on Docker networking
If external fails → Check Wolf/Sunshine protocol compatibility
