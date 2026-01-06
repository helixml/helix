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

## Next Steps

1. Check Wolf logs for video port negotiation
2. Check if moonlight-common-c logged "Terminating connection" or "Video Receive" errors
3. Compare RTSP negotiated video port vs actual socket binding
4. Add logging to VideoReceiveThreadProc to capture exit reason
