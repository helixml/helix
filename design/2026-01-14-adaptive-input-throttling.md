# Adaptive Input Throttling Based on RTT/Frame Lag

**Date:** 2026-01-14
**Status:** Design
**Author:** Claude Code

## Problem Statement

With damage-based video capture (PipeWire ScreenCast on GNOME, ext-image-copy-capture on Sway), the frame rate is directly controlled by screen activity. When the screen is static, we only send keepalive frames (now 2 FPS with the 500ms timer). But when the user is actively moving the mouse or scrolling, each input event causes screen damage, potentially generating frames at the input event rate.

**Current behavior:**
- Frontend sends mouse/scroll events at configured FPS (already throttled)
- Each event causes screen damage in the remote desktop
- Backend captures and encodes a frame for each damage event
- At 60 FPS input → 60 FPS video output over WebSocket

**The problem:**
- With high network latency (RTT), frames queue up
- User experiences growing input lag as frames buffer
- The video becomes a slideshow of stale frames
- Reducing input rate would reduce frame generation and prevent queueing

## Existing Implementation

The frontend (`websocket-stream.ts`) already has significant infrastructure:

### 1. RTT Measurement (lines 1222-1310)
```typescript
private currentRttMs = 0
private readonly PING_INTERVAL_MS = 1000  // Ping every second
private readonly MAX_RTT_SAMPLES = 10     // Moving average
```
- Sends Ping/Pong every second
- Calculates moving average RTT
- Exposed via `getStats().rttMs`

### 2. Frame Drift / Latency (lines 1183-1190)
```typescript
private currentFrameLatencyMs = 0  // How late current frame arrived (ms)
private readonly FRAME_LATENCY_THRESHOLD_MS = 200
```
- Compares frame arrival time to expected time based on PTS
- **Negative values** (e.g., -166ms) indicate encoder pipeline latency, not network delay
- This is normal: encoder captures at T, processes for ~166ms, outputs with PTS=T

### 3. Static Input Throttling (lines 203-209, 276)
```typescript
// Already throttles to configured FPS
this.mouseThrottleMs = Math.floor(1000 / settings.fps)
```

### 4. Input Buffer Congestion Detection (lines 1320-1350)
```typescript
/**
 * Check if the input send buffer is congested
 * Strategy: Skip mouse moves if buffer hasn't drained since last send.
 * This prevents "ghost moves" - stale positions that arrive late.
 */
private isInputBufferCongested(): boolean { ... }
```
- Already skips mouse moves when WebSocket buffer backs up!
- Tracks `inputsDroppedDueToCongestion` for stats

### What's Missing

The current implementation has static throttling (to configured FPS) and reactive congestion detection (skip when buffer backs up). What's missing is **proactive adaptive throttling** based on RTT trends - reducing input rate BEFORE congestion occurs.

## Proposed Solution

Implement adaptive input throttling on the **frontend** that dynamically adjusts the input send rate based on measured RTT and frame lag.

### Throttle Targets

The user configures their desired frame rate in the agent settings (e.g., 60 FPS, 30 FPS). The throttle operates as a **ratio** of that configured rate:

| RTT / Frame Lag | Throttle Ratio | Example @ 60 FPS | Example @ 30 FPS |
|-----------------|----------------|------------------|------------------|
| < 50ms | 100% | 60 Hz | 30 Hz |
| 50-100ms | 75% | 45 Hz | 22.5 Hz |
| 100-150ms | 50% | 30 Hz | 15 Hz |
| 150-250ms | 33% | 20 Hz | 10 Hz |
| > 250ms | 25% | 15 Hz | 7.5 Hz |

This respects the user's configured quality/bandwidth tradeoff while still adapting to network conditions.

### What Gets Throttled

1. **Mouse movement** (absolute and relative)
   - Most frequent input type
   - Each movement triggers a frame
   - Throttle aggressively

2. **Scroll events** (wheel and smooth scroll)
   - High frequency during active scrolling
   - Each scroll step triggers a frame
   - Throttle similarly to mouse movement

3. **NOT throttled:**
   - **Keyboard input** - Always send immediately (typing latency is very noticeable)
   - **Mouse clicks** - Always send immediately (click latency breaks UX)
   - **Touch events** - May need special handling for gestures

### Measurement Mechanisms

#### 1. RTT via Ping/Pong (Already Implemented)

The backend already supports RTT measurement via binary Ping/Pong messages:

