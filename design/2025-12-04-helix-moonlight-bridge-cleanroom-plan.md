# Helix Moonlight Bridge: Cleanroom Go Implementation Plan

**Repository:** `~/pm/helix-moonlight-bridge`
**Status:** In Progress
**Last Updated:** 2025-12-04

---

## Progress Tracker

### Overall Status
- [x] Phase 1: Project Setup & NVHTTP Client (skeleton)
- [ ] Phase 2: RTSP Session Client
- [ ] Phase 3: Media Streams (RTP/Decryption)
- [ ] Phase 4: Control Stream (Input)
- [ ] Phase 5: WebSocket Bridge (complete)
- [ ] Phase 6: Integration & Testing

### Detailed Progress

#### Phase 1: Project Setup & NVHTTP ✅ COMPLETE
- [x] Create git repository
- [x] Initialize Go module
- [x] Port WebSocket binary protocol from ws_protocol.rs
- [x] Implement protocol message encode/decode
- [x] NVHTTP client skeleton (client.go)
- [x] Certificate generation (crypto.go)
- [x] Pairing flow skeleton (pairing.go) - needs testing
- [x] App list/launch/cancel stubs
- [x] WebSocket server skeleton
- [x] Session handling skeleton
- [x] Initial commit

#### Phase 2: RTSP Session Client 🔄 IN PROGRESS
- [ ] Research: Capture real RTSP traffic with Wireshark
- [ ] Add gortsplib dependency
- [ ] Implement RTSP client wrapper
- [ ] ANNOUNCE request for launching app
- [ ] DESCRIBE request to get SDP
- [ ] Parse SDP for codec info and encryption keys
- [ ] SETUP request for video track
- [ ] SETUP request for audio track
- [ ] PLAY request to start streaming
- [ ] TEARDOWN for cleanup
- [ ] Test against Wolf

#### Phase 3: Media Streams ⏳ PENDING
- [ ] RTP packet receiver (UDP socket)
- [ ] Sequence number tracking and reordering
- [ ] Encryption key extraction from RTSP
- [ ] AES-GCM decryption implementation
- [ ] H264 NAL unit reassembly from RTP
- [ ] H265 NAL unit reassembly (if needed)
- [ ] Opus audio frame extraction
- [ ] Jitter buffer (optional for v1)
- [ ] Forward frames to WebSocket bridge

#### Phase 4: Control Stream ⏳ PENDING
- [ ] Research: Capture ENet traffic with Wireshark
- [ ] Implement minimal reliable UDP (pure Go)
- [ ] Connection handshake
- [ ] Keepalive messages
- [ ] Keyboard input encoding
- [ ] Mouse movement encoding
- [ ] Mouse button encoding
- [ ] Mouse wheel encoding
- [ ] Controller state encoding (optional for v1)
- [ ] Test input with Wolf

#### Phase 5: WebSocket Bridge ✅ SKELETON COMPLETE
- [x] WebSocket server endpoint
- [x] Session management
- [x] Input message decoding
- [ ] Connect session to Moonlight stream
- [ ] Forward video frames to browser
- [ ] Forward audio frames to browser
- [ ] Forward input to control stream
- [ ] Handle reconnection
- [ ] Handle multiple sessions

#### Phase 6: Integration & Testing ⏳ PENDING
- [ ] End-to-end test: pair with Wolf
- [ ] End-to-end test: launch app
- [ ] End-to-end test: receive video
- [ ] End-to-end test: receive audio
- [ ] End-to-end test: send keyboard input
- [ ] End-to-end test: send mouse input
- [ ] Test with TypeScript WebSocketStream client
- [ ] Performance testing (latency, CPU)
- [ ] Dockerfile for sandbox deployment
- [ ] Integration with helix sway container

---

## Goal

