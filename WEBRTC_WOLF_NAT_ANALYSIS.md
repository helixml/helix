# WebRTC Streaming, Wolf Lobbies, Moonlight Protocol & NAT Traversal - Comprehensive Code Review

**Date**: 2025-10-08
**Purpose**: Detailed analysis of existing battle-tested code for WebRTC streaming implementation

---

## Executive Summary

### Current State
- **WebRTC**: No existing implementation in Helix
- **Wolf Lobbies**: Full multi-user streaming infrastructure on `wolf-ui` branch
- **Moonlight Protocol**: Complete routing/proxy skeleton (partial implementation)
- **NAT Traversal**: Working revdial implementation (reverse dial for NAT penetration)

### Key Finding
Wolf lobbies provide a **complete, battle-tested streaming infrastructure** using GStreamer + interpipe. This is the foundation we should build on for WebRTC, NOT replace it.

---

## 1. Wolf GStreamer & WebRTC Architecture

### Current Status: NO WebRTC in Wolf
**Important**: Wolf does NOT use WebRTC. It uses:
- **Moonlight protocol** (UDP-based RTP streaming)
- **GStreamer pipelines** for video/audio encoding
- **Interpipe** for dynamic source switching

### Wolf Lobby Architecture (wolf-ui branch)

#### File: `/home/luke/pm/wolf/src/moonlight-server/sessions/lobbies.cpp`

**Lobby Creation Flow**:
```cpp
// Lines 72-148: CreateLobbyEvent handler
void setup_lobbies_handlers(...) {
  // 1. Create Wayland compositor + GStreamer video producer
  streaming::start_video_producer(
    lobby->id,
    lobby_settings->video_settings.video_producer_buffer_caps,
    lobby_settings->video_settings.wayland_render_node,
    {.width, .height, .refreshRate},
    on_ready,
    ev_bus
  );

  // 2. Start audio virtual sink + producer
  streaming::start_audio_producer(
    lobby->id,
    ev_bus,
    channel_count,
    sink_name,
    audio_server_name
  );

  // 3. Start runner (Docker container or process)
  start_runner(lobby->runner, ...);
}
```

**Key Components**:
- **Wayland Display**: Custom compositor for GPU capture (`waylanddisplaysrc`)
- **Virtual Audio Sink**: PulseAudio virtual sink for audio capture
- **GStreamer Producers**: Video/audio pipelines using `interpipesink`
- **Runner**: Docker container or process that renders content

#### File: `/home/luke/pm/wolf/src/moonlight-server/streaming/streaming.cpp`

**Video Producer Pipeline** (Lines 64-122):
```cpp
auto pipeline = fmt::format(
  "waylanddisplaysrc name=wolf_wayland_source render_node={render_node} ! "
  "{buffer_format}, width={width}, height={height}, framerate={fps}/1 ! "
  "{pipeline_fix}"  // NVIDIA OpenGL fix if needed
  "interpipesink sync=true async=false name={session_id}_video max-buffers=1"
);
```

**Audio Producer Pipeline** (Lines 124-173):
```cpp
auto pipeline = fmt::format(
  "pulsesrc device=\"{sink_name}\" server=\"{server_name}\" ! "
  "audio/x-raw, channels={channels}, channel-mask=(bitmask){channel_mask}, rate=48000 ! "
  "queue leaky=downstream max-size-buffers=3 ! "
  "interpipesink name=\"{session_id}_audio\" sync=true async=false max-buffers=3"
);
```

**Video Consumer Pipeline** (Lines 249-356):
```cpp
auto pipeline = fmt::format(
  "interpipesrc name=interpipesrc_{}_video listen-to={session_id}_video "
  "is-live=true stream-sync=restart-ts max-bytes=0 max-buffers=1 leaky-type=downstream ! "

  "{video_params} ! "  // Scaling, color conversion
  "{encoder_pipeline} ! "  // H264/HEVC/AV1 encoder

  "rtpmoonlightpay_video name=moonlight_pay "
  "payload_size={payload_size} fec_percentage={fec_percentage} ! "
  "appsink sync=false name=wolf_udp_sink"
);
```

**Audio Consumer Pipeline** (Lines 362-453):
```cpp
auto pipeline = fmt::format(
  "interpipesrc name=interpipesrc_{}_audio listen-to={session_id}_audio "
  "is-live=true stream-sync=restart-ts max-bytes=0 max-buffers=3 block=false ! "

  "queue max-size-buffers=3 leaky=downstream ! audiorate ! audioconvert ! "
  "opusenc bitrate={bitrate} bitrate-type=cbr frame-size={packet_duration} ! "

  "rtpmoonlightpay_audio name=moonlight_pay packet_duration={packet_duration} ! "
  "appsink name=wolf_udp_sink"
);
```

**Interpipe Architecture**:
```
Producer                    Consumer (per client)
─────────                   ─────────────────────
[Wayland Capture]           [interpipesrc]
       ↓                             ↓
[interpipesink]  ←─────────  [listen-to: session_id]
 (session_id)                        ↓
                            [Scale/Convert/Encode]
                                     ↓
                            [RTP Payload/Packetize]
                                     ↓
                                 [UDP Send]
```

**Dynamic Source Switching** (Lines 323-342):
```cpp
// Switch between lobby video and personal session video
auto switch_producer_handler = event_bus->register_handler<SwitchStreamProducerEvents>(
  [sess_id, pipeline](const SwitchStreamProducerEvents &switch_ev) {
    if (switch_ev->session_id == sess_id) {
      // Get interpipesrc element
      auto src = gst_bin_get_by_name(GST_BIN(pipeline.get()), "interpipesrc_*_video");

      // Switch which producer to listen to
      auto video_interpipe = fmt::format("{}_video", switch_ev->interpipe_src_id);
      g_object_set(src, "listen-to", video_interpipe.c_str(), nullptr);
    }
  }
);
```