```
// ws_stream.go lines 869-885
Ping (0x40): [msgType:1][seq:4][clientTime:8] = 13 bytes
Pong (0x41): [msgType:1][seq:4][clientTime:8][serverTime:8] = 21 bytes
```

**Frontend implementation:**
```typescript
// Send ping every 1 second
const ping = new ArrayBuffer(13);
const view = new DataView(ping);
view.setUint8(0, 0x40);  // Ping type
view.setUint32(1, seqNum++, false);  // Sequence number (big-endian)
view.setBigUint64(5, BigInt(Date.now() * 1000), false);  // Client time in microseconds
ws.send(ping);

// On Pong received, calculate RTT
const rtt = Date.now() - (Number(clientTimeMicros) / 1000);
```

#### 2. Frame Timing Analysis

Track the delta between expected and actual frame arrival times:

```typescript
interface FrameStats {
  lastFrameTime: number;
  expectedInterval: number;  // Based on current input rate
  recentIntervals: number[];  // Rolling window
  lagScore: number;  // 0-1, higher = more lag
}

function updateFrameStats(stats: FrameStats, now: number) {
  const interval = now - stats.lastFrameTime;
  stats.recentIntervals.push(interval);
  if (stats.recentIntervals.length > 30) {
    stats.recentIntervals.shift();
  }

  // Calculate lag score based on variance from expected
  const avgInterval = average(stats.recentIntervals);
  const variance = calculateVariance(stats.recentIntervals);

  // High variance = inconsistent delivery = network congestion
  stats.lagScore = Math.min(1, variance / (avgInterval * 0.5));
  stats.lastFrameTime = now;
}
```

### Implementation: Changes to websocket-stream.ts

The existing infrastructure makes this straightforward. We need to:

1. **Make `mouseThrottleMs` dynamic** instead of static
2. **Add throttle ratio calculation** based on RTT
3. **Update throttle on RTT change**

```typescript
// ADD: Adaptive throttle state (near other private members)
private adaptiveThrottleRatio = 1.0  // 1.0 = full rate, 0.25 = 25% rate

// MODIFY: getAdaptiveThrottleMs() replaces static mouseThrottleMs usage
private getAdaptiveThrottleMs(): number {
  const baseThrottleMs = 1000 / this.settings.fps  // Configured FPS
  return baseThrottleMs / this.adaptiveThrottleRatio
}

// CHANGE: Increase ping frequency from 1000ms to 500ms (matches keepalive rate)
private readonly PING_INTERVAL_MS = 500

// ADD: Update throttle ratio based on RTT (call from handlePong)
private updateAdaptiveThrottle() {
  const rtt = this.currentRttMs

  // Calculate ratio based on RTT thresholds
  let ratio: number
  if (rtt < 50) {
    ratio = 1.0     // 100% - full configured rate
  } else if (rtt < 100) {
    ratio = 0.75    // 75%
  } else if (rtt < 150) {
    ratio = 0.5     // 50%
  } else if (rtt < 250) {
    ratio = 0.33    // 33%
  } else {
    ratio = 0.25    // 25% - minimum
  }

  // Smooth transitions with exponential moving average
  this.adaptiveThrottleRatio = this.adaptiveThrottleRatio * 0.7 + ratio * 0.3
}
```

### Minimal Code Changes

**Change `PING_INTERVAL_MS`** (line 172):
```typescript
private readonly PING_INTERVAL_MS = 500  // Was 1000, now 500ms for faster RTT feedback
```

**In `handlePong()`** (after line 1273):
```typescript
this.currentRttMs = sum / this.rttSamples.length
this.updateAdaptiveThrottle()  // ADD: update throttle on RTT change
```

**In `sendMouseMove()`, `sendMousePosition()`, `sendScrollThrottled()`**:
Replace `this.mouseThrottleMs` with `this.getAdaptiveThrottleMs()`

**In `getStats()`** - add to return object:
```typescript
adaptiveThrottleRatio: this.adaptiveThrottleRatio,
effectiveInputFps: this.settings.fps * this.adaptiveThrottleRatio,
```

### Coalescing (Already Implemented)

The frontend already coalesces events during throttling (lines 1443-1458, 1609-1666):
- `pendingMousePosition` - stores latest absolute position (overwrites)
- `pendingMouseMove` - accumulates relative deltas
- `pendingScroll` - accumulates scroll deltas

No additional coalescing code needed.

