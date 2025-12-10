# Video Frame Batching to Handle TCP Bufferbloat

**Date:** 2025-12-10
**Status:** Investigation / Design
**Author:** Helix Team

---

## Problem Statement

When streaming 60fps video over WebSocket (one packet every ~16.67ms), the connection becomes laggy even on fast internet connections with low ping (17-72ms). The video gets progressively more delayed, causing a poor user experience.

### Symptoms

- Stream starts smoothly
- After some time, latency increases dramatically
- Ping remains low (17-72ms) but video is many seconds behind
- This occurs even on very fast internet connections

### Root Cause: TCP Bufferbloat

WebSocket runs over TCP, which has built-in flow control and reliability. When frames arrive faster than they can be sent:

1. TCP buffers the data in kernel send buffers
2. TCP congestion control kicks in, reducing send rate
3. Frames queue up, causing latency to increase
4. TCP doesn't drop data - it retransmits, adding more latency

This is known as "bufferbloat" - the buffers are working correctly but causing unacceptable latency.

**Key insight:** WebSocket/TCP does NOT require waiting for acknowledgment before sending the next frame. It's full-duplex with asynchronous flow control. The issue is that TCP's reliability guarantees (no data loss, in-order delivery) conflict with real-time streaming requirements.

### Hypothesis Under Test: Message Count vs Data Volume

**Initial intuition:** The number of WebSocket messages per second (60 for 60fps) is what causes latency buildup.

**Counter-analysis:** This intuition may not hold because:
- WebSocket frame overhead is 2-14 bytes per message (negligible vs 10-50KB video frames)
- 60 write() syscalls/sec is trivial for modern kernels
- Batching 6 frames into 1 message reduces message count but not data volume
- Same total bytes = same bandwidth requirement = same TCP congestion behavior

**What actually causes buildup:**
1. Momentary congestion → packet loss → TCP retransmits → delay
2. While TCP retransmits, new frames keep arriving from encoder
3. Large kernel send buffers (often 4MB+ on Linux) queue them silently
4. TCP guarantees in-order delivery - old frames MUST arrive before new ones
5. By the time network recovers, seconds of frames are queued

**What would actually help:**
- Adaptive bitrate (reduce data volume during congestion)
- Frame dropping (detect congestion, stop sending stale frames)
- UDP transport (WebRTC/WebTransport can drop packets natively)

**Why we're testing batching anyway:**
- Need empirical data to validate or invalidate the intuition
- Batching infrastructure could later enable frame dropping if detection works
- May reveal unexpected behavior in the Azure LB / RevDial path
- Stats-for-nerds will show whether batching correlates with latency

---

## Current Architecture

### Video Streaming Path (WebSocket-only, L7 compatible)

This focuses on the WebSocket-only streaming mode used when WebRTC is not available (L7 load balancers, enterprise proxies):

```
┌─────────────┐    ┌─────────────┐    ┌─────────────┐    ┌─────────────┐    ┌─────────────┐
│   Wolf      │───>│ Moonlight   │───>│   RevDial   │───>│  Helix API  │───>│  Frontend   │
│ (Encoder)   │    │    Web      │    │   Tunnel    │    │   (Proxy)   │    │  (Browser)  │
└─────────────┘    └─────────────┘    └─────────────┘    └─────────────┘    └─────────────┘
      |                  |                  |                  |                  |
  H264/265          WebSocket           TCP/TLS            Azure LB         WebCodecs
  Frames            Binary                                 (L7)             Decoder
```

**Note:** The bufferbloat can occur at multiple points:
1. Moonlight Web → RevDial (local, low latency)
2. RevDial → Helix API (container networking)
3. Helix API → Azure LB (datacenter network)
4. Azure LB → Internet → Client (where most latency is)

### Current Frontend Implementation (`websocket-stream.ts`)

The frontend already has:
- RTT measurement via Ping/Pong (lines 824-916)
- Stats tracking: FPS, bitrate, frames decoded/dropped (lines 1006-1028)
- Binary protocol parsing for video frames (lines 624-683)

### Current WebSocket Protocol (`moonlight-web-stream/moonlight-web/common/src/ws_protocol.rs`)

**VideoFrame message format:**
```
┌────────────┬────────────┬────────────┬────────────┬────────────┬───────────┐
│ Type (1B)  │ Codec (1B) │ Flags (1B) │ PTS (8B)   │ Width (2B) │ Height(2B)│
├────────────┴────────────┴────────────┴────────────┴────────────┴───────────┤
│ NAL Unit Data (remaining bytes)                                            │
└─────────────────────────────────────────────────────────────────────────────┘
```