#### File: `/home/luke/pm/wolf/src/moonlight-server/state/default/config.v6.toml`

**GStreamer Configuration** (Lines 315-560):
```toml
[gstreamer.video]
default_source = 'interpipesrc name=interpipesrc_{}_video listen-to={session_id}_video is-live=true stream-sync=restart-ts max-bytes=0 max-buffers=1 leaky-type=downstream'

default_sink = """
rtpmoonlightpay_video name=moonlight_pay \
payload_size={payload_size} fec_percentage={fec_percentage} !
appsink sync=false name=wolf_udp_sink
"""

# Multiple encoder options (HW accelerated preferred)
[[gstreamer.video.hevc_encoders]]
plugin_name = "nvcodec"
encoder_pipeline = """
nvh265enc gop-size=-1 bitrate={bitrate} aud=false rc-mode=cbr
zerolatency=true preset=p1 tune=ultra-low-latency !
h265parse ! video/x-h265, profile=main, stream-format=byte-stream
"""

[[gstreamer.video.h264_encoders]]
plugin_name = "nvcodec"
encoder_pipeline = """
nvh264enc preset=low-latency-hq zerolatency=true gop-size=0 rc-mode=cbr-ld-hq !
h264parse ! video/x-h264, profile=main, stream-format=byte-stream
"""
```

**Audio Configuration** (Lines 545-560):
```toml
[gstreamer.audio]
default_source = """
interpipesrc name=interpipesrc_{}_audio listen-to={session_id}_audio
is-live=true stream-sync=restart-ts max-bytes=0 max-buffers=3 block=false
"""

default_opus_encoder = """
opusenc bitrate={bitrate} bitrate-type=cbr frame-size={packet_duration}
bandwidth=fullband audio-type=restricted-lowdelay max-payload-size=1400
"""
```

### Multi-User Lobby Support

#### File: `/home/luke/pm/wolf/src/moonlight-server/sessions/lobbies.cpp`

**Join Lobby Flow** (Lines 175-236):
```cpp
void join_lobby(lobby, session) {
  // 1. Switch input devices to lobby
  session->mouse->emplace(WaylandMouse(lobby->wayland_display));
  session->keyboard->emplace(WaylandKeyboard(lobby->wayland_display));

  // 2. Switch controllers to lobby
  for (auto [_joypad_nr, joypad] : session->joypads->load()) {
    // Plug into lobby
    fire_event(PlugDeviceEvent{.session_id = lobby->id, ...});
    // Unplug from session
    fire_event(UnplugDeviceEvent{.session_id = session->session_id, ...});
  }

  // 3. Switch A/V stream producers
  fire_event(SwitchStreamProducerEvents{
    .session_id = session->session_id,
    .interpipe_src_id = lobby->id  // Switch to lobby's video/audio
  });
}
```

**Leave Lobby Flow** (Lines 16-64):
```cpp
void leave_lobby(lobby, session) {
  // 1. Remove session from lobby
  lobby->connected_sessions->update([session](auto sessions) {
    return sessions | filter([session](auto id) {
      return *id != session.session_id;
    });
  });

  // 2. Switch back to personal session
  session->mouse->emplace(WaylandMouse(session->wayland_display));
  session->keyboard->emplace(WaylandKeyboard(session->wayland_display));

  // 3. Switch controllers back
  for (auto joypad : session->joypads->load()) {
    fire_event(PlugDeviceEvent{.session_id = session->session_id, ...});
    fire_event(UnplugDeviceEvent{.session_id = lobby->id, ...});
  }

  // 4. Switch A/V back to personal session
  fire_event(SwitchStreamProducerEvents{
    .session_id = session->session_id,
    .interpipe_src_id = session->session_id  // Back to personal video/audio
  });

  // 5. Auto-stop lobby if empty
  if (lobby->stop_when_everyone_leaves && lobby->connected_sessions->size() == 0) {
    fire_event(StopLobbyEvent{.lobby_id = lobby->id});
  }
}
```

**PIN Authentication** (Wolf API):
```cpp
// File: /home/luke/pm/wolf/src/moonlight-server/api/endpoints.cpp (Lines 28-45)
void endpoint_Pair(const HTTPRequest &req, std::shared_ptr<UnixSocket> socket) {
  auto event = rfl::json::read<PairRequest>(req.body);

  if (auto pair_request = app_state->pairing_atom->load()->find(event.pair_secret)) {
    // Resolve PIN promise - user enters PIN, promise completes
    pair_request->get().user_pin->set_value(event.pin.value());

    send_http(socket, 200, GenericSuccessResponse{.success = true});
  } else {
    send_http(socket, 500, GenericErrorResponse{.error = "Invalid pair secret"});
  }
}
```

### Wolf API for Lobby Management

#### File: `/home/luke/pm/wolf/src/moonlight-server/api/endpoints.cpp`

**Available Endpoints** (from grep search):
```cpp
POST /api/v1/lobbies/create     // Create new lobby
GET  /api/v1/lobbies             // List lobbies
POST /api/v1/lobbies/{id}/join   // Join lobby
POST /api/v1/lobbies/{id}/leave  // Leave lobby
POST /api/v1/lobbies/{id}/stop   // Stop lobby
GET  /api/v1/events              // Server-Sent Events stream
```

