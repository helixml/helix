# WebSocket Streaming Congestion Control and Latency Management

**Date:** 2025-12-06
**Status:** Design Draft
**Author:** Claude + Luke

## Problem Statement

On some networks, the WebSocket video stream lags significantly behind real-time. Users experience:
- Delayed visual feedback after keypresses
- Video that progressively gets further behind
- No indication of connection quality
- No mechanism to recover from congestion

The root cause is that TCP streams are reliable—unlike UDP/WebRTC, we can't just drop packets. When the network can't keep up with the video bitrate, frames queue up in TCP send buffers, causing latency to accumulate indefinitely.

## Current Architecture

```
Wolf (GPU encoder)
    → 60fps H264 @ 20Mbps
    → NAL units via IPC (base64 JSON)
    → moonlight-web-stream (Rust)
    → WebSocket binary frames
    → Browser (WebCodecs decoder)
    → Canvas rendering
```

**Current latency strategy**: "Render immediately on frame arrival - no buffering, no pacing."

This works well on fast networks but provides no mechanism to handle congestion.

## Proposed Solutions

### 1. Ping/Pong Latency Measurement (Clock-Drift Resistant)

We need to measure end-to-end latency accurately even when browser and server clocks are not synchronized.

#### RTT-Based Measurement (Network Latency)

```
Browser                          Server
   │                                │
   │─── Ping {seq: 1, t1: 12345} ──▶│
   │                                │ (receives at server_time)
   │◀── Pong {seq: 1, t1: 12345} ───│
   │                                │
   │ t2 = now()                     │
   │ rtt = t2 - t1                  │
```

**Protocol extension** - new message types:
```
0x40 - Ping (client → server)
0x41 - Pong (server → client)
```

**Ping format:**
```
┌────────────┬────────────┬────────────┐
│ Type (1B)  │ Seq (4B)   │ T1 (8B)    │
│ 0x40       │ big-endian │ micros     │
└────────────┴────────────┴────────────┘
```

**Pong format:**
```
┌────────────┬────────────┬────────────┬────────────┐
│ Type (1B)  │ Seq (4B)   │ T1 (8B)    │ ServerRx   │
│ 0x41       │ (echoed)   │ (echoed)   │ (8B, opt)  │
└────────────┴────────────┴────────────┴────────────┘
```

The browser calculates RTT using its own monotonic clock. No clock sync needed.

#### Video Pipeline Latency Measurement

To measure total latency (keypress → screen update), we need to instrument the full pipeline:

**Server-side instrumentation:**
```rust
struct FrameTimingInfo {
    frame_seq: u64,
    wolf_encode_time_us: u64,    // When Wolf produced the frame
    ipc_receive_time_us: u64,    // When web-server received from streamer
    ws_send_time_us: u64,        // When queued for WebSocket send
}
```

**Extended video frame format:**
```
┌────────────┬────────────┬────────────┬────────────┬────────────┬──────────────┐
│ Type (1B)  │ Codec (1B) │ Flags (1B) │ PTS (8B)   │ Size (4B)  │ Server TX    │
│ 0x01       │            │            │            │            │ time (8B)    │
├────────────┴────────────┴────────────┴────────────┴────────────┴──────────────┤
│ NAL Unit Data                                                                  │
└────────────────────────────────────────────────────────────────────────────────┘
```

**Browser-side measurement:**
```typescript
class LatencyTracker {
    private rttSamples: number[] = []
    private frameLatencySamples: number[] = []
    private pingSeq = 0
    private pendingPings = new Map<number, number>()  // seq → sendTime

    // Called when frame is rendered
    onFrameRendered(serverTxTime: bigint, renderTime: number) {
        // We can't directly compare serverTxTime to renderTime (clock drift)
        // But we CAN track relative changes and detect accumulating delay

        // Track frame arrival rate vs expected rate (60fps = 16.67ms)
        // If frames are arriving slower than expected, we're congested
    }

    sendPing() {
        const seq = this.pingSeq++
        const t1 = performance.now()
        this.pendingPings.set(seq, t1)
        ws.send(encodePing(seq, t1))
    }

    onPong(seq: number, t1: number) {
        const t2 = performance.now()
        const rtt = t2 - t1
        this.rttSamples.push(rtt)
        this.pendingPings.delete(seq)
    }

    // Moving average RTT
    getAverageRtt(): number {
        const recent = this.rttSamples.slice(-10)
        return recent.reduce((a, b) => a + b, 0) / recent.length
    }

    // Detect congestion by tracking queue depth
    getEstimatedBufferDelay(): number {
        // If RTT is stable but video is delayed, frames are buffered
        // Estimate based on decode queue size and frame interval
        return videoDecoder.decodeQueueSize * (1000 / 60)  // ms
    }
}
```

