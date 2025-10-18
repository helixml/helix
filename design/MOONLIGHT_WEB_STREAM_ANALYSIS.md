# Moonlight Web Stream - Architecture Analysis & Helix Integration Strategy

## Executive Summary

**moonlight-web-stream** (checked out at `~/pm/moonlight-web-stream`) is a **battle-tested, production-ready solution** for browser-based Moonlight streaming. Unlike my initial implementation which attempted to build a WebCodecs-based client from scratch, moonlight-web-stream uses a **server-side Moonlight client** that bridges to browsers via **WebRTC**.

**Key Insight**: This is the CORRECT architecture - we should integrate moonlight-web-stream as a sidecar service rather than building our own protocol bridge.

## Architecture Comparison

### My Initial Approach (❌ Incomplete)

```
Browser (WebCodecs) <--WebSocket--> Helix API (RTSP/RTP Bridge) <--RTSP--> Wolf
```

**Problems**:
- Reinventing the wheel - building RTSP/RTP protocol handling from scratch
- WebCodecs not supported in Safari
- Audio streaming (Opus) not implemented
- Input protocol translation incomplete
- RTSP handshake/SDP parsing missing

### moonlight-web-stream Approach (✅ Production-Ready)

```
Browser (WebRTC) <--WebSocket--> Rust Streamer (Moonlight Client) <--Moonlight Protocol--> Wolf/Sunshine
```

**Advantages**:
- ✅ **Battle-tested**: Uses moonlight-common-c library (same as native clients)
- ✅ **Complete**: Video (H.264/H.265/AV1), Audio (Opus), Input (mouse/keyboard/gamepad)
- ✅ **WebRTC**: Universal browser support via WebRTC (includes Safari fallback)
- ✅ **Production-ready**: NAT traversal (STUN/TURN), secure contexts, gamepad API
- ✅ **Maintained**: Active open-source project

## moonlight-web-stream Components

### 1. Rust Streamer Process (`moonlight-web/streamer/`)

**Purpose**: Acts as Moonlight client, bridges to WebRTC

**Key Files**:
- `src/main.rs` - Process lifecycle, WebRTC peer connection setup
- `src/video.rs` - Video codec handling (H.264/H.265/AV1)
- `src/audio.rs` - Opus audio decode/encode
- `src/input.rs` - Input event translation to Moonlight protocol
- `src/connection.rs` - WebRTC connection management

**Functionality**:
```rust
// Connects to Moonlight host (Wolf in our case)
let moonlight_instance = MoonlightInstance::new(...);
let stream = moonlight_instance.stream_app(app_id).await?;

// Sets up WebRTC peer connection
let peer_connection = RTCPeerConnection::new(rtc_config).await?;

// Bridges Moonlight video frames to WebRTC video track
let video_track = Arc::new(TrackLocalStaticSample::new(...));
peer_connection.add_track(video_track).await?;

// Forwards browser input events to Moonlight
stream.send_input_event(input_event).await?;
```

### 2. Web Server (`moonlight-web/web-server/`)

**Purpose**: HTTP/WebSocket server for browser clients

**Key Files**:
- `src/api/stream.rs` - WebSocket endpoint `/host/stream`
- `src/web.rs` - Static file serving, authentication
- `src/data.rs` - Runtime state (hosts, apps, streamers)

**WebSocket Protocol**:
```typescript
// Client → Server: Authenticate and start stream
{
  "AuthenticateAndInit": {
    "credentials": "password",
    "host_id": 0,
    "app_id": 1,
    "bitrate": 20000,
    "fps": 60,
    "width": 1920,
    "height": 1080,
    // ... video/audio settings
  }
}

// Server spawns Rust streamer process
// Server ↔ Client: WebRTC signaling
{
  "Signaling": {
    "type": "offer" | "answer",
    "sdp": "..."
  }
}

// Video/audio streams via WebRTC data channels
```

### 3. Browser Frontend (`moonlight-web/web-server/web/`)

**Key Files**:
- `stream.ts` - Main streaming UI component
- `stream/index.ts` - WebRTC peer connection, media stream handling
- `stream/input.ts` - Keyboard/mouse/touch/gamepad input capture
- `stream/video.ts` - Video codec capabilities detection

**WebRTC Integration**:
```typescript
// Creates RTCPeerConnection
const peerConnection = new RTCPeerConnection({
  iceServers: [
    { urls: "stun:stun.l.google.com:19302" },
    // TURN servers for NAT traversal
  ]
});

// Receives video/audio tracks from streamer
peerConnection.ontrack = (event) => {
  videoElement.srcObject = event.streams[0];
};

// Sends input events via data channel
const inputChannel = peerConnection.createDataChannel("input");
inputChannel.send(JSON.stringify(inputEvent));
```