**Lobby Creation API** (Lines ~150-200):
```cpp
void UnixSocketServer::endpoint_LobbyCreate(const HTTPRequest &req, ...) {
  auto create_lobby_req = rfl::json::read<CreateLobbyRequest>(req.body);

  auto create_lobby_ev = events::CreateLobbyEvent{
    .id = generate_lobby_id(),
    .name = create_lobby_req.name,
    .profile_id = create_lobby_req.profile_id,
    .multi_user = create_lobby_req.multi_user,
    .pin = create_lobby_req.pin,
    .video_settings = create_lobby_req.video_settings,
    .audio_settings = create_lobby_req.audio_settings,
    .runner = create_lobby_req.runner,
    .on_setup_over = on_setup_over
  };

  app_state->event_bus->fire_event(create_lobby_ev);
  on_setup_over.get_future().get();  // Wait for async setup

  send_http(socket, 200, LobbyCreateResponse{.lobby_id = lobby_id});
}
```

**Data Structures**:
```cpp
struct CreateLobbyRequest {
  std::string name;
  std::string profile_id;
  bool multi_user;
  std::optional<std::string> pin;
  VideoSettings video_settings;
  AudioSettings audio_settings;
  Runner runner;
};

struct Lobby {
  std::string id;
  std::string name;
  std::string started_by_profile_id;
  std::string icon_png_path;
  bool multi_user;
  std::optional<std::string> pin;
  bool stop_when_everyone_leaves;
  Runner runner;

  // Runtime state
  std::shared_ptr<AtomicBox<WaylandDisplay>> wayland_display;
  std::shared_ptr<AtomicBox<AudioDevice>> audio_sink;
  std::shared_ptr<AtomicBox<vector<string>>> connected_sessions;
  std::shared_ptr<TSQueue<PlugDeviceEvent>> plugged_devices_queue;
};
```

---

## 2. Existing Helix WebRTC Implementations

### Status: NO WebRTC Code Found

**Search Results**: No files containing:
- `RTCPeerConnection`
- `RTCDataChannel`
- `webrtc` (case-insensitive)
- `signaling`

**Conclusion**: WebRTC streaming must be built from scratch OR adapted from Wolf's GStreamer infrastructure.

---

## 3. Moonlight Protocol & Routing

### Helix Implementation (Partial)

#### File: `/home/luke/pm/helix/api/pkg/moonlight/proxy.go`

**UDP-over-TCP Encapsulation Protocol**:
```go
// Constants (Lines 17-25)
const (
  UDP_PACKET_MAGIC = uint32(0xDEADBEEF)  // Magic bytes for packet identification
  MAX_UDP_PACKET_SIZE = 8192              // Max Moonlight RTP packet size
  UDP_HEADER_SIZE = 18                    // magic(4) + length(4) + session_id(8) + port(2)
)

// Packet Header (Lines 27-33)
type UDPPacketHeader struct {
  Magic     uint32  // 0xDEADBEEF
  Length    uint32  // Length of UDP payload
  SessionID uint64  // Moonlight session ID for routing
  Port      uint16  // Original UDP port (47999, 48100, 48200)
}
```

**Moonlight Proxy Architecture** (Lines 49-130):
```go
type MoonlightProxy struct {
  // Connection management
  connman *connman.ConnectionManager  // Reverse dial connection manager

  // Session tracking
  sessions     map[uint64]*MoonlightSession  // sessionID -> session
  sessionsByIP map[string]*MoonlightSession  // clientIP -> session (for routing)

  // UDP listeners for Moonlight ports
  videoListener   net.PacketConn  // Port 48100
  audioListener   net.PacketConn  // Port 48200
  controlListener net.PacketConn  // Port 47999

  // Configuration
  basePort   int     // Base port for TCP (47984, 47989)
  publicHost string  // Public hostname for clients
}

// Session structure (Lines 35-47)
type MoonlightSession struct {
  SessionID      uint64
  AppID          uint64
  HelixSessionID string
  RunnerID       string            // For revdial routing
  ClientIP       string            // For UDP routing
  SecretPayload  [16]byte          // RTP identification
  TCPConn        net.Conn          // TCP tunnel to backend
  UDPPorts       map[uint16]string // port -> purpose
}
```

**UDP Packet Routing** (Lines 198-221):
```go
func (mp *MoonlightProxy) routeUDPPacket(packet []byte, clientAddr net.Addr, port uint16, streamType string) {
  clientIP := clientAddr.(*net.UDPAddr).IP.String()

  // Find session by client IP
  session, exists := mp.sessionsByIP[clientIP]
  if !exists {
    log.Debug("No session found for client IP")
    return
  }

  // Forward packet to backend via TCP tunnel
  mp.forwardUDPPacket(session, packet, port)
}

func (mp *MoonlightProxy) forwardUDPPacket(session *MoonlightSession, packet []byte, port uint16) {
  // Serialize header
  header := UDPPacketHeader{
    Magic:     UDP_PACKET_MAGIC,
    Length:    uint32(len(packet)),
    SessionID: session.SessionID,
    Port:      port,
  }

  // Send: [header][packet] over TCP to backend
  session.TCPConn.Write(headerBytes)
  session.TCPConn.Write(packet)
}
```