#### End-to-End Latency Estimation (Clock-Drift Resistant)

Since we can't directly compare server and browser clocks, we use **differential measurement**:

```typescript
class E2ELatencyEstimator {
    private baselineServerTime: bigint | null = null
    private baselineBrowserTime: number | null = null

    onFrameReceived(serverTxTime: bigint) {
        const browserRxTime = performance.now()

        if (this.baselineServerTime === null) {
            // First frame establishes baseline
            this.baselineServerTime = serverTxTime
            this.baselineBrowserTime = browserRxTime
            return
        }

        // Calculate expected browser time based on server time delta
        const serverDelta = Number(serverTxTime - this.baselineServerTime) / 1000  // ms
        const expectedBrowserTime = this.baselineBrowserTime + serverDelta

        // Difference is accumulated network latency
        const accumulatedDelay = browserRxTime - expectedBrowserTime

        // If accumulatedDelay grows, congestion is building
        // If it's stable, network is keeping up
        // If it shrinks, we're catching up

        return accumulatedDelay
    }
}
```

**Key insight**: We don't need absolute clock sync. We only need to detect when latency is *increasing* over time, which indicates congestion.

### 2. Frame Dropping When Send Buffer is Full

#### Approach A: Application-Level Queue with Bounded Size

Instead of pushing frames directly to the WebSocket, use a bounded channel:

**Rust implementation (web-server/src/api/stream.rs):**
```rust
use tokio::sync::mpsc;

const MAX_QUEUED_FRAMES: usize = 5;  // ~83ms at 60fps

async fn handle_ws_stream(socket: WebSocket, ...) {
    let (frame_tx, mut frame_rx) = mpsc::channel::<Vec<u8>>(MAX_QUEUED_FRAMES);

    // Frame forwarding task
    let forward_task = spawn(async move {
        let mut frames_dropped = 0u64;
        let mut last_keyframe: Option<Vec<u8>> = None;

        while let Some(frame) = ipc_frame_rx.recv().await {
            let is_keyframe = frame[2] & 0x01 != 0;  // Check flags byte

            if is_keyframe {
                last_keyframe = Some(frame.clone());
            }

            match frame_tx.try_send(frame) {
                Ok(_) => {}
                Err(mpsc::error::TrySendError::Full(_)) => {
                    // Queue is full - network can't keep up
                    frames_dropped += 1;

                    if frames_dropped % 60 == 0 {  // Log every second at 60fps
                        warn!("[WsStream] Dropped {} frames due to congestion", frames_dropped);
                    }

                    // If we're dropping, prioritize keyframes
                    // (could also request a new keyframe from Wolf here)
                }
                Err(mpsc::error::TrySendError::Closed(_)) => break,
            }
        }
    });

    // WebSocket send task
    let send_task = spawn(async move {
        while let Some(frame) = frame_rx.recv().await {
            if ws_tx.send(Message::Binary(frame)).await.is_err() {
                break;
            }
        }
    });
}
```

#### Approach B: Proactive Frame Skipping Based on RTT

If RTT exceeds threshold, drop frames proactively:

```rust
const RTT_THRESHOLD_MS: u64 = 100;  // Drop frames if RTT > 100ms
const FRAME_SKIP_RATIO: u32 = 2;    // Keep every Nth frame

async fn forward_with_adaptive_skip(
    mut frame_rx: Receiver<Frame>,
    ws_tx: Sender<Message>,
    rtt: Arc<AtomicU64>,
) {
    let mut frame_count = 0u32;
    let mut skip_mode = false;

    while let Some(frame) = frame_rx.recv().await {
        let current_rtt = rtt.load(Ordering::Relaxed);

        if current_rtt > RTT_THRESHOLD_MS {
            skip_mode = true;
        } else if current_rtt < RTT_THRESHOLD_MS / 2 {
            skip_mode = false;  // Recovered
        }

        let is_keyframe = frame.is_keyframe();
        let should_send = is_keyframe || !skip_mode || (frame_count % FRAME_SKIP_RATIO == 0);

        if should_send {
            ws_tx.send(frame.into_message()).await?;
        }

        frame_count = frame_count.wrapping_add(1);
    }
}
```

