# Direct Video Streaming (Wolf/Moonlight-Free)

**Date:** 2026-01-09
**Status:** Design
**Author:** Claude (with Luke)

## Overview

This design proposes streaming video directly from the desktop container to the browser via WebSocket, completely bypassing Wolf and Moonlight. This mirrors the existing input WebSocket pattern (browser → container) but in reverse (container → browser).

## Current Architecture (Wolf + Moonlight)

```
GNOME ScreenCast (PipeWire)
    ↓ DMA-BUF/SHM frames
Wolf pipewirezerocopysrc / shmsrc
    ↓ CUDA upload + NVENC
Wolf H.264 encoder
    ↓ RTP/RTSP
Moonlight Web (WebRTC)
    ↓ WebRTC
Browser <video> element
```

**Problems with current approach:**
1. Cross-container PipeWire authorization issues (streams stuck at Paused)
2. Complex Wolf + Moonlight dependency chain
3. WebRTC overhead for simple streaming use case
4. Difficult to debug multi-component pipeline

## Existing Frontend Infrastructure (REUSE)

The frontend already has complete WebSocket and SSE video streaming implementations:

### WebSocketStream (`frontend/src/lib/moonlight-web-ts/stream/websocket-stream.ts`)
- **1900+ lines** of production-ready code
- WebCodecs VideoDecoder for H.264/H.265/AV1
- Input handling (keyboard, mouse, wheel, touch)
- Ping/pong RTT measurement
- Adaptive bitrate detection
- Reconnection with exponential backoff
- Stats tracking (FPS, bitrate, latency, decode queue)

### SseStream (`frontend/src/lib/moonlight-web-ts/stream/sse-stream.ts`)
- Server-Sent Events for unidirectional video
- Base64-encoded frames in JSON
- Can pair with WebSocket for input (separate channels)
- Simpler implementation for video-only

### Video codec detection (`frontend/src/lib/moonlight-web-ts/stream/video.ts`)
- WebCodecs hardware decoder probing
- Fallback to software decoding
- Codec capability bits for server negotiation

**Key insight:** We should REUSE these existing components, not create new ones.

## Proposed Architecture (Direct WebSocket)

**Key insight:** Move the WebSocket protocol handling from Helix API to screenshot-server.
Helix API becomes a pure proxy, just like it already does for input.

```
Browser
    ↓ WebSocket connect
EXISTING WebSocketStream class  <-- REUSE (no changes!)
    ↓ /api/v1/sessions/{id}/stream
Helix API (pure WebSocket proxy)
    ↓ Proxy to container:9876/ws/stream
screenshot-server (NEW: ws_stream.go)
    ↓ Implements full WebSocket protocol (init, video, audio, input, ping/pong)
    ↓ Input: forward to GNOME Mutter D-Bus
    ↓ Video: GStreamer pipewiresrc → nvh264enc → WebSocket frames
GNOME ScreenCast (PipeWire)
```

**This approach:**
- Frontend code: **NO CHANGES** (uses existing WebSocketStream)
- Helix API: **PURE PROXY** (like /api/v1/sessions/{id}/input already)
- Wolf/Moonlight: **BYPASSED ENTIRELY**
- screenshot-server: **NEW** - implements the WebSocket binary protocol

**Critical benefit: ONE protocol, not two.**

If we invented a new protocol between container→API and another API→frontend, we'd have:
- Protocol A: screenshot-server → Helix API (custom encoding)
- Protocol B: Helix API → Browser (different encoding)
- Translation layer in Helix API (complexity, latency, bugs)

Instead, with pure proxy:
- **ONE protocol:** Browser ↔ Helix API ↔ screenshot-server
- All speak the SAME binary WebSocket format
- Helix API is just a pipe (no protocol understanding needed)
- screenshot-server implements what Wolf currently does

## Video Capture Options Analysis

A comprehensive analysis of solutions for PipeWire → H.264 → WebSocket streaming.

### Option A: GStreamer pipewiresrc (RECOMMENDED)

```bash
gst-launch-1.0 pipewiresrc path=<node_id> ! nvh264enc ! appsink
```

**Pros:**
- Already in our stack (Wolf uses GStreamer)
- Direct PipeWire integration
- NVENC hardware encoding
- Simple command-line or Go bindings (go-gst)
- No additional dependencies

**Cons:**
- Need to implement WebSocket framing ourselves

**Verdict:** Best choice - minimal complexity, we control everything.

### Option B: OBS Studio

OBS can capture from PipeWire and stream via RTMP/WHIP.

