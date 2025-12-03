# WebSocket-Only Streaming: Architecture Deep Dive

**Reading time:** ~45 minutes
**Purpose:** Understand the full implementation, find gaps, think about edge cases

---

## The Problem We're Solving

WebRTC requires TURN servers for NAT traversal, which need UDP port ranges or non-standard TCP ports (3478, 5349). Enterprise deployments only allow HTTP/HTTPS through L7 load balancers. **WebSocket-only streaming bypasses WebRTC entirely** - everything goes through standard HTTPS.

---

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────────────┐
│                              BROWSER                                     │
│                                                                          │
│  ┌──────────────────────┐     ┌──────────────────────────────────────┐  │
│  │ MoonlightStreamViewer│     │ WebSocketStream                      │  │
│  │ (React Component)    │────▶│ - WebSocket connection               │  │
│  │                      │     │ - WebCodecs VideoDecoder             │  │
│  │ - Mode toggle button │     │ - WebCodecs AudioDecoder             │  │
│  │ - Canvas (WS mode)   │     │ - Input event → binary → WS send     │  │
│  │ - Video (WebRTC mode)│     │ - Renders to Canvas                  │  │
│  └──────────────────────┘     └──────────────────────────────────────┘  │
│                                          │                               │
└──────────────────────────────────────────│───────────────────────────────┘
                                           │ WebSocket (wss://)
                                           │ Binary frames
                                           ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                         HELIX API / PROXY                                │
│                                                                          │
│  nginx/ingress → /moonlight/* → moonlight-web-stream                    │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
                                           │
                                           ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                      MOONLIGHT-WEB-STREAM                                │
│                                                                          │
│  ┌─────────────────┐          ┌─────────────────────────────────────┐   │
│  │ web-server      │◀────────▶│ streamer process                    │   │
│  │                 │   IPC    │                                     │   │
│  │ /api/ws/stream  │ stdin/   │ - WebSocketVideoDecoder             │   │
│  │ endpoint        │ stdout   │ - WebSocketAudioDecoder             │   │
│  │                 │          │ - Input forwarding to Moonlight     │   │
│  │ Forwards frames │          │ - NO WebRTC (websocket_only_mode)   │   │
│  │ to browser      │          │                                     │   │
│  └─────────────────┘          └─────────────────────────────────────┘   │
│                                          │                               │
└──────────────────────────────────────────│───────────────────────────────┘
                                           │ Moonlight Protocol
                                           ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                              WOLF                                        │
│                                                                          │
│  - NVIDIA hardware encoder (H264/HEVC/AV1)                              │
│  - Opus audio encoder                                                    │
│  - Receives input events from Moonlight                                  │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## Source Files to Read (In Order)

### 1. Start with the Binary Protocol (10 min)

**File:** `moonlight-web-stream/moonlight-web/common/src/ws_protocol.rs`

This defines the wire format. Everything else builds on this.

```rust
pub enum WsMessageType {
    VideoFrame = 0x01,
    AudioFrame = 0x02,
    KeyboardInput = 0x10,
    MouseClick = 0x11,
    MouseAbsolute = 0x12,
    MouseRelative = 0x13,
    // ...
}
```

**Questions to think about:**
- Is the protocol versioned? What happens if we need to change it?
- Are there any alignment/padding issues in the binary format?
- What about endianness? (Currently: big-endian for multi-byte values)

---

### 2. IPC Messages (5 min)

**File:** `moonlight-web-stream/moonlight-web/common/src/ipc.rs`

The streamer process communicates with web-server via stdin/stdout JSON.

Key additions for WebSocket mode:
```rust
pub enum StreamerIpcMessage {
    // ... existing ...
    VideoFrame { codec, timestamp_us, keyframe, width, height, data: String }, // base64
    AudioFrame { timestamp_us, channels, data: String }, // base64
}

pub enum ServerIpcMessage {
    // ... existing ...
    Input(InputIpcMessage),
}

pub enum InputIpcMessage {
    Keyboard(Vec<u8>),
    MouseClick(Vec<u8>),
    MouseAbsolute(Vec<u8>),
    MouseRelative(Vec<u8>),
    // ...
}
```

**Questions to think about:**
- Base64 encoding adds ~33% overhead. Is this acceptable for 60fps 1080p?
- Could we use a more efficient IPC mechanism (shared memory, Unix socket)?

---

### 3. Video Decoder (Server-Side) (10 min)

**File:** `moonlight-web-stream/moonlight-web/streamer/src/video.rs`

Look for `WebSocketVideoDecoder`. It implements Moonlight's `VideoDecoder` trait.

```rust
impl VideoDecoder for WebSocketVideoDecoder {
    fn submit_decode_unit(&mut self, unit: VideoDecodeUnit<'_>) -> DecodeResult {
        // Concatenates NAL units
        // Sends via IPC channel
        // NO RTP packetization (that was WebRTC)
    }
}
```

**Key insight:** We're sending raw NAL units (H264 access units) directly, not RTP packets. The browser's WebCodecs API expects this format.

**Questions to think about:**
- Are we including SPS/PPS in every keyframe? WebCodecs needs them.
- What happens on codec change mid-stream?
- Is there frame dropping logic for slow connections?

---

### 4. Audio Decoder (Server-Side) (5 min)

**File:** `moonlight-web-stream/moonlight-web/streamer/src/audio.rs`

Look for `WebSocketAudioDecoder`. Similar pattern to video.

**Questions to think about:**
- Opus frames are small (~20ms each). Is the per-frame IPC overhead significant?
- Could we batch audio frames?

---

### 5. Main Streamer Logic (15 min)

**File:** `moonlight-web-stream/moonlight-web/streamer/src/main.rs`

Find `run_websocket_only_mode()` function. This is the heart of WebSocket-only streaming.

```rust
async fn run_websocket_only_mode(
    mut ipc_sender: IpcSender<StreamerIpcMessage>,
    mut ipc_receiver: IpcReceiver<ServerIpcMessage>,
    // ... params
) {
    // 1. Create host, set pairing info
    // 2. Create WebSocket decoders (not WebRTC track senders!)
    // 3. Start Moonlight stream
    // 4. Main loop: receive input from IPC, forward to Moonlight
}
```

**Input handling is critical.** Look at how each input type is parsed and forwarded:
- Keyboard: `stream.send_keyboard_event(key_code, action, modifiers)`
- Mouse button: `stream.send_mouse_button(action, button)`
- Mouse move: `stream.send_mouse_move(dx, dy)` or `send_mouse_position(x, y, ref_w, ref_h)`
- Scroll: `stream.send_high_res_scroll()` / `send_scroll()`

**Questions to think about:**
- What's the input latency path? Browser → WS → web-server → IPC → streamer → Moonlight → Wolf
- Are we handling all input types? (Touch, gamepad are TODO)
- What about key repeat? Browser sends repeat events, should we filter them?

---

### 6. Web Server Endpoint (5 min)

**File:** `moonlight-web-stream/moonlight-web/web-server/src/api/stream.rs`

Look for the `/api/ws/stream` handler. It should:
1. Upgrade HTTP → WebSocket
2. Spawn task to forward frames from IPC to WebSocket
3. Receive input from WebSocket, forward to IPC

**Questions to think about:**
- Is the WebSocket upgrade properly authenticated?
- What happens if frames back up (slow client)?
- Is there backpressure handling?

---

### 7. Frontend: WebSocketStream Class (15 min)

**File:** `frontend/src/lib/moonlight-web-ts/stream/websocket-stream.ts`

This is the browser-side counterpart. Read the whole file.

**Constructor:**
```typescript
constructor(api, hostId, appId, settings, supportedVideoFormats, viewerScreenSize, sessionId) {
    // Initialize input handler
    // Patch StreamInput methods to use WebSocket
    // Connect WebSocket
}
```

**Key methods:**
- `connect()` - Builds absolute WS URL, sets up event handlers
- `initVideoDecoder()` - Configures WebCodecs with Annex B format
- `handleVideoFrame()` - Parses binary, creates EncodedVideoChunk, decodes
- `renderVideoFrame()` - Draws VideoFrame to canvas
- `initAudioDecoder()` - WebCodecs AudioDecoder for Opus
- `playAudioData()` - PTS-based scheduling via AudioContext
- `sendKey()`, `sendMouseMove()`, etc. - Binary message construction

**Questions to think about:**
- WebCodecs is not supported in all browsers. What's the fallback?
- The `receivedFirstKeyframe` flag - is this enough to handle stream recovery?
- Audio scheduling uses first frame as baseline. What if there's clock drift?
- What happens if VideoDecoder queue gets too long? (no backpressure currently)

---

### 8. Frontend: React Integration (10 min)

**File:** `frontend/src/components/external-agent/MoonlightStreamViewer.tsx`

Focus on:
1. `streamingMode` state and toggle logic
2. Conditional stream creation (WebSocketStream vs Stream)
3. Canvas element for WebSocket mode
4. `getStreamRect()` - coordinate mapping for mouse events

**Questions to think about:**
- The mode toggle reconnects the stream. Is this UX acceptable?
- Canvas vs Video element - are mouse coordinates calculated correctly for both?
- What happens during reconnection? Is there a loading state?

---

## Data Flow Diagrams

### Video Frame Flow

```
Wolf (GPU Encoder)
    │
    │ H264 NAL units
    ▼
Moonlight Library
    │
    │ VideoDecodeUnit
    ▼
WebSocketVideoDecoder.submit_decode_unit()
    │
    │ Concatenate NAL units, base64 encode
    ▼
IPC stdout (JSON)
    │
    │ { "VideoFrame": { "data": "base64...", "keyframe": true, ... } }
    ▼
web-server
    │
    │ Decode base64, wrap in binary protocol
    ▼
WebSocket (binary frame)
    │
    │ [0x01][codec][flags][pts][width][height][NAL data...]
    ▼
WebSocketStream.handleVideoFrame()
    │
    │ Parse header, create EncodedVideoChunk
    ▼
VideoDecoder.decode()
    │
    │ Hardware decode (GPU)
    ▼
VideoDecoder output callback
    │
    │ VideoFrame
    ▼
renderVideoFrame()
    │
    │ ctx.drawImage(frame, 0, 0)
    ▼
Canvas (displayed)
```

### Input Flow (Keyboard Example)

```
User presses key
    │
    ▼
KeyboardEvent (browser)
    │
    ▼
MoonlightStreamViewer.handleKeyDown()
    │
    │ streamRef.current.getInput().onKeyDown(event)
    ▼
StreamInput.onKeyDown()
    │
    │ Calls patched sendKey()
    ▼
WebSocketStream.sendKey()
    │
    │ Construct binary: [subType][isDown][modifiers][keyCode]
    ▼
sendInputMessage(WsMessageType.KeyboardInput, payload)
    │
    │ ws.send([0x10][payload])
    ▼
web-server receives
    │
    │ Parse message type, forward via IPC
    ▼
streamer receives Input(Keyboard(data))
    │
    │ Parse: sub_type, is_down, modifiers, key_code
    ▼
stream.send_keyboard_event(key_code, action, modifiers)
    │
    ▼
Moonlight → Wolf → App
```

---

## Potential Holes to Investigate

### 1. **Base64 Overhead in IPC**

At 60fps 1080p with 20 Mbps bitrate:
- ~2.5 MB/sec of video data
- Base64 adds 33% → ~3.3 MB/sec
- JSON framing adds more overhead

**Is this acceptable?** Probably, but worth measuring. Alternative: binary IPC protocol or shared memory.

### 2. **No Backpressure**

If the browser is slow to decode, frames will queue up:
- Server keeps sending at source rate
- VideoDecoder queue grows
- Memory usage increases
- Eventually decode errors or dropped frames

**Potential fix:** Track decoder queue depth, signal backpressure to server.

### 3. **Audio/Video Sync**

Video and audio are independent streams with their own PTS. Currently:
- Video: Rendered immediately on decode
- Audio: Scheduled based on PTS relative to first frame

**What could go wrong:**
- Clock drift between audio and video timelines
- Network jitter affecting one stream more than other
- No explicit A/V sync mechanism

### 4. **Browser Compatibility**

WebCodecs support:
- Chrome 94+ ✓
- Edge 94+ ✓
- Safari 16.4+ ✓
- Firefox 130+ ✓ (very recent!)

**What's the fallback?** Currently none. The design doc mentions MSE and Broadway.js as potential fallbacks, but they're not implemented.

### 5. **Reconnection State**

When WebSocket disconnects and reconnects:
- `receivedFirstKeyframe` is reset ✓
- Audio timing is reset ✓
- But what about decoder state?

The `resetStreamState()` method resets flags but doesn't close/recreate decoders. If a decoder is in an error state, it might not recover.

### 6. **Codec Negotiation**

Current implementation hardcodes H264:
```typescript
video_supported_formats: createSupportedVideoFormatsBits({
    H264: true,
    // ... all others false
})
```

What if Wolf is configured for HEVC or AV1? The codec byte in the protocol supports them, but browser might not.

### 7. **Input Sub-type Parsing**

In the Rust streamer, input messages have a sub-type byte:
```rust
let sub_type = data[0];
match sub_type {
    2 if data.len() >= 3 => { /* button click */ }
    3 if data.len() >= 5 => { /* high-res scroll */ }
    // ...
}
```

But in TypeScript, the message type (0x10, 0x11, etc.) is separate from sub-type. Are these aligned correctly?

Looking at the code:
- `sendKey`: sub-type 0 for key input
- `sendMouseButton`: sub-type 2 for button
- `sendMouseWheelHighRes`: sub-type 3 for high-res wheel

This gets sent as `MouseClick` message type (0x11) with sub-type in payload. The Rust code expects sub-type 2 and 3 for `MouseClick`... but wait, button is sent with `MouseClick` (0x11) and wheels are also sent with `MouseClick`. That seems intentional.

**But:** `sendMouseMove` uses `MouseRelative` (0x13), while `sendMousePosition` uses `MouseAbsolute` (0x12). These are separate message types, not sub-types. Need to verify the Rust parsing matches.

### 8. **Session/Authentication**

The WebSocket URL includes `session_id`:
```typescript
const queryParams = this.sessionId
    ? `?session_id=${encodeURIComponent(this.sessionId)}`
    : ""