## Integration Strategy for Helix

### Option 1: Sidecar Service (✅ RECOMMENDED)

Deploy moonlight-web-stream as a **sidecar container** alongside Wolf:

```yaml
# docker-compose.dev.yaml
services:
  wolf:
    image: wolf:latest
    # ... existing config

  moonlight-web:
    build:
      context: ../moonlight-web-stream
      dockerfile: Dockerfile
    ports:
      - "8081:8080"  # Web interface
    environment:
      - MOONLIGHT_HOST=wolf
      - MOONLIGHT_HTTP_PORT=47989
    volumes:
      - ./moonlight-web-config:/server
    networks:
      - helix_default
```

**Benefits**:
- ✅ **Zero code changes** to moonlight-web-stream (battle-tested as-is)
- ✅ **Minimal Helix changes** (just add iframe or reverse proxy)
- ✅ **Independent scaling** (can run multiple moonlight-web instances)
- ✅ **Easy updates** (pull upstream moonlight-web-stream changes)

**Implementation Steps**:

1. **Add moonlight-web service** to docker-compose
2. **Configure moonlight-web** to connect to Wolf (localhost:47989)
3. **Add reverse proxy** in Helix API to forward `/moonlight/*` to moonlight-web
4. **Update ScreenshotViewer** to render iframe pointing to moonlight-web stream

```typescript
// In ScreenshotViewer.tsx - streaming mode
<iframe
  src={`/moonlight/stream.html?hostId=0&appId=${wolfLobbyId}`}
  width="100%"
  height="100%"
  allow="autoplay; fullscreen; gamepad"
/>
```

### Option 2: Embedded Library (⚠️ Complex)

Compile moonlight-web-stream as WebAssembly and embed in Helix frontend.

**Benefits**:
- Single binary deployment
- No additional services

