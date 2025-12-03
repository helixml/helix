# WebSocket-Only Streaming: Architecture Deep Dive

**Reading time:** ~90 minutes (extended version)
**Purpose:** Understand the full implementation, find gaps, think about edge cases

---

## The Problem We're Solving

WebRTC requires TURN servers for NAT traversal, which need UDP port ranges or non-standard TCP ports (3478, 5349). Enterprise deployments only allow HTTP/HTTPS through L7 load balancers. **WebSocket-only streaming bypasses WebRTC entirely** - everything goes through standard HTTPS.

### Why This Matters for Helix

Helix runs AI agents in GPU-accelerated containers. Users view these containers via Moonlight streaming. The typical enterprise deployment:

```
Internet → CloudFlare/Akamai → L7 Load Balancer → Kubernetes Ingress → Helix
                                    │
                                    └─ ONLY allows HTTP/HTTPS on 443
```

WebRTC's TURN/STUN requires:
- UDP 3478 (STUN)
- TCP/UDP 3478 (TURN)
- UDP 49152-65535 (media relay)

These ports are blocked. Our solution: **tunnel everything through the existing HTTPS WebSocket path**.

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

## What I Actually Implemented

### Overview of Changes

**Rust side (moonlight-web-stream repo):**
1. `common/src/ws_protocol.rs` - Binary message type constants
2. `common/src/ipc.rs` - New IPC message types for frames and input
3. `streamer/src/video.rs` - `WebSocketVideoDecoder` that sends raw NAL units
4. `streamer/src/audio.rs` - `WebSocketAudioDecoder` that sends Opus frames
5. `streamer/src/main.rs` - `run_websocket_only_mode()` function
6. `web-server/src/api/stream.rs` - `/api/ws/stream` WebSocket endpoint

**TypeScript side (helix repo):**
1. `frontend/src/lib/moonlight-web-ts/stream/websocket-stream.ts` - Complete WebSocket client
2. `frontend/src/lib/moonlight-web-ts/component/settings_menu.ts` - `StreamingMode` type
3. `frontend/src/components/external-agent/MoonlightStreamViewer.tsx` - Mode toggle UI

---

## Source Files to Read (In Order)

### 1. Binary Protocol Constants (5 min)

**File:** `moonlight-web-stream/moonlight-web/common/src/ws_protocol.rs`

This defines message type bytes used everywhere:

```rust
/// Message types for WebSocket binary protocol
pub mod WsMessageType {
    pub const VIDEO_FRAME: u8 = 0x01;
    pub const AUDIO_FRAME: u8 = 0x02;
    pub const KEYBOARD_INPUT: u8 = 0x10;
    pub const MOUSE_CLICK: u8 = 0x11;
    pub const MOUSE_ABSOLUTE: u8 = 0x12;
    pub const MOUSE_RELATIVE: u8 = 0x13;
    pub const TOUCH_EVENT: u8 = 0x14;
    pub const CONTROLLER_EVENT: u8 = 0x15;
    pub const CONTROLLER_STATE: u8 = 0x16;
    pub const CONTROL_MESSAGE: u8 = 0x20;
    pub const STREAM_INIT: u8 = 0x30;
    pub const STREAM_ERROR: u8 = 0x31;
}

/// Video codec identifiers
pub mod WsVideoCodec {
    pub const H264: u8 = 0x01;
    pub const H264_HIGH_444: u8 = 0x02;
    pub const H265: u8 = 0x10;
    pub const H265_MAIN10: u8 = 0x11;
    pub const H265_REXT8_444: u8 = 0x12;
    pub const H265_REXT10_444: u8 = 0x13;
    pub const AV1_MAIN8: u8 = 0x20;
    pub const AV1_MAIN10: u8 = 0x21;
    pub const AV1_HIGH8_444: u8 = 0x22;
    pub const AV1_HIGH10_444: u8 = 0x23;
}
```

**TypeScript counterpart** in `websocket-stream.ts`:
```typescript
const WsMessageType = {
  VideoFrame: 0x01,
  AudioFrame: 0x02,
  KeyboardInput: 0x10,
  MouseClick: 0x11,
  MouseAbsolute: 0x12,
  MouseRelative: 0x13,
  TouchEvent: 0x14,
  ControllerEvent: 0x15,
  ControllerState: 0x16,
  ControlMessage: 0x20,
  StreamInit: 0x30,
  StreamError: 0x31,
} as const

const WsVideoCodec = {
  H264: 0x01,
  H264High444: 0x02,
  H265: 0x10,
  H265Main10: 0x11,
  H265Rext8_444: 0x12,
  H265Rext10_444: 0x13,
  Av1Main8: 0x20,
  Av1Main10: 0x21,
  Av1High8_444: 0x22,
  Av1High10_444: 0x23,
} as const
```

**Questions:**
- Is the protocol versioned? **No.** If we need changes, we'll need a version negotiation mechanism.
- Endianness: **Big-endian** for all multi-byte values (network byte order)
- Alignment: **None** - packed binary format

---

### 2. IPC Messages (10 min)

**File:** `moonlight-web-stream/moonlight-web/common/src/ipc.rs`

The streamer process communicates with web-server via stdin/stdout. Messages are JSON-serialized.

**New message types I added:**

```rust
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(tag = "type")]
pub enum StreamerIpcMessage {
    // ... existing WebSocket, Stop, etc ...

    /// Video frame for WebSocket-only mode (base64-encoded NAL units)
    VideoFrame {
        codec: u8,
        timestamp_us: u64,
        keyframe: bool,
        width: u32,
        height: u32,
        data: String,  // base64-encoded
    },

    /// Audio frame for WebSocket-only mode (base64-encoded Opus)
    AudioFrame {
        timestamp_us: u64,
        channels: u8,
        data: String,  // base64-encoded
    },
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(tag = "type")]
pub enum ServerIpcMessage {
    // ... existing Init, WebSocket, Stop, ClientJoined ...

    /// Input from browser (for WebSocket-only mode)
    Input(InputIpcMessage),
}

/// Input message types for WebSocket-only mode
#[derive(Debug, Clone, Serialize, Deserialize)]
pub enum InputIpcMessage {
    Keyboard(Vec<u8>),
    MouseClick(Vec<u8>),
    MouseAbsolute(Vec<u8>),
    MouseRelative(Vec<u8>),
    Touch(Vec<u8>),
    ControllerEvent(Vec<u8>),
    ControllerState { controller_id: u8, data: Vec<u8> },
}
```