#### Approach C: TCP Send Buffer Inspection (Linux-specific)

On Linux, we can query the kernel's TCP buffer state:

```rust
use std::os::unix::io::AsRawFd;

fn get_tcp_send_buffer_used(socket: &TcpStream) -> std::io::Result<usize> {
    let fd = socket.as_raw_fd();
    let mut outq: libc::c_int = 0;

    unsafe {
        if libc::ioctl(fd, libc::TIOCOUTQ, &mut outq) == -1 {
            return Err(std::io::Error::last_os_error());
        }
    }

    Ok(outq as usize)
}

// In frame sending loop:
let buffer_used = get_tcp_send_buffer_used(&tcp_socket)?;
let buffer_capacity = get_tcp_send_buffer_capacity(&tcp_socket)?;  // SO_SNDBUF

if buffer_used > buffer_capacity * 80 / 100 {  // >80% full
    // Skip this frame
    continue;
}
```

**Note**: This requires access to the underlying TCP socket, which may not be directly exposed through tungstenite/axum abstractions. May need to use lower-level socket handling.

### 3. Secondary Low-Quality Stream with Orange Border

#### Critical: Frame Decimation with H.264 P-Frames

H.264 uses inter-frame prediction:
- **I-frames (keyframes)**: Complete picture, can be decoded independently
- **P-frames (predicted)**: Only stores differences from the **previous** frame

**The problem with naive frame dropping:**
```
Frame 1 (I) → decode OK
Frame 2 (P) → DROP
Frame 3 (P) → DROP
Frame 4 (P) → DROP
Frame 5 (P) → decode FAILS (references dropped frame 4)
```

Dropping P-frames causes the decoder to fail or produce artifacts (blocky corruption, green/pink glitches, frozen regions) because each P-frame depends on the previous frame in the chain.

**Solution: Request frequent keyframes when decimating**

When switching to low-quality mode:
1. Request a keyframe from Wolf immediately
2. Configure Wolf to send keyframes more frequently (e.g., every 500ms instead of every 2s)
3. After each keyframe, we can safely drop the subsequent P-frames until the next keyframe arrives

**Trade-off**: Keyframes are 5-10x larger than P-frames, so bandwidth doesn't drop proportionally to frame rate reduction.

**Alternative: Separate low-bitrate encode session**

Start a second Moonlight session with Wolf configured for 720p@15fps@2Mbps. This produces properly encoded low-framerate video without the P-frame dependency issues. More resource-intensive but produces better quality.