Create a GPL-free Moonlight client in Go that can:
1. Connect to Wolf/Sunshine GameStream servers
2. Receive video/audio streams
3. Forward to browsers via WebSocket (reusing Luke's ws_protocol.rs design)
4. Handle input from browsers and send to the server

## Why Go?

1. **Natural cleanroom barrier** - Different language forces different implementation
2. **Better Helix integration** - API is already Go
3. **Excellent libraries available** - gortsplib, crypto/tls, etc.
4. **Single binary deployment** - No Rust nightly toolchain

## Protocol Overview (From Public Sources)

The Moonlight/GameStream protocol consists of:

### 1. NVHTTP API (Port 47984 HTTP, 47989 HTTPS)
XML-based REST API for:
- Server discovery (`/serverinfo`)
- Pairing (`/pair` with challenge/response)
- App listing (`/applist`)
- Launch app (`/launch`)
- Quit app (`/cancel`)

### 2. RTSP Session Negotiation (Port 48010)
Standard RTSP with extensions for:
- Codec negotiation (H264/H265/AV1)
- Resolution/FPS selection
- Audio format selection (Opus)
- Encryption key exchange

### 3. Control Stream (ENet over UDP)
Reliable UDP for:
- Keyboard input events
- Mouse movement/clicks
- Controller state
- Control messages (keepalive, etc.)

### 4. Video/Audio Streams (RTP over UDP)
- Encrypted RTP packets
- Video: H264/H265/AV1 NAL units
- Audio: Opus frames

## Architecture

```
┌─────────────────────────────────────────────────────────────────────────┐
│                              BROWSER                                     │
│                                                                          │
│  ┌──────────────────────────────────────────────────────────────────┐   │
│  │ WebSocketStream (TypeScript - already implemented by Luke)       │   │
│  │ - WebSocket connection to helix-moonlight-bridge                 │   │
│  │ - WebCodecs VideoDecoder/AudioDecoder                            │   │
│  │ - Input encoding → binary → WebSocket                            │   │
│  └──────────────────────────────────────────────────────────────────┘   │
│                                          │                               │
└──────────────────────────────────────────│───────────────────────────────┘
                                           │ WebSocket (wss://)
                                           │ Binary frames (Luke's protocol)
                                           ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                    HELIX-MOONLIGHT-BRIDGE (Go)                          │
│                                                                          │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │ WebSocket Handler                                                │   │
│  │ - Accepts browser connections                                    │   │
│  │ - Encodes video/audio to Luke's binary protocol                  │   │
│  │ - Decodes input messages from browser                            │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│                           │                        ▲                     │
│                           ▼                        │                     │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │ Session Manager                                                  │   │
│  │ - Manages Moonlight sessions                                     │   │
│  │ - Handles pairing/authentication                                 │   │
│  │ - Coordinates NVHTTP, RTSP, and streams                          │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│                           │                                              │
│       ┌───────────────────┼───────────────────────┐                     │
│       ▼                   ▼                       ▼                     │
│  ┌──────────┐      ┌──────────────┐        ┌──────────────┐            │
│  │ NVHTTP   │      │ RTSP Client  │        │ Media Stream │            │
│  │ Client   │      │ (gortsplib)  │        │ Handler      │            │
│  │          │      │              │        │              │            │
│  │ -Discovery│      │ -Session     │        │ -RTP receive │            │
│  │ -Pairing │      │  negotiation │        │ -Decryption  │            │
│  │ -App mgmt│      │ -Codec setup │        │ -NAL parsing │            │
│  └──────────┘      └──────────────┘        └──────────────┘            │
│       │                   │                       │                     │
└───────┼───────────────────┼───────────────────────┼─────────────────────┘
        │                   │                       │
        │ HTTPS             │ RTSP                  │ RTP (encrypted)
        ▼                   ▼                       ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                         WOLF / SUNSHINE                                  │
│                                                                          │
│  - NVIDIA hardware encoder                                              │
│  - Opus audio encoder                                                   │
│  - GameStream protocol server                                           │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
```

## Go Dependencies (All permissively licensed)

| Library | License | Purpose |
|---------|---------|---------|
| `github.com/bluenviron/gortsplib/v4` | MIT | RTSP client, RTP handling |
| `github.com/gorilla/websocket` | BSD-2 | WebSocket server |
| `golang.org/x/crypto` | BSD-3 | TLS, AES-GCM, X.509 certs |
| Standard library | BSD-3 | HTTP client, crypto, etc. |

**ENet: Pure Go Implementation (Decision Made)**
Wolf uses a FORK of ENet with unknown modifications. Rather than depend on CGO and potentially incompatible ENet libraries, we'll implement a minimal reliable UDP protocol in pure Go. Benefits:
- No CGO dependency = simpler cross-compilation
- Full control to match Wolf's fork behavior
- Control stream is low-bandwidth, so custom implementation is feasible
- Can be adjusted as we discover protocol details

## Deployment: Runs IN THE SANDBOX

**Critical Architecture Decision:** The bridge runs inside the sandbox container alongside Wolf, NOT in the Helix API.

```
┌────────────────────────────────────────────────────────────────┐
│                     SANDBOX CONTAINER                           │
│                                                                  │
│  ┌──────────────────┐     ┌─────────────────────────────────┐  │
│  │      WOLF        │     │  HELIX-MOONLIGHT-BRIDGE (Go)    │  │
│  │                  │◄───►│                                 │  │
│  │ - Video encoder  │     │ - Moonlight client              │  │
│  │ - Audio encoder  │     │ - WebSocket server (:8081)      │  │
│  │ - GameStream srv │     │ - Forwards to browser           │  │
│  └──────────────────┘     └─────────────────────────────────┘  │
│         ▲                              ▲                        │
│         │ localhost                    │ Exposed via revdial    │
│         │                              │                        │
└─────────┼──────────────────────────────┼────────────────────────┘
          │                              │
     Internal only                  ┌────┴────┐
     (Moonlight protocol)           │ Browser │
                                    └─────────┘
```

Why in sandbox:
- Low latency to Wolf (localhost)
- Same network namespace
- Can access Wolf's localhost ports (47984, 47989, 48010)
- Deployed as part of sway container image

## Implementation Phases

### Phase 1: NVHTTP Client (Week 1)
- [ ] XML parsing for server responses
- [ ] Server discovery (`GET /serverinfo`)
- [ ] Certificate generation (RSA 2048-bit)
- [ ] Pairing flow (`GET /pair` with PIN challenge)
- [ ] App listing (`GET /applist`)
- [ ] Launch/cancel app

### Phase 2: RTSP Session (Week 2)
- [ ] Integrate gortsplib as client
- [ ] ANNOUNCE/DESCRIBE handshake
- [ ] Video/audio codec negotiation
- [ ] Encryption key exchange
- [ ] SETUP/PLAY for starting stream

### Phase 3: Media Streams (Week 3)
- [ ] RTP packet reception
- [ ] AES-GCM decryption
- [ ] H264/H265 NAL unit extraction
- [ ] Opus audio frame extraction
- [ ] Buffering/jitter handling

### Phase 4: Control Stream (Week 4)
- [ ] ENet-compatible reliable UDP (or use CGO wrapper)
- [ ] Input event encoding (keyboard, mouse, gamepad)
- [ ] Control messages (keepalive, etc.)

### Phase 5: WebSocket Bridge (Week 5)
- [ ] WebSocket server endpoint
- [ ] Video/audio frame encoding (Luke's binary protocol)
- [ ] Input decoding from browser
- [ ] Session management (create/join/keepalive)

### Phase 6: Integration & Testing (Week 6)
- [ ] Integration with Helix API
- [ ] End-to-end testing with Wolf
- [ ] Performance optimization
- [ ] Documentation

## Reusable Code From Luke's Moonlight-Web-Stream

Since Luke authored these, we can directly reuse:

1. **WebSocket Binary Protocol** (`ws_protocol.rs` → Go port)
   - Message type constants (0x01 video, 0x02 audio, 0x10-0x16 input)
   - Video frame format (header + NAL data)
   - Audio frame format (header + Opus data)
   - Input message formats

2. **IPC Message Types** (`ipc.rs` → already TypeScript)
   - VideoFrame, AudioFrame, StreamInit
   - Input message types

3. **TypeScript Client** (already in helix repo)
   - WebSocketStream class
   - WebCodecs decoders
   - Input handling

## Protocol Details to Implement

### NVHTTP Pairing Flow

```
1. Client → Server: GET /serverinfo
   Response: XML with uniqueid, mac, etc.

2. Client → Server: GET /pair?uniqueid=X&devicename=Y&updateState=1&phrase=getservercert
   Response: XML with plaincert (server's certificate)

3. Client generates random 4-digit PIN (shown to user)

4. Client → Server: GET /pair?uniqueid=X&devicename=Y&updateState=1&clientchallenge=Z
   (clientchallenge = AES-encrypt(random bytes, key=SHA1(PIN + salt)))
   Response: XML with challengeresponse

5. Client → Server: GET /pair?uniqueid=X&devicename=Y&updateState=1&serverchallengeresp=W
   Response: XML with pairingsecret

6. Client → Server: GET /pair?uniqueid=X&devicename=Y&updateState=1&clientpairingsecret=V
   Response: XML with paired=1
```

### RTSP Messages

```
ANNOUNCE rtsp://host:48010/launch?appid=123 RTSP/1.0
...

DESCRIBE rtsp://host:48010 RTSP/1.0
...
(Response includes SDP with codec info, encryption keys)

SETUP rtsp://host:48010/video RTSP/1.0
Transport: RTP/AVP/UDP;unicast;client_port=47998-47999
...

PLAY rtsp://host:48010 RTSP/1.0
...
```

### RTP Packet Format

```
┌────────────────────────────────────────────────────────────────┐
│ RTP Header (12 bytes)                                          │
├────────────────────────────────────────────────────────────────┤
│ Sequence Number (for ordering)                                 │
│ Timestamp                                                       │
│ SSRC (stream identifier)                                       │
├────────────────────────────────────────────────────────────────┤
│ Encrypted Payload (AES-GCM or AES-CBC)                         │
│ - Decrypts to NAL unit (video) or Opus frame (audio)           │
└────────────────────────────────────────────────────────────────┘
```

### Control/Input Message Format

```
Keyboard: [type=0x0A][keyCode:2][modifiers:1][down:1]
Mouse Move: [type=0x08][deltaX:2][deltaY:2]
Mouse Button: [type=0x08][button:1][down:1]
Controller: [type=0x0C][controllerId:1][buttons:4][leftStick:4][rightStick:4][triggers:2]
```

## Risk Mitigation

### Risk 1: Protocol Edge Cases
The Moonlight protocol was reverse-engineered. Edge cases may only be documented in GPL code.

**Mitigation:**
- Test extensively against Wolf (our server)
- Use protocol analyzers (Wireshark) to capture real traffic
- File issues on moonlight-docs for clarification

### Risk 2: ENet Compatibility
Moonlight uses a modified ENet. Standard ENet may not work.

**Mitigation:**
- Start with standard ENet (go-enet CGO wrapper)
- If incompatible, implement minimal reliable UDP ourselves
- The control stream is low-bandwidth, so custom implementation is feasible

### Risk 3: Encryption Differences
Stream encryption may have Moonlight-specific quirks.

**Mitigation:**
- Test with Wolf (which we control)
- Use Wireshark to compare packet formats
- Wolf's Sunshine fork may have simpler encryption

### Risk 4: CGO Dependency (go-enet)
CGO complicates cross-compilation and deployment.

**Mitigation:**
- Build static binaries in Docker
- Consider pure-Go ENet implementation if CGO is problematic
- Control stream is simple enough to reimplement if needed

## Directory Structure

```
helix-moonlight-bridge/
├── cmd/
│   └── bridge/
│       └── main.go           # Entry point
├── internal/
│   ├── nvhttp/
│   │   ├── client.go         # NVHTTP API client
│   │   ├── pairing.go        # Pairing logic
│   │   └── crypto.go         # Certificate generation
│   ├── rtsp/
│   │   ├── client.go         # RTSP session management
│   │   └── sdp.go            # SDP parsing
│   ├── media/
│   │   ├── video.go          # Video stream handler
│   │   ├── audio.go          # Audio stream handler
│   │   └── decrypt.go        # AES decryption
│   ├── control/
│   │   ├── enet.go           # ENet control stream
│   │   └── input.go          # Input encoding
│   ├── wsbridge/
│   │   ├── server.go         # WebSocket server
│   │   ├── protocol.go       # Binary protocol (Luke's format)
│   │   └── session.go        # Session management
│   └── config/
│       └── config.go         # Configuration
├── pkg/
│   └── protocol/
│       ├── messages.go       # Shared message types
│       └── constants.go      # Protocol constants
├── go.mod
├── go.sum
├── Dockerfile
└── README.md
```

## Success Criteria

1. **Functional:** Can connect to Wolf and stream video/audio to browser
2. **Clean:** No GPL code, all dependencies are permissively licensed
3. **Compatible:** Works with Luke's existing TypeScript WebSocket client
4. **Maintainable:** Well-documented, tested, follows Go idioms
5. **Performant:** Handles 1080p60 at <100ms latency

## Open Questions

1. ~~**ENet or custom?**~~ → **Decided: Pure Go implementation**
2. **Encryption:** Does Wolf use the same encryption as Sunshine? Any simplifications?
3. **Session tokens:** How does Wolf's session ID flow work with the bridge?
4. **Auto-pairing:** Can we extend Wolf's auto-pairing to work with the Go bridge?
5. **Wolf's ENet fork:** What modifications exist? Need to analyze network traffic.

## Protocol Research Strategy

Since the protocol is reverse-engineered without formal docs, we'll:

1. **Use Wireshark** to capture real Moonlight ↔ Wolf traffic
2. **Compare** with Moonlight client behavior
3. **Document** our findings as we implement
4. **Iterate** based on what works

### Research Tools

```bash
# Capture Moonlight traffic
tcpdump -i any -w moonlight.pcap 'port 47984 or port 47989 or port 48010 or portrange 47998-48010'

# Analyze RTSP
tshark -r moonlight.pcap -Y 'rtsp'

# Analyze RTP
tshark -r moonlight.pcap -Y 'rtp'
```

## Session Log

### 2025-12-04: Initial Implementation
- Created repository at `~/pm/helix-moonlight-bridge`
- Implemented Phase 1 (project setup, NVHTTP skeleton, WebSocket skeleton)
- ~1,700 lines of Go code
- All dependencies permissively licensed (MIT/BSD)
- Committed initial cleanroom implementation

**Next Steps:**
1. Research RTSP protocol by capturing Wolf traffic
2. Implement RTSP client using gortsplib
3. Test against live Wolf instance

---

## First Steps ✅ DONE

1. ~~**Create the repo** at `~/pm/helix-moonlight-bridge`~~
2. ~~**Initialize Go module** with dependencies~~
3. ~~**Start with NVHTTP client** - simplest part, well-documented~~
4. **Test pairing** against Wolf ← Next priority
5. **Implement RTSP** ← After pairing works

## References

- [Moonlight Docs Wiki](https://github.com/moonlight-stream/moonlight-docs/wiki)
- [gortsplib](https://github.com/bluenviron/gortsplib) - MIT licensed RTSP/RTP for Go
- [go-enet](https://github.com/codecat/go-enet) - MIT licensed ENet wrapper
- [Moonlight Security Advisory](https://github.com/moonlight-stream/moonlight-ios/security/advisories/GHSA-g298-gp8q-h6j3) - Documents pairing flow