**Backend UDP Decapsulation** (`/home/luke/pm/helix/api/pkg/moonlight/backend.go`):
```go
// Lines 140-204: Read from proxy, forward to Wolf
func (mb *MoonlightBackend) handleProxyToWolf() {
  for {
    // Read UDP packet header from proxy TCP connection
    headerBytes := make([]byte, UDP_HEADER_SIZE)
    io.ReadFull(mb.proxyConn, headerBytes)

    // Parse header
    magic := binary.BigEndian.Uint32(headerBytes[0:4])
    length := binary.BigEndian.Uint32(headerBytes[4:8])
    sessionID := binary.BigEndian.Uint64(headerBytes[8:16])
    port := binary.BigEndian.Uint16(headerBytes[16:18])

    // Read payload
    payload := make([]byte, length)
    io.ReadFull(mb.proxyConn, payload)

    // Forward to appropriate Wolf port
    mb.forwardToWolf(payload, port)
  }
}

// Lines 256-281: Forward to Wolf sockets
func (mb *MoonlightBackend) forwardToWolf(payload []byte, port uint16) error {
  var socket net.Conn

  switch port {
  case 47999:
    socket = mb.controlSocket  // Control stream
  case 48100:
    socket = mb.videoSocket    // Video stream
  case 48200:
    socket = mb.audioSocket    // Audio stream
  }

  socket.Write(payload)  // Send raw UDP payload
}
```

**Session Registration** (Lines 273-316):
```go
func (mp *MoonlightProxy) RegisterSession(sessionID, appID uint64, helixSessionID, runnerID, clientIP string, secretPayload [16]byte) error {
  // Get reverse dial connection to runner
  tcpConn, err := mp.connman.Dial(ctx, runnerID)
  if err != nil {
    return fmt.Errorf("failed to dial runner %s: %w", runnerID, err)
  }

  session := &MoonlightSession{
    SessionID:      sessionID,
    AppID:          appID,
    HelixSessionID: helixSessionID,
    RunnerID:       runnerID,
    ClientIP:       clientIP,
    SecretPayload:  secretPayload,
    TCPConn:        tcpConn,
    UDPPorts:       map[uint16]string{
      47999: "control",
      48100: "video",
      48200: "audio",
    },
  }

  mp.sessions[sessionID] = session
  mp.sessionsByIP[clientIP] = session

  // Start bidirectional packet forwarding
  go mp.handleBackendUDPPackets(session)
}
```

#### File: `/home/luke/pm/helix/api/pkg/moonlight/handlers.go`

**HTTP Endpoints** (Lines 122-148):
```go
func (ms *MoonlightServer) RegisterRoutes(router *mux.Router) {
  moonlightRouter := router.PathPrefix("/moonlight").Subrouter()

  // Standard Moonlight protocol endpoints
  moonlightRouter.HandleFunc("/serverinfo", ms.handleServerInfo).Methods("GET")
  moonlightRouter.HandleFunc("/pair", ms.handlePair).Methods("GET")
  moonlightRouter.HandleFunc("/applist", ms.authMiddleware(ms.handleAppList)).Methods("GET")
  moonlightRouter.HandleFunc("/launch", ms.authMiddleware(ms.handleLaunch)).Methods("GET")
  moonlightRouter.HandleFunc("/resume", ms.authMiddleware(ms.handleResume)).Methods("GET")
  moonlightRouter.HandleFunc("/cancel", ms.authMiddleware(ms.handleCancel)).Methods("GET")
  moonlightRouter.HandleFunc("/quit", ms.authMiddleware(ms.handleQuit)).Methods("GET")

  // RTSP proxy for session negotiation
  moonlightRouter.HandleFunc("/rtsp", ms.handleRTSP).Methods("GET", "POST", "SETUP", "PLAY", "TEARDOWN")
}
```

**Pairing Flow** (Lines 186-239):
```go
func (ms *MoonlightServer) handlePair(w http.ResponseWriter, r *http.Request) {
  pin := r.URL.Query().Get("pin")
  uniqueID := r.URL.Query().Get("uniqueid")

  // Validate PIN against Helix user session
  userID, err := ms.validatePairingPin(pin)
  if err != nil {
    ms.writeErrorResponse(w, 401, "Invalid PIN")
    return
  }

  // Generate client certificate
  cert := ms.generateClientCertificate(uniqueID)

  // Store paired client
  client := &PairedClient{
    UniqueID:     uniqueID,
    Certificate:  cert,
    UserID:       userID,
    PairedAt:     time.Now(),
  }

  ms.pairedClients[cert] = client

  // Return certificate to client
  w.Write([]byte(`<certificate>` + cert + `</certificate>`))
}
```

**Launch Flow** (Lines 397-466):
```go
func (ms *MoonlightServer) handleLaunch(w http.ResponseWriter, r *http.Request) {
  appID, _ := strconv.ParseUint(r.URL.Query().Get("appid"), 10, 64)

  // Find app
  app := ms.apps[appID]

  // Generate session ID
  sessionID := uint64(time.Now().UnixNano())

  // Get client IP
  clientIP := strings.Split(r.RemoteAddr, ":")[0]

  // Create secret payload for RTP identification
  var secretPayload [16]byte
  // ... generate secret ...

  // Register session with proxy
  err = ms.proxy.RegisterSession(
    sessionID,
    appID,
    app.HelixSessionID,
    app.RunnerID,
    clientIP,
    secretPayload
  )

  // Build RTSP URL
  rtspURL := fmt.Sprintf("rtsp://%s:48010", r.Host)

  // Return launch response
  w.Write([]byte(`
    <gamesession>` + sessionID + `</gamesession>
    <sessionUrl0>` + rtspURL + `</sessionUrl0>
  `))
}
```

**Port Usage**:
- **47984**: HTTPS (pairing, serverinfo)
- **47989**: HTTP (legacy)
- **47999**: Control stream UDP
- **48010**: RTSP
- **48100**: Video stream UDP
- **48200**: Audio stream UDP

**Session Affinity**: Based on client IP address (`sessionsByIP` map)

**Load Balancing**: Not implemented (single proxy instance)

---

## 4. Revdial & NAT Traversal

### Full Implementation Exists

#### File: `/home/luke/pm/helix/api/pkg/revdial/revdial.go`

