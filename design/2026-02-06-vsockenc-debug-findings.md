# vsockenc Debug Findings - GST_DEBUG Output Analysis

**Date:** 2026-02-06 08:05
**Session:** ses_01kgryn4cx0mvcj8k319x1yx0s

## 🔍 Key Discovery: Receive Thread Started But Not Processing Responses

Successfully enabled `GST_DEBUG=vsockenc:5` and captured detailed logs from the vsockenc GStreamer element.

### vsockenc Debug Output

```
0:00:33.989729752   333 0xe7a354066d30 DEBUG            vsockenc gstvsockenc.c:601:gst_vsockenc_recv_thread:<vsockenc0> Receive thread started
0:00:34.030171437   333 0xe7a330000c00 INFO             vsockenc gstvsockenc.c:322:gst_vsockenc_connect:<vsockenc0> Connected via TCP to 10.0.2.2:5900
0:00:34.030190228   333 0xe7a330000c00 WARN             vsockenc gstvsockenc.c:373:gst_vsockenc_get_resource_id:<vsockenc0> Buffer is not DMA-BUF backed
0:00:34.030192145   333 0xe7a330000c00 WARN             vsockenc gstvsockenc.c:509:gst_vsockenc_handle_frame:<vsockenc0> Failed to get resource ID for frame
0:00:34.030216978   333 0xe7a330000c00 DEBUG            vsockenc gstvsockenc.c:570:gst_vsockenc_handle_frame:<vsockenc0> Sent frame 1, resource_id=0, size=1920x1080, keyframe=1
0:00:34.058096622   333 0xe7a330000c00 WARN             vsockenc gstvsockenc.c:373:gst_vsockenc_get_resource_id:<vsockenc0> Buffer is not DMA-BUF backed
0:00:34.058111414   333 0xe7a330000c00 WARN             vsockenc gstvsockenc.c:509:gst_vsockenc_handle_frame:<vsockenc0> Failed to get resource ID for frame
0:00:34.058183872   333 0xe7a330000c00 DEBUG            vsockenc gstvsockenc.c:570:gst_vsockenc_handle_frame:<vsockenc0> Sent frame 2, resource_id=0, size=1920x1080, keyframe=0
0:00:34.325731689   333 0xe7a330000c00 WARN             vsockenc gstvsockenc.c:373:gst_vsockenc_get_resource_id:<vsockenc0> Buffer is not DMA-BUF backed
0:00:34.325744355   333 0xe7a330000c00 WARN             vsockenc gstvsockenc.c:509:gst_vsockenc_handle_frame:<vsockenc0> Failed to get resource ID for frame
0:00:34.325829231   333 0xe7a330000c00 DEBUG            vsockenc gstvsockenc.c:570:gst_vsockenc_handle_frame:<vsockenc0> Sent frame 3, resource_id=0, size=1920x1080, keyframe=0
```

### Analysis

#### ✅ What's Working

1. **Receive Thread Started** (line 601)
   - `gst_vsockenc_recv_thread` is running
   - Thread ID: 0xe7a354066d30

2. **TCP Connection Established** (line 322)
   - Connected to 10.0.2.2:5900 successfully
   - Socket connection is ESTABLISHED (verified with `netstat`)

3. **Frame Requests Sent** (line 570)
   - Frames 1, 2, 3, 4... being sent to QEMU
   - resource_id=0 (expected - no DMA-BUF)
   - Size: 1920x1080
   - Keyframe flags correct

4. **DMA-BUF Fallback Behavior Confirmed** (line 373, 509)
   - "Buffer is not DMA-BUF backed" - as expected on virtio-gpu
   - "Failed to get resource ID for frame" - fallback to resource_id=0
   - This confirms Mutter provides SHM, not DMA-BUF

#### ❌ What's Missing

**NO LOGS ABOUT RECEIVING RESPONSES**

The receive thread logs show it started, but there are NO subsequent logs like:
- "Failed to read frame response"
- "Invalid message magic"
- "Finished frame pts=..."
- "Connection lost reading header"
- "No pending frame for pts=..."

