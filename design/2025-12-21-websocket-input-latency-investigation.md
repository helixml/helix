# WebSocket Input Latency Investigation

**Date:** 2025-12-21

## Problem Statement

A user with a "spiky latency connection" reported severe mouse-to-display latency (seconds behind) when using WebSocket or SSE video mode, despite frame latency showing only ~30ms. Screenshot mode (which still sends input over WebSocket) had no such lag.

## Key Observations

1. **Frame latency was 30ms** - Video frames arriving on time
2. **Mouse lag was seconds** - Input severely delayed
3. **Screenshot mode worked fine** - Same input path, no video flowing
4. **Both WebSocket and SSE video modes affected** - Not transport-specific
5. **Correlates with variable latency/packet loss** - Network condition dependent

## Investigation

### Initial Hypotheses (Ruled Out)

1. **Base64 decoding blocking main thread (SSE-specific)**
   - Ruled out: WebSocket video mode had same issue, no base64 involved

2. **Server-side input processing bottleneck**
   - Ruled out: Issue correlates with network conditions, not server load

3. **Event loop blocking from video frame processing**
   - Partially relevant but doesn't explain network correlation

### Root Cause: TCP Send Buffer Queueing

The WebSocket API in JavaScript is non-blocking:
- `ws.send()` returns immediately after queuing data
- Data goes into browser's internal send buffer
- `ws.bufferedAmount` reflects queued bytes
- Actual TCP transmission happens on browser's network thread

When the network has issues (packet loss, congestion, variable latency):
1. TCP struggles to transmit data (retransmissions, flow control)
2. `ws.send()` keeps accepting data (non-blocking)
3. `bufferedAmount` grows as data piles up
4. Mouse movements queue up in the send buffer
5. When network recovers, stale positions are finally transmitted
6. Server receives mouse positions from seconds ago

**Why screenshot mode works:**
- Very little data in either direction
- Fewer packets = fewer chances for loss
- Buffer rarely fills up

**Why video mode is affected:**
- High bandwidth video (20+ Mbps) means thousands of packets/sec
- Packet loss causes TCP retransmissions
- Shared TCP connection means both directions affected
- Upload path (input) suffers even if download (video) seems fine

### Why Frame Latency Shows 30ms

Frame latency measures: `arrival_time - expected_arrival_time_based_on_PTS`

This only captures network transit time for video frames. It does NOT capture:
- Time data sits in JavaScript event queue
- Time input waits in send buffer
- Asymmetric network conditions (upload vs download)

If download works fine but upload is congested, frame latency looks good while input is severely delayed.

## Solution

### 1. Input Buffer Congestion Detection

Skip mouse moves immediately if buffer hasn't drained. This prevents "ghost moves" - stale positions that arrive late and make it look like something else is controlling the cursor.

```typescript
private isInputBufferCongested(): boolean {
  if (!this.ws) return false
  const buffered = this.ws.bufferedAmount

  if (buffered === 0) {
    // Buffer is empty - network is keeping up, safe to send
    this.lastBufferDrainTime = performance.now()
    return false
  }

  // Buffer has data - skip immediately, don't pile up stale moves
  // The one move already in the buffer will transmit; we'll send
  // fresh position when buffer drains
  return true
}
```

### 2. Skip Stale Input When Congested

When buffer hasn't drained, don't add more stale mouse positions. For absolute positioning (default mode), we just store the latest position and send it when buffer clears:

```typescript
sendMousePosition(x: number, y: number, refWidth: number, refHeight: number) {
  if (this.isInputBufferCongested()) {
    // Just store latest position - it replaces any previous pending
    this.pendingMousePosition = { x, y, refW: refWidth, refH: refHeight }
    this.inputsDroppedDueToCongestion++
    this.scheduleMouseFlush(this.mouseThrottleMs)
    return
  }
  // ... normal send logic
}
```

This means at most ONE mouse position is ever queued in the WebSocket buffer. When the network catches up and drains it, we send the current (fresh) position, not accumulated stale ones.

### 3. Instrumentation

Added stats to diagnose the issue:

| Stat | Purpose |
|------|---------|
| `inputBufferBytes` | Current `ws.bufferedAmount` |
| `maxInputBufferBytes` | Peak buffer size seen |
| `avgInputBufferBytes` | Moving average of buffer size |
| `inputsSent` | Total inputs sent |
| `inputsDroppedDueToCongestion` | Mouse moves skipped due to congestion |
| `inputCongested` | True if currently congested (buffer > 0) |
| `bufferStaleMs` | How long buffer has been non-empty |
| `lastSendDurationMs` | How long `ws.send()` took (should be ~0) |
| `maxSendDurationMs` | Peak send duration |
| `avgSendDurationMs` | Average send duration |
| `bufferedAmountBeforeSend` | Buffer size before last send |
| `bufferedAmountAfterSend` | Buffer size after last send |
| `eventLoopLatencyMs` | Current event loop latency (excess delay for setTimeout(0)) |
| `maxEventLoopLatencyMs` | Peak event loop latency seen |
| `avgEventLoopLatencyMs` | Average event loop latency |

### 4. Event Loop Latency Tracking

Measure actual event loop responsiveness using `setTimeout(0)` heartbeat:

```typescript
private scheduleEventLoopCheck() {
  this.eventLoopCheckScheduledAt = performance.now()

  setTimeout(() => {
    const elapsed = performance.now() - this.eventLoopCheckScheduledAt
    // setTimeout(0) has ~4-8ms baseline delay in browsers
    const excessLatency = Math.max(0, elapsed - 8)
    this.eventLoopLatencyMs = excessLatency
    // ... track samples

    // Schedule next check
    if (!this.closed) {
      setTimeout(() => this.scheduleEventLoopCheck(), 100)  // Check every 100ms
    }
  }, 0)
}
```

This measures ANY event loop blocking (video decoding, DOM operations, etc.) - not just mouse-related.

### What to Look For

1. **`maxSendDurationMs` > 1ms** → `ws.send()` is blocking (unexpected, browser bug?)
2. **`inputBufferBytes` grows during lag** → Confirms TCP send buffer backing up
3. **`inputsDroppedDueToCongestion` increasing** → Our mitigation is working
4. **High `bufferedAmountAfterSend`** → Data accepted but not transmitted
5. **`eventLoopLatencyMs` > 50ms** → Main thread is blocked (video decoding, etc.)

## Future Improvements

1. **Separate connections for video and input** - Isolate input from video congestion
2. **WebTransport/QUIC** - Avoids TCP head-of-line blocking (limited browser support)
3. **Adaptive bitrate** - Reduce video bitrate when congestion detected
4. **UDP for input** - Not possible with WebSocket, would need WebRTC data channels

## Files Modified

- `frontend/src/lib/moonlight-web-ts/stream/websocket-stream.ts`
  - Added input buffer congestion detection
  - Added send latency instrumentation
  - Added new stats to `getStats()` return type