**Core Concept** (Lines 7-18):
```go
// Package revdial implements a Dialer and Listener which work together
// to turn an accepted connection (for instance, a Hijacked HTTP request) into
// a Dialer which can then create net.Conns connecting back to the original
// dialer, which then gets a net.Listener accepting those conns.
//
// The motivation is that sometimes you want to run a server on a
// machine deep inside a NAT. Rather than connecting to the machine
// directly (which you can't, because of the NAT), you have the
// sequestered machine connect out to a public machine. Both sides
// then use revdial and the public machine can become a client for the
// NATed machine.
```

**Dialer (Public Server Side)** (Lines 49-221):
```go
type Dialer struct {
  conn         net.Conn  // hijacked client conn
  path         string    // e.g. "/revdial"
  uniqID       string    // unique identifier
  pickupPath   string    // path with uniqID for pickup
  incomingConn chan net.Conn
  pickupFailed chan error
  connReady    chan bool
}

// Create new dialer from hijacked connection
func NewDialer(c net.Conn, connPath string) *Dialer {
  d := &Dialer{
    path:         connPath,
    uniqID:       newUniqID(),
    conn:         c,
    incomingConn: make(chan net.Conn),
    pickupFailed: make(chan error),
    connReady:    make(chan bool),
  }

  d.pickupPath = connPath + "?" + dialerUniqParam + "=" + d.uniqID
  d.register()  // Register in global map
  go d.serve()  // Start control loop
  return d
}

// Dial creates a new connection back to the Listener
func (d *Dialer) Dial(ctx context.Context) (net.Conn, error) {
  // Signal we want a connection
  select {
  case d.connReady <- true:
  case <-ctx.Done():
    return nil, ctx.Err()
  }

  // Wait for connection or error
  select {
  case c := <-d.incomingConn:
    return c, nil
  case err := <-d.pickupFailed:
    return nil, err
  case <-ctx.Done():
    return nil, ctx.Err()
  }
}

// Control message loop (Lines 164-212)
func (d *Dialer) serve() error {
  // Read control messages from listener
  go func() {
    br := bufio.NewReader(d.conn)
    for {
      line, _ := br.ReadSlice('\n')
      var msg controlMsg
      json.Unmarshal(line, &msg)

      switch msg.Command {
      case "pickup-failed":
        d.pickupFailed <- fmt.Errorf("pickup failed: %v", msg.Err)
      }
    }
  }()

  // Send periodic keep-alives and connection requests
  for {
    d.sendMessage(controlMsg{Command: "keep-alive"})

    select {
    case <-time.After(dialerPingInterval):  // 18 seconds
      continue
    case <-d.connReady:
      d.sendMessage(controlMsg{
        Command:  "conn-ready",
        ConnPath: d.pickupPath,
      })
    }
  }
}
```

**Listener (Behind NAT Side)** (Lines 224-384):
```go
type Listener struct {
  sc    net.Conn  // server connection (outbound from NAT)
  connc chan net.Conn
  dial  func(ctx, path string) (*websocket.Conn, *http.Response, error)
}

func NewListener(serverConn net.Conn, dialServer func(...) (*websocket.Conn, ...)) *Listener {
  ln := &Listener{
    sc:    serverConn,
    dial:  dialServer,
    connc: make(chan net.Conn, 8),
  }
  go ln.run()
  return ln
}

// Listen for control messages (Lines 264-307)
func (ln *Listener) run() {
  br := bufio.NewReader(ln.sc)
  for {
    line, _ := br.ReadSlice('\n')
    var msg controlMsg
    json.Unmarshal(line, &msg)

    switch msg.Command {
    case "keep-alive":
      // No-op, keeps NAT mapping alive
    case "conn-ready":
      go ln.grabConn(msg.ConnPath)  // Fetch new connection
    }
  }
}

// Grab connection from server (Lines 315-340)
func (ln *Listener) grabConn(path string) {
  ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
  defer cancel()

  // Dial out to pickup path
  wsConn, resp, err := ln.dial(ctx, path)
  if err != nil {
    ln.sendMessage(controlMsg{
      Command:  "pickup-failed",
      ConnPath: path,
      Err:      err.Error(),
    })
    return
  }

  if resp.StatusCode != 101 {  // WebSocket upgrade
    wsConn.Close()
    return
  }

  // Deliver connection to Accept()
  ln.connc <- wsconnadapter.New(wsConn)
}

// Accept implements net.Listener (Lines 350-362)
func (ln *Listener) Accept() (net.Conn, error) {
  c, ok := <-ln.connc
  if !ok {
    return nil, ErrListenerClosed
  }
  return c, nil
}
```

**HTTP Handler (Public Server)** (Lines 395-415):
```go
func ConnHandler(upgrader websocket.Upgrader) http.Handler {
  return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    dialerUniq := r.FormValue(dialerUniqParam)

    // Find dialer by unique ID
    d, ok := dialers[dialerUniq]
    if !ok {
      http.Error(w, "unknown dialer", 500)
      return
    }

    // Upgrade to WebSocket
    wsConn, err := upgrader.Upgrade(w, r, nil)
    if err != nil {
      http.Error(w, "upgrade failed", 500)
      return
    }

    // Match connection to waiting Dial() call
    d.matchConn(wsconnadapter.New(wsConn))
  })
}
```

**Control Messages** (Lines 256-260):
```go
type controlMsg struct {
  Command  string `json:"command,omitempty"`   // "keep-alive", "conn-ready", "pickup-failed"
  ConnPath string `json:"connPath,omitempty"`  // pickup URL path
  Err      string `json:"err,omitempty"`       // error message if pickup-failed
}
```