Key fields:
- **PTS (8 bytes)**: Presentation timestamp in microseconds - already present!
- **Flags**: Includes keyframe indicator

The protocol already has timestamps for each frame, which is essential for the batching approach.

---

## Proposed Solution: Adaptive Frame Batching

Instead of sending one frame per WebSocket message, batch multiple frames together when TCP buffers are filling up. The frontend plays them in sequence based on PTS.

### New Message Types

```rust
/// Batched video frames message (server → client)
/// Sent when frames are backing up to reduce overhead
#[repr(u8)]
pub enum WsMessageType {
    // ... existing types ...
    VideoBatch = 0x03,  // NEW: Multiple frames in one message
}

/// Video batch format:
/// ┌────────────┬────────────┬──────────────────────────────────────┐
/// │ Type (1B)  │ Count (2B) │ Frame 1 (with length prefix)         │
/// │ 0x03       │            │ ┌────────────┬─────────────────────┐ │
/// │            │            │ │ Length(4B) │ VideoFrame bytes    │ │
/// │            │            │ └────────────┴─────────────────────┘ │
/// │            │            │ Frame 2...                           │
/// │            │            │ Frame N...                           │
/// └────────────┴────────────┴──────────────────────────────────────┘
```

### Detection of Backup Condition

Detecting when TCP buffers are backing up is challenging, especially through the multi-hop path:

```
Sandbox (Moonlight Web) → RevDial → Helix API → Azure LB → Internet → Azure LB → Client WebSocket
```

**Option A: WebSocket Write Latency (Server-side, Moonlight Web)**

Measure how long `write()` takes. When TCP send buffer is full, `write()` blocks:

```rust
// In moonlight-web-stream/moonlight-web/web-server/src/api/stream.rs

struct AdaptiveSender {
    ws: WebSocket,
    batch_threshold_ms: u64,    // e.g., 50ms
    pending_frames: Vec<VideoFrame>,
    last_write_duration: Duration,
}

impl AdaptiveSender {
    async fn send_frame(&mut self, frame: VideoFrame) {
        let start = Instant::now();

        if self.last_write_duration.as_millis() > self.batch_threshold_ms as u128 {
            // TCP buffer is backing up - start batching
            self.pending_frames.push(frame);

            // Send batch when we have enough or timer expires
            if self.pending_frames.len() >= 10 || timer_expired {
                self.send_batch().await;
            }
        } else {
            // Normal mode - send immediately
            self.ws.send(frame.encode()).await;
        }

        self.last_write_duration = start.elapsed();
    }
}
```

**Pros:** Simple, no OS-specific code, works through RevDial
**Cons:** Reactive (only detects backup after it occurs), subject to scheduling jitter

**Option B: RTT Measurement (Already in Protocol)**

The Moonlight protocol already has Ping/Pong for RTT measurement. Use increasing RTT as an early warning:

```rust
// Existing in protocol - can be leveraged
pub enum WsMessageType {
    Ping = 0x05,
    Pong = 0x06,
}

struct CongestionDetector {
    baseline_rtt_us: u64,      // Measured at session start
    current_rtt_us: u64,       // Rolling average
    congestion_threshold: f64, // e.g., 2.0x baseline
}

impl CongestionDetector {
    fn is_congested(&self) -> bool {
        self.current_rtt_us > (self.baseline_rtt_us as f64 * self.congestion_threshold) as u64
    }
}
```

**Pros:** Proactive (detects congestion before backup is severe), already implemented
**Cons:** Measures end-to-end latency (includes Azure LBs, Internet), may false-positive on network jitter

**Option C: Raw TCP Buffer Inspection (Linux-specific)**

On Linux, use `getsockopt(TCP_INFO)` to read kernel TCP state:

```rust
use std::os::unix::io::AsRawFd;
use libc::{getsockopt, tcp_info, SOL_TCP, TCP_INFO};

fn get_tcp_buffer_state(socket: &TcpStream) -> Option<TcpBufferState> {
    let fd = socket.as_raw_fd();
    let mut info: tcp_info = unsafe { std::mem::zeroed() };
    let mut len = std::mem::size_of::<tcp_info>() as libc::socklen_t;

    let result = unsafe {
        getsockopt(fd, SOL_TCP, TCP_INFO,
                   &mut info as *mut _ as *mut libc::c_void,
                   &mut len)
    };

    if result == 0 {
        Some(TcpBufferState {
            // Bytes in send queue
            unacked_bytes: info.tcpi_unacked * info.tcpi_snd_mss as u32,
            // Current congestion window
            cwnd_bytes: info.tcpi_snd_cwnd * info.tcpi_snd_mss as u32,
            // RTT in microseconds
            rtt_us: info.tcpi_rtt,
            // Retransmits (congestion indicator)
            retransmits: info.tcpi_retransmits,
        })
    } else {
        None
    }
}

// Detect backup: unacked approaching cwnd = buffer pressure
fn is_buffer_backing_up(state: &TcpBufferState) -> bool {
    let buffer_usage = state.unacked_bytes as f64 / state.cwnd_bytes as f64;
    buffer_usage > 0.8 // 80% full = danger zone
}
```

