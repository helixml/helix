# WebRTC Browser Streaming for Wolf Lobbies - Technical Design

**Status**: Design Review
**Created**: 2025-10-08
**Priority**: URGENT (Production deployment in days)

## Executive Summary

**Goal**: Enable browser-based streaming to Wolf lobbies without external Moonlight client, supporting multi-backend geographic distribution with NAT traversal.

**Critical Production Constraint**: Must deploy multi-backend Wolf infrastructure in **days**, not weeks.

**Recommendation**: **Hybrid Approach** - Deploy Moonlight virtual hosting NOW for production, add WebRTC as progressive enhancement.

---

## 1. Requirements

### Functional
- **F1**: Browser-based streaming (no external client)
- **F2**: Bidirectional audio
- **F3**: Input devices (mouse, keyboard, touch)
- **F4**: Multi-backend Wolf servers
- **F5**: Geographic distribution (latency optimization)
- **F6**: NAT traversal for Wolf agents

### Non-Functional
- **NF1**: Production-ready in **days** (not weeks)
- **NF2**: No single point of failure
- **NF3**: GPU memory scaling across hosts
- **NF4**: Enterprise firewall friendly
- **NF5**: Secure (encrypted, authenticated)

---

## 2. Current Architecture Analysis

### Wolf Lobby Infrastructure (wolf-ui branch)

**Strengths** ✅:
```
Producer (Wayland + PulseAudio)
    ↓ interpipe (zero-copy)
Consumer 1 (Moonlight Client 1)
    ├─> H.264 encode → RTP → UDP
Consumer 2 (Moonlight Client 2)
    ├─> H.264 encode → RTP → UDP
```

- **Battle-tested** multi-user support
- **Zero-copy** video with interpipe
- **Dynamic** source switching
- **PIN authentication**
- **HW encoder fallbacks** (NVENC → VAAPI → QSV → SW)

**Limitations** ❌:
- Moonlight protocol requires external client
- Complex port routing (HTTPS, RTSP, RTP, Control)
- NAT traversal challenges

### Helix Moonlight Proxy (Existing)

**File**: `/home/luke/pm/helix/api/pkg/moonlight/moonlight_proxy.go`

```go
// UDP-over-TCP encapsulation
type ProxyPacket struct {
    MagicHeader uint32  // 0xDEADBEEF
    SessionID   string  // Routing key
    ClientIP    string  // Source tracking
    Payload     []byte  // Actual UDP data
}
```

**Current State**:
- ✅ HTTP endpoints (serverinfo, pair, launch)
- ✅ Session management
- ✅ Packet forwarding
- ❌ RTSP handler stubbed (incomplete)
- ❌ No load balancing
- ❌ Single backend only

### Revdial NAT Traversal (Production Ready)

**File**: `/home/luke/pm/helix/api/pkg/revdial/revdial.go`

```go
// WebSocket-based reverse dial
type ReverseDialer struct {
    wsConn *websocket.Conn
    conns  map[string]net.Conn
}

// Agent initiates, Helix accepts
func (rd *ReverseDialer) Dial(network, address string) (net.Conn, error)
```

**Features**:
- ✅ Battle-tested in production
- ✅ 18s keep-alive (NAT mapping)
- ✅ Multi-connection multiplexing
- ✅ Proper error handling

---

## 3. Architectural Options Comparison

### Option A: Moonlight Virtual Hosting (IMMEDIATE)

**Timeline**: 2-3 days
**Complexity**: LOW
**Risk**: LOW

**Architecture**:
```
Browser (Moonlight Web Client)
    ↓ HTTPS/WSS
Helix API (Reverse Proxy)
    ├─> Backend 1 (Wolf + revdial) [us-east]
    ├─> Backend 2 (Wolf + revdial) [eu-west]
    └─> Backend 3 (Wolf + revdial) [ap-south]
```