## Alternative Approaches Considered

### 1. Backend Throttling
- **Rejected**: Backend doesn't have visibility into network conditions from client perspective
- Backend would need to buffer events and potentially introduce jitter

### 2. Fixed Throttle Rate
- **Rejected**: One size doesn't fit all - 30 Hz is too slow for LAN, too fast for poor connections
- Adaptive is more user-friendly

### 3. Frame-Based Throttling (wait for frame before next input)
- **Rejected**: Creates artificial coupling between input and video
- Would make input feel sluggish even on good connections
- Better to throttle by time with RTT feedback

### 4. Server-Initiated Backpressure
- **Considered for future**: Server could send "slow down" messages
- More complex protocol, but could be more responsive
- Could be added later as enhancement

## Testing Plan

1. **Unit tests** for throttle calculation and coalescing
2. **Integration test** with simulated latency (tc netem)
3. **Benchmark CLI** extension:
   ```bash
   helix spectask benchmark ses_xxx --test-throttle --inject-latency 100ms
   ```
4. **Manual testing** across network conditions:
   - Local (< 5ms RTT)
   - LAN (5-20ms RTT)
   - WAN same region (20-50ms RTT)
   - WAN cross-region (100-200ms RTT)
   - Poor connection (> 200ms RTT, packet loss)

## Metrics to Track

Frontend should expose (via console or debug panel):
- Current RTT (ms)
- Current lag score (0-1)
- Current throttle rate (Hz)
- Input events generated vs. sent
- Frame FPS

Backend already logs:
- VIDEO LATENCY STATS every 5 seconds
- Average WebSocket send time
- Frame size and keyframe status

## Rollout Plan

1. **Phase 1**: Implement and test locally
2. **Phase 2**: Add feature flag (`?adaptiveThrottle=true`)
3. **Phase 3**: A/B test with subset of users
4. **Phase 4**: Enable by default, remove flag

## Open Questions

1. **Hysteresis**: How much RTT improvement before increasing rate?
   - Suggest: Require 20% improvement sustained for 3 seconds before rate increase
   - Prevents oscillation

2. **Minimum throttle during drag operations?**
   - Users may expect smoother cursor during drag
   - Could detect mousedown state and maintain higher rate

3. **Touch event handling?**
   - Pinch-to-zoom needs high rate for smooth feel
   - May need gesture-specific logic

## Files to Modify

**Frontend:**
- `frontend/src/lib/helix-stream/stream/websocket-stream.ts`:
  - Add `adaptiveThrottleRatio` property
  - Add `getAdaptiveThrottleMs()` method
  - Add `updateAdaptiveThrottle()` method
  - Call from `handlePong()`
  - Replace `mouseThrottleMs` with `getAdaptiveThrottleMs()` in throttling logic
  - Add stats to `getStats()`

**Backend (no changes required):**
- `api/pkg/desktop/ws_stream.go` - Already supports Ping/Pong
- `api/pkg/desktop/ws_input.go` - No changes needed

## Implementation Effort

**Estimated: ~50 lines of code changes**

Given that RTT measurement, input throttling, coalescing, and congestion detection all already exist, this is a small incremental change:
1. Add `adaptiveThrottleRatio` state variable
2. Add `updateAdaptiveThrottle()` with threshold logic
3. Call it from `handlePong()`
4. Replace static `mouseThrottleMs` with dynamic calculation

## Conclusion

The frontend already has most of the infrastructure needed:
- RTT measurement via Ping/Pong ✓
- Input throttling to configured FPS ✓
- Event coalescing during throttling ✓
- Buffer congestion detection and reactive throttling ✓

What's missing is **proactive** adaptive throttling that reduces input rate based on RTT trends before congestion occurs. This requires ~50 lines of code to add a throttle ratio that scales with RTT and applies to the existing throttling mechanism.

### Why RTT Is Sufficient

Everything runs over a single TCP WebSocket connection:

```
Server → Client: [VideoFrame][VideoFrame][Pong][VideoFrame]...
Client → Server: [Input][Input][Ping][Input]...
```

**RTT captures congestion effectively**: Pong packets queue behind video frames in the TCP stream. When congested, RTT spikes because Pong waits behind backed-up video frames. This makes RTT a valid proxy for overall connection health.

**Ping at 500ms** (matching keepalive rate) provides fast enough feedback for adaptive throttling without the complexity of interpreting frame drift metrics.