```

But is the WebSocket connection authenticated? The HTTP request goes through the proxy which validates Helix auth via cookie, but does the WebSocket upgrade preserve this?

### 9. **Proxy WebSocket Upgrade**

The `/moonlight` path is proxied. Does the proxy correctly handle WebSocket upgrade headers?
- `Connection: Upgrade`
- `Upgrade: websocket`
- `Sec-WebSocket-*` headers

This is a common misconfiguration in nginx/ingress.

### 10. **Canvas Sizing**

The canvas auto-resizes based on frame dimensions:
```typescript
if (this.canvas.width !== frame.displayWidth || this.canvas.height !== frame.displayHeight) {
    this.canvas.width = frame.displayWidth
    this.canvas.height = frame.displayHeight
}
```

But the CSS has `width: 100%`, `height: 100%`. This means the canvas has a native size (e.g., 1920x1080) but is displayed scaled. Is mouse coordinate mapping correct?

The `getStreamRect()` function in MoonlightStreamViewer calculates aspect-ratio-aware bounds, but does it account for canvas scaling vs video element scaling?

---

## Testing Checklist

When you get to testing:

1. **Basic connectivity**
   - [ ] WebSocket connects successfully
   - [ ] No CORS or authentication errors
   - [ ] Proxy handles WebSocket upgrade

2. **Video**
   - [ ] First keyframe received and decoded
   - [ ] Subsequent delta frames decode
   - [ ] Canvas displays video
   - [ ] No decode errors in console

3. **Audio**
   - [ ] Audio plays
   - [ ] No crackling or gaps
   - [ ] Roughly synced with video

4. **Input**
   - [ ] Mouse position (absolute) works
   - [ ] Mouse buttons work
   - [ ] Mouse wheel scrolls
   - [ ] Keyboard keys work
   - [ ] No stuck modifiers

5. **Reconnection**
   - [ ] Disconnect/reconnect recovers
   - [ ] Mode toggle reconnects cleanly
   - [ ] No memory leaks after multiple reconnects

6. **Performance**
   - [ ] CPU usage acceptable
   - [ ] Memory stable over time
   - [ ] Frame rate matches source
   - [ ] Latency feels responsive

---

## Summary

This is a significant architectural change that replaces WebRTC's media transport with a custom WebSocket-based protocol. The key insight is that we don't need peer-to-peer - it's always client-server, so WebSocket works fine.

**What's solid:**
- Binary protocol is clean and minimal
- WebCodecs provides hardware acceleration
- Input forwarding mirrors existing DataChannel format

**What needs attention:**
- IPC efficiency (base64 overhead)
- Backpressure / flow control
- Browser fallbacks for WebCodecs
- A/V sync under adverse conditions

Happy reading!