**Recommended approach for initial implementation:** Use frequent keyframes (approach #1). It's simpler and doesn't require Wolf to support multiple simultaneous encodes.

#### Architecture

```
                              ┌─────────────────────────────┐
                              │      Browser                │
                              │                             │
┌─────────┐                   │  ┌───────────────────────┐ │
│  Wolf   │                   │  │ Primary WS Stream     │ │
│         │──60fps 20Mbps────▶│  │ (high quality)        │ │
│ Encoder │                   │  │                       │ │
│         │                   │  │ ┌───────────────────┐ │ │
│         │──15fps 2Mbps─────▶│  │ │ Fallback WS Stream│ │ │
└─────────┘                   │  │ │ (low quality)     │ │ │
                              │  │ │ Orange border     │ │ │
                              │  │ └───────────────────┘ │ │
                              │  └───────────────────────┘ │
                              └─────────────────────────────┘
```

#### Option A: Server-Side Frame Decimation (Simplest)

The server reuses the same video stream but sends fewer frames to the low-quality connection:

```rust
enum StreamQuality {
    High,  // All frames
    Low,   // Every 4th frame (15fps from 60fps)
}

async fn forward_frames(
    quality: StreamQuality,
    mut frame_rx: Receiver<Frame>,
    ws_tx: Sender<Message>,
) {
    let mut frame_count = 0u32;
    let skip_ratio = match quality {
        StreamQuality::High => 1,
        StreamQuality::Low => 4,
    };

    while let Some(frame) = frame_rx.recv().await {
        let is_keyframe = frame.is_keyframe();

        // Always send keyframes, otherwise send every Nth frame
        if is_keyframe || frame_count % skip_ratio == 0 {
            ws_tx.send(frame.into_message()).await?;
        }

        frame_count = frame_count.wrapping_add(1);
    }
}
```

**WebSocket endpoint:**
```
/api/ws/stream              - High quality (60fps)
/api/ws/stream?quality=low  - Low quality (15fps)
```

#### Option B: Separate Wolf Encoding Sessions (Better Quality)

For true low-quality stream with lower bitrate (not just dropped frames):
- Start a second Moonlight session to Wolf with different encoding parameters
- Requires Wolf/Moonlight to support multiple simultaneous encodes
- More complex but produces better low-quality output

**Not recommended for initial implementation** - the frame decimation approach is simpler and may be sufficient.

#### Frontend Implementation

```typescript
class DualStreamManager {
    private primaryWs: WebSocket
    private fallbackWs: WebSocket | null = null
    private activeStream: 'primary' | 'fallback' = 'primary'
    private latencyTracker: LatencyTracker

    private readonly FALLBACK_RTT_THRESHOLD = 150  // ms
    private readonly RECOVERY_RTT_THRESHOLD = 80   // ms
    private readonly ORANGE_BORDER_STYLE = '4px solid #ff9800'

    constructor(primaryUrl: string, fallbackUrl: string) {
        this.primaryWs = new WebSocket(primaryUrl)
        this.latencyTracker = new LatencyTracker()

        // Pre-establish fallback connection (but don't use it)
        this.fallbackWs = new WebSocket(fallbackUrl)
        this.fallbackWs.binaryType = 'arraybuffer'
    }

    private checkAndSwitchStreams() {
        const rtt = this.latencyTracker.getAverageRtt()
        const bufferDelay = this.latencyTracker.getEstimatedBufferDelay()
        const totalLatency = rtt + bufferDelay

        if (this.activeStream === 'primary' && totalLatency > this.FALLBACK_RTT_THRESHOLD) {
            this.switchToFallback()
        } else if (this.activeStream === 'fallback' && totalLatency < this.RECOVERY_RTT_THRESHOLD) {
            this.switchToPrimary()
        }
    }

    private switchToFallback() {
        console.log('[DualStream] Switching to low-quality stream due to congestion')
        this.activeStream = 'fallback'
        this.canvas.style.border = this.ORANGE_BORDER_STYLE

        // Show indicator
        this.showConnectionWarning('Slow connection - reduced quality')
    }

    private switchToPrimary() {
        console.log('[DualStream] Network recovered, switching to high-quality stream')
        this.activeStream = 'primary'
        this.canvas.style.border = 'none'
        this.hideConnectionWarning()
    }

    private onMessage(source: 'primary' | 'fallback', data: ArrayBuffer) {
        // Only process frames from active stream
        if (source !== this.activeStream) return

        this.handleFrame(new Uint8Array(data))
        this.checkAndSwitchStreams()
    }
}
```

### 4. Screenshot Fallback Mode (Future - Document Only)

When network is extremely degraded (RTT > 500ms or severe packet loss):

```
┌────────────────────────────────────────────────────────────────┐
│                   SCREENSHOT MODE                               │
│                                                                 │
│  - Disable WebSocket video stream                              │
│  - Request screenshots via HTTP at 1fps                        │
│  - Display with red border and warning message                 │
│  - Continue accepting input (queued, best-effort delivery)     │
│  - Periodically probe if connection recovers                   │
│                                                                 │
└────────────────────────────────────────────────────────────────┘
```

**API:**
```
GET /api/ws/screenshot?session_id=xxx&quality=80
→ JPEG image of current frame
```

**Frontend behavior:**
```typescript
class ScreenshotFallback {
    private readonly SCREENSHOT_INTERVAL = 1000  // 1fps
    private readonly RED_BORDER_STYLE = '4px solid #f44336'

    async startScreenshotMode() {
        this.canvas.style.border = this.RED_BORDER_STYLE
        this.showCriticalWarning('Very slow connection - screenshot mode')

        while (this.inScreenshotMode) {
            const img = await this.fetchScreenshot()
            this.drawToCanvas(img)
            await sleep(this.SCREENSHOT_INTERVAL)

            // Periodically try to recover
            if (this.probeCounter++ % 10 === 0) {
                this.probeStreamRecovery()
            }
        }
    }
}
```

**Not implementing now** - this is an extreme degradation mode that may not be needed.

## Additional Creative Solutions

### 5. Temporal Frame Coalescing (Skip Stale Frames)

When multiple frames queue in the browser, skip older ones:

```typescript
class FrameCoalescer {
    private pendingFrames: EncodedVideoChunk[] = []
    private readonly MAX_PENDING = 3

    onFrameReceived(frame: EncodedVideoChunk) {
        this.pendingFrames.push(frame)

        // If too many pending, drop oldest non-keyframes
        while (this.pendingFrames.length > this.MAX_PENDING) {
            const oldest = this.pendingFrames[0]
            if (oldest.type !== 'key') {
                this.pendingFrames.shift()  // Drop oldest
            } else {
                break  // Don't drop keyframes
            }
        }

        this.scheduleRender()
    }

    private scheduleRender() {
        requestAnimationFrame(() => {
            // Render only the most recent frame
            const frame = this.pendingFrames.pop()
            if (frame) {
                this.decoder.decode(frame)
                // Clear remaining stale frames
                this.pendingFrames = []
            }
        })
    }
}
```

### 6. Encode-Time Keyframe Requests

When congestion is detected, request Wolf to produce more keyframes:

```rust
// New control message type
0x50 - RequestKeyframe (client → server)

// Server forwards to streamer, which calls Moonlight API:
stream.request_idr_frame()
```

**Why?** When frames are dropped, the decoder needs a keyframe to resync. Proactively requesting keyframes reduces "catch-up" time after congestion.

### 7. Quality-of-Service Signaling

Let the client tell the server its network conditions:

```
0x51 - QoSReport (client → server)
┌────────────┬────────────┬────────────┬────────────┐
│ Type (1B)  │ RTT (4B)   │ Loss% (1B) │ Queue (2B) │
│ 0x51       │ ms         │ 0-100      │ frames     │
└────────────┴────────────┴────────────┴────────────┘
```

Server can use this to:
- Adjust frame decimation ratio
- Request keyframes
- Log for diagnostics
- Potentially restart stream with different parameters

### 8. WebTransport / HTTP/3 (Future Investigation)

**WebTransport** provides QUIC-based transport with:
- **Unreliable datagrams**: Like UDP, packets can be dropped without blocking
- **Ordered streams**: Like TCP, for reliable delivery when needed
- **Unordered streams**: Deliver as soon as received

For video streaming:
- Send video frames as unreliable datagrams (can drop on congestion)
- Send input as ordered stream (must be reliable)
- Send control messages as ordered stream

**Status**: Supported in Chrome 97+, Firefox 114+, not yet in Safari.

**Trade-offs**:
- Requires HTTP/3 support at proxy level (not all L7 proxies support yet)
- More complex protocol handling
- But solves the TCP head-of-line blocking problem fundamentally

**Recommendation**: Investigate after WebSocket-based solutions are deployed.

### 9. Predictive Frame Skipping Based on Buffer Trends

Instead of reacting to congestion, predict it:

```rust
struct CongestionPredictor {
    buffer_samples: VecDeque<(Instant, usize)>,  // (time, buffer_level)

    fn predict_congestion(&self) -> bool {
        if self.buffer_samples.len() < 10 {
            return false;
        }

        // Calculate buffer growth rate
        let (t1, b1) = self.buffer_samples.front().unwrap();
        let (t2, b2) = self.buffer_samples.back().unwrap();
        let elapsed = t2.duration_since(*t1).as_secs_f64();
        let growth_rate = (*b2 as f64 - *b1 as f64) / elapsed;

        // If buffer is growing, congestion is building
        growth_rate > 1000.0  // bytes/sec threshold
    }
}
```

## Implementation Status

### ✅ Completed (Phase 1 - Measurement)

1. **Ping/Pong RTT measurement**
   - Added message types `0x40` (Ping) and `0x41` (Pong) to `ws_protocol.rs`
   - Server responds to pings immediately with pong (echoing client timestamp)
   - Browser sends ping every 1 second, calculates RTT from response
   - Moving average of last 10 samples for stable measurement

2. **Stats overlay with RTT**
   - RTT displayed in "Stats for Nerds" panel when in WebSocket mode
   - Shows warning indicator when RTT > 150ms

3. **High latency warning banner**
   - Orange warning banner appears when RTT exceeds 150ms threshold
   - Displays current RTT value
   - Auto-hides when latency recovers

### ✅ Completed (Phase 3 - Dual-Stream Adaptive Quality)

4. **DualStreamManager**
   - Created `dual-stream-manager.ts` that manages two parallel WebSocket streams
   - High-quality stream: 60fps @ 20Mbps
   - Low-quality stream: 15fps @ 5Mbps (same resolution to avoid scaling issues)
   - Both streams connect to Wolf independently - Wolf handles dual encoding
   - Automatic switching based on RTT:
     - Switch to fallback when RTT > 150ms
     - Switch back to primary when RTT < 80ms
     - Minimum 5 second interval between switches

5. **UI Integration**
   - Adaptive Quality toggle button in toolbar (Speed icon)
   - Orange border on canvas when on low-quality stream
   - Warning banner shows "Slow connection - reduced to 15fps"
   - Stats overlay shows active stream and both stream RTTs

6. **Files created/modified:**
   - `frontend/src/lib/moonlight-web-ts/stream/dual-stream-manager.ts` (new)
   - `frontend/src/components/external-agent/MoonlightStreamViewer.tsx` (updated)

### 🔲 Not Yet Implemented

- Frame dropping when send buffer is full (Phase 2) - Skipped in favor of dual-stream
- QoS reporting from client to server (Phase 2)
- Keyframe requests on congestion (Phase 4)
- Screenshot fallback mode (Phase 4)
- WebTransport investigation (Phase 4)

## Implementation Priority

### Phase 1: Measurement (Essential) ✅ COMPLETE

1. **Ping/Pong RTT measurement**
   - Add message types 0x40/0x41
   - Implement in TypeScript and Rust
   - Display in stats overlay

2. **Accumulated delay detection**
   - Track frame arrival times vs expected
   - Calculate drift from baseline
   - Log and display congestion state

### Phase 2: Server-Side Frame Dropping (High Priority)

3. **Bounded frame queue**
   - Add mpsc channel with MAX_QUEUED_FRAMES
   - Drop frames when queue is full
   - Prioritize keyframes

4. **QoS reporting**
   - Client sends RTT/queue depth to server
   - Server logs for diagnostics

### Phase 3: Dual Stream Mode (Medium Priority)

5. **Low-quality stream endpoint**
   - `/api/ws/stream?quality=low`
   - Server-side frame decimation (15fps)

6. **Frontend stream switching**
   - Detect high latency
   - Switch to low-quality stream
   - Orange border indicator

### Phase 4: Advanced Features (Future)

7. **Keyframe requests on congestion**
8. **Screenshot fallback mode**
9. **WebTransport investigation**

## Files to Modify

**moonlight-web-stream (Rust):**
- `common/src/ws_protocol.rs` - Add Ping/Pong, QoS message types
- `web-server/src/api/stream.rs` - Bounded queue, frame dropping, quality param
- `streamer/src/main.rs` - RTT tracking, QoS handling

**helix (TypeScript):**
- `frontend/src/lib/moonlight-web-ts/stream/websocket-stream.ts` - Latency tracking, dual stream
- `frontend/src/lib/moonlight-web-ts/stream/latency-tracker.ts` - New file
- `frontend/src/components/external-agent/MoonlightStreamViewer.tsx` - Orange border, stats

## Reproducing the Issue in Development

### Method 1: Chrome DevTools Network Throttling (Simplest)

Chrome's DevTools can throttle WebSocket connections:

1. **Open DevTools** (F12 or Cmd+Opt+I)
2. **Go to Network tab**
3. **Click the throttling dropdown** (defaults to "No throttling")
4. **Select or create a custom profile**:
   - "Slow 3G" (400ms latency, 400kbps down) - will cause severe buffering
   - "Fast 3G" (100ms latency, 1.5Mbps down) - moderate degradation

**To create custom profile for video streaming:**
1. Click throttling dropdown → "Add..."
2. Create "Congested Network":
   - Download: 5000 Kbps (5 Mbps - less than 20Mbps stream)
   - Upload: 1000 Kbps
   - Latency: 100 ms

**Limitation**: Chrome DevTools throttling affects ALL network traffic in that tab, including input. It's a good approximation but not perfect for simulating asymmetric congestion.

### Method 2: Chrome WebSocket Frame Delay (Better for Testing)

For more precise WebSocket testing, use a Chrome extension or intercept in code:

```typescript
// Temporary dev-mode delay injection
const SIMULATED_DELAY_MS = 200;

// In websocket-stream.ts, wrap onmessage:
if (process.env.NODE_ENV === 'development' && window.SIMULATE_LATENCY) {
    const originalOnMessage = this.ws.onmessage;
    this.ws.onmessage = (event) => {
        setTimeout(() => originalOnMessage.call(this.ws, event), SIMULATED_DELAY_MS);
    };
}
```

### Method 3: tc (Traffic Control) on Server (Most Realistic)

If running locally with Docker:

```bash
# Find the container's network interface
CONTAINER_ID=$(docker ps -qf "name=moonlight")
VETH=$(docker exec $CONTAINER_ID cat /sys/class/net/eth0/iflink | xargs -I {} grep -l {} /sys/class/net/veth*/ifindex | xargs dirname | xargs basename)

# Add latency and bandwidth limit
sudo tc qdisc add dev $VETH root netem delay 100ms 20ms rate 5mbit

# Verify
tc qdisc show dev $VETH

# Remove when done
sudo tc qdisc del dev $VETH root
```

**Presets for testing:**

```bash
# Moderate congestion (noticeable delay)
sudo tc qdisc add dev $VETH root netem delay 50ms 10ms rate 10mbit

# Severe congestion (stream falls behind)
sudo tc qdisc add dev $VETH root netem delay 100ms 30ms rate 3mbit

# Packet loss (video artifacts)
sudo tc qdisc add dev $VETH root netem delay 50ms loss 5%

# Jitter (variable latency)
sudo tc qdisc add dev $VETH root netem delay 50ms 100ms distribution normal
```

### Method 4: Server-Side Artificial Delay (Best for Development)

Add a debug mode to the streaming server:

```rust
// In web-server/src/api/stream.rs

const DEBUG_FRAME_DELAY_MS: u64 = std::env::var("WS_FRAME_DELAY_MS")
    .ok()
    .and_then(|s| s.parse().ok())
    .unwrap_or(0);

async fn forward_frame(ws_tx: &mut SplitSink<...>, frame: Vec<u8>) {
    if DEBUG_FRAME_DELAY_MS > 0 {
        tokio::time::sleep(Duration::from_millis(DEBUG_FRAME_DELAY_MS)).await;
    }
    ws_tx.send(Message::Binary(frame)).await.ok();
}
```

Then run with:
```bash
WS_FRAME_DELAY_MS=100 cargo run --bin web-server
```

### Method 5: Browser DevTools WebSocket Inspector

To observe WebSocket frame timing without modifying code:

1. Open DevTools → Network tab
2. Filter by "WS" (WebSocket)
3. Click on the WebSocket connection
4. Go to "Messages" sub-tab
5. Observe frame arrival times and sizes

Look for:
- **Increasing time gaps** between frames = congestion building
- **Burst of frames** after gap = TCP buffer catch-up
- **Frame size vs time** = effective throughput

### Automated Testing Script

```typescript
// tests/congestion-simulation.ts

describe('WebSocket Congestion Handling', () => {
    let stream: WebSocketStream;

    beforeEach(() => {
        // Enable latency simulation
        window.SIMULATE_LATENCY = true;
        window.SIMULATED_DELAY_MS = 0;
    });

    it('should detect increasing RTT', async () => {
        stream = new WebSocketStream(testUrl);
        await stream.connect();

        // Wait for baseline
        await sleep(1000);
        const baselineRtt = stream.latencyTracker.getAverageRtt();

        // Increase simulated delay
        window.SIMULATED_DELAY_MS = 200;

        // Wait for detection
        await sleep(2000);
        const congestedRtt = stream.latencyTracker.getAverageRtt();

        expect(congestedRtt).toBeGreaterThan(baselineRtt + 150);
    });

    it('should switch to low-quality stream on congestion', async () => {
        const dualStream = new DualStreamManager(primaryUrl, fallbackUrl);
        await dualStream.connect();

        // Simulate congestion
        window.SIMULATED_DELAY_MS = 300;

        // Wait for switch
        await waitFor(() => dualStream.activeStream === 'fallback', 5000);

        expect(dualStream.canvas.style.border).toContain('orange');
    });

    it('should recover when congestion clears', async () => {
        const dualStream = new DualStreamManager(primaryUrl, fallbackUrl);
        await dualStream.connect();

        // Simulate congestion then recovery
        window.SIMULATED_DELAY_MS = 300;
        await sleep(3000);

        window.SIMULATED_DELAY_MS = 0;
        await waitFor(() => dualStream.activeStream === 'primary', 5000);

        expect(dualStream.canvas.style.border).not.toContain('orange');
    });
});
```

### Visual Testing Checklist

Manual testing steps:

1. **Baseline (no throttling)**:
   - [ ] Video plays smoothly at 60fps
   - [ ] Input feels instantaneous
   - [ ] RTT shown in stats overlay is <50ms

2. **Moderate congestion (100ms latency, 10Mbps)**:
   - [ ] Video still plays but may stutter slightly
   - [ ] RTT increases to ~100-150ms
   - [ ] No orange border yet

3. **Severe congestion (200ms latency, 3Mbps)**:
   - [ ] Video clearly falls behind (type text, see delay)
   - [ ] RTT exceeds 150ms threshold
   - [ ] Orange border appears (if dual stream implemented)
   - [ ] Stats show frames being dropped

4. **Recovery (remove throttling)**:
   - [ ] Video catches up within a few seconds
   - [ ] RTT returns to baseline
   - [ ] Orange border disappears
   - [ ] No persistent artifacts

## Testing Plan

1. **Simulate slow network** using Chrome DevTools or tc:
   ```bash
   # Using tc on Linux (most realistic)
   sudo tc qdisc add dev eth0 root netem delay 100ms 20ms rate 5mbit

   # Or use Chrome DevTools throttling for quick tests
   ```

2. **Verify RTT measurement** matches actual network delay

3. **Verify frame dropping** kicks in when buffer fills

4. **Verify dual stream switching** on degraded connection

5. **Measure improvement** in user-perceived latency during congestion

## Metrics to Track

- **RTT (ms)**: Ping/Pong round-trip time
- **Buffer delay (ms)**: Estimated queue depth × frame interval
- **Frames dropped (server)**: Per second
- **Frames skipped (client)**: Per second
- **Stream switches**: Count of high→low and low→high transitions
- **Time in degraded mode**: Percentage of session time on low-quality stream

## Summary

The core insight is that **we're fighting against TCP's reliable delivery guarantees** when streaming real-time video. Unlike WebRTC/RTP which can drop packets, TCP buffers them, causing latency to accumulate.

Our solutions:
1. **Detect** congestion early (RTT + differential timing)
2. **Drop** frames at the server before they enter the TCP buffer
3. **Degrade** to lower quality stream when congestion is severe
4. **Recover** automatically when network improves

This maintains the simplicity of WebSocket transport while providing the adaptive behavior needed for variable network conditions.

## References

- [WebCodecs API](https://developer.mozilla.org/en-US/docs/Web/API/WebCodecs_API)
- [WebTransport API](https://developer.mozilla.org/en-US/docs/Web/API/WebTransport_API)
- [TCP_INFO socket option](https://man7.org/linux/man-pages/man7/tcp.7.html)
- [GStreamer adaptive streaming](https://gstreamer.freedesktop.org/documentation/tutorials/playback/step-9.html)
- [Current WebSocket streaming implementation](./2025-12-03-websocket-only-streaming.md)