### Understanding Frame Drift (for reference)

The frame drift calculation (`websocket-stream.ts` lines 892-929) compares each frame's arrival time to expected arrival based on PTS delta from the first frame.

**Why -166ms?**

The first frame has higher encoder latency due to warmup:
- First frame: encoder warmup takes E₀ ≈ 200ms
- Subsequent frames: encoder takes E₁ ≈ 34ms
- Drift = E₁ - E₀ = 34 - 200 = **-166ms**

**The sign is meaningful:**
- **Negative** (e.g., -166ms): Stable - encoder warmed up, frames arriving consistently faster than first frame
- **Trending toward 0**: Could indicate encoder latency increasing or network slowing
- **Positive**: Congestion - frames arriving LATER than first frame baseline

**Display suggestion:**
Rather than showing raw drift, could show:
```
Frame Timing: 166ms ahead (stable)    // negative drift
Frame Timing: 50ms behind (lagging)   // positive drift
```

Or reset baseline after warmup (after first 30 frames) to show drift from steady-state.

## Related Optimizations

### 1. Increase Keyframe Interval (GOP)

**Current setting:** GOP = 120 (keyframe every 2 seconds at 60fps)

**Problem:** Keyframes are 5-10x larger than P-frames, causing bandwidth spikes every 2 seconds that make the stream less smooth.

**Why keyframes are less important for us:**
- We use reliable TCP WebSocket transport (not lossy UDP)
- No packet loss means no need for periodic recovery points
- Each new frontend connection triggers a new encoder pipeline → fresh keyframe
- Mid-stream keyframes are only needed for error recovery (rare on TCP)

**Proposed change:** Increase GOP to 1800+ (30 seconds at 60fps) or use "infinite GOP"

**Trade-offs:**
- Larger GOP = smoother bandwidth = better perceived quality
- If encoder state corruption occurs, longer recovery time (extremely rare)
- Stream seeking/recording use cases would need keyframes (not applicable to us)

**Implementation:** Change `gop-size=%d` in `ws_stream.go` from `getGOPSize()` (returns 120) to a much larger value.

### 2. Backend Encoder Latency Measurement (Future)

**Goal:** Measure time from PipeWire capture to encoded frame output, separate from network latency.

**Available data:**
- `frame.PTS` - GStreamer presentation timestamp (from PipeWire capture time)
- `frame.Timestamp` - Wall clock when appsink callback fires (after encoding)

**Approach:**
1. On first frame, record `firstFrameWallTime` and `firstFramePTS`
2. For each frame: `expectedWallTime = firstFrameWallTime + (currentPTS - firstFramePTS)`
3. `encoderLatencyMs = frame.Timestamp - expectedWallTime`

**Benefits:**
- Separates encoder latency from network latency in the frontend stats
- Helps identify whether lag is due to capture/encoding or network
- Could be sent to frontend in Pong message or new stats message

**Note:** Input latency (WebSocket receive → D-Bus/Wayland inject) is difficult to measure because neither D-Bus nor Wayland provides feedback about when events are actually processed by the compositor.

**Implementation sketch:**

In `ws_stream.go`, add encoder latency tracking:
```go
// In readFramesAndSend()
var firstFrameWallTime time.Time
var firstFramePTS uint64
var encoderLatencyMs float64

for frame := range frameCh {
    if firstFrameWallTime.IsZero() {
        firstFrameWallTime = frame.Timestamp
        firstFramePTS = frame.PTS
    } else {
        // Expected wall time based on PTS delta
        ptsDeltaUs := frame.PTS - firstFramePTS
        expectedWall := firstFrameWallTime.Add(time.Duration(ptsDeltaUs) * time.Microsecond)
        latency := frame.Timestamp.Sub(expectedWall)
        // Exponential moving average
        encoderLatencyMs = encoderLatencyMs*0.9 + float64(latency.Milliseconds())*0.1
    }
    // ...
}
```

New WebSocket message type for stats:
```
Stats (0x50): [msgType:1][encoderLatencyMs:2][reserved:4] = 7 bytes
```

Send periodically (every 1 second) alongside existing Pong responses, or embed in Pong:
```
Extended Pong (0x41): [msgType:1][seq:4][clientTime:8][serverTime:8][encoderLatencyMs:2] = 23 bytes
```

Frontend would display:
```
RTT: 49 ms | Encoder: 34 ms | Total: ~83 ms
```