**Why base64?**

The IPC protocol is JSON over stdin/stdout. Binary data must be encoded. Base64 adds ~33% overhead but is simple and reliable.

**Alternative approaches I considered:**
1. **Binary IPC protocol** - More efficient but requires custom framing
2. **Unix socket with binary** - Even more efficient but adds complexity
3. **Shared memory** - Fastest but complex synchronization

For v1, base64 JSON is acceptable. At 60fps 1080p 20Mbps:
- ~2.5 MB/sec raw video
- ~3.3 MB/sec base64
- JSON overhead: ~100 bytes/frame → negligible

---

### 3. Video Decoder - Server Side (15 min)

**File:** `moonlight-web-stream/moonlight-web/streamer/src/video.rs`

This is where video frames from Wolf/Moonlight are captured and sent to the browser.

**The key struct:**

```rust
pub struct WebSocketVideoDecoder {
    sender: UnboundedSender<StreamerIpcMessage>,
    codec: VideoCodec,
    width: u32,
    height: u32,
}

impl WebSocketVideoDecoder {
    pub fn new(
        sender: UnboundedSender<StreamerIpcMessage>,
        supported_formats: VideoSupportedFormats,
    ) -> Self {
        // Determine best codec from what browser supports
        let codec = if supported_formats.contains(VideoSupportedFormats::H264) {
            VideoCodec::H264
        } else {
            VideoCodec::H264 // Fallback - H264 is always supported
        };

        Self {
            sender,
            codec,
            width: 0,
            height: 0,
        }
    }
}
```

**The critical method - `submit_decode_unit`:**

```rust
impl VideoDecoder for WebSocketVideoDecoder {
    fn submit_decode_unit(&mut self, unit: VideoDecodeUnit<'_>) -> DecodeResult {
        // Concatenate all NAL unit buffers into one frame
        let mut frame_data = Vec::new();
        for buffer in unit.buffers {
            frame_data.extend_from_slice(buffer.data);
        }

        // Update dimensions from frame
        self.width = unit.width;
        self.height = unit.height;

        // Encode as base64 and send via IPC
        let encoded = base64::engine::general_purpose::STANDARD.encode(&frame_data);

        let _ = self.sender.send(StreamerIpcMessage::VideoFrame {
            codec: self.codec as u8,
            timestamp_us: unit.presentation_time.as_micros() as u64,
            keyframe: unit.is_idr,
            width: self.width,
            height: self.height,
            data: encoded,
        });

        DecodeResult::Ok
    }
}
```

**Key insight: Raw NAL units, not RTP**

In WebRTC mode, the `TrackSampleVideoDecoder` wraps NAL units in RTP packets with sequence numbers, timestamps, etc. WebCodecs doesn't need this - it expects raw H264 access units (NAL units with Annex B start codes).

Wolf's encoder produces NAL units like:
```
[00 00 00 01][SPS NAL][00 00 00 01][PPS NAL][00 00 00 01][IDR slice]
```

For keyframes (IDR), SPS and PPS are included. This is crucial - WebCodecs needs SPS/PPS to initialize the decoder.

**What I changed from WebRTC mode:**
- Removed RTP packetization
- Removed sequence number tracking
- Removed jitter buffer considerations
- Just: concatenate buffers → base64 → IPC

---

### 4. Audio Decoder - Server Side (10 min)

**File:** `moonlight-web-stream/moonlight-web/streamer/src/audio.rs`

Similar pattern to video:

```rust
pub struct WebSocketAudioDecoder {
    sender: UnboundedSender<StreamerIpcMessage>,
}

impl AudioDecoder for WebSocketAudioDecoder {
    fn submit_decode_unit(&mut self, unit: AudioDecodeUnit<'_>) -> AudioDecodeResult {
        let encoded = base64::engine::general_purpose::STANDARD.encode(unit.data);

        let _ = self.sender.send(StreamerIpcMessage::AudioFrame {
            timestamp_us: unit.presentation_time.as_micros() as u64,
            channels: unit.channels,
            data: encoded,
        });

        AudioDecodeResult::Ok
    }
}
```

**Opus frames are small:**
- ~20ms of audio per frame
- 48kHz stereo Opus at 128kbps ≈ 320 bytes/frame
- 50 frames/second
- ~16 KB/sec total

The per-frame IPC overhead is relatively higher for audio than video, but still acceptable.

---

### 5. Main Streamer Logic (25 min) ⭐ CRITICAL

**File:** `moonlight-web-stream/moonlight-web/streamer/src/main.rs`

This is the heart of WebSocket-only streaming. Find `run_websocket_only_mode()`:

```rust
async fn run_websocket_only_mode(
    mut ipc_sender: IpcSender<StreamerIpcMessage>,
    mut ipc_receiver: IpcReceiver<ServerIpcMessage>,
    stream_settings: StreamSettings,
    host_address: String,
    host_http_port: u16,
    client_private_key_pem: String,
    client_certificate_pem: String,
    server_certificate_pem: String,
    app_id: u32,
) {
    info!("[WebSocket-Only]: Initializing...");

    // 1. Create Moonlight host connection
    let mut host = ReqwestMoonlightHost::new(host_address, host_http_port, None)
        .expect("failed to create host");

    // 2. Set pairing credentials (for encrypted Moonlight protocol)
    host.set_pairing_info(
        &ClientAuth {
            private_key: Pem::from_str(&client_private_key_pem).expect("..."),
            certificate: Pem::from_str(&client_certificate_pem).expect("..."),
        },
        &Pem::from_str(&server_certificate_pem).expect("..."),
    ).expect("failed to set pairing info");

    // 3. Get global Moonlight instance
    let moonlight = MoonlightInstance::global().expect("failed to find moonlight");

    // 4. Create IPC channel for frames
    let (frame_tx, mut frame_rx) = unbounded_channel::<StreamerIpcMessage>();

    // 5. Create WebSocket decoders (NOT WebRTC track senders!)
    let video_decoder = WebSocketVideoDecoder::new(
        frame_tx.clone(),
        stream_settings.video_supported_formats,
    );
    let audio_decoder = WebSocketAudioDecoder::new(frame_tx.clone());

    // 6. Spawn task to forward frames to IPC
    let ipc_sender_for_frames = ipc_sender.clone();
    spawn(async move {
        while let Some(msg) = frame_rx.recv().await {
            ipc_sender_for_frames.clone().send(msg).await;
        }
    });

    // 7. Start Moonlight stream with our decoders
    let (stream, _client_id) = host.start_stream(
        &moonlight,
        app_id,
        stream_settings.width,
        stream_settings.height,
        stream_settings.fps,
        /* ... other params ... */
        video_decoder,
        audio_decoder,
    ).await.expect("failed to start stream");

    // 8. Send ConnectionComplete to browser
    ipc_sender.send(StreamerIpcMessage::WebSocket(
        StreamServerMessage::ConnectionComplete { /* ... */ }
    )).await;

    // 9. MAIN LOOP: Handle input from browser
    while let Some(msg) = ipc_receiver.recv().await {
        match msg {
            ServerIpcMessage::Input(input) => {
                // Forward to Moonlight/Wolf
                handle_input(&stream, input);
            }
            ServerIpcMessage::Stop => {
                break;
            }
            _ => {}
        }
    }

    // 10. Cleanup
    drop(stream);
    host.cancel().await.ok();
}
```

**Input Handling - The Tricky Part**

This is where I spent the most time. The Moonlight library has a specific API that's different from the DataChannel binary format.

```rust
fn handle_input(stream: &MoonlightStream, input: InputIpcMessage) {
    match input {
        InputIpcMessage::Keyboard(data) => {
            // Format: subType(1) + isDown(1) + modifiers(1) + keyCode(2)
            if data.len() >= 5 {
                let sub_type = data[0];
                if sub_type == 0 {
                    // Key input
                    let is_down = data[1] != 0;
                    let modifiers_byte = data[2];
                    let key_code = u16::from_be_bytes([data[3], data[4]]) as i16;

                    let action = if is_down {
                        KeyAction::Down
                    } else {
                        KeyAction::Up
                    };
                    let modifiers = KeyModifiers::from_bits_truncate(modifiers_byte as i8);

                    // ⭐ Correct API: send_keyboard_event, not send_keyboard_input
                    stream.send_keyboard_event(key_code, action, modifiers);
                }
            }
        }

        InputIpcMessage::MouseClick(data) => {
            // Format: subType(1) + ...
            // subType 2 = button: isDown(1) + button(1)
            // subType 3 = wheel high-res: deltaX(2) + deltaY(2)
            // subType 4 = wheel normal: deltaX(1) + deltaY(1)

            let sub_type = data[0];
            match sub_type {
                2 if data.len() >= 3 => {
                    let is_down = data[1] != 0;
                    let button_byte = data[2];

                    let action = if is_down {
                        MouseButtonAction::Press
                    } else {
                        MouseButtonAction::Release
                    };

                    // Button mapping: browser → Moonlight
                    let button = match button_byte {
                        1 => MouseButton::Left,
                        2 => MouseButton::Middle,
                        3 => MouseButton::Right,
                        4 => MouseButton::X1,
                        5 => MouseButton::X2,
                        _ => MouseButton::Left,
                    };

                    // ⭐ Correct API: send_mouse_button(action, button)
                    stream.send_mouse_button(action, button);
                }
                3 if data.len() >= 5 => {
                    // High-res scroll
                    let delta_x = i16::from_be_bytes([data[1], data[2]]);
                    let delta_y = i16::from_be_bytes([data[3], data[4]]);

                    // ⭐ Separate methods for X and Y!
                    if delta_y != 0 {
                        stream.send_high_res_scroll(delta_y);
                    }
                    if delta_x != 0 {
                        stream.send_high_res_horizontal_scroll(delta_x);
                    }
                }
                4 if data.len() >= 3 => {
                    // Normal scroll
                    let delta_x = data[1] as i8;
                    let delta_y = data[2] as i8;

                    if delta_y != 0 {
                        stream.send_scroll(delta_y);
                    }
                    if delta_x != 0 {
                        stream.send_horizontal_scroll(delta_x);
                    }
                }
                _ => {}
            }
        }

        InputIpcMessage::MouseAbsolute(data) => {
            // Format: subType(1) + x(2) + y(2) + refWidth(2) + refHeight(2)
            if data.len() >= 9 {
                let x = i16::from_be_bytes([data[1], data[2]]);
                let y = i16::from_be_bytes([data[3], data[4]]);
                let ref_width = i16::from_be_bytes([data[5], data[6]]);
                let ref_height = i16::from_be_bytes([data[7], data[8]]);

                stream.send_mouse_position(x, y, ref_width, ref_height);
            }
        }

        InputIpcMessage::MouseRelative(data) => {
            // Format: subType(1) + dx(2) + dy(2)
            if data.len() >= 5 {
                let dx = i16::from_be_bytes([data[1], data[2]]);
                let dy = i16::from_be_bytes([data[3], data[4]]);

                stream.send_mouse_move(dx, dy);
            }
        }

        // Touch and Controller: TODO
        _ => {}
    }
}
```

**API Discovery Pain**

