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

## Proposed Architecture (Direct WebSocket)

Mirror the input WebSocket pattern but in reverse:

```
GNOME ScreenCast (PipeWire)
    ↓ PipeWire frames
GStreamer pipewiresrc (in container)
    ↓ Raw video frames
GStreamer nvh264enc / x264enc
    ↓ H.264 NAL units
Go screenshot-server (video.go)
    ↓ WebSocket frames
Helix API WebSocket proxy
    ↓ /api/v1/sessions/{id}/video
Browser MediaSource Extensions
    ↓ fMP4 segments
Browser <video> element
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

```bash
# Option A: Hardware encoding (NVIDIA)
gst-launch-1.0 pipewiresrc path=<node_id> do-timestamp=true ! \
  video/x-raw,format=BGRx ! \
  cudaupload ! cudaconvert ! \
  nvh264enc preset=low-latency-hq bitrate=8000 ! \
  h264parse ! \
  fdsink fd=<pipe_to_go>

# Option B: Software encoding (fallback)
gst-launch-1.0 pipewiresrc path=<node_id> do-timestamp=true ! \
  videoconvert ! \
  x264enc tune=zerolatency bitrate=8000 speed-preset=ultrafast ! \
  h264parse ! \
  fdsink fd=<pipe_to_go>

# Option C: Using appsink for Go integration
gst-launch-1.0 pipewiresrc path=<node_id> ! \
  nvh264enc preset=low-latency-hq ! \
  appsink name=sink emit-signals=true
```

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

## Implementation Plan

### Phase 1: Proof of Concept (Day 1)
1. Add GStreamer H.264 encoding to screenshot-server
2. Create `/ws/video` WebSocket endpoint in screenshot-server
3. Create `/api/v1/sessions/{id}/video` proxy in Helix API
4. Create simple HTML page with WebCodecs decoder

### Phase 2: Integration (Day 2)
1. Add video player component to frontend
2. Detect WebCodecs support, fallback to jMuxer if needed
3. Add bitrate/quality controls
4. Integrate with existing session UI

### Phase 3: Optimization (Day 3)
1. Add adaptive bitrate based on connection quality
2. Add keyframe request mechanism (for seeking/reconnect)
3. Measure and optimize latency
4. Add metrics/monitoring

## Files to Create/Modify

### New Files
- `api/pkg/desktop/video_encoder.go` - GStreamer pipeline management
- `api/pkg/desktop/video_websocket.go` - WebSocket handler
- `frontend/src/components/sessions/DirectVideoPlayer.tsx` - WebCodecs player

### Modified Files
- `api/pkg/desktop/session.go` - Add video encoder initialization
- `api/pkg/desktop/server.go` - Add /ws/video route
- `api/pkg/server/session_handlers.go` - Add video proxy endpoint
- `api/pkg/server/routes.go` - Register video route

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

## Conclusion

The direct WebSocket approach is simpler, lower-latency, and eliminates Wolf/Moonlight dependencies. It mirrors the already-working input WebSocket pattern, making the architecture symmetric and easier to understand.

The main trade-off is implementing the H.264→WebCodecs pipeline, but this is well-documented and libraries exist. The WebCodecs API is supported in all modern browsers and provides hardware-accelerated decoding.

**Recommendation:** Implement as experiment alongside SHM approach, then deprecate SHM if WebSocket works well.