**Implementation**:
1. Complete RTSP handler in moonlight_proxy.go
2. Add backend selection (geo-routing or load-based)
3. Integrate revdial for NAT traversal
4. Deploy Moonlight web client (existing OSS)

**Code Changes**:
```go
// moonlight_proxy.go - Add RTSP routing
func (p *MoonlightProxy) handleRTSP(w http.ResponseWriter, r *http.Request) {
    sessionID := extractSessionID(r)
    backend := p.selectBackend(sessionID, r.Header.Get("X-User-Location"))

    // Proxy RTSP over revdial
    conn, err := p.revdial.Dial("tcp", backend.RTSPAddr)
    if err != nil {
        http.Error(w, "Backend unavailable", 502)
        return
    }

    // Hijack HTTP, forward RTSP
    hijacker, _ := w.(http.Hijacker)
    clientConn, _, _ := hijacker.Hijack()

    go io.Copy(conn, clientConn)
    io.Copy(clientConn, conn)
}

// Backend selection
func (p *MoonlightProxy) selectBackend(sessionID, location string) *Backend {
    // Sticky session (same client → same backend)
    if backend, exists := p.sessions[sessionID]; exists {
        return backend
    }

    // Geo-routing
    backend := p.geoRoute(location)
    p.sessions[sessionID] = backend
    return backend
}
```

**Pros** ✅:
- Reuses Wolf's proven infrastructure
- No Wolf code changes
- Works with existing Moonlight clients (desktop, mobile, web)
- Can deploy TODAY

**Cons** ❌:
- Still requires Moonlight protocol (may be blocked in some enterprises)
- Multiple ports (HTTPS:47984, HTTPS:47989, RTSP:48010, RTP:47998-48000, Control:47999)
- Not ideal for pure browser use case

---

### Option B: WebRTC Hybrid (PROGRESSIVE)

**Timeline**: 6-8 weeks
**Complexity**: MEDIUM
**Risk**: MEDIUM

**Architecture**:
```
Wayland + PulseAudio
    ↓ interpipe
    ├─> Moonlight Consumer (existing)
    └─> WebRTC Consumer (NEW)
            ├─> webrtcbin (GStreamer)
            ├─> STUN/TURN
            └─> Signaling Server (Helix)
```

**Wolf Modification** (Minimal):
```cpp
// streaming.cpp - Add WebRTC consumer pipeline
void create_webrtc_consumer(const std::string &session_id) {
    auto pipeline = gst::PipelineBuilder()
        .add("interpipesrc", {
            {"listen-to", session_id + "_video"},
            {"is-live", true}
        })
        .add("videoconvert")
        .add("queue")
        .add("vp8enc", {  // WebRTC requires VP8/VP9/H.264
            {"deadline", 1},
            {"target-bitrate", 2000000}
        })
        .add("rtpvp8pay")
        .add("webrtcbin", {
            {"name", "webrtc_" + session_id},
            {"stun-server", "stun://stun.l.google.com:19302"}
        })
        .build();

    // Handle signaling
    g_signal_connect(webrtc, "on-negotiation-needed",
        G_CALLBACK(on_negotiation_needed), session_id);
    g_signal_connect(webrtc, "on-ice-candidate",
        G_CALLBACK(on_ice_candidate), session_id);
}
```

**Helix Signaling Server**:
```go
// webrtc_signaling.go (new file)
type WebRTCSignalingServer struct {
    wolfClients map[string]*websocket.Conn  // Wolf backend WS
    browserClients map[string]*websocket.Conn  // Browser WS
}

// Browser → Helix → Wolf
func (s *WebRTCSignalingServer) handleBrowserOffer(sessionID string, sdp string) {
    wolfConn := s.wolfClients[sessionID]
    wolfConn.WriteJSON(SignalingMessage{
        Type: "offer",
        SDP:  sdp,
    })
}

// Wolf → Helix → Browser
func (s *WebRTCSignalingServer) handleWolfAnswer(sessionID string, sdp string) {
    browserConn := s.browserClients[sessionID]
    browserConn.WriteJSON(SignalingMessage{
        Type: "answer",
        SDP:  sdp,
    })
}

// ICE candidate relay (both directions)
func (s *WebRTCSignalingServer) relayICECandidate(from, to string, candidate ICECandidate) {
    // Relay ICE candidates for NAT traversal
}
```