I initially got the Moonlight API wrong. The library has multiple similar methods:
- `send_keyboard_input(code, modifiers, is_down)` - **WRONG** (doesn't exist)
- `send_keyboard_event(code, KeyAction, KeyModifiers)` - **CORRECT**
- `send_mouse_button(button, is_down)` - **WRONG** (signature is different)
- `send_mouse_button(MouseButtonAction, MouseButton)` - **CORRECT**

The fix required importing the correct types:
```rust
use moonlight_common::stream::bindings::{
    KeyAction, KeyModifiers, MouseButtonAction, MouseButton,
};
```

---

### 6. Web Server WebSocket Endpoint (10 min)

**File:** `moonlight-web-stream/moonlight-web/web-server/src/api/stream.rs`

The endpoint handles:
1. WebSocket upgrade
2. Forwarding frames from streamer to browser
3. Receiving input from browser, forwarding to streamer

```rust
pub async fn ws_stream_handler(
    ws: WebSocketUpgrade,
    State(state): State<AppState>,
    Query(params): Query<StreamParams>,
) -> Response {
    ws.on_upgrade(|socket| handle_ws_stream(socket, state, params.session_id))
}

async fn handle_ws_stream(
    socket: WebSocket,
    state: AppState,
    session_id: Option<String>,
) {
    let (mut tx, mut rx) = socket.split();

    // Get streamer IPC channels for this session
    let (frame_rx, input_tx) = state.get_streamer_channels(session_id);

    // Task 1: Forward frames to browser
    let tx_task = spawn(async move {
        while let Some(frame) = frame_rx.recv().await {
            let binary = encode_frame_to_binary(&frame);
            if tx.send(Message::Binary(binary)).await.is_err() {
                break;
            }
        }
    });

    // Task 2: Receive input from browser
    let rx_task = spawn(async move {
        while let Some(Ok(msg)) = rx.next().await {
            if let Message::Binary(data) = msg {
                let input = decode_input_message(&data);
                input_tx.send(input).await.ok();
            }
        }
    });

    // Wait for either task to complete
    tokio::select! {
        _ = tx_task => {},
        _ = rx_task => {},
    }
}
```

**Frame encoding:**
```rust
fn encode_frame_to_binary(msg: &StreamerIpcMessage) -> Vec<u8> {
    match msg {
        StreamerIpcMessage::VideoFrame { codec, timestamp_us, keyframe, width, height, data } => {
            let raw = base64::decode(data).unwrap();
            let mut buf = Vec::with_capacity(15 + raw.len());

            buf.push(WsMessageType::VIDEO_FRAME);  // 0x01
            buf.push(*codec);
            buf.push(if *keyframe { 0x01 } else { 0x00 });
            buf.extend_from_slice(&timestamp_us.to_be_bytes());  // 8 bytes
            buf.extend_from_slice(&(*width as u16).to_be_bytes());  // 2 bytes
            buf.extend_from_slice(&(*height as u16).to_be_bytes());  // 2 bytes
            buf.extend_from_slice(&raw);

            buf
        }
        // ... similar for audio
    }
}
```

---

### 7. Frontend: WebSocketStream Class (30 min) ⭐ CRITICAL

**File:** `frontend/src/lib/moonlight-web-ts/stream/websocket-stream.ts`

This is the main browser-side implementation. Let me walk through each section:

#### Constructor

```typescript
export class WebSocketStream {
  private api: Api
  private hostId: number
  private appId: number
  private settings: StreamSettings
  private sessionId?: string

  private ws: WebSocket | null = null
  private eventTarget = new EventTarget()

  // Canvas for rendering
  private canvas: HTMLCanvasElement | null = null
  private canvasCtx: CanvasRenderingContext2D | null = null

  // WebCodecs decoders
  private videoDecoder: VideoDecoder | null = null
  private audioDecoder: AudioDecoder | null = null
  private audioContext: AudioContext | null = null

  // Input handling
  private input: StreamInput

  // Stream state
  private streamerSize: [number, number]
  private connected = false
  private reconnectAttempts = 0
  private maxReconnectAttempts = 5
  private reconnectDelay = 1000

  // Frame timing
  private lastFrameTime = 0
  private frameCount = 0

  constructor(
    api: Api,
    hostId: number,
    appId: number,
    settings: StreamSettings,
    supportedVideoFormats: VideoCodecSupport,
    viewerScreenSize: [number, number],
    sessionId?: string
  ) {
    this.api = api
    this.hostId = hostId
    this.appId = appId
    this.settings = settings
    this.sessionId = sessionId
    this.streamerSize = this.calculateStreamerSize(viewerScreenSize)

    // Initialize input handler (reuses existing StreamInput class)
    const streamInputConfig = defaultStreamInputConfig()
    Object.assign(streamInputConfig, {
      mouseScrollMode: this.settings.mouseScrollMode,
      controllerConfig: this.settings.controllerConfig,
    })
    this.input = new StreamInput(streamInputConfig)

    // ⭐ CRITICAL: Patch StreamInput's send methods to use WebSocket
    // This is done ONCE in constructor, not on every getInput() call
    this.patchInputMethods()

    // Connect WebSocket
    this.connect()
  }
```

#### Input Method Patching

```typescript
  private patchInputMethods() {
    const wsStream = this

    // StreamInput normally sends via RTCDataChannel
    // We override to send via WebSocket instead

    // @ts-ignore - accessing private methods for patching
    this.input.sendKey = (isDown: boolean, key: number, modifiers: number) => {
      wsStream.sendKey(isDown, key, modifiers)
    }
    // @ts-ignore
    this.input.sendMouseMove = (movementX: number, movementY: number) => {
      wsStream.sendMouseMove(movementX, movementY)
    }
    // @ts-ignore
    this.input.sendMousePosition = (x: number, y: number, refW: number, refH: number) => {
      wsStream.sendMousePosition(x, y, refW, refH)
    }
    // @ts-ignore
    this.input.sendMouseButton = (isDown: boolean, button: number) => {
      wsStream.sendMouseButton(isDown, button)
    }
    // @ts-ignore
    this.input.sendMouseWheelHighRes = (deltaX: number, deltaY: number) => {
      wsStream.sendMouseWheelHighRes(deltaX, deltaY)
    }
    // @ts-ignore
    this.input.sendMouseWheel = (deltaX: number, deltaY: number) => {
      wsStream.sendMouseWheel(deltaX, deltaY)
    }
  }
```

**Why patch instead of subclass?**

The `StreamInput` class is complex - it handles keyboard mapping, modifier tracking, pointer lock, gamepad polling, etc. Rather than duplicate all that logic, I patch just the send methods to use WebSocket transport. The rest of the input processing stays the same.

#### WebSocket Connection

```typescript
  private connect() {
    this.dispatchInfoEvent({ type: "connecting" })
    this.resetStreamState()

    // ⭐ CRITICAL: WebSocket URL must be ABSOLUTE
    // new WebSocket('/relative/path') throws error!
    const queryParams = this.sessionId
      ? `?session_id=${encodeURIComponent(this.sessionId)}`
      : ""

    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    const wsUrl = `${protocol}//${window.location.host}${this.api.host_url}/api/ws/stream${queryParams}`

    console.log("[WebSocketStream] Connecting to:", wsUrl)
    this.ws = new WebSocket(wsUrl)
    this.ws.binaryType = "arraybuffer"  // ⭐ Important for binary frames

    this.ws.addEventListener("open", this.onOpen.bind(this))
    this.ws.addEventListener("close", this.onClose.bind(this))
    this.ws.addEventListener("error", this.onError.bind(this))
    this.ws.addEventListener("message", this.onMessage.bind(this))
  }
