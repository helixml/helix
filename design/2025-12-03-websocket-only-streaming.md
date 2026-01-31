# WebSocket-Only Streaming: Replacing WebRTC/TURN

**Date:** 2025-12-03
**Status:** Implementation Complete - Ready for Testing
**Author:** Claude + Luke
**Branch:** `fix/revdial-websocket-control` (helix) + separate branch for moonlight-web-stream

## Implementation Summary

### Completed Components

**moonlight-web-stream (Rust):**
- `common/src/ws_protocol.rs` - Binary message protocol (VideoFrame, AudioFrame, StreamInit, input types)
- `common/src/ipc.rs` - New IPC messages for video/audio frames with base64 encoding, InputIpcMessage for input forwarding
- `streamer/src/video.rs` - `WebSocketVideoDecoder` sends raw NAL units via IPC
- `streamer/src/audio.rs` - `WebSocketAudioDecoder` sends Opus frames via IPC with PTS
- `streamer/src/main.rs` - `run_websocket_only_mode()` function with input handling using correct MoonlightStream API
- `web-server/src/api/stream.rs` - `/api/ws/stream` endpoint for WebSocket-only mode

**helix (TypeScript):**
- `frontend/src/lib/moonlight-web-ts/stream/websocket-stream.ts` - Complete WebSocket stream client
  - WebCodecs VideoDecoder with Annex B format for in-band SPS/PPS
  - WebCodecs AudioDecoder for Opus with PTS-based scheduling
  - Canvas rendering with desynchronized mode
  - WebSocket-based input methods (keyboard, mouse, wheel)
  - Auto-reconnect with exponential backoff
  - Binary protocol matching Rust implementation
- `frontend/src/lib/moonlight-web-ts/component/settings_menu.ts` - Added `StreamingMode` type and `streamingMode` setting (default: 'websocket')
- `frontend/src/components/external-agent/MoonlightStreamViewer.tsx` - Updated to support both modes:
  - Added streaming mode toggle button in toolbar (WiFi icon = WebSocket, Signal icon = WebRTC)
  - Conditional stream creation: `WebSocketStream` for websocket mode, `Stream` for webrtc mode
  - Canvas element for WebSocket mode (WebCodecs renders to canvas), video element for WebRTC mode
  - Stats overlay shows transport mode
  - Default: WebSocket-only mode

### Issues Fixed (Previously in "Remaining Work")

1. ✅ **H264 SPS/PPS handling** - Configured WebCodecs with `avc: { format: "annexb" }` to handle in-band SPS/PPS.
   Added keyframe detection to skip delta frames until first keyframe is received.

2. ✅ **Audio scheduling** - Implemented PTS-based audio scheduling using `AudioContext.currentTime`.
   Audio frames are now scheduled relative to the first frame's PTS, with stale frame dropping.

3. ✅ **Input byte format** - Fixed Rust parsing to correctly handle sub-type bytes in each message.
   TypeScript WebSocket input methods now properly construct binary messages matching the expected format.

4. ✅ **MoonlightStream API** - Fixed to use correct API methods:
   - `send_keyboard_event(code, KeyAction, KeyModifiers)` instead of `send_keyboard_input`
   - `send_mouse_button(MouseButtonAction, MouseButton)` with proper enums
   - `send_high_res_scroll()` and `send_horizontal_scroll()` for wheel events

### Integration Testing Needed
- [ ] Compile both repos and verify no errors
- [ ] Test with actual Wolf/Moonlight setup
- [ ] Verify video decodes correctly (H264 with in-band SPS/PPS)
- [ ] Verify audio plays correctly (with proper timing)
- [ ] Verify keyboard/mouse input works
- [ ] Performance benchmarking vs WebRTC

## Design Decisions

- **Video decoding**: WebCodecs API (hardware-accelerated, lowest latency)
- **Audio decoding**: WebCodecs AudioDecoder (consistent with video approach)
- **Latency strategy**: Optimize for lowest latency - no buffering, render immediately
- **Bitrate adaptation**: Not supported (Moonlight limitation) - use fixed bitrate
- **Reconnection**: Seamless auto-reconnect with visual indicator, preserve session state
- **Migration approach**: Full migration (video + audio + input) in single feature branch

## Problem Statement

WebRTC/TURN is extremely difficult to terminate in customer infrastructure that only has L7 (HTTP/HTTPS) ingress available. TURN servers require:
- UDP port ranges for media relay
- TCP fallback on non-standard ports (3478, 5349)
- Credential management for TURN authentication
- Complex NAT traversal configuration

Most enterprise deployments only allow HTTP/HTTPS traffic through L7 load balancers or ingress controllers. This makes WebRTC streaming impractical.