**Headless options:**
- [obs-headless (Docker + gRPC)](https://github.com/a-rose/obs-headless)
- [obs-headless (nelsonxb)](https://github.com/nelsonxb/obs-headless)
- Virtual display (Xvfb) + obs-websocket control

**Pros:**
- Feature-rich (scenes, transitions, overlays)
- [PipeWire Video Capture](https://www.gamingonlinux.com/2024/03/obs-studio-30-1-out-now-with-av1-support-for-va-api-pipewire-video-capture/) in OBS 30.1+
- [obs-websocket](https://github.com/obsproject/obs-websocket) for remote control

**Cons:**
- Designed for RTMP/WHIP, not raw WebSocket
- Requires virtual display or X11 session
- Heavy dependency (~100MB+)
- Overkill for our use case (no scenes/overlays needed)
- [Headless mode still a feature request](https://ideas.obsproject.com/posts/16/add-a-headless-mode-that-allows-full-control-via-scripting-api)

**Verdict:** Too heavy, wrong protocol (RTMP vs WebSocket).

### Option C: FFmpeg

**Current status:** Native PipeWire support is [still under development](https://trac.ffmpeg.org/ticket/10742).

A [`pipewiregrab` filter patch](https://ffmpeg.org/pipermail/ffmpeg-devel/2024-May/327141.html) was submitted in 2024:
- Uses XDG Desktop Portal ScreenCast interface
- Supports DMA buffer sharing
- **Not yet merged into mainline FFmpeg**

**Workaround (if pipewiregrab gets merged):**
```bash
ffmpeg -f lavfi -i "pipewiregrab=capture_type=desktop" \
    -c:v h264_nvenc -f mpegts pipe:1
```

**Pros:**
- Universal tool, widely understood
- Many output formats (mpegts, rtmp, etc.)
- Would be simpler than GStreamer if working

**Cons:**
- [PipeWire support not merged yet](https://trac.ffmpeg.org/ticket/10742) (as of 2024)
- Designed for file/stream output, not raw WebSocket
- Would need to parse container format and reframe for WebSocket
- Less control over pipeline than GStreamer

**Verdict:** Wait for pipewiregrab to merge. Until then, GStreamer is the only option with native PipeWire support.

### Option D: WebRTC-Streamer

[webrtc-streamer](https://github.com/mpromonet/webrtc-streamer) - WebRTC streamer for V4L2, RTSP, and screen capture.

```bash
./webrtc-streamer -u screen://
```

**Pros:**
- Purpose-built for browser streaming
- WebRTC output (low latency)
- Hardware H.264 encoding support

**Cons:**
- Uses X11/Xvfb for screen capture, not native PipeWire
- WebRTC instead of raw WebSocket (more overhead)
- Another C++ dependency

**Verdict:** Good for WebRTC, but we want raw WebSocket for simplicity.

### Option E: SRS Media Server

[SRS](https://github.com/ossrs/srs) - Real-time media server supporting RTMP, WebRTC, HLS, HTTP-FLV.

**Pros:**
- Production-ready media server
- H.264, H.265, AV1 support
- WebRTC output

**Cons:**
- Expects RTMP input (would need ffmpeg/gstreamer to feed it)
- Adds another server component
- Overkill for single-viewer streaming

**Verdict:** Good for multi-viewer broadcasting, overkill for our use case.

### Option F: Browser-Native Screen Sharing (Reference)

[Firefox 84+](https://bugzilla.mozilla.org/show_bug.cgi?id=1672944) and [Chromium 110+](https://wiki.archlinux.org/title/PipeWire#WebRTC_screen_sharing) support PipeWire screen sharing via WebRTC.

This is what users do when sharing their screen in a video call. Not applicable for server-side capture, but shows PipeWire+WebRTC is mature.

### Option G: websocket-mse-demo Pattern

[websocket-mse-demo](https://github.com/elsampsa/websocket-mse-demo) - Stream H264 to browsers with WebSocket + Media Source Extensions.

**Architecture:**
```
Video source → H.264 encoder → WebSocket server → Browser MSE
```

**Pros:**
- Exactly what we want to build
- Proven pattern (Raspberry Pi streaming projects use this)
- Low latency

**Cons:**
- Not a library, just a demo
- We'd implement the same thing ourselves

**Verdict:** This IS our approach - validates the architecture.

### Summary Table

| Option | PipeWire Native | Output Protocol | Complexity | Status |
|--------|----------------|-----------------|------------|--------|
| **GStreamer** | ✅ Yes | stdout/appsink | Low | **Recommended** |
| OBS | ✅ Yes | RTMP/WHIP | High | Too heavy |
| FFmpeg | 🚧 Patch pending | File/mpegts | Medium | Wait |
| webrtc-streamer | ❌ X11 | WebRTC | Medium | Wrong capture |
| SRS | ❌ Needs input | RTMP/WebRTC | High | Overkill |
| websocket-mse-demo | ❌ V4L2 | WebSocket | Low | Our pattern |

### Recommendation

**Use GStreamer pipewiresrc directly.** It's already proven (Wolf uses it), we control the pipeline, and we just need to add WebSocket framing in Go.

```go
// Minimal implementation in screenshot-server
cmd := exec.Command("gst-launch-1.0",
    "pipewiresrc", fmt.Sprintf("path=%d", nodeID),
    "!", "nvh264enc", "preset=low-latency-hq",
    "!", "video/x-h264,stream-format=byte-stream",
    "!", "fdsink", "fd=1")

stdout, _ := cmd.StdoutPipe()
go func() {
    // Read H.264 NAL units, frame into WebSocket binary messages
    for {
        nalUnit := readNALUnit(stdout)
        ws.WriteMessage(websocket.BinaryMessage, frameVideoMessage(nalUnit))
    }
}()
```

## Comparison with Input WebSocket Pattern

### Input (Browser → Container) - Already Working

```
Browser WheelEvent
    ↓ Raw event data
Frontend PipeWire mode detection
    ↓ Connect to /api/v1/sessions/{id}/input WebSocket
Helix API WebSocket proxy
    ↓ Proxy to container's :9876/ws/input
Go screenshot-server (input.go)
    ↓ Convert browser values → GNOME D-Bus values
GNOME Mutter RemoteDesktop D-Bus API
```

### Video (Container → Browser) - Proposed

```
GNOME Mutter ScreenCast D-Bus API
    ↓ PipeWire node ID
GStreamer pipewiresrc path=<node_id>
    ↓ Raw video frames (BGRx)
GStreamer nvh264enc / x264enc
    ↓ H.264 NAL units
Go screenshot-server (video.go)
    ↓ Send over WebSocket
Helix API WebSocket proxy
    ↓ /api/v1/sessions/{id}/video
Browser MediaSource Extensions API
    ↓ Decode H.264 in browser
Browser <video> element
```

## Implementation Details

### 1. Container-Side: GStreamer Pipeline

The encoder is selected at runtime based on available hardware. Priority order:
1. **NVIDIA NVENC** (`nvh264enc`) - fastest, lowest latency
2. **Intel QSV** (`qsvh264enc`) - Intel Quick Sync Video
3. **VA-API** (`vah264enc`) - Intel/AMD hardware
4. **VA-API Low Power** (`vah264lpenc`) - Intel low-power mode
5. **x264** (`x264enc`) - software fallback

```bash
# NVIDIA NVENC - fastest, lowest latency
gst-launch-1.0 pipewiresrc path=<node_id> do-timestamp=true ! \
  videoconvert ! videoscale ! video/x-raw,width=1920,height=1080,framerate=60/1 ! \
  nvh264enc preset=low-latency-hq zerolatency=true gop-size=15 rc-mode=cbr-ld-hq bitrate=8000 aud=false ! \
  h264parse ! video/x-h264,stream-format=byte-stream ! \
  fdsink fd=1

# Intel QSV (Quick Sync Video)
gst-launch-1.0 pipewiresrc path=<node_id> do-timestamp=true ! \
  videoconvert ! videoscale ! video/x-raw,width=1920,height=1080,framerate=60/1 ! \
  qsvh264enc b-frames=0 gop-size=15 idr-interval=1 ref-frames=1 bitrate=8000 rate-control=cbr target-usage=6 ! \
  h264parse ! video/x-h264,stream-format=byte-stream ! \
  fdsink fd=1

# VA-API (Intel/AMD)
gst-launch-1.0 pipewiresrc path=<node_id> do-timestamp=true ! \
  videoconvert ! videoscale ! video/x-raw,width=1920,height=1080,framerate=60/1 ! \
  vah264enc aud=false b-frames=0 ref-frames=1 bitrate=8000 cpb-size=8000 key-int-max=1024 rate-control=cqp target-usage=6 ! \
  h264parse ! video/x-h264,stream-format=byte-stream ! \
  fdsink fd=1

# Software x264 (fallback)
gst-launch-1.0 pipewiresrc path=<node_id> do-timestamp=true ! \
  videoconvert ! videoscale ! video/x-raw,width=1920,height=1080,framerate=60/1 ! \
  x264enc pass=qual tune=zerolatency speed-preset=superfast b-adapt=false bframes=0 ref=1 bitrate=8000 aud=false ! \
  h264parse ! video/x-h264,stream-format=byte-stream ! \
  fdsink fd=1
```

The encoder detection uses `gst-inspect-1.0` to check element availability at runtime.

### 2. Container-Side: Go Video Server (video.go)

```go
// VideoStreamer captures video and sends H.264 frames over WebSocket
type VideoStreamer struct {
    nodeID     uint32
    pipeline   *gst.Pipeline  // Using go-gst bindings
    websockets map[*websocket.Conn]bool
    mu         sync.RWMutex
}

func (v *VideoStreamer) Start(ctx context.Context) error {
    // Create GStreamer pipeline
    pipeline := fmt.Sprintf(
        "pipewiresrc path=%d do-timestamp=true ! "+
        "video/x-raw,format=BGRx ! "+
        "cudaupload ! cudaconvert ! "+
        "nvh264enc preset=low-latency-hq bitrate=8000 rc-mode=cbr ! "+
        "video/x-h264,stream-format=byte-stream ! "+
        "appsink name=sink emit-signals=true max-buffers=2 drop=true",
        v.nodeID)

    // ... create pipeline and get appsink

    // On each sample, broadcast to all WebSocket clients
    appsink.SetCallbacks(&gst.AppSinkCallbacks{
        NewSampleFunc: func(sink *gst.AppSink) gst.FlowReturn {
            sample := sink.PullSample()
            buffer := sample.GetBuffer()
            data := buffer.Map(gst.MapRead).Bytes()

            v.broadcast(data)
            return gst.FlowOK
        },
    })
}

func (v *VideoStreamer) broadcast(nalUnit []byte) {
    v.mu.RLock()
    defer v.mu.RUnlock()

    for ws := range v.websockets {
        ws.WriteMessage(websocket.BinaryMessage, nalUnit)
    }
}
```

### 3. API-Side: WebSocket Proxy

Add to `api/pkg/server/session_handlers.go`:

```go
// handleVideoWebSocket proxies video stream from container to browser
func (s *HelixAPIServer) handleVideoWebSocket(w http.ResponseWriter, r *http.Request) {
    sessionID := chi.URLParam(r, "session_id")

    // Upgrade to WebSocket
    conn, err := upgrader.Upgrade(w, r, nil)
    if err != nil {
        return
    }
    defer conn.Close()

    // Get container's video WebSocket URL
    session, _ := s.Controller.GetSession(r.Context(), sessionID)
    containerURL := fmt.Sprintf("ws://%s:9876/ws/video", session.ContainerIP)

    // Connect to container
    containerConn, _, err := websocket.DefaultDialer.Dial(containerURL, nil)
    if err != nil {
        return
    }
    defer containerConn.Close()

    // Proxy: container → browser (video only flows one direction)
    for {
        messageType, data, err := containerConn.ReadMessage()
        if err != nil {
            break
        }
        conn.WriteMessage(messageType, data)
    }
}
```

Route registration:
```go
r.Get("/api/v1/sessions/{session_id}/video", s.handleVideoWebSocket)
```

### 4. Browser-Side: MediaSource Extensions

```typescript
// VideoPlayer.tsx
class DirectVideoPlayer {
    private video: HTMLVideoElement;
    private mediaSource: MediaSource;
    private sourceBuffer: SourceBuffer | null = null;
    private ws: WebSocket | null = null;

    async connect(sessionId: string) {
        // Create MediaSource
        this.mediaSource = new MediaSource();
        this.video.src = URL.createObjectURL(this.mediaSource);

        await new Promise(resolve => {
            this.mediaSource.addEventListener('sourceopen', resolve, { once: true });
        });

        // Add source buffer for H.264
        this.sourceBuffer = this.mediaSource.addSourceBuffer('video/mp4; codecs="avc1.64001f"');

        // Connect to WebSocket
        this.ws = new WebSocket(`wss://${location.host}/api/v1/sessions/${sessionId}/video`);
        this.ws.binaryType = 'arraybuffer';

        this.ws.onmessage = (event) => {
            this.appendH264Data(new Uint8Array(event.data));
        };
    }

    private appendH264Data(nalUnits: Uint8Array) {
        // Need to wrap H.264 NAL units in fMP4 container for MSE
        const fmp4Segment = this.wrapInFMP4(nalUnits);

        if (!this.sourceBuffer.updating) {
            this.sourceBuffer.appendBuffer(fmp4Segment);
        }
    }

    private wrapInFMP4(nalUnits: Uint8Array): ArrayBuffer {
        // Use mux.js or jMuxer to wrap H.264 in fMP4
        // This is required because MSE doesn't accept raw H.264
        return this.muxer.feed(nalUnits);
    }
}
```

## H.264 to fMP4 Conversion

The browser's MediaSource Extensions API requires fragmented MP4 (fMP4), not raw H.264. Options:

### Option A: Client-Side Muxing (jMuxer)

```typescript
import JMuxer from 'jmuxer';

const jmuxer = new JMuxer({
    node: 'video-element',
    mode: 'video',
    flushingTime: 0,
    fps: 60,
    debug: false
});

ws.onmessage = (event) => {
    jmuxer.feed({
        video: new Uint8Array(event.data)
    });
};
```

### Option B: Server-Side fMP4 Muxing (GStreamer)

```bash
# Produce fMP4 segments directly
gst-launch-1.0 pipewiresrc path=<node_id> ! \
  nvh264enc ! h264parse ! \
  mp4mux streamable=true fragment-duration=100 ! \
  fdsink
```

### Option C: WebCodecs API (Modern Browsers)

```typescript
// Use WebCodecs for direct H.264 decoding (no MSE/fMP4 needed)
const decoder = new VideoDecoder({
    output: (frame) => {
        ctx.drawImage(frame, 0, 0);
        frame.close();
    },
    error: (e) => console.error(e)
});

decoder.configure({
    codec: 'avc1.64001f',
    hardwareAcceleration: 'prefer-hardware'
});

ws.onmessage = (event) => {
    decoder.decode(new EncodedVideoChunk({
        type: isKeyframe ? 'key' : 'delta',
        timestamp: performance.now() * 1000,
        data: event.data
    }));
};
```

## Recommended Approach

**Use WebCodecs API with H.264 Annex B stream:**

1. **Container:** GStreamer produces H.264 Annex B byte stream (NAL units with start codes)
2. **Transport:** Raw NAL units sent over WebSocket
3. **Browser:** WebCodecs VideoDecoder decodes directly to canvas

This is the simplest approach:
- No fMP4 muxing complexity
- Hardware-accelerated decoding in browser
- Lowest latency (no buffering for fragmentation)
- Modern browser support (Chrome 94+, Edge 94+, Safari 16.4+)

## Performance Considerations

### Latency Budget

| Component | Latency |
|-----------|---------|
| PipeWire capture | ~1-2ms |
| NVENC encoding | ~2-5ms |
| WebSocket transport | ~1-5ms (LAN) |
| WebCodecs decode | ~2-5ms |
| **Total** | **~6-17ms** |

Compare to Wolf/Moonlight/WebRTC: ~30-50ms

### Bandwidth

At 1080p60:
- H.264 CBR 8 Mbps = 1 MB/s
- H.264 CBR 15 Mbps = 1.9 MB/s (high quality)

At 4K60:
- H.264 CBR 25 Mbps = 3.1 MB/s
- H.264 CBR 50 Mbps = 6.25 MB/s (high quality)

WebSocket can easily handle these rates.

### CPU/GPU Usage

| Component | CPU | GPU |
|-----------|-----|-----|
| NVENC encoding | ~5% | ~10% |
| WebCodecs decode | ~2% | ~5% (hw) |

## Comparison: SHM vs Direct WebSocket

| Aspect | SHM (Current) | Direct WebSocket (Proposed) |
|--------|---------------|----------------------------|
| Encoding location | Wolf (sandbox) | Container |
| Transport | shmsink/shmsrc → Wolf pipeline | WebSocket |
| Protocol to browser | WebRTC (Moonlight) | WebSocket + WebCodecs |
| Dependencies | Wolf, Moonlight Web | None (just GStreamer) |
| Debugging | Complex multi-component | Simple end-to-end |
| Cross-container issues | Needs shared socket | None |
| Latency | ~30-50ms | ~6-17ms |
| 4K60 support | Needs testing | Should work easily |

## Combined WebSocket Protocol (from moonlight-web-stream)

**Key insight:** We implement the EXACT SAME protocol that moonlight-web-stream uses,
so the frontend requires ZERO CHANGES.

The protocol is defined in `moonlight-web-stream/moonlight-web/common/src/ws_protocol.rs`:

### Message Types

| Type | Direction | Purpose |
|------|-----------|---------|
| 0x01 | S→C | VideoFrame |
| 0x02 | S→C | AudioFrame |
| 0x03 | S→C | VideoBatch (congestion) |
| 0x10 | C→S | KeyboardInput |
| 0x11 | C→S | MouseClick |
| 0x12 | C→S | MouseAbsolute |
| 0x13 | C→S | MouseRelative |
| 0x14 | C→S | TouchEvent |
| 0x20 | Bi | ControlMessage |
| 0x30 | S→C | StreamInit |
| 0x31 | S→C | StreamError |
| 0x40 | C→S | Ping |
| 0x41 | S→C | Pong |

### StreamInit (0x30) - 13 bytes
```
[type:1][codec:1][width:2][height:2][fps:1][audio_channels:1][sample_rate:4][touch:1]
```

### VideoFrame (0x01) - 15 byte header + NAL data
```
[type:1][codec:1][flags:1][pts:8][width:2][height:2][nal_data...]
```

### Input Messages (0x10-0x14)
Same binary format as ws_input.go already implements.

### Ping/Pong (0x40/0x41)
```
Ping: [type:1][seq:4][client_time_us:8]
Pong: [type:1][seq:4][client_time_us:8][server_time_us:8]
```

## Implementation Plan

### Phase 1: Combined Protocol (Complete)
1. ✅ Create `ws_stream.go` with GStreamer pipeline + WebSocket framing
2. ✅ Add `/ws/stream` route to screenshot-server
3. ✅ Add WebSocket proxy in Helix API (`/api/v1/external-agents/{id}/ws/stream`)
4. ✅ Merge input handling from ws_input.go into ws_stream.go (combined protocol)
5. ✅ Add multi-GPU support (NVIDIA NVENC, Intel QSV, VA-API, x264 fallback)
6. 🔄 Testing and debugging

### Phase 2: Frontend Integration
1. Frontend WebSocketStream already speaks this protocol - NO CHANGES needed
2. Test with existing streaming UI
3. Verify input works through combined WebSocket

### Phase 3: Optimization
1. Add NVENC hardware encoding support
2. Add adaptive bitrate based on connection quality
3. Add keyframe request mechanism (for seeking/reconnect)
4. Measure and optimize latency

## Files Created/Modified

### New Files
- `api/pkg/desktop/ws_stream.go` - Combined WebSocket handler (video + input)

### Modified Files
- `api/pkg/desktop/desktop.go` - Add `/ws/stream` route
- `api/pkg/server/external_agent_handlers.go` - Add `proxyStreamWebSocket`
- `api/pkg/server/server.go` - Register `/external-agents/{id}/ws/stream`

## Fallback Strategy

If WebCodecs not supported:
1. Check `window.VideoDecoder` existence
2. Fall back to jMuxer + MSE
3. If neither works, fall back to Wolf/Moonlight (current approach)

```typescript
function createVideoPlayer(sessionId: string): VideoPlayer {
    if ('VideoDecoder' in window) {
        return new WebCodecsPlayer(sessionId);
    } else if ('MediaSource' in window) {
        return new JMuxerPlayer(sessionId);
    } else {
        return new MoonlightPlayer(sessionId);
    }
}
```

## Questions to Resolve

1. **GStreamer Go bindings:** Use go-gst or spawn gst-launch subprocess?
   - Recommendation: Start with subprocess (simpler), migrate to bindings if needed

2. **Keyframe interval:** How often to send I-frames?
   - Recommendation: Every 2 seconds, plus on-demand via WebSocket message

3. **Multiple viewers:** Support multiple browser tabs viewing same session?
   - Recommendation: Yes, broadcast to all connected WebSockets

4. **Audio:** Include audio in same WebSocket or separate?
   - Recommendation: Separate WebSocket for audio (simpler, matches video pattern)

## Session Sharing Without Wolf Lobbies

Wolf has a "lobby" concept where multiple viewers can watch the same desktop session.
Without Wolf, we need an alternative approach for multi-viewer support.

### Current Wolf Lobby Approach
- Wolf creates one NVENC encoding session per lobby
- Multiple Moonlight clients connect to the same lobby
- Each client receives the same encoded video stream
- Efficient: encode once, broadcast many

### Options for Wolf-Free Multi-Viewer

#### Option A: Multiple PipeWire Streams (Recommended for simplicity)
Each viewer gets their own independent pipeline:

```
Viewer 1 WebSocket → screenshot-server instance 1 → pipewiresrc node=44 → nvh264enc → frames
Viewer 2 WebSocket → screenshot-server instance 2 → pipewiresrc node=44 → nvh264enc → frames
```

**Pros:**
- Simple implementation (each connection is independent)
- Each viewer can have different bitrate/resolution
- No fan-out complexity

**Cons:**
- Multiple NVENC sessions (GPU encoder limit: typically 3-5 concurrent)
- Higher GPU encoding load

**When to use:** Low viewer count (1-5 viewers per session)

#### Option B: Single Encoder with WebSocket Fan-Out
One encoding pipeline, broadcast to multiple WebSocket connections:

```
pipewiresrc node=44 → nvh264enc → [screenshot-server hub]
                                       ↓
                                ├── Viewer 1 WebSocket
                                ├── Viewer 2 WebSocket
                                └── Viewer N WebSocket
```

**Pros:**
- Only one NVENC session per desktop
- Scales to many viewers
- Same encoded frames for all (efficient)

**Cons:**
- All viewers get same bitrate/resolution
- Need hub/broadcast logic in screenshot-server
- Keyframe requests affect all viewers

**When to use:** Many viewers watching same session (webinar-style)

#### Option C: External SFU/Media Server
Route video through a Selective Forwarding Unit:

```
screenshot-server → Janus/Mediasoup/Livekit → Multiple viewers
```

**Pros:**
- Purpose-built for multi-viewer
- Adaptive bitrate per viewer possible
- Recording, analytics built-in

**Cons:**
- Additional infrastructure component
- More deployment complexity
- Latency increase

**When to use:** Enterprise deployments with strict multi-viewer requirements

### Recommendation

**Start with Option A (multiple PipeWire streams)** for MVP:
1. Simple implementation
2. NVENC supports 3-5 concurrent sessions (enough for typical use)
3. Each viewer isolated (one viewer's network issues don't affect others)

**Upgrade to Option B** if we see:
- Users hitting NVENC session limits
- High GPU encoding load with multiple viewers
- Need for efficient broadcasting

### Multiple PipeWire Sessions - How It Works

GNOME ScreenCast allows multiple consumers of the same node:

```go
// Each WebSocket connection creates its own capture session
func (s *Server) handleWebSocketStream(ws *websocket.Conn) {
    // Create D-Bus ScreenCast session for this viewer
    screencast := gnome.NewScreenCastSession()
    nodeID := screencast.RecordVirtualMonitor()

    // Create GStreamer pipeline for this viewer
    pipeline := fmt.Sprintf(
        "pipewiresrc path=%d ! nvh264enc ! appsink",
        nodeID,
    )

    // Each viewer has independent session and pipeline
}
```

PipeWire handles the multi-consumer case efficiently - all pipelines
read from the same compositor output without additional copying.

## Eliminating Wolf and Moonlight-Web Entirely

After implementing Wolf-free video streaming, the **only remaining Wolf dependency** is:
- **Container lifecycle management** - starting/stopping desktop containers via Docker API

### Current Sandbox Architecture

```
helix-sandbox container
├── Wolf (C++ server) ─────────────────────────┐
│   ├── Docker API: start/stop desktop containers
│   ├── Video: NVENC encoding + RTP/RTSP      │ Replace with
│   ├── Audio: Opus encoding                   │ Go program
│   └── Input: relay to containers             │
├── Moonlight Web (Rust) ──────────────────────┘
│   └── WebRTC bridge to browser
└── Init scripts (cont-init.d)
    ├── 04-start-dockerd.sh
    ├── 05-init-wolf-config.sh
    └── ...
```

### Proposed: Go Program in Sandbox

Replace Wolf and Moonlight-Web with a single Go program:

```
helix-sandbox container
├── helix-sandbox-server (NEW Go program) ─────┐
│   ├── Docker API: start/stop desktop containers
│   ├── WebSocket: direct connection to browsers │ Single program
│   ├── Video: spawn gst-launch for encoding    │ replaces Wolf +
│   ├── Audio: spawn gst-launch for encoding    │ Moonlight-Web
│   └── Proxy: WebSocket ↔ screenshot-server    │
└── Init scripts (cont-init.d)
    ├── 04-start-dockerd.sh
    └── 05-start-helix-sandbox-server.sh (NEW)
```

**Note:** Currently there is NO Go program running in the sandbox. This would be new.

### What the Go Program Would Handle

1. **Container Lifecycle** (currently Wolf)
   ```go
   // Start desktop container for session
   func (s *Server) StartDesktop(sessionID, image string) error {
       ctx := context.Background()
       resp, err := s.docker.ContainerCreate(ctx, &container.Config{
           Image: image,
           Env:   []string{"HELIX_SESSION_ID=" + sessionID, ...},
       }, hostConfig, nil, nil, containerName)
       return s.docker.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{})
   }
   ```

2. **WebSocket Proxy** (currently Moonlight-Web)
   ```go
   // Proxy browser WebSocket to container's screenshot-server
   func (s *Server) HandleStream(w http.ResponseWriter, r *http.Request) {
       browserWS := upgradeToWebSocket(w, r)
       containerWS := dialContainer(sessionID, ":9876/ws/stream")
       go io.Copy(browserWS, containerWS)  // video/audio: container → browser
       go io.Copy(containerWS, browserWS)  // input: browser → container
   }
   ```

3. **Health Monitoring** (currently Wolf watchdog)
   ```go
   // Monitor container health, restart if needed
   func (s *Server) MonitorContainers() {
       for range time.Tick(10 * time.Second) {
           for _, session := range s.sessions {
               if !s.isContainerHealthy(session.ContainerID) {
                   s.restartContainer(session)
               }
           }
       }
   }
   ```

### Migration Path

**Phase 1: Wolf-free video streaming (this design doc)**
- screenshot-server handles encoding
- Wolf still manages containers
- Helix API proxies to Wolf's WebSocket

**Phase 2: Go sandbox server (future)**
- New Go program handles container lifecycle
- WebSocket proxy moves from Wolf to Go program
- Wolf removed from sandbox

**Phase 3: Complete removal**
- Remove Wolf from Dockerfile.sandbox
- Remove Moonlight-Web from Dockerfile.sandbox
- Sandbox image shrinks significantly (~1GB smaller)

### Benefits of Go Replacement

| Aspect | Wolf (C++) | Go Program |
|--------|-----------|------------|
| Language | C++17 | Go |
| Complexity | ~50K lines | ~5K lines estimated |
| Dependencies | Boost, GStreamer, libpulse, etc. | Docker SDK, gorilla/websocket |
| Debug | GDB, core dumps | go tool pprof, delve |
| Build time | ~10 min | ~30 sec |
| Binary size | ~50MB + libs | ~20MB static |
| Maintainability | Requires C++ expertise | Standard Go |

### Files to Create

```
api/cmd/sandbox-server/main.go     # Entry point
api/pkg/sandbox/server.go          # HTTP/WebSocket server
api/pkg/sandbox/docker.go          # Container lifecycle
api/pkg/sandbox/proxy.go           # WebSocket proxy
api/pkg/sandbox/health.go          # Health monitoring
sandbox/05-start-sandbox-server.sh # Init script
```

## Future: WebRTC with Pion

If we need lower latency than WebSocket can provide, we can add WebRTC support using **Pion** (https://github.com/pion/webrtc):

- **Pure Go** - no CGO, compiles to static binary
- **Full WebRTC stack** - data channels, media tracks, TURN/STUN
- **Production-proven** - used by Cloudflare, Twitch, Discord, Jitsi, Livekit
- **Active development** - ~6K GitHub stars, regular releases

### WebRTC vs WebSocket Trade-offs

| Aspect | WebSocket | WebRTC (Pion) |
|--------|-----------|---------------|
| Latency | ~50-100ms | ~20-50ms |
| NAT traversal | Requires proxy | Built-in (STUN/TURN) |
| Congestion control | Manual | Built-in (REMB, TWCC) |
| Complexity | Simple | More complex |
| Browser support | Universal | Universal |

### When to Consider WebRTC

- Sub-50ms latency requirements
- Direct peer-to-peer without proxy overhead
- Adaptive bitrate with congestion feedback
- Audio/video sync requirements

For now, WebSocket + WebCodecs provides sufficient latency for desktop streaming use cases.

## Conclusion

The direct WebSocket approach is simpler, lower-latency, and eliminates Wolf/Moonlight dependencies. It mirrors the already-working input WebSocket pattern, making the architecture symmetric and easier to understand.

The main trade-off is implementing the H.264→WebCodecs pipeline, but this is well-documented and libraries exist. The WebCodecs API is supported in all modern browsers and provides hardware-accelerated decoding.

**Recommendation:** Implement as experiment alongside SHM approach, then deprecate SHM if WebSocket works well.

**Long-term:** Replace Wolf entirely with a Go sandbox server for simpler maintenance and faster iteration.