```

#### Video Decoder Initialization

```typescript
  private async initVideoDecoder(codec: WsVideoCodecType, width: number, height: number) {
    if (!("VideoDecoder" in window)) {
      console.error("[WebSocketStream] WebCodecs VideoDecoder not supported")
      this.dispatchInfoEvent({ type: "error", message: "WebCodecs not supported" })
      return
    }

    const codecString = codecToWebCodecsString(codec)
    console.log(`[WebSocketStream] Initializing video decoder: ${codecString} ${width}x${height}`)

    // Check if codec is supported
    const support = await VideoDecoder.isConfigSupported({
      codec: codecString,
      codedWidth: width,
      codedHeight: height,
      hardwareAcceleration: "prefer-hardware",
    })

    if (!support.supported) {
      this.dispatchInfoEvent({ type: "error", message: `Codec ${codecString} not supported` })
      return
    }

    this.videoDecoder = new VideoDecoder({
      output: (frame: VideoFrame) => {
        this.renderVideoFrame(frame)
      },
      error: (e: Error) => {
        console.error("[WebSocketStream] Video decoder error:", e)
      },
    })

    // ⭐ CRITICAL: Annex B format for in-band SPS/PPS
    const config: VideoDecoderConfig = {
      codec: codecString,
      codedWidth: width,
      codedHeight: height,
      hardwareAcceleration: "prefer-hardware",
    }

    // Tell WebCodecs to expect Annex B NAL units (00 00 00 01 start codes)
    // This is how Wolf sends H264 - with SPS/PPS inline in keyframes
    if (codecString.startsWith("avc1")) {
      // @ts-ignore - avc property exists but TypeScript doesn't know
      config.avc = { format: "annexb" }
    }
    if (codecString.startsWith("hvc1") || codecString.startsWith("hev1")) {
      // @ts-ignore
      config.hevc = { format: "annexb" }
    }

    this.videoDecoder.configure(config)
  }
```

#### Handling Video Frames

```typescript
  // Track if we've received first keyframe (decoder won't work without SPS/PPS)
  private receivedFirstKeyframe = false

  private async handleVideoFrame(data: Uint8Array) {
    if (!this.videoDecoder || this.videoDecoder.state !== "configured") {
      return
    }

    // Parse header
    // Format: type(1) + codec(1) + flags(1) + pts(8) + width(2) + height(2) + data(...)
    if (data.length < 15) return

    const view = new DataView(data.buffer, data.byteOffset, data.byteLength)
    const flags = data[2]
    const isKeyframe = (flags & 0x01) !== 0
    const ptsUs = view.getBigUint64(3, false)  // big-endian

    const frameData = data.slice(15)

    // ⭐ Skip delta frames until we get first keyframe
    // Keyframe contains SPS/PPS needed for decoder initialization
    if (!this.receivedFirstKeyframe) {
      if (!isKeyframe) {
        console.log("[WebSocketStream] Waiting for first keyframe...")
        return
      }
      console.log(`[WebSocketStream] First keyframe received (${frameData.length} bytes)`)
      this.receivedFirstKeyframe = true
    }

    try {
      const chunk = new EncodedVideoChunk({
        type: isKeyframe ? "key" : "delta",
        timestamp: Number(ptsUs),  // microseconds
        data: frameData,
      })

      this.videoDecoder.decode(chunk)
    } catch (e) {
      console.error("[WebSocketStream] Decode error:", e)
    }
  }
```

#### Audio with PTS-Based Scheduling

```typescript
  // Audio timing state
  private audioStartTime = 0      // AudioContext.currentTime when first audio played
  private audioPtsBase = 0        // PTS of first audio frame (microseconds)
  private audioInitialized = false

  private playAudioData(data: AudioData) {
    if (!this.audioContext) {
      data.close()
      return
    }

    // Create buffer from AudioData
    const buffer = this.audioContext.createBuffer(
      data.numberOfChannels,
      data.numberOfFrames,
      data.sampleRate
    )

    // Copy decoded samples to buffer
    for (let i = 0; i < data.numberOfChannels; i++) {
      const channelData = new Float32Array(data.numberOfFrames)
      data.copyTo(channelData, { planeIndex: i, format: "f32-planar" })
      buffer.copyToChannel(channelData, i)
    }

    // ⭐ PTS-based scheduling
    const ptsUs = data.timestamp

    if (!this.audioInitialized) {
      // First frame establishes timing baseline
      this.audioStartTime = this.audioContext.currentTime
      this.audioPtsBase = ptsUs
      this.audioInitialized = true
    }

    // Calculate when this frame should play
    const ptsDelta = (ptsUs - this.audioPtsBase) / 1_000_000  // Convert to seconds
    const scheduledTime = this.audioStartTime + ptsDelta

    // If too far behind, skip
    const now = this.audioContext.currentTime
    if (scheduledTime < now - 0.1) {
      data.close()
      return
    }

    // Schedule playback
    const source = this.audioContext.createBufferSource()
    source.buffer = buffer
    source.connect(this.audioContext.destination)
    source.start(Math.max(scheduledTime, now))

    data.close()
  }