**Frontend** (React):
```typescript
// WebRTCPlayer.tsx
const WebRTCPlayer: React.FC<{sessionId: string}> = ({sessionId}) => {
    const [pc, setPc] = useState<RTCPeerConnection | null>(null);
    const videoRef = useRef<HTMLVideoElement>(null);
    const ws = useRef<WebSocket | null>(null);

    useEffect(() => {
        // Connect to signaling server
        ws.current = new WebSocket(`wss://helix.api/ws/webrtc/${sessionId}`);

        // Create peer connection
        const peerConnection = new RTCPeerConnection({
            iceServers: [
                {urls: 'stun:stun.l.google.com:19302'},
                {urls: 'turn:turn.helix.api:3478', credential: '...'}
            ]
        });

        // Handle tracks (video/audio)
        peerConnection.ontrack = (event) => {
            videoRef.current!.srcObject = event.streams[0];
        };

        // Handle ICE candidates
        peerConnection.onicecandidate = (event) => {
            if (event.candidate) {
                ws.current!.send(JSON.stringify({
                    type: 'ice-candidate',
                    candidate: event.candidate
                }));
            }
        };

        // Create offer
        peerConnection.createOffer().then(offer => {
            peerConnection.setLocalDescription(offer);
            ws.current!.send(JSON.stringify({
                type: 'offer',
                sdp: offer.sdp
            }));
        });

        // Handle signaling messages
        ws.current.onmessage = (event) => {
            const msg = JSON.parse(event.data);
            if (msg.type === 'answer') {
                peerConnection.setRemoteDescription(new RTCSessionDescription({
                    type: 'answer',
                    sdp: msg.sdp
                }));
            } else if (msg.type === 'ice-candidate') {
                peerConnection.addIceCandidate(new RTCIceCandidate(msg.candidate));
            }
        };

        setPc(peerConnection);

        return () => {
            peerConnection.close();
            ws.current?.close();
        };
    }, [sessionId]);

    return <video ref={videoRef} autoPlay playsInline />;
};
```

**Pros** ✅:
- Browser-native (no external client)
- Enterprise firewall friendly (HTTPS/WSS only)
- Can use TURN for NAT traversal
- Progressive enhancement (add alongside Moonlight)

**Cons** ❌:
- Requires Wolf code changes (webrtcbin integration)
- Need TURN server for strict NAT
- 6-8 weeks to production-ready
- Encoder format change (VP8 vs H.264)

---

### Option C: Pure WebRTC (FUTURE)

**Timeline**: 12-16 weeks
**Complexity**: HIGH
**Risk**: HIGH

Replace Moonlight entirely with WebRTC.

**Pros** ✅:
- Cleanest browser integration
- Single protocol

**Cons** ❌:
- Major Wolf rewrite
- Lose proven Moonlight infrastructure
- High risk for production deployment
- NOT suitable for "days" timeline

**Status**: NOT RECOMMENDED for immediate deployment

---

## 4. Recommended Hybrid Approach

### Phase 1: Moonlight Virtual Hosting (Days 1-3)

**Deploy NOW for production**:

1. **Complete RTSP handler** (4 hours)
   - Add RTSP routing to moonlight_proxy.go
   - Test with Moonlight client

2. **Integrate revdial** (4 hours)
   - Connect to Wolf backends via revdial
   - Handle reconnection

3. **Add backend selection** (4 hours)
   ```go
   type BackendPool struct {
       backends []*Backend
       sessions map[string]*Backend  // Sticky sessions
       mu       sync.RWMutex
   }

   func (bp *BackendPool) SelectBackend(sessionID, geo string) *Backend {
       bp.mu.Lock()
       defer bp.mu.Unlock()

       // Sticky session
       if backend, exists := bp.sessions[sessionID]; exists {
           return backend
       }

       // Geo-routing
       backend := bp.geoRoute(geo)

       // GPU memory check
       if backend.GPUMemoryUsage > 0.9 {
           backend = bp.leastLoaded()
       }

       bp.sessions[sessionID] = backend
       return backend
   }
   ```

4. **Deploy Moonlight Web Client** (2 hours)
   - Use existing OSS Moonlight.js
   - Embed in Helix frontend

**Result**: Production multi-backend system in 3 days

### Phase 2: WebRTC Consumer (Weeks 1-6)

**Add WebRTC alongside Moonlight**:

1. **Wolf webrtcbin integration** (Week 1-2)
   - Add WebRTC consumer pipeline
   - Test with single client

2. **Helix signaling server** (Week 2-3)
   - WebSocket signaling
   - SDP/ICE relay

3. **Frontend WebRTC player** (Week 3-4)
   - React component
   - Input device handling

4. **TURN server** (Week 4-5)
   - Deploy coturn
   - Configure for NAT traversal

5. **Testing & optimization** (Week 5-6)
   - Multi-backend testing
   - Performance tuning

**Result**: Browser-native streaming available, Moonlight still works

### Phase 3: Migration (Weeks 7-8)

- Gradual rollout of WebRTC to users
- Keep Moonlight as fallback
- Monitor metrics (latency, quality, errors)

---

## 5. Multi-Backend Routing Strategy

### Geographic Routing

```go
type GeoRouter struct {
    regions map[string][]*Backend  // "us-east" → [backend1, backend2]
}