**Expected flow:**
```
Receive thread started
  ↓
[Loop: read header from socket]
  ↓
[Read HelixFrameResponse]
  ↓
[Read NAL data]
  ↓
"Finished frame pts=X"
  ↓
[Repeat...]
```

**Actual behavior:**
```
Receive thread started
  ↓
[SILENCE - no further logs]
```

### Possible Causes

1. **QEMU Not Sending Responses**
   - QEMU logs are empty (helix-debug.log = 0 bytes)
   - No evidence of QEMU encoding frames
   - Possible: QEMU's helix-frame-export not receiving connections
   - Possible: socat proxy not forwarding data bidirectionally

2. **Receive Thread Blocked on read()**
   - Thread might be stuck in `read_exact()` waiting for data
   - If QEMU never sends anything, thread blocks forever
   - No error, no timeout - just waiting

3. **Protocol Mismatch**
   - QEMU might be sending data in wrong format
   - Magic number validation failing silently
   - Struct alignment issues

### QEMU Socket Architecture

```
Guest (vsockenc)
  ↓ TCP connect to 10.0.2.2:5900
QEMU user-mode network
  ↓ Forward to host 127.0.0.1:5900
socat (PID 59230)
  ↓ Proxy TCP → Unix socket
Unix socket: /Users/luke/Library/Group Containers/.../helix-frame-export.sock
  ↓
QEMU helix-frame-export module
```

**Verified:**
- ✅ Unix socket exists (created at 07:40 when QEMU started)
- ✅ socat is listening on TCP 127.0.0.1:5900
- ✅ socat is configured to proxy to the Unix socket
- ✅ vsockenc connects to TCP 10.0.2.2:5900
- ❓ QEMU accepting connections on Unix socket?
- ❓ QEMU sending H.264 responses?

## Next Steps

### 1. Verify QEMU Is Actually Receiving Connections

Add debug logging to vsockenc to log raw socket reads:
```c
// In gst_vsockenc_recv_thread, before read_exact():
GST_DEBUG_OBJECT(self, "About to read header (%zu bytes)", sizeof(header));
// After read_exact():
GST_DEBUG_OBJECT(self, "Read %zu bytes, magic=0x%08x", sizeof(header), header.magic);
```

### 2. Check QEMU Logs via Alternative Method

The helix-debug.log file is empty. Try:
- Check macOS Console.app for QEMU stderr
- Add explicit `fprintf(stderr, ...)` before `helix_log()`
- Check if QEMU process has stderr redirected

### 3. Test QEMU Socket with Manual Client

Write a simple test that:
1. Connects to the Unix socket
2. Sends a `HelixFrameRequest`
3. Waits for `HelixFrameResponse`
4. Logs what happens

### 4. Add Timeout to Receive Thread

Modify vsockenc to use `select()` or `poll()` with timeout:
```c
// Before blocking read, add timeout
struct timeval tv = { .tv_sec = 1, .tv_usec = 0 };
setsockopt(fd, SOL_SOCKET, SO_RCVTIMEO, &tv, sizeof(tv));
```

This would allow the thread to log "timeout waiting for response" instead of blocking forever.

## Summary

The vsockenc element is:
- ✅ Connecting to QEMU
- ✅ Sending frame requests
- ✅ Receive thread running
- ❌ **NOT receiving any responses from QEMU**

The blocker is now clearly isolated: **QEMU is not sending H.264 responses back to vsockenc**.

Need to determine if:
1. QEMU is not receiving the requests (socat issue)
2. QEMU is receiving but not encoding (initialization issue)
3. QEMU is encoding but not sending (socket write failing)
4. QEMU is sending but data is lost (socat bidirectional issue)

## Files Modified

- `api/pkg/external-agent/hydra_executor.go` - Added `GST_DEBUG=vsockenc:5`
- `api/pkg/hydra/devcontainer.go` - (Initially added GST_DEBUG here, but wrong location)

## Commits

- 03a2e76c8: "fix: Add GST_DEBUG to correct location in hydra_executor.go"
- f76ec2fd4: "debug: Enable GST_DEBUG=vsockenc:5 for debugging receive thread"