**Usage Pattern** (from `/home/luke/pm/helix/revdial.md`):
```go
// Server side (public):
func initiateDeviceConnection(w http.ResponseWriter, r *http.Request) {
  device := getDeviceFromContext(r.Context())

  withHijackedWebSocketConnection(w, r, func(clientConn net.Conn) {
    connman.Set(device.ID, clientConn)  // Store for later Dial()
  })
}

// Later, when server wants to connect to device:
deviceConn, err := connman.Dial(ctx, device.ID)
// Now deviceConn is a connection TO the device behind NAT

// Agent side (behind NAT):
func serveRemote(ctx context.Context) error {
  conn, err := client.InitiateDeviceConnection(ctx)  // Connect to server

  listener := revdial.NewListener(conn, revdial)
  defer listener.Close()

  return remoteServer.Serve(listener)  // Serve on reverse dial listener
}

func revdial(ctx context.Context, path string) (*websocket.Conn, *http.Response, error) {
  return websocket.DefaultDialer.Dial(
    ctx,
    getWebsocketURL(serverURL, path),
    nil,
  )
}
```

**NAT Traversal Flow**:
```
Agent (NAT)                    Server (Public)
───────────                    ───────────────
[1] Connect WS ─────────────────> Accept WS
                                  Create Dialer
                                  Store in map
[2] Create Listener
    Listen for control msgs <──── Keep-alive (18s)

[3] Server wants connection:
                            <──── conn-ready + pickupPath
[4] Dial pickupPath ────────────> Find Dialer
    (with uniqID)                 Upgrade to WS
                                  matchConn()
[5] Accept() receives conn <───── Dial() receives conn

[6] Bidirectional data flow over WebSocket tunnel
```

**Key Features**:
- **Keep-alive**: 18-second pings maintain NAT mapping
- **WebSocket-based**: Reliable bidirectional communication
- **Multiple connections**: Can create many connections over one initial WS
- **Timeout handling**: 20-second timeout for connection pickup
- **Error reporting**: pickup-failed messages sent back to server

---

## 5. Screenshot Server (For Comparison)

#### File: `/home/luke/pm/helix/api/cmd/screenshot-server/main.go`

**Simple HTTP Server in Container**:
```go
func main() {
  port := os.Getenv("SCREENSHOT_PORT")
  if port == "" {
    port = "9876"
  }

  http.HandleFunc("/screenshot", handleScreenshot)
  http.HandleFunc("/health", healthCheck)

  http.ListenAndServe(":"+port, nil)
}

func handleScreenshot(w http.ResponseWriter, r *http.Request) {
  // Create temp file
  filename := filepath.Join(tmpDir, fmt.Sprintf("screenshot-%d.png", time.Now().UnixNano()))

  // Capture screenshot using grim (Wayland screenshot tool)
  cmd := exec.Command("grim", filename)

  // Set environment for Wayland
  cmd.Env = append(os.Environ(),
    fmt.Sprintf("WAYLAND_DISPLAY=%s", waylandDisplay),
    fmt.Sprintf("XDG_RUNTIME_DIR=%s", xdgRuntimeDir),
  )

  // Execute
  output, err := cmd.CombinedOutput()

  // Read screenshot
  data, _ := os.ReadFile(filename)

  // Serve PNG
  w.Header().Set("Content-Type", "image/png")
  w.Write(data)
}
```

**Deployment Pattern**:
- Simple Go binary compiled into container
- Listens on configurable port (9876)
- Uses Wayland tools (`grim`) for capture
- Environment variables for Wayland socket selection

**Similar Pattern Could Work for WebRTC**:
- Small Go binary in container
- WebRTC signaling server
- GStreamer pipeline control
- Environment-based configuration

---

## 6. Architecture Diagrams

### Wolf Lobby Multi-User Streaming
```
┌─────────────────────────────────────────────────────────┐
│                      WOLF LOBBY                         │
│                                                         │
│  ┌──────────────┐    ┌─────────────────────────┐      │
│  │   Runner     │───>│  Wayland Compositor     │      │
│  │  (Docker/    │    │  (waylanddisplaysrc)    │      │
│  │   Process)   │    └───────────┬─────────────┘      │
│  └──────────────┘                │                      │
│                                   ▼                      │
│  ┌──────────────┐    ┌─────────────────────────┐      │
│  │ PulseAudio   │───>│  interpipesink          │      │
│  │ Virtual Sink │    │  (session_id_video)     │      │
│  └──────────────┘    │  (session_id_audio)     │      │
│                      └───────────┬─────────────┘      │
└───────────────────────────────────┼──────────────────┘
                                    │
                ┌───────────────────┴───────────────────┐
                │                                       │
        ┌───────▼──────┐                       ┌───────▼──────┐
        │  Client A    │                       │  Client B    │
        │              │                       │              │
        │ interpipesrc │                       │ interpipesrc │
        │      ↓       │                       │      ↓       │
        │  Encoder     │                       │  Encoder     │
        │      ↓       │                       │      ↓       │
        │  RTP Pay     │                       │  RTP Pay     │
        │      ↓       │                       │      ↓       │
        │  UDP Send    │                       │  UDP Send    │
        └──────────────┘                       └──────────────┘
```