```

#### Input Message Construction

```typescript
  private inputBuffer = new Uint8Array(64)
  private inputView = new DataView(this.inputBuffer.buffer)

  sendKey(isDown: boolean, key: number, modifiers: number) {
    // Format: subType(1) + isDown(1) + modifiers(1) + keyCode(2)
    this.inputBuffer[0] = 0  // sub-type 0 = key input
    this.inputBuffer[1] = isDown ? 1 : 0
    this.inputBuffer[2] = modifiers
    this.inputView.setUint16(3, key, false)  // big-endian
    this.sendInputMessage(WsMessageType.KeyboardInput, this.inputBuffer.subarray(0, 5))
  }

  sendMouseButton(isDown: boolean, button: number) {
    // Format: subType(1) + isDown(1) + button(1)
    this.inputBuffer[0] = 2  // sub-type 2 = button
    this.inputBuffer[1] = isDown ? 1 : 0
    this.inputBuffer[2] = button
    this.sendInputMessage(WsMessageType.MouseClick, this.inputBuffer.subarray(0, 3))
  }

  sendMouseWheelHighRes(deltaX: number, deltaY: number) {
    // Format: subType(1) + deltaX(2) + deltaY(2)
    this.inputBuffer[0] = 3  // sub-type 3 = high-res wheel
    this.inputView.setInt16(1, Math.round(deltaX), false)
    this.inputView.setInt16(3, Math.round(deltaY), false)
    this.sendInputMessage(WsMessageType.MouseClick, this.inputBuffer.subarray(0, 5))
  }

  private sendInputMessage(type: number, payload: Uint8Array) {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
      return
    }

    const message = new Uint8Array(1 + payload.length)
    message[0] = type
    message.set(payload, 1)
    this.ws.send(message.buffer)
  }
```

---

### 8. Frontend: React Integration (15 min)

**File:** `frontend/src/components/external-agent/MoonlightStreamViewer.tsx`

#### State and Refs

```typescript
const videoRef = useRef<HTMLVideoElement>(null)
const canvasRef = useRef<HTMLCanvasElement>(null)  // ⭐ NEW: Canvas for WS mode
const streamRef = useRef<Stream | WebSocketStream | null>(null)

const [streamingMode, setStreamingMode] = useState<StreamingMode>('websocket')
const previousStreamingModeRef = useRef<StreamingMode>(streamingMode)
```

#### Conditional Stream Creation

```typescript
let stream: Stream | WebSocketStream

if (streamingMode === 'websocket') {
  // WebSocket-only mode: use WebSocketStream
  console.log('[MoonlightStreamViewer] Using WebSocket-only streaming mode')

  stream = new WebSocketStream(
    api,
    hostId,
    actualAppId,
    settings,
    supportedFormats,
    [width, height],
    sessionId
  )

  // Set canvas for WebSocket stream rendering
  if (canvasRef.current) {
    stream.setCanvas(canvasRef.current)
  }
} else {
  // WebRTC mode: use original Stream class
  stream = new Stream(
    api,
    hostId,
    actualAppId,
    settings,
    supportedFormats,
    [width, height],
    "create",
    sessionId,
    undefined,
    uniqueClientId
  )
}

streamRef.current = stream

// Attach media stream (WebRTC only)
if (streamingMode === 'webrtc' && videoRef.current && stream instanceof Stream) {
  videoRef.current.srcObject = stream.getMediaStream()
  videoRef.current.play()
}
```

#### Mode Toggle Auto-Reconnect

```typescript
// Reconnect when streaming mode changes
useEffect(() => {
  if (previousStreamingModeRef.current !== streamingMode) {
    console.log('[MoonlightStreamViewer] Mode changed:',
      previousStreamingModeRef.current, '→', streamingMode)
    previousStreamingModeRef.current = streamingMode
    reconnect()
  }
}, [streamingMode, reconnect])
```

#### Toolbar Toggle Button

```typescript
<IconButton
  size="small"
  onClick={() => {
    setStreamingMode(prev => prev === 'websocket' ? 'webrtc' : 'websocket')
  }}
  sx={{ color: streamingMode === 'websocket' ? 'primary.main' : 'white' }}
  title={`Transport: ${streamingMode === 'websocket' ? 'WebSocket (L7)' : 'WebRTC'}`}
>
  {streamingMode === 'websocket'
    ? <Wifi fontSize="small" />
    : <SignalCellularAlt fontSize="small" />}
</IconButton>
```

#### Canvas Element

```typescript
{/* Canvas for WebSocket mode */}
<canvas
  ref={canvasRef}
  onMouseDown={handleMouseDown}
  onMouseUp={handleMouseUp}
  onMouseMove={handleMouseMove}
  onContextMenu={handleContextMenu}
  style={{
    width: '100%',
    height: '100%',
    backgroundColor: '#000',
    cursor: 'none',
    display: streamingMode === 'websocket' ? 'block' : 'none',
  }}
/>
```

---

## Data Flow Diagrams

### Video Frame Flow (Detailed)

```
Wolf NVIDIA Encoder
    │
    │ Encoded H264 bitstream
    │ [00 00 00 01][SPS][00 00 00 01][PPS][00 00 00 01][IDR Slice]...
    ▼
┌─────────────────────────────────────────────────────────────────┐
│ Moonlight Library (C++/Rust bindings)                           │
│                                                                  │
│ VideoDecodeUnit {                                               │
│   buffers: [SPS buffer, PPS buffer, slice buffer, ...],        │
│   presentation_time: Duration,                                  │
│   is_idr: bool,                                                 │
│   width: u32, height: u32                                       │
│ }                                                               │
└─────────────────────────────────────────────────────────────────┘
    │
    │ VideoDecoder::submit_decode_unit()
    ▼
┌─────────────────────────────────────────────────────────────────┐
│ WebSocketVideoDecoder (Rust)                                    │
│                                                                  │
│ 1. Concatenate buffers: frame_data = buf1 + buf2 + ...         │
│ 2. Base64 encode: encoded = base64::encode(frame_data)          │
│ 3. Send IPC: StreamerIpcMessage::VideoFrame { data: encoded }   │
└─────────────────────────────────────────────────────────────────┘
    │
    │ JSON over stdout
    │ {"VideoFrame":{"codec":1,"timestamp_us":12345,"keyframe":true,...}}
    ▼
┌─────────────────────────────────────────────────────────────────┐
│ web-server (Rust)                                               │
│                                                                  │
│ 1. Parse JSON from IPC                                          │
│ 2. Decode base64: raw = base64::decode(data)                   │
│ 3. Build binary frame:                                          │
│    [0x01][codec][flags][pts:8][w:2][h:2][raw NAL units...]     │
│ 4. Send via WebSocket: ws.send(Message::Binary(frame))         │
└─────────────────────────────────────────────────────────────────┘
    │
    │ WebSocket binary frame (HTTPS)
    ▼