## Proposed Solution

Replace WebRTC entirely with a **WebSocket-only transport** for both:
1. **Video/Audio** (server → client): Encoded media frames sent as binary WebSocket messages
2. **Input** (client → server): Mouse/keyboard/gamepad events as binary WebSocket messages

### Why This Works

Unlike WebRTC, we don't need peer-to-peer connectivity. Our architecture is strictly client-server:
- Browser connects to moonlight-web-stream server
- Server receives encoded video from Wolf/Moonlight
- Server forwards to browser
- Browser sends input back

WebSockets work perfectly with L7 ingress controllers (nginx, traefik, envoy, etc.) and only require HTTP/HTTPS.

## Current Architecture (WebRTC)

```
┌─────────────────┐     WebSocket      ┌──────────────────────┐
│  Browser        │◄───────────────────►│  moonlight-web       │
│  (Helix FE)     │     (signaling)     │  (Rust web-server)   │
│                 │                     │                      │
│  RTCPeerConn    │◄═══════════════════►│  RTCPeerConnection   │
│  - video track  │     WebRTC/UDP      │  - video sender      │
│  - audio track  │     (or TURN relay) │  - audio sender      │
│  - input DC     │                     │  - input receiver    │
└─────────────────┘                     └──────────────────────┘
                                                  │
                                                  │ IPC (stdin/stdout)
                                                  ▼
                                        ┌──────────────────────┐
                                        │  streamer process    │
                                        │  (Rust)              │
                                        │  - WebRTC peer       │
                                        │  - video payloader   │
                                        │  - Moonlight client  │
                                        └──────────────────────┘
                                                  │
                                                  │ Moonlight protocol
                                                  ▼
                                        ┌──────────────────────┐
                                        │  Wolf (NVIDIA enc)   │
                                        │  - H264/HEVC/AV1     │
                                        └──────────────────────┘
```

### Current Data Channels (WebRTC)

| Channel | Direction | Content |
|---------|-----------|---------|
| video track | server→client | RTP-packetized H264/H265/AV1 |
| audio track | server→client | RTP-packetized Opus |
| keyboard | client→server | Binary: key events |
| mouseClicks | client→server | Binary: button events |
| mouseAbsolute | client→server | Binary: absolute position |
| mouseRelative | client→server | Binary: relative movement |
| touch | bidirectional | Binary: touch events |
| controllers | bidirectional | Binary: gamepad connect/disconnect |
| controllerN | client→server | Binary: gamepad state |
| general | server→client | JSON: status updates |

## New Architecture (WebSocket-Only)

```
┌─────────────────┐                     ┌──────────────────────┐
│  Browser        │     WebSocket       │  moonlight-web       │
│  (Helix FE)     │◄═══════════════════►│  (Rust web-server)   │
│                 │     (single conn)   │                      │
│  MediaSource    │                     │                      │
│  or WebCodecs   │     ┌───────────┐   │                      │
│  for decoding   │     │ Frames    │   │  Frame extractor     │
│                 │     │ (binary)  │   │  (NAL units)         │
│  Input sender   │     ├───────────┤   │                      │
│                 │     │ Input     │   │  Input handler       │
│                 │     │ (binary)  │   │                      │
└─────────────────┘     └───────────┘   └──────────────────────┘
                                                  │
                                                  │ IPC (stdin/stdout)
                                                  ▼
                                        ┌──────────────────────┐
                                        │  streamer process    │
                                        │  (modified)          │
                                        │  - NO WebRTC         │
                                        │  - Frame extraction  │
                                        │  - Moonlight client  │
                                        └──────────────────────┘
```

### Transport Protocol

Single WebSocket connection with binary message framing:

```
┌────────────┬────────────┬────────────────────────────────────┐
│ Type (1B)  │ Length (4B)│ Payload (variable)                 │
└────────────┴────────────┴────────────────────────────────────┘

Message Types:
  0x01 - Video Frame (server → client)
  0x02 - Audio Frame (server → client)
  0x10 - Keyboard Input (client → server)
  0x11 - Mouse Click (client → server)
  0x12 - Mouse Absolute (client → server)
  0x13 - Mouse Relative (client → server)
  0x14 - Touch Event (bidirectional)
  0x15 - Controller Event (bidirectional)
  0x16 - Controller State (client → server)
  0x20 - Control Message (bidirectional, JSON)
```

### Video Frame Format