**Pros:** Direct kernel visibility, proactive, precise
**Cons:** Linux-only, requires raw socket access, only sees local buffer (not through RevDial tunnel)

**Option D: Channel Queue Depth (Application-level, Current Approach)**

The current code already has queue-based sending in `sender.rs`:

```rust
// Current code drops when queue is full
if let Err(err) = sender.blocking_send(sample) {
    warn!("[Stream]: Dropped packet - channel full (queue size: {}): {}",
        self.channel_queue_size, err);
}
```

We could modify this to batch instead of drop:
- Monitor queue depth
- If queue > 30 (half a second at 60fps), start batching
- Send batched frames together

**Pros:** Simple, no OS-specific code
**Cons:** Only detects when queue is already full (too late)

**Option E: Client Feedback (Bidirectional)**

```rust
// Client sends back buffer status
pub struct BufferStatusMessage {
    pub buffered_frames: u16,  // How many frames waiting to be decoded
    pub decode_backlog_ms: u32, // Estimated time behind
}
```

Server adjusts batching based on client's decoder backlog.

**Pros:** Measures true end-to-end state, accounts for all hops
**Cons:** Adds latency (feedback loop delay), requires frontend changes

### Recommended Detection Strategy

Use a **hybrid approach** combining multiple signals:

1. **Primary:** RTT measurement (already exists, proactive)
2. **Secondary:** Write latency monitoring (simple, reactive)
3. **Tertiary:** Client feedback (accurate, but slower feedback loop)

```rust
struct CongestionState {
    rtt_ratio: f64,           // current_rtt / baseline_rtt
    write_latency_ms: u64,    // Time to write last frame
    client_buffer_ms: u32,    // Reported by client (if available)
}

fn should_batch(state: &CongestionState) -> bool {
    // Any signal indicating congestion triggers batching
    state.rtt_ratio > 2.0
        || state.write_latency_ms > 50
        || state.client_buffer_ms > 100
}
```

### Azure Load Balancer Considerations

The path includes Azure Application Gateway / Load Balancer at two points:
- Sandbox → Helix API (internal)
- Client → Helix API (external, public internet)

**Implications:**
- RTT measurements include ALB processing time (typically 1-5ms each)
- ALB has its own TCP buffers that can add latency
- ALB idle timeout may close connections (mitigated by keepalives, commit cba4129dd)
- Cannot inspect ALB's internal buffer state

**Mitigations:**
- Measure RTT baseline after connection established (includes ALB latency)
- Use relative RTT changes rather than absolute values
- Consider client-side feedback as ground truth

### Frontend Handling

The frontend needs to handle both individual frames and batched frames:

```typescript
// In MoonlightStreamViewer.tsx or similar

class FramePlayer {
    private frameQueue: VideoFrame[] = [];
    private lastPts: number = 0;

    handleMessage(data: ArrayBuffer) {
        const view = new DataView(data);
        const messageType = view.getUint8(0);

        if (messageType === 0x03) { // VideoBatch
            const frameCount = view.getUint16(1);
            let offset = 3;

            for (let i = 0; i < frameCount; i++) {
                const frameLength = view.getUint32(offset);
                offset += 4;
                const frameData = data.slice(offset, offset + frameLength);
                offset += frameLength;

                const frame = VideoFrame.decode(frameData);
                this.frameQueue.push(frame);
            }

            // Play frames in sequence based on PTS
            this.playFramesInOrder();
        } else if (messageType === 0x01) { // VideoFrame
            const frame = VideoFrame.decode(data);
            this.frameQueue.push(frame);
            this.playFramesInOrder();
        }
    }

    playFramesInOrder() {
        // Sort by PTS and play
        this.frameQueue.sort((a, b) => a.pts - b.pts);

        while (this.frameQueue.length > 0) {
            const frame = this.frameQueue[0];

            // Check if it's time to display this frame
            if (frame.pts <= this.getCurrentPlaybackTime()) {
                this.frameQueue.shift();
                this.decoder.decode(frame);
            } else {
                // Not time yet, schedule for later
                break;
            }
        }
    }
}
```