┌─────────────────────────────────────────────────────────────────┐
│ WebSocketStream (TypeScript)                                    │
│                                                                  │
│ 1. Receive binary: ws.onmessage(ArrayBuffer)                   │
│ 2. Parse header: type=0x01, codec, flags, pts, w, h            │
│ 3. Extract payload: frameData = data.slice(15)                 │
│ 4. Skip if waiting for keyframe                                 │
│ 5. Create chunk: new EncodedVideoChunk({                       │
│      type: isKeyframe ? 'key' : 'delta',                       │
│      timestamp: ptsUs,                                          │
│      data: frameData                                            │
│    })                                                           │
│ 6. Decode: videoDecoder.decode(chunk)                          │
└─────────────────────────────────────────────────────────────────┘
    │
    │ WebCodecs hardware decode (GPU)
    ▼
┌─────────────────────────────────────────────────────────────────┐
│ VideoDecoder output callback                                    │
│                                                                  │
│ VideoFrame {                                                    │
│   displayWidth: 1920, displayHeight: 1080,                     │
│   timestamp: 12345 (microseconds),                             │
│   format: 'I420' or 'NV12',                                    │
│   ... pixel data on GPU                                         │
│ }                                                               │
└─────────────────────────────────────────────────────────────────┘
    │
    │ renderVideoFrame()
    ▼
┌─────────────────────────────────────────────────────────────────┐
│ Canvas 2D Context                                               │
│                                                                  │
│ 1. Resize if needed: canvas.width = frame.displayWidth         │
│ 2. Draw: ctx.drawImage(frame, 0, 0)                            │
│ 3. Release GPU resource: frame.close()                         │
│ 4. Update FPS counter                                           │
└─────────────────────────────────────────────────────────────────┘
    │
    ▼
User sees video on screen
```

### Input Flow (Mouse Click Example)

```
User clicks mouse button
    │
    ▼
┌─────────────────────────────────────────────────────────────────┐
│ Browser MouseEvent                                              │
│                                                                  │
│ event.button = 0 (left), 1 (middle), 2 (right)                 │
│ event.type = 'mousedown' or 'mouseup'                          │
└─────────────────────────────────────────────────────────────────┘
    │
    │ React onMouseDown handler
    ▼
┌─────────────────────────────────────────────────────────────────┐
│ MoonlightStreamViewer.handleMouseDown()                         │
│                                                                  │
│ streamRef.current.getInput().onMouseDown(event, getStreamRect())│
└─────────────────────────────────────────────────────────────────┘
    │
    ▼
┌─────────────────────────────────────────────────────────────────┐
│ StreamInput.onMouseDown()                                       │
│                                                                  │
│ 1. Map button: 0 → 1 (Left), 1 → 2 (Middle), 2 → 3 (Right)    │
│ 2. Call patched sendMouseButton(isDown=true, button=1)          │
└─────────────────────────────────────────────────────────────────┘
    │
    ▼
┌─────────────────────────────────────────────────────────────────┐
│ WebSocketStream.sendMouseButton()                               │
│                                                                  │
│ inputBuffer[0] = 2      // sub-type = button                   │
│ inputBuffer[1] = 1      // isDown = true                       │
│ inputBuffer[2] = 1      // button = Left                       │
│                                                                  │
│ sendInputMessage(0x11, inputBuffer[0..3])                       │
│                                                                  │
│ Final message: [0x11][0x02][0x01][0x01]                        │
│                  │     │     │     └─ button (Left)            │
│                  │     │     └─ isDown (true)                   │
│                  │     └─ sub-type (2 = button)                 │
│                  └─ message type (MouseClick)                   │
└─────────────────────────────────────────────────────────────────┘
    │
    │ ws.send(ArrayBuffer)
    ▼
┌─────────────────────────────────────────────────────────────────┐
│ web-server (Rust)                                               │
│                                                                  │
│ 1. Receive binary message                                       │
│ 2. Parse: type = 0x11 (MouseClick)                             │
│ 3. Forward via IPC: ServerIpcMessage::Input(                   │
│      InputIpcMessage::MouseClick(vec![0x02, 0x01, 0x01])       │
│    )                                                            │
└─────────────────────────────────────────────────────────────────┘
    │
    │ JSON over stdin
    ▼
┌─────────────────────────────────────────────────────────────────┐
│ streamer process (Rust)                                         │
│                                                                  │
│ match input {                                                   │
│   InputIpcMessage::MouseClick(data) => {                       │
│     let sub_type = data[0];  // 2                              │
│     match sub_type {                                            │
│       2 => {                                                    │
│         let is_down = data[1] != 0;  // true                   │
│         let button = data[2];  // 1 = Left                     │
│         stream.send_mouse_button(                               │
│           MouseButtonAction::Press,                             │
│           MouseButton::Left                                     │
│         );                                                      │
│       }                                                         │
│     }                                                           │
│   }                                                             │
│ }                                                               │
└─────────────────────────────────────────────────────────────────┘
    │
    │ Moonlight protocol (encrypted)
    ▼