```
┌───────────────────────────────────────────────────────────────┐
│ Video Frame Message (Type 0x01)                               │
├────────────┬────────────┬────────────┬────────────┬───────────┤
│ Codec (1B) │ Flags (1B) │ PTS (8B)   │ Width (2B) │ Height(2B)│
├────────────┴────────────┴────────────┴────────────┴───────────┤
│ NAL Unit Data (raw H264/H265/AV1 access units)                │
└───────────────────────────────────────────────────────────────┘

Codec: 0x01=H264, 0x02=H265, 0x03=AV1
Flags: 0x01=keyframe, 0x02=config_changed
PTS: Presentation timestamp in microseconds
```

### Audio Frame Format

```
┌───────────────────────────────────────────────────────────────┐
│ Audio Frame Message (Type 0x02)                               │
├────────────┬────────────┬────────────┬────────────────────────┤
│ Codec (1B) │ Channels   │ PTS (8B)   │ Opus Frame Data        │
│            │ (1B)       │            │                        │
└────────────┴────────────┴────────────┴────────────────────────┘

Codec: 0x01=Opus (only option for now)
```

## Browser Video Decoding Options

### Option 1: WebCodecs API (Recommended)

The [WebCodecs API](https://developer.mozilla.org/en-US/docs/Web/API/WebCodecs_API) provides low-level access to video decoders:

```typescript
// Initialize decoder
const decoder = new VideoDecoder({
  output: (frame: VideoFrame) => {
    // Draw frame to canvas
    ctx.drawImage(frame, 0, 0);
    frame.close();
  },
  error: (e) => console.error(e),
});

// Configure for H264
await decoder.configure({
  codec: 'avc1.4d0032', // H264 Main Profile
  codedWidth: 1920,
  codedHeight: 1080,
  hardwareAcceleration: 'prefer-hardware',
});

// Decode incoming frame
ws.onmessage = (event) => {
  const data = new Uint8Array(event.data);
  if (data[0] === 0x01) { // Video frame
    const chunk = new EncodedVideoChunk({
      type: data[5] & 0x01 ? 'key' : 'delta',
      timestamp: readPTS(data), // microseconds
      data: data.slice(HEADER_SIZE),
    });
    decoder.decode(chunk);
  }
};
```

**Pros:**
- Hardware-accelerated decoding
- Low latency (no buffering like MSE)
- Direct frame access for canvas rendering
- Supported in Chrome 94+, Edge 94+, Safari 16.4+, Firefox 130+

**Cons:**
- Requires handling codec configuration manually
- Need to extract SPS/PPS from H264 stream for init

### Option 2: MediaSource Extensions (MSE) - Fallback

For older browsers, use MSE with fMP4 container:

```typescript
const mediaSource = new MediaSource();
video.src = URL.createObjectURL(mediaSource);

mediaSource.addEventListener('sourceopen', () => {
  const sourceBuffer = mediaSource.addSourceBuffer('video/mp4; codecs="avc1.4d0032"');

  // Need to wrap NAL units in fMP4 segments
  sourceBuffer.appendBuffer(createFmp4Segment(nalUnits));
});
```

**Pros:**
- Wider browser support
- Uses existing video element

**Cons:**
- Requires fMP4 muxing (complex)
- Higher latency due to buffering
- Cannot achieve ultra-low latency

### Option 3: Broadway.js (Pure JS) - Emergency Fallback

JavaScript H264 decoder for environments without hardware decode:

```typescript
const player = new Player({ useWorker: true });
player.onPictureDecoded = (data, width, height) => {
  // Draw YUV data to canvas
};

ws.onmessage = (event) => {
  player.decode(new Uint8Array(event.data));
};
```

**Cons:**
- Software decoding = high CPU
- Maximum ~720p30 on modern hardware
- Only H264 support

### Recommended Strategy

```typescript
async function createVideoDecoder(): Promise<VideoDecoderInterface> {
  // Try WebCodecs first
  if ('VideoDecoder' in window) {
    const support = await VideoDecoder.isConfigSupported({
      codec: 'avc1.4d0032',
      codedWidth: 1920,
      codedHeight: 1080,
    });
    if (support.supported) {
      return new WebCodecsDecoder();
    }
  }

  // Fallback to MSE
  if ('MediaSource' in window && MediaSource.isTypeSupported('video/mp4; codecs="avc1.4d0032"')) {
    return new MSEDecoder();
  }

  // Last resort: Broadway.js
  return new BroadwayDecoder();
}
```

## Server-Side Changes (moonlight-web-stream)

### 1. Remove WebRTC from Streamer

Current `video.rs` creates RTP packets and sends via WebRTC track. Replace with:

```rust
// New: Send raw NAL units over IPC to web-server
pub struct WebSocketVideoEncoder {
    sender: IpcSender<StreamerIpcMessage>,
    codec: VideoCodec,
}

impl VideoDecoder for WebSocketVideoEncoder {
    fn submit_decode_unit(&mut self, unit: VideoDecodeUnit<'_>) -> DecodeResult {
        // Concatenate NAL units
        let mut frame_data = Vec::new();
        for buffer in unit.buffers {
            frame_data.extend_from_slice(buffer.data);
        }

        // Send via IPC as WebSocket frame
        self.sender.blocking_send(StreamerIpcMessage::VideoFrame {
            codec: self.codec,
            timestamp_us: unit.presentation_time.as_micros() as u64,
            keyframe: unit.is_idr,
            data: frame_data,
        });

        DecodeResult::Ok
    }
}
```

### 2. New IPC Messages

```rust
pub enum StreamerIpcMessage {
    // Existing messages...

    // New: Raw video/audio frames for WebSocket transport
    VideoFrame {
        codec: VideoCodec,
        timestamp_us: u64,
        keyframe: bool,
        width: u32,
        height: u32,
        data: Vec<u8>,
    },
    AudioFrame {
        timestamp_us: u64,
        data: Vec<u8>,
    },
}

pub enum ServerIpcMessage {
    // Existing messages...

    // New: Input from WebSocket
    KeyboardInput { data: Vec<u8> },
    MouseInput { data: Vec<u8> },
    TouchInput { data: Vec<u8> },
    ControllerInput { data: Vec<u8> },
}
```

### 3. Web Server WebSocket Handler

```rust
async fn handle_websocket(ws: WebSocketUpgrade) -> Response {
    ws.on_upgrade(|socket| async move {
        let (mut tx, mut rx) = socket.split();

        // Spawn frame sender task
        tokio::spawn(async move {
            while let Some(frame) = video_rx.recv().await {
                let msg = encode_video_frame(&frame);
                tx.send(Message::Binary(msg)).await?;
            }
        });

        // Handle incoming input
        while let Some(Ok(msg)) = rx.next().await {
            if let Message::Binary(data) = msg {
                handle_input_message(&data).await;
            }
        }
    })
}
```

## Client-Side Changes (Helix Frontend)

### 1. New WebSocket Stream Class

Replace `Stream` class in `frontend/src/lib/moonlight-web-ts/stream/index.ts`:

```typescript
export class WebSocketStream {
  private ws: WebSocket;
  private videoDecoder: VideoDecoderInterface;
  private audioDecoder: AudioDecoderInterface;
  private input: StreamInput;
  private canvas: HTMLCanvasElement;

  constructor(url: string, canvas: HTMLCanvasElement) {
    this.canvas = canvas;
    this.ws = new WebSocket(url);
    this.ws.binaryType = 'arraybuffer';

    this.ws.onmessage = this.handleMessage.bind(this);
    this.input = new WebSocketStreamInput(this.ws);
  }

  private async handleMessage(event: MessageEvent) {
    const data = new Uint8Array(event.data);
    const type = data[0];

    switch (type) {
      case 0x01: // Video
        await this.handleVideoFrame(data);
        break;
      case 0x02: // Audio
        await this.handleAudioFrame(data);
        break;
      case 0x14: // Touch capability response
        this.input.handleTouchResponse(data);
        break;
      case 0x15: // Controller rumble
        this.input.handleControllerMessage(data);
        break;
      case 0x20: // Control message
        this.handleControlMessage(data);
        break;
    }
  }

  private async handleVideoFrame(data: Uint8Array) {
    const frame = parseVideoFrame(data);
    await this.videoDecoder.decode(frame);
  }
}
```

### 2. WebSocket Input Handler

Modify `input.ts` to send over WebSocket instead of RTCDataChannel:

```typescript
export class WebSocketStreamInput {
  private ws: WebSocket;
  private buffer: ByteBuffer = new ByteBuffer(1024);

  constructor(ws: WebSocket) {
    this.ws = ws;
  }

  private send(type: number, buffer: ByteBuffer) {
    if (this.ws.readyState !== WebSocket.OPEN) return;

    buffer.flip();
    const payload = buffer.getReadBuffer();

    // Prepend message type
    const message = new Uint8Array(1 + payload.length);
    message[0] = type;
    message.set(payload, 1);

    this.ws.send(message.buffer);
  }

  sendKey(isDown: boolean, key: number, modifiers: number) {
    this.buffer.reset();
    this.buffer.putU8(0); // sub-type for keyboard
    this.buffer.putBool(isDown);
    this.buffer.putU8(modifiers);
    this.buffer.putU16(key);
    this.send(0x10, this.buffer);
  }

  // ... similar for mouse, touch, gamepad
}
```

## Latency Considerations

### Current WebRTC Latency
- RTP packetization + NACK/PLI retransmission: ~1-2 frames
- Jitter buffer: ~0-50ms
- TURN relay (if used): +20-50ms RTT
- **Total: ~50-150ms**

### WebSocket-Only Latency
- No jitter buffer (we accept frames as they arrive)
- No retransmission (lossy is OK for real-time video)
- Direct TCP/TLS: +10-30ms vs UDP
- **Estimated Total: ~30-80ms**

WebSocket may actually be **lower latency** because:
1. No jitter buffer needed (we render immediately)
2. No TURN relay overhead
3. Direct connection through L7 ingress

However, TCP has head-of-line blocking. For lossy networks, some frames may stall while waiting for retransmission. We can mitigate with:
- Aggressive keyframe insertion on packet loss detection
- Client-side frame dropping for stale frames

## Implementation Plan

Full migration in single feature branch. No phased approach.

### Server-Side (moonlight-web-stream)

1. **New WebSocket endpoint** (`/api/ws/stream`)
   - Binary message framing protocol
   - Authentication via existing cookie/token
   - Session routing for RevDial

2. **Streamer process changes**
   - Remove WebRTC peer creation
   - Send raw NAL units via IPC instead of RTP packets
   - Forward input messages to Moonlight

3. **Web server changes**
   - New WebSocket handler for streaming
   - Frame message serialization
   - Input message deserialization and IPC forwarding

### Client-Side (Helix Frontend)

1. **WebCodecs video decoder**
   - Hardware-accelerated H264/H265/AV1 decoding
   - Canvas rendering with `requestVideoFrameCallback`
   - Codec configuration from stream init message

2. **WebCodecs audio decoder**
   - Opus decoding
   - Web Audio API for playback (`AudioContext`)
   - Minimal buffering for smooth playback

3. **WebSocket input handler**
   - Same binary protocol as current DataChannels
   - Keyboard, mouse, touch, gamepad support
   - Replace `StreamInput` class to use WebSocket

4. **Stream component updates**
   - Replace `useMoonlightStream` hook
   - New `WebSocketStream` class
   - Auto-reconnect with visual indicator

### Files to Modify

**moonlight-web-stream:**
- `moonlight-web/streamer/src/main.rs` - Remove WebRTC, add frame forwarding
- `moonlight-web/streamer/src/video.rs` - Send NAL units instead of RTP
- `moonlight-web/streamer/src/audio.rs` - Send Opus frames instead of RTP
- `moonlight-web/streamer/src/sender.rs` - Remove (WebRTC-specific)
- `moonlight-web/web-server/src/api/stream.rs` - New WebSocket handler
- `moonlight-web/common/src/ipc.rs` - New IPC message types
- `moonlight-web/common/src/api_bindings.rs` - New message types

**helix frontend:**
- `frontend/src/lib/moonlight-web-ts/stream/index.ts` - WebSocket stream class
- `frontend/src/lib/moonlight-web-ts/stream/input.ts` - WebSocket input
- `frontend/src/lib/moonlight-web-ts/stream/video.ts` - WebCodecs decoder
- `frontend/src/lib/moonlight-web-ts/stream/audio.ts` - WebCodecs audio decoder (new)
- `frontend/src/hooks/useMoonlightStream.ts` - Update for WebSocket
- `frontend/src/components/external-agent/MoonlightStreamViewer.tsx` - Canvas rendering

## Resolved Design Questions

1. **Audio handling**: WebCodecs AudioDecoder for consistency with video and lowest latency

2. **Frame pacing**: Render immediately on frame arrival - no buffering, no pacing. Let the encoder's frame rate drive the display. Use `requestVideoFrameCallback` for smooth rendering when available.

3. **Bandwidth adaptation**: Not implemented (Moonlight doesn't support dynamic bitrate). Use configured fixed bitrate.

4. **Connection resilience**: Auto-reconnect with exponential backoff. Show visual indicator during reconnect. Preserve session state (Moonlight session persists on server, just need new WebSocket).

5. **Multiple streams**: Out of scope for initial implementation.

## References

- [WebCodecs API](https://developer.mozilla.org/en-US/docs/Web/API/WebCodecs_API)
- [MediaSource Extensions](https://developer.mozilla.org/en-US/docs/Web/API/MediaSource)
- [Broadway.js H264 Decoder](https://github.com/nicholaslanam/Broadway)
- [Current moonlight-web-stream](~/pm/moonlight-web-stream)
- [Helix Frontend Stream Code](frontend/src/lib/moonlight-web-ts/stream/)