### Moonlight Protocol Routing (Helix)
```
Moonlight Client                 Helix Server (Public)                   Runner (NAT)
───────────────                  ─────────────────────                   ────────────
[UDP 48100] ──────┐
[UDP 48200] ──────┼─────────> [UDP Listeners]
[UDP 47999] ──────┘               │
                                  │ routeUDPPacket()
                                  │ (by clientIP)
                                  ▼
                          [MoonlightSession]
                                  │ sessionID
                                  │ runnerID
                                  │ TCPConn ←──────────────────┐
                                  ▼                            │
                          [Encapsulate]                        │
                          magic(4) + len(4) +                  │
                          session(8) + port(2)                 │
                                  │                            │
                                  ▼                            │
                          [Write to TCPConn] ─────────────────┤
                                                               │
                                                               │ Reverse Dial
                                                               │ (via revdial)
                                                               │
                                                               ▼
                                                       [MoonlightBackend]
                                                               │
                                                               ▼ Decapsulate
                                                       [Parse Header]
                                                               │
                                                               ▼
                                                    ┌──────────┴──────────┐
                                                    │                     │
                                            [47999 TCP] ───> Wolf     [48100 TCP] ───> Wolf
                                            [48200 TCP] ───> Wolf

                                            Wolf Moonlight Server
                                                    │
                                                    ▼
                                            [GStreamer Pipelines]
                                                    │
                                                    ▼
                                            [Wayland Display]
```

### Revdial NAT Traversal
```
Agent (Behind NAT)                         Server (Public)
──────────────────                         ───────────────
[1] WebSocket Connect ──────────────────> Accept WS
                                           ┌─────────────────┐
                                           │ Dialer          │
                                           │ - conn: WS      │
                                           │ - uniqID: abc123│
                                           │ - pickupPath:   │
                                           │   /revdial?     │
                                           │   dialer=abc123 │
                                           └─────────────────┘
                                                    │
┌─────────────────┐                                │
│ Listener        │ <───── keep-alive ─────────────┤
│ - sc: WS conn   │        (every 18s)             │
│ - dial: fn      │                                │
└─────────────────┘                                │
         │                                         │
         │                                  [Server wants conn]
         │                                         │
         │ <────── conn-ready ────────────────────┤
         │         ConnPath=/revdial?dialer=abc123│
         │                                         │
         ├─── Dial pickupPath ───────────────────>│
         │                                  [Find Dialer abc123]
         │                                         │
         │ <────── WebSocket Upgrade ─────────────┤
         │         (Status 101)                    │
         │                                         │
         ├─── deliver to connc ───>        [matchConn()]
         │                                         │
[Accept() returns conn]                    [Dial() returns conn]
         │                                         │
         ◀──────────── Data Flow ─────────────────▶
                   (over WebSocket)
```

---

## 7. Reusable Patterns & Battle-Tested Code

### Wolf Lobby System (HIGHLY REUSABLE)
✅ **Interpipe Architecture**: Dynamic source switching without pipeline rebuild
✅ **Multi-user support**: Proven input device management
✅ **PIN authentication**: Simple pairing flow
✅ **HW acceleration**: Multiple encoder fallbacks (NVENC, VAAPI, QSV, SW)
✅ **Event-driven**: Clean event bus for state management

**Recommendation**: Use Wolf's lobby system as the **foundation** for WebRTC streaming. Add WebRTC as an alternative consumer pipeline.

### Revdial NAT Traversal (PRODUCTION READY)
✅ **Battle-tested**: In use in Helix for agent connections
✅ **WebSocket-based**: Works through most firewalls
✅ **Keep-alive mechanism**: Maintains NAT mappings
✅ **Error handling**: Proper timeout and failure reporting

**Recommendation**: Use revdial for connecting to runners behind NAT. No need to reinvent.

### Moonlight Protocol Routing (FOUNDATION COMPLETE)
✅ **UDP encapsulation protocol**: Header format defined
✅ **Session management**: sessionID and IP-based routing
✅ **Packet forwarding**: Bidirectional proxy logic
⚠️ **Incomplete**: RTSP handling is stubbed, needs full implementation

**Recommendation**: Complete the RTSP handler and test with Moonlight client. Use as reference for WebRTC signaling.

---

## 8. Current Limitations & Gaps

### Wolf System
❌ **No WebRTC support**: Only Moonlight (UDP RTP) protocol
❌ **No browser clients**: Requires Moonlight app installation
❌ **No STUN/TURN**: Relies on direct UDP or proxy
❌ **No H.264 baseline**: Uses Main/High profiles (not all browsers)

### Helix Implementation
❌ **No WebRTC code**: Must be built from scratch
❌ **Partial Moonlight**: RTSP handling incomplete
❌ **No signaling server**: WebRTC needs SDP exchange mechanism
❌ **No ICE handling**: No STUN/TURN integration

### Integration Gaps
❌ **Runner communication**: Need to expose Wolf API via revdial
❌ **GStreamer WebRTC elements**: Need `webrtcbin` integration
❌ **Certificate management**: WebRTC requires DTLS certificates
❌ **Browser compatibility**: Need to test codec support matrix

---

## 9. Recommendations

### Option 1: WebRTC on Top of Wolf Lobbies (RECOMMENDED)
**Approach**: Keep Wolf's battle-tested streaming infrastructure, add WebRTC as an alternative consumer.

**Architecture**:
```
Wolf Lobby (Producer)
├── waylanddisplaysrc → interpipesink (session_id_video)
├── pulsesrc → interpipesink (session_id_audio)
│
├─> Moonlight Consumer (existing)
│   └── interpipesrc → h264enc → rtpmoonlightpay → UDP
│
└─> WebRTC Consumer (new)
    └── interpipesrc → h264enc → webrtcbin → DTLS/SRTP
```

**Implementation Steps**:
1. Add `webrtcbin` GStreamer element support to Wolf
2. Create WebRTC consumer pipeline (similar to Moonlight consumer)
3. Build signaling server in Helix (WebSocket-based SDP exchange)
4. Integrate with revdial for NAT traversal
5. Add STUN/TURN server support for browser clients