func (gr *GeoRouter) Route(clientIP string) *Backend {
    // IP geolocation
    geo := geolocate(clientIP)

    // Find nearest region
    region := gr.nearestRegion(geo)

    // Load balance within region
    backends := gr.regions[region]
    return backends[rand.Intn(len(backends))]
}
```

### GPU Memory Aware

```go
type Backend struct {
    ID              string
    Location        string
    GPUMemoryUsage  float64  // 0.0 - 1.0
    ActiveSessions  int
    MaxSessions     int
}

func (bp *BackendPool) leastLoaded() *Backend {
    var best *Backend
    minLoad := 1.0

    for _, backend := range bp.backends {
        if backend.ActiveSessions >= backend.MaxSessions {
            continue
        }

        load := backend.GPUMemoryUsage
        if load < minLoad {
            minLoad = load
            best = backend
        }
    }

    return best
}
```

### Health Checking

```go
func (bp *BackendPool) healthCheck(ctx context.Context) {
    ticker := time.NewTicker(10 * time.Second)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            for _, backend := range bp.backends {
                // Check GPU memory
                gpuMem, err := backend.GetGPUMemory(ctx)
                if err != nil {
                    backend.Healthy = false
                    continue
                }
                backend.GPUMemoryUsage = gpuMem
                backend.Healthy = true
            }
        }
    }
}
```

---

## 6. NAT Traversal Architecture

### Wolf Behind NAT (Revdial)

```
Wolf Agent (Corporate NAT)
    ↓ WebSocket (outbound, port 443)
Helix API (Public)
    ├─> Accept revdial connections
    └─> Proxy Moonlight/WebRTC traffic

// Wolf initiates
revdialClient.Connect("wss://helix.api/revdial")

// Helix accepts when needed
conn, err := revdialServer.Dial("tcp", "wolf-backend-123:48010")
```

### WebRTC TURN Fallback

```
Browser
    ├─> STUN (public IP discovery)
    ├─> Direct P2P (if possible)
    └─> TURN relay (if NAT blocked)
            ↓
        Wolf (via TURN)