**Drawbacks**:
- ❌ Requires significant Rust/WASM expertise
- ❌ Complex build pipeline
- ❌ Harder to update/maintain
- ❌ May lose performance benefits of native Rust
- ❌ WebRTC still requires signaling server (can't eliminate backend)

**Not Recommended**: Complexity outweighs benefits.

### Option 3: Fork & Customize (⚠️ Maintenance Burden)

Fork moonlight-web-stream and integrate deeply into Helix codebase.

**Drawbacks**:
- ❌ Lose ability to pull upstream fixes/features
- ❌ Ongoing maintenance burden
- ❌ Requires Rust expertise in team

**Not Recommended**: Only consider if deep customization needed.

## Recommended Implementation Plan

### Phase 1: Proof of Concept (1 Day)

1. **Build moonlight-web-stream**:
   ```bash
   cd ~/pm/moonlight-web-stream
   cargo build --release
   ```

2. **Configure for Wolf**:
   ```json
   {
     "bind_address": "0.0.0.0:8081",
     "credentials": "helix",
     "webrtc_ice_servers": [
       { "urls": ["stun:stun.l.google.com:19302"] }
     ]
   }
   ```

3. **Test manually**:
   - Start Wolf lobby
   - Start moonlight-web
   - Add Wolf as host (localhost:47989)
   - Pair with Wolf
   - Stream to browser

### Phase 2: Docker Integration (2 Days)

1. **Create Dockerfile** (moonlight-web doesn't ship one):
   ```dockerfile
   FROM rust:latest as builder
   WORKDIR /build
   COPY . .
   RUN cargo build --release

   FROM debian:bookworm-slim
   RUN apt-get update && apt-get install -y \
       libssl3 \
       ca-certificates
   COPY --from=builder /build/target/release/web-server /usr/local/bin/
   COPY --from=builder /build/moonlight-web/dist /usr/local/share/moonlight-web/static
   WORKDIR /server
   CMD ["/usr/local/bin/web-server"]
   ```

2. **Add to docker-compose.dev.yaml**

3. **Configure auto-pairing** with Wolf (pre-generate certificates)

### Phase 3: Helix Integration (3 Days)

1. **Add reverse proxy** in Helix API:
   ```go
   // api/pkg/server/server.go
   authRouter.PathPrefix("/moonlight/").Handler(
     httputil.NewSingleHostReverseProxy(moonlightWebURL),
   )
   ```

2. **Update ScreenshotViewer** with iframe mode:
   ```tsx
   {streamingMode === 'stream' && (
     <iframe
       src={`/moonlight/stream.html?hostId=0&appId=${wolfLobbyId}`}
       // ... fullscreen, gamepad permissions
     />
   )}
   ```

3. **Auto-host configuration** via Helix API:
   - API endpoint to add Wolf host to moonlight-web
   - Auto-pair using Wolf lobby PIN
   - Map Wolf lobby ID → moonlight app ID

### Phase 4: Production Hardening (1 Week)

1. **TURN server** for NAT traversal:
   ```yaml
   coturn:
     image: coturn/coturn:latest
     ports:
       - "3478:3478/udp"
       - "3478:3478/tcp"
   ```

2. **Multi-backend routing**:
   - Helix API selects nearest Wolf backend
   - moonlight-web configured with dynamic host list

3. **Authentication integration**:
   - Replace moonlight-web auth with Helix session tokens
   - Custom authentication middleware

4. **Monitoring & metrics**:
   - WebRTC connection stats
   - Bitrate/FPS monitoring
   - Error tracking

## Comparison: My Implementation vs moonlight-web-stream

| Feature | My Implementation | moonlight-web-stream |
|---------|-------------------|---------------------|
| **Video Streaming** | ⚠️ Partial (WebCodecs) | ✅ Complete (WebRTC) |
| **Audio Streaming** | ❌ Not implemented | ✅ Complete (Opus) |
| **Input Handling** | ⚠️ Events captured, not sent | ✅ Complete (all inputs) |
| **Browser Support** | ❌ Chrome/Edge only | ✅ All modern browsers |
| **RTSP Protocol** | ❌ Incomplete handshake | ✅ Full Moonlight client |
| **Gamepad Support** | ❌ Not implemented | ✅ Complete |
| **NAT Traversal** | ❌ Not implemented | ✅ STUN/TURN support |
| **Production Ready** | ❌ Prototype only | ✅ Battle-tested |
| **Maintenance** | ❌ Build from scratch | ✅ Active open-source |
| **Time to Deploy** | 4-6 weeks | 1 week |

## Reusable Components from My Implementation

While moonlight-web-stream is the better choice for the streaming core, some parts of my implementation remain useful:

### ✅ Keep: Frontend UI Components

- **ScreenshotViewer toggle UI** - Works great for mode switching
- **Toolbar controls** - Fullscreen, refresh, stats display
- **Integration points** - Session.tsx, PersonalDevEnvironments.tsx updates

### ✅ Keep: API Endpoints Structure

- `/api/v1/sessions/{id}/stream` - Repurpose as reverse proxy
- `/api/v1/personal-dev-environments/{id}/stream` - Same

### ❌ Replace: Streaming Implementation

- `websocket_moonlight_stream.go` - Replace with reverse proxy to moonlight-web
- `MoonlightWebPlayer.tsx` - Replace with iframe to moonlight-web

## Files to Modify for Integration

### New Files to Create

```
helix/
├── Dockerfile.moonlight-web         # New: Build moonlight-web image
├── moonlight-web-config/
│   └── config.json                   # New: moonlight-web configuration
└── docker-compose.dev.yaml           # Modify: Add moonlight-web service
```

### Files to Modify

```
helix/api/pkg/server/
├── server.go                         # Modify: Add reverse proxy routes
└── moonlight_proxy.go                # New: Proxy handler

helix/frontend/src/components/external-agent/
├── ScreenshotViewer.tsx              # Modify: Add iframe mode
└── MoonlightWebPlayer.tsx            # Replace: Use iframe instead
```

### Files to Remove

```
helix/api/pkg/server/
└── websocket_moonlight_stream.go     # Remove: Replace with proxy

helix/docs/
└── BROWSER_MOONLIGHT_STREAMING.md    # Update: Document new architecture
```

## Next Steps

1. **Immediate**: Build and test moonlight-web-stream locally
2. **Short-term**: Dockerize and integrate as sidecar
3. **Medium-term**: Auto-configuration and authentication integration
4. **Long-term**: Production hardening (TURN, monitoring, multi-backend)

## Conclusion

**Recommendation**: **Adopt moonlight-web-stream as a sidecar service** instead of my from-scratch implementation.

**Rationale**:
- Saves 4-6 weeks of development time
- Gets production-ready streaming immediately
- Proven in real-world deployments
- Active maintenance and upstream updates
- Complete feature set (audio, all inputs, all browsers)

My initial WebCodecs approach was a good learning exercise, but moonlight-web-stream is the battle-tested solution we should use.

---

**Last Updated**: 2025-10-08
**Author**: Claude Code
**Status**: Analysis Complete, Awaiting Implementation Decision