**Benefits**:
- Reuses proven GStreamer infrastructure
- Minimal changes to Wolf core
- Can support both Moonlight and WebRTC clients
- Leverages interpipe for multi-user support

### Option 2: Standalone WebRTC Server
**Approach**: Build separate WebRTC streaming server, bypass Wolf.

**Architecture**:
```
Runner Container
├── Wayland Display
│   └── grim/wf-recorder → WebRTC GStreamer Pipeline
│       └── waylandsrc → h264enc → webrtcbin → Browser
│
└── PulseAudio
    └── pulsesrc → opusenc → webrtcbin → Browser
```

**Benefits**:
- Simpler architecture (no Wolf dependency)
- Direct browser streaming
- Easier debugging

**Drawbacks**:
- No multi-user support
- No lobby system
- Must reimplement video/audio capture
- No HW encoder fallback logic

### Option 3: Hybrid Approach
**Approach**: Use Wolf for orchestration, direct WebRTC for streaming.

**Architecture**:
```
Wolf API ─> Create Lobby ─> Start Runner
                │
                └─> Runner Container
                    ├── Wolf Wayland Display (for Moonlight)
                    └── Separate WebRTC Server (for browser)
                        └── webrtcbin → DTLS → Browser
```

**Benefits**:
- Best of both worlds
- Can use Wolf's lobby management
- Direct browser streaming without Wolf proxy

**Drawbacks**:
- Duplicate video capture
- More complex deployment
- Resource overhead

---

## 10. Next Steps

### Phase 1: Proof of Concept (1-2 weeks)
1. ✅ **Review Wolf lobby code** (DONE - this document)
2. 🔲 **Set up Wolf lobby test environment**
   - Build Wolf with wolf-ui branch
   - Create test lobby via API
   - Verify Moonlight streaming works
3. 🔲 **Test GStreamer webrtcbin**
   - Create simple webrtcbin pipeline
   - Test with browser peer connection
   - Verify H.264 baseline support

### Phase 2: WebRTC Consumer Pipeline (2-3 weeks)
1. 🔲 **Add webrtcbin to Wolf**
   - Modify streaming.cpp to support WebRTC consumer
   - Create WebRTC-specific encoder config
   - Test with single client
2. 🔲 **Build signaling server in Helix**
   - WebSocket endpoint for SDP exchange
   - ICE candidate relay
   - Session management
3. 🔲 **Test NAT traversal**
   - Deploy STUN server (coturn)
   - Test browser → runner connection
   - Verify revdial integration

### Phase 3: Production Features (3-4 weeks)
1. 🔲 **Multi-user support**
   - Test multiple WebRTC consumers on same lobby
   - Implement lobby join/leave for browser clients
   - Add PIN authentication for browsers
2. 🔲 **Quality improvements**
   - Add adaptive bitrate
   - Implement FEC for packet loss
   - Optimize encoder settings for latency
3. 🔲 **Browser client**
   - React component for WebRTC streaming
   - Input device forwarding
   - Connection quality indicators

### Phase 4: Integration & Deployment (2-3 weeks)
1. 🔲 **Helix API integration**
   - Create PDE with WebRTC support
   - External agent WebRTC streaming
   - Session management
2. 🔲 **Testing & optimization**
   - End-to-end latency testing
   - Multi-user stress testing
   - Cross-browser compatibility
3. 🔲 **Documentation**
   - Architecture diagrams
   - API documentation
   - Deployment guide

---

## 11. Code Reference Index

### Wolf Lobbies (wolf-ui branch)
- **Lobby Management**: `/home/luke/pm/wolf/src/moonlight-server/sessions/lobbies.cpp`
- **GStreamer Pipelines**: `/home/luke/pm/wolf/src/moonlight-server/streaming/streaming.cpp`
- **Config Schema**: `/home/luke/pm/wolf/src/moonlight-server/state/default/config.v6.toml`
- **API Endpoints**: `/home/luke/pm/wolf/src/moonlight-server/api/endpoints.cpp`
- **Event Types**: `/home/luke/pm/wolf/src/moonlight-server/events/events.hpp`

### Moonlight Protocol (Helix)
- **Proxy Logic**: `/home/luke/pm/helix/api/pkg/moonlight/proxy.go`
- **Backend Handler**: `/home/luke/pm/helix/api/pkg/moonlight/backend.go`
- **HTTP Handlers**: `/home/luke/pm/helix/api/pkg/moonlight/handlers.go`

### Revdial NAT Traversal
- **Core Implementation**: `/home/luke/pm/helix/api/pkg/revdial/revdial.go`
- **Connection Manager**: `/home/luke/pm/helix/api/pkg/connman/connman.go`
- **Usage Guide**: `/home/luke/pm/helix/revdial.md`

### Reference Implementations
- **Screenshot Server**: `/home/luke/pm/helix/api/cmd/screenshot-server/main.go`
- **Wolf API Client**: `/home/luke/pm/helix/api/pkg/wolf/client.go`

---

## Conclusion

**Key Takeaway**: Wolf's lobby system provides a complete, battle-tested streaming infrastructure. Instead of building WebRTC from scratch, we should:

1. **Leverage Wolf's interpipe architecture** for producer/consumer separation
2. **Add webrtcbin as an alternative consumer** alongside Moonlight
3. **Use revdial for NAT traversal** (already proven in Helix)
4. **Build WebRTC signaling** in Helix Go backend (minimal new code)

This approach maximizes code reuse, minimizes risk, and provides a clear migration path from Moonlight to WebRTC while maintaining backward compatibility.

**Estimated Timeline**: 8-12 weeks for production-ready WebRTC streaming with multi-user lobby support.