---

## Alternative Approaches

### 1. Adaptive Bitrate (Already Available)

Wolf supports adaptive bitrate encoding. When congestion is detected, reduce quality:
- Lower resolution
- Lower frame rate
- Higher compression

This works but reduces quality. Batching preserves quality while handling transient congestion.

### 2. Frame Skipping

Drop non-keyframes when backing up. Problem:
- Can't decode P/B frames without preceding frames
- Causes visual corruption until next keyframe (GOP)
- Already partially implemented but causes glitches

### 3. Separate Control/Data Connections

Use separate TCP connections for control (low latency) and data (high throughput). The control connection stays responsive while data can buffer.

**Current state:** Already have separate paths:
- RevDial control connection (for signaling)
- Moonlight WebSocket (for streaming)

### 4. WebTransport (Future)

WebTransport provides UDP-like datagrams over QUIC:
- Can mark frames as droppable
- Server can drop old frames when congested
- True real-time streaming semantics

Requires:
- Browser support for WebTransport (available in Chrome/Edge)
- QUIC support in our infrastructure
- Moonlight Web changes

### 5. Server-Sent Events (SSE) Transport (Future Investigation)

Empirical observation: LLM text streaming over WebSocket had a maximum message rate issue that was fixed by switching to SSE. This suggests proxies and L7 load balancers may handle SSE more efficiently than WebSocket.

**Why SSE might perform better:**
- SSE is native HTTP - proxies understand it deeply and optimize for it
- HTTP/2 multiplexing provides stream-level flow control independent of TCP
- SSE servers explicitly flush after each event (vs WebSocket library buffering)
- Azure Application Gateway and similar L7 proxies are optimized for HTTP patterns
- WebSocket is treated as an opaque bytestream after the upgrade handshake

**RevDial context:** The sandbox-to-API tunnel uses RevDial, which hijacks a WebSocket connection and adapts it to a `net.Conn` interface for tunneling TCP (similar to minimal SOCKS5). This means video frames go: Moonlight WebSocket → RevDial (hijacked WebSocket tunneling TCP) → Helix API → Azure LB → Client. The nested WebSocket-over-WebSocket path may have different buffering characteristics than a direct SSE stream.

**Implementation approach:**
- Video frames sent as binary base64 or raw chunks in SSE events
- Input sent back via separate POST requests or a parallel WebSocket
- Could use HTTP/2 server push for lower latency

**Trade-offs:**
- SSE is unidirectional (server→client only), need separate channel for input
- Base64 encoding adds ~33% overhead for binary data
- Less browser API support for binary SSE (would need custom parsing)

Worth investigating if batching alone doesn't solve the latency issues.

---

## Implementation Plan

### Phase 1: Server-Side Detection + Batching (Moonlight Web) ✅ IMPLEMENTED

1. **Add write latency monitoring** to WebSocket sender ✅
2. **Implement batching logic** when latency exceeds threshold ✅
3. **Add VideoBatch message type** to ws_protocol.rs ✅
4. **Test with simulated network conditions**

### Phase 2: Frontend Handling ✅ IMPLEMENTED

1. **Add VideoBatch decoder** to frontend ✅
2. **Implement frame queue** with PTS-based ordering (simplified - WebSocket guarantees order)
3. **Add jitter buffer** to smooth playback (not needed - TCP handles this)
4. **Test end-to-end**

### Phase 3: Adaptive Optimization

1. **Add client feedback** on buffer status
2. **Dynamic threshold adjustment** based on network conditions
3. **Graceful fallback** to frame dropping when batching isn't enough

---

## Implementation Notes (2025-12-10)

### Hysteresis for Mode Switching

To prevent oscillation between batching and non-batching modes, the implementation uses hysteresis with separate thresholds:

```rust
const BATCH_ENTER_THRESHOLD_MS: u128 = 50;  // Enter batching if write takes >50ms
const BATCH_EXIT_THRESHOLD_MS: u128 = 20;   // Exit batching only if write drops <20ms
const MAX_BATCH_SIZE: usize = 6;            // Max frames per batch (~100ms at 60fps)
```

This creates a "dead zone" between 20-50ms where the mode stays unchanged, preventing rapid switching that would cause inconsistent behavior.

### Key Implementation Details