┌─────────────────────────────────────────────────────────────────┐
│ Wolf                                                            │
│                                                                  │
│ Receives mouse button event                                     │
│ Injects into Linux input subsystem                              │
│ Application receives click                                      │
└─────────────────────────────────────────────────────────────────┘
```

---

## Potential Holes to Investigate

### 1. **Base64 Overhead in IPC**

At 60fps 1080p with 20 Mbps bitrate:
- ~2.5 MB/sec of video data
- Base64 adds 33% → ~3.3 MB/sec
- JSON framing adds more overhead

**Is this acceptable?** Probably, but worth measuring. Alternative: binary IPC protocol or shared memory.

**Concrete concern:** At 4K60 40Mbps, this becomes ~6.6 MB/sec through JSON parsing. CPU overhead could be noticeable.

### 2. **No Backpressure**

If the browser is slow to decode, frames will queue up:
- Server keeps sending at source rate
- VideoDecoder queue grows
- Memory usage increases
- Eventually decode errors or dropped frames

**The code does NOT check `videoDecoder.decodeQueueSize`.**

Potential fix:
```typescript
if (this.videoDecoder.decodeQueueSize > 5) {
  // Signal backpressure to server
  // Or drop this frame
}
```

### 3. **Audio/Video Sync**

Video and audio are independent streams with their own PTS. Currently:
- Video: Rendered immediately on decode (no scheduling)
- Audio: Scheduled based on PTS relative to first frame

**What could go wrong:**
- Clock drift between audio and video timelines
- Network jitter affecting one stream more than other
- No explicit A/V sync mechanism
- Video might get ahead if decode is fast

**The video renderer does NOT use PTS for timing.** It just draws immediately.

### 4. **Browser Compatibility**

WebCodecs support:
- Chrome 94+ ✓
- Edge 94+ ✓
- Safari 16.4+ ✓
- Firefox 130+ ✓ (released August 2024!)

**Firefox users on older versions will get nothing.** No fallback implemented.

### 5. **Reconnection State**

When WebSocket disconnects and reconnects:
- `receivedFirstKeyframe` is reset ✓
- `audioInitialized` is reset ✓
- But VideoDecoder is NOT closed/recreated

```typescript
private resetStreamState() {
  this.receivedFirstKeyframe = false
  this.audioInitialized = false
  this.audioStartTime = 0
  this.audioPtsBase = 0
  // ⚠️ VideoDecoder not touched!
}
```

If the decoder is in an error state, it won't recover on reconnect.

### 6. **Codec Negotiation**

Hardcoded to H264:
```typescript
video_supported_formats: createSupportedVideoFormatsBits({
    H264: true,
    // ... all others false
})
```

What if Wolf is configured for HEVC or AV1? The server will send the wrong codec and decoding will fail.

### 7. **Input Sub-type Alignment**

I need to verify these match:

| TypeScript | Rust |
|------------|------|
| sub-type 0 (key) | sub-type 0 for key |
| sub-type 1 (text) | sub-type 1 for text |
| sub-type 2 (button) via MouseClick | sub-type 2 |
| sub-type 3 (high-res wheel) via MouseClick | sub-type 3 |
| sub-type 4 (normal wheel) via MouseClick | sub-type 4 |

Looking at the code... they DO match. But it's fragile - no shared constant file.

### 8. **Session/Authentication**

The WebSocket URL includes `session_id`:
```typescript
const wsUrl = `${protocol}//${host}${api.host_url}/api/ws/stream?session_id=${sessionId}`
```

Authentication flow:
1. Browser has Helix JWT in cookie (HttpOnly)
2. WebSocket HTTP upgrade request includes cookie
3. Proxy validates cookie, injects moonlight auth
4. WebSocket connection is now authenticated

**Question:** Does the proxy correctly forward cookies on WebSocket upgrade?

### 9. **Proxy WebSocket Upgrade**

The `/moonlight` path is proxied. Common nginx misconfiguration:

```nginx
# ❌ WRONG - missing WebSocket upgrade
location /moonlight {
    proxy_pass http://moonlight-web;
}

# ✓ CORRECT
location /moonlight {
    proxy_pass http://moonlight-web;
    proxy_http_version 1.1;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection "upgrade";
}
```

**If this isn't configured, WebSocket will fail silently.**

### 10. **Canvas Sizing vs Mouse Coordinates**

The canvas resizes to match frame dimensions:
```typescript
if (this.canvas.width !== frame.displayWidth || ...) {
    this.canvas.width = frame.displayWidth
    this.canvas.height = frame.displayHeight
}
```

CSS scales it:
```css
width: 100%
height: 100%
```

So a 1920x1080 canvas is displayed at whatever size the container is.

**Mouse coordinate mapping in MoonlightStreamViewer:**
```typescript
const getStreamRect = useCallback((): DOMRect => {
  const element = streamingMode === 'websocket' ? canvasRef.current : videoRef.current
  // ... calculates aspect-ratio-aware bounds
}, [streamingMode])
```

This SHOULD work, but needs testing. Canvas and video elements have slightly different behavior with `getBoundingClientRect()`.

---

## Testing Checklist

When you get to testing:

### 1. Basic connectivity
- [ ] WebSocket connects successfully (check Network tab)
- [ ] No CORS errors in console
- [ ] No authentication errors
- [ ] Proxy handles WebSocket upgrade (check response headers)

### 2. Video
- [ ] First keyframe received and decoded
- [ ] Subsequent delta frames decode
- [ ] Canvas displays video
- [ ] No decode errors in console
- [ ] Resolution matches expected (check canvas dimensions)
- [ ] FPS log shows ~60fps

### 3. Audio
- [ ] Audio plays (might need click to unlock AudioContext)
- [ ] No crackling or gaps
- [ ] Roughly synced with video
- [ ] Volume works

### 4. Input
- [ ] Mouse position (absolute) works - cursor tracks
- [ ] Mouse buttons work - can click UI elements
- [ ] Mouse wheel scrolls - can scroll in browser windows
- [ ] Keyboard keys work - can type text
- [ ] Modifier keys work - Shift, Ctrl, Alt
- [ ] No stuck modifiers after Alt-Tab

### 5. Reconnection
- [ ] Disconnect/reconnect recovers
- [ ] Mode toggle reconnects cleanly
- [ ] No memory leaks after multiple reconnects (check heap)

### 6. Performance
- [ ] CPU usage acceptable (check Activity Monitor)
- [ ] Memory stable over time
- [ ] Frame rate matches source
- [ ] Latency feels responsive (<100ms)

---

## Summary

This is a significant architectural change that replaces WebRTC's media transport with a custom WebSocket-based protocol.

**What I built:**
- Binary protocol for video/audio frames and input
- Rust server-side frame extraction (bypassing RTP/WebRTC)
- Rust input forwarding with correct Moonlight API
- TypeScript WebCodecs-based decoder
- React UI with mode toggle

**What's solid:**
- Binary protocol is clean and minimal
- WebCodecs provides hardware acceleration
- Input forwarding mirrors existing format

**What needs attention:**
- IPC efficiency (base64 overhead)
- Backpressure / flow control
- Browser fallbacks for WebCodecs
- A/V sync under adverse conditions
- Proxy WebSocket configuration

Happy reading! 🛫
