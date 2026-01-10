# VideoRecv Thread Missing - Root Cause Analysis

**Date:** 2026-01-06
**Status:** In Progress
**Component:** moonlight-web-stream (moonlight-common-c)

## Problem

WebSocket video streaming shows 0 video frames while audio works correctly.

## Key Findings

### 1. Authorization Issue (FIXED)
- moonlight-web had stale pairing data in `/opt/moonlight-web/server/data.json`
- Wolf rejected requests: "client which wasn't previously paired"
- **Fix:** Removed `paired` key from data.json, ran `/opt/moonlight-web/auto-pair.sh`

### 2. Missing VideoRecv Thread (ROOT CAUSE)

Thread inventory for streamer process (PID 7126):

| Thread | Status | Purpose |
|--------|--------|---------|
| AudioRecv | EXISTS | Reads audio UDP packets |
| AudioDec | EXISTS | Decodes audio |
| AudioPing | EXISTS | Audio keepalive |
| VideoDec | EXISTS | Decodes video frames |
| VideoPing | EXISTS | Video keepalive |
| **VideoRecv** | **MISSING** | Should read video UDP packets |

**Evidence:**
- Video UDP socket (fd 11, port 59252) has 424KB queued in recv-q
- No thread is reading from this socket
- Audio socket (fd 9) is being actively read by AudioRecv thread
- AudioFrame JSON messages flow on stdout, but no VideoFrame messages

### 3. Code Analysis

VideoRecv is created in `moonlight-common-c/src/VideoStream.c:343`:
```c
err = PltCreateThread("VideoRecv", VideoReceiveThreadProc, NULL, &receiveThread);
```

Since VideoDec and VideoPing threads exist (created after VideoRecv), the thread WAS created but then **exited**.

Exit conditions in `VideoReceiveThreadProc`:
1. `malloc()` failure (lines 114-117, 130-134)
2. `recvUdpSocket()` failure (lines 142-144)
3. 10-second timeout waiting for first video packet (lines 151-154)
4. 10-second timeout waiting for first complete frame (lines 173-176)

## Hypothesis

The VideoRecv thread likely exited due to one of:

1. **Port mismatch** - Wolf sending to different port than moonlight-common-c is listening on
2. **10-second timeout** - Thread started, didn't receive packets, and terminated
3. **Socket error** - recvUdpSocket() returned an error

The fact that 424KB is queued suggests packets ARE arriving at the correct port, but the thread may have already exited before they arrived (timing issue during connection setup).

## Investigation Progress

### Logging Path
- moonlight-web stdout is piped through `sed -u 's/^/[MOONLIGHT] /'` in startup-app.sh
- streamer stderr goes to moonlight-web via IPC with prefix `[WsStreamer]`
- However, no `[MOONLIGHT]` or `[WsStreamer]` logs visible in sandbox container logs
- Possible buffering issue or logs going to /dev/null

### Audio Works, Video Doesn't
- AudioRecv thread (12019) IS running and actively reading UDP packets
- AudioFrame JSON messages ARE being output on streamer stdout
- VideoRecv thread is MISSING - never appears in thread list
- VideoDec and VideoPing threads exist (created after VideoRecv in code)

### Timing Theory
The VideoRecv thread likely:
1. Started and began waiting for UDP video packets
2. Timed out after 10 seconds (`FIRST_FRAME_TIMEOUT_SEC`) with no packets
3. Called `connectionTerminated(ML_ERROR_NO_VIDEO_TRAFFIC)` and exited
4. Wolf started sending video AFTER the thread exited
5. Packets now queue up (424KB+) with no reader

## Investigation Session 2 (2026-01-06 13:30+)

### Current Streamer State (PID 11957)

| Thread | LWP | Status |
|--------|-----|--------|
| AudioPing | 12009 | EXISTS |
| ControlRecv | 12012 | EXISTS |
| LossStats | 12013 | EXISTS |
| ReqIdrFrame | 12014 | EXISTS |
| CtrlAsyncCb | 12015 | EXISTS |
| (gap) | 12016 | MISSING (was VideoRecv) |
| VideoDec | 12017 | EXISTS |
| VideoPing | 12018 | EXISTS |
| AudioRecv | 12019 | EXISTS |
| AudioDec | 12020 | EXISTS |
| InputSend | 12021 | EXISTS |

### UDP Socket State

```
fd=9  (port 38078): recv-q=0      - Audio (actively read)
fd=10 (port 49274): recv-q=0      - Control
fd=11 (port 37495): recv-q=423936 - Video (NO READER!)
```

Video packets ARE arriving at port 37495 (424KB queued), but the VideoRecv thread (LWP 12016) exited.

### Limelog Callbacks Not Working

Critical finding: The `log_message` callback (which receives Limelog output) is NOT producing any visible logs.

Expected logs that should appear but DON'T:
- `[WebSocket-Only]: Terminating connection due to lack of video traffic`
- `[WebSocket-Only]: Received first audio packet after X ms`
- `[WebSocket-Only]: Connection terminated (error -1006)`

The WsConnectionListener implements `log_message` with `info!("[WebSocket-Only]: {}", message)` but these never appear in logs.

### Timeline Analysis

```
13:30:09.350Z - RTSP launch request
13:30:09.448Z - moonlight-common-c: "Video stream started"
13:30:09.950Z - Wolf starts video pipeline (interpipesrc -> nvh264enc -> rtpmoonlightpay)
13:30:09.951Z - Wolf pipeline thread started
```

The gap between moonlight-common-c saying "Video stream started" (13:30:09.448) and Wolf starting its video pipeline (13:30:09.950) is only ~500ms. This is well within the 10-second timeout.

### Possible Root Causes

1. **VideoPing not reaching Wolf** - If UDP pings from moonlight-common-c don't reach Wolf, Wolf won't know where to send video
2. **Callback panic** - The `printf_compat::format()` function in the Rust `log_message` callback might be panicking silently
3. **Early thread exit without logging** - VideoRecv might be exiting before any logging happens (e.g., malloc failure without logging)

## Next Steps

1. Check if VideoPing thread is actually sending UDP pings (tcpdump)
2. Verify Wolf receives the pings and responds with video
3. Add direct `eprintln!` logging to the video callback code (bypass log framework)
4. Check if there's a RUST_LOG filter hiding INFO level logs