```

**TURN Server** (coturn):
```bash
# /etc/coturn/turnserver.conf
listening-port=3478
tls-listening-port=5349
min-port=49152
max-port=65535
realm=helix.api
user=helix:$TURN_PASSWORD
lt-cred-mech
```

---

## 7. Security Considerations

### Authentication Flow

```
1. User requests stream → Helix API
2. Helix validates user/session
3. Generate short-lived token (JWT, 5min TTL)
4. Return stream URL + token
5. Browser/Client presents token to Wolf
6. Wolf validates with Helix
7. Stream established
```

**Token Structure**:
```go
type StreamToken struct {
    UserID    string
    SessionID string
    LobbyID   string
    ExpiresAt time.Time
}

// JWT signed with Helix private key
token := jwt.NewWithClaims(jwt.SigningMethodRS256, StreamToken{...})
```

### Encryption

- **Moonlight**: TLS for HTTPS, AES for RTP (built-in)
- **WebRTC**: DTLS-SRTP (built-in)
- **Signaling**: WSS (WebSocket Secure)

### Input Validation

```go
// Prevent injection attacks
func sanitizeInput(event InputEvent) InputEvent {
    // Validate coordinates
    event.X = clamp(event.X, 0, maxWidth)
    event.Y = clamp(event.Y, 0, maxHeight)

    // Validate key codes
    if !isValidKeyCode(event.KeyCode) {
        return InputEvent{Type: "noop"}
    }

    return event
}
```

---

## 8. Production Deployment (3-Day Plan)

### Day 1: Infrastructure

**Morning**:
- [ ] Deploy revdial server on Helix API
- [ ] Configure Wolf backends in 3 regions (us-east, eu-west, ap-south)
- [ ] Establish revdial connections

**Afternoon**:
- [ ] Complete RTSP handler in moonlight_proxy.go
- [ ] Add backend pool management
- [ ] Test with single Moonlight client

### Day 2: Routing & Load Balancing

**Morning**:
- [ ] Implement geo-routing
- [ ] Add GPU memory monitoring
- [ ] Sticky session management

**Afternoon**:
- [ ] Health check system
- [ ] Failover logic
- [ ] Load testing (simulate 100 concurrent users)

### Day 3: Frontend & Testing

**Morning**:
- [ ] Embed Moonlight.js in frontend
- [ ] Wire up API integration
- [ ] Test from different regions

**Afternoon**:
- [ ] Security audit
- [ ] Performance tuning
- [ ] Documentation
- [ ] Deploy to production 🚀

---

## 9. Monitoring & Metrics

### Key Metrics

```go
type StreamingMetrics struct {
    // Per-backend
    ActiveStreams    int
    GPUMemoryUsage   float64
    CPUUsage         float64
    NetworkBandwidth int64  // bytes/sec

    // Per-stream
    Latency          time.Duration
    PacketLoss       float64
    VideoQuality     string  // "1080p60", "720p30", etc.
    AudioQuality     string

    // Errors
    ConnectionErrors int
    EncodingErrors   int
    DecodingErrors   int
}
```

### Alerts

- GPU memory > 90% → Provision new backend
- Packet loss > 5% → Check network
- Latency > 100ms → Route to closer backend
- Connection errors > 10/min → Backend unhealthy

---

## 10. Cost Analysis

### Moonlight Virtual Hosting (Phase 1)

**Infrastructure**:
- 3 Wolf backends (GPU instances): $2/hour each = $6/hour
- Helix API (existing): $0 incremental
- No TURN server needed: $0

**Total**: ~$150/day for 3-region deployment

### WebRTC Hybrid (Phase 2)

**Additional**:
- TURN server (coturn): $0.50/hour = $12/day
- Bandwidth (if TURN relay): ~$0.10/GB

**Total**: ~$160/day

**Note**: Direct P2P (STUN only) reduces TURN costs significantly

---

## 11. Comparison Matrix

| Criteria | Moonlight Virtual Hosting | WebRTC Hybrid | Pure WebRTC |
|----------|---------------------------|---------------|-------------|
| **Timeline** | **3 days** ✅ | 6-8 weeks | 12-16 weeks |
| **Browser Native** | No (needs Moonlight.js) | **Yes** ✅ | **Yes** ✅ |
| **Enterprise Friendly** | Moderate (multiple ports) | **High** ✅ | **High** ✅ |
| **Wolf Code Changes** | **None** ✅ | Minimal | Major |
| **Risk** | **Low** ✅ | Medium | High |
| **Multi-Backend** | **Yes** ✅ | **Yes** ✅ | **Yes** ✅ |
| **NAT Traversal** | Revdial | **TURN** ✅ | **TURN** ✅ |
| **Production Ready** | **Now** ✅ | 6-8 weeks | 12-16 weeks |

---

## 12. Decision Matrix

### Choose Moonlight Virtual Hosting (Phase 1) IF:
- ✅ Need production deployment in **days**
- ✅ Want to reuse proven Wolf infrastructure
- ✅ Can accept external client requirement (temporary)
- ✅ Want **zero Wolf code changes**

### Add WebRTC Hybrid (Phase 2) IF:
- ✅ Need browser-native experience
- ✅ Have 6-8 weeks for development
- ✅ Want enterprise firewall friendliness
- ✅ Can tolerate minimal Wolf changes

### Skip Pure WebRTC IF:
- ❌ Timeline is urgent
- ❌ Risk tolerance is low
- ❌ Don't want major Wolf rewrite

---

## 13. Recommendation: HYBRID PHASED APPROACH

### Immediate (Day 1-3): Moonlight Virtual Hosting
✅ Deploy multi-backend Moonlight infrastructure
✅ Use revdial for NAT traversal
✅ Geo-routing and load balancing
✅ **Production ready in 3 days**

### Progressive (Week 1-8): Add WebRTC
✅ Keep Moonlight running
✅ Add WebRTC as alternative
✅ Browser-native experience
✅ **Zero downtime migration**

### Future (Month 3+): Optimize
✅ Monitor usage patterns
✅ Potentially deprecate Moonlight if WebRTC proves superior
✅ Add advanced features (spatial audio, haptic feedback)

---

## 14. Success Criteria

### Phase 1 (Moonlight)
- [ ] Multi-region deployment (3+ regions)
- [ ] < 50ms latency intra-region
- [ ] < 150ms latency cross-region
- [ ] 99.9% uptime
- [ ] GPU scaling (no single host > 80% memory)

### Phase 2 (WebRTC)
- [ ] Browser-native streaming works
- [ ] < 100ms glass-to-glass latency
- [ ] 60fps video in optimal conditions
- [ ] Bidirectional audio < 50ms latency
- [ ] Input latency < 10ms

---

## 15. Appendix: Code References

### Wolf Lobby (Reusable)
- `/home/luke/pm/wolf/src/moonlight-server/streaming.cpp:45-89` - Lobby creation
- `/home/luke/pm/wolf/src/moonlight-server/streaming.cpp:450-580` - Consumer pipelines
- `/home/luke/pm/wolf/src/moonlight-server/control/control.cpp:67-89` - Input handling

### Helix Infrastructure (Existing)
- `/home/luke/pm/helix/api/pkg/moonlight/moonlight_proxy.go:1-300` - Moonlight proxy
- `/home/luke/pm/helix/api/pkg/revdial/revdial.go:1-250` - NAT traversal

### Frontend (To Build)
- Moonlight.js: https://github.com/moonlight-stream/moonlight-chrome (OSS)
- WebRTC: Native browser RTCPeerConnection API

---

**Conclusion**: Deploy Moonlight virtual hosting NOW (3 days), add WebRTC progressively (6-8 weeks). This minimizes risk while achieving all goals.