1. **Batching state** is tracked per-session in `ws_stream_handler()`
2. **Keyframes flush** pending batch before being added (GOP alignment)
3. **Write timing** is measured with `Instant::now()` around `session.binary()`
4. **Stats** track batched vs individual frames for visibility in stats-for-nerds

---

## Key Files to Modify

| Component | File | Changes |
|-----------|------|---------|
| Protocol | `moonlight-web-stream/...ws_protocol.rs` (external) | Add VideoBatch message type (0x03) |
| Server | `moonlight-web-stream/.../stream.rs` (external) | Add congestion detection + batching logic |
| Frontend | `frontend/src/lib/moonlight-web-ts/stream/websocket-stream.ts` | Handle VideoBatch, add stats tracking |
| Stats UI | `frontend/src/components/external-agent/MoonlightStreamViewer.tsx` | Display batching stats in stats overlay |
| Proxy | `api/pkg/server/moonlight_proxy.go` | Pass through (no changes needed) |

---

## Stats for Nerds - Batching Visibility

Add batching statistics to the existing stats display so users can see when batching is active:

### New Stats Fields

```typescript
// In websocket-stream.ts getStats()
getStats(): {
  // ... existing fields ...
  fps: number
  rttMs: number

  // NEW: Batching stats
  batchedFramesReceived: number    // Total frames received in batches
  batchesReceived: number          // Total number of batch messages
  avgBatchSize: number             // Average frames per batch (0 = no batching)
  batchingActive: boolean          // True if recent frames were batched
  batchingRatio: number            // % of frames that arrived batched (0-100)
}
```

### Frontend Display

In the stats overlay (stats-for-nerds mode):

```
FPS: 60 | RTT: 45ms | Bitrate: 15.2 Mbps
Batching: 23% (avg 3.2 frames/batch)   ← NEW
Decoded: 14520 | Dropped: 12
```

When batching is not active:
```
Batching: OFF
```

### Protocol Notes

No sequence number is needed in the VideoBatch message:
- WebSocket runs over TCP, which guarantees reliable in-order delivery
- On reconnection, the frontend reinitializes the entire stream from scratch
- Statistics can be calculated from frame count alone (no need to detect drops)

The simple format from the Proposed Solution section is sufficient:
```
/// ┌────────────┬────────────┬──────────────────────────────────────┐
/// │ Type (1B)  │ Count (2B) │ Frame 1 (with length prefix)         │
/// │ 0x03       │            │ ┌────────────┬─────────────────────┐ │
/// │            │            │ │ Length(4B) │ VideoFrame bytes    │ │
/// │            │            │ └────────────┴─────────────────────┘ │
/// └────────────┴────────────┴──────────────────────────────────────┘
```

### Keyframe Handling (RESOLVED)

**Rule: Flush pending batch when keyframe arrives.**

Keyframes (I-frames) are self-contained and start a new GOP. P/B-frames depend on previous frames.

Implementation:
```rust
fn send_frame(&mut self, frame: VideoFrame) {
    if frame.flags.is_keyframe() && !self.pending_batch.is_empty() {
        // Flush pending P/B-frames before starting new GOP
        self.flush_batch();
    }

    if self.should_batch() {
        self.pending_batch.push(frame);
        // Flush on size limit or timer
    } else {
        self.send_immediate(frame);
    }
}
```

This ensures:
- Batches align with GOP boundaries
- Decoder can always start decoding from first frame of batch
- Keyframes never get stuck waiting in a pending batch

---

## Open Questions

1. **Optimal batch threshold?** Need to balance latency vs. overhead
2. **Maximum batch size?** Too large = more latency, too small = overhead
3. ~~**How to handle keyframes?**~~ RESOLVED: Flush batch on keyframe arrival
4. **Audio sync?** Audio must stay synchronized with batched video
5. ~~**GOP boundaries?**~~ RESOLVED: Batches align with GOPs via keyframe flush rule
6. **Stats granularity?** Should we show batching stats per-second or as rolling average?

---

## Related Issues

- TCP keepalives added in commit cba4129dd (separate issue)
- Screenshot mode fallback (works around the issue by using JPEG polling)

---

## References

- [Bufferbloat.net](https://www.bufferbloat.net/) - Understanding bufferbloat
- [WebTransport](https://web.dev/webtransport/) - Future alternative to WebSocket
- Frontend WebSocket Stream: `frontend/src/lib/moonlight-web-ts/stream/websocket-stream.ts`
- Moonlight Protocol: `moonlight-web-stream/moonlight-web/common/src/ws_protocol.rs` (external repo)
- Helix Moonlight Proxy: `api/pkg/server/moonlight_proxy.go`
