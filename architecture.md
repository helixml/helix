# Wolf Streaming Platform Architecture

## Overview

Wolf is a Moonlight-compatible streaming server that enables remote desktop streaming via the Moonlight protocol. It manages applications, clients, and streaming sessions through a sophisticated event-driven architecture.

## Core Components

### 1. Applications (`events::App`)

**Structure**: Applications are the fundamental units that can be streamed. Each app has:

```cpp
struct App {
  moonlight::App base;                    // Basic app metadata (id, title, icon)
  std::string h264_gst_pipeline;          // GStreamer pipeline for H.264 encoding
  std::string hevc_gst_pipeline;          // GStreamer pipeline for HEVC encoding
  std::string av1_gst_pipeline;           // GStreamer pipeline for AV1 encoding
  std::string opus_gst_pipeline;          // GStreamer pipeline for audio
  bool start_virtual_compositor;          // Whether to start Wayland compositor
  bool start_audio_server;                // Whether to start audio server
  std::shared_ptr<Runner> runner;         // How to execute the app (Docker, process, etc.)
}
```

**App Types**:
- **Docker apps**: Run in containers with full desktop environments
- **Process apps**: Run as system processes
- **Child session apps**: Inherit from parent sessions

**App Management**:
- Apps are stored in `state->config->apps` (persistent across restarts)
- Apps can be added/removed via Wolf's REST API (`/api/v1/apps/add`, `/api/v1/apps/delete`)
- Each app has a unique string ID (not necessarily numeric)

### 2. Clients (`state::PairedClient`)

**Structure**: Clients represent Moonlight devices that have been paired with Wolf:

```cpp
struct PairedClient {
  std::string client_cert;               // X.509 certificate for authentication
  std::string app_state_folder;          // Per-client app state directory
  wolf::config::ClientSettings settings; // Client-specific settings (UID, GID, controllers, etc.)
}
```

**Client Lifecycle**:
1. **Pairing**: 4-phase Moonlight pairing protocol over HTTP
2. **Authentication**: HTTPS requests use client certificates for auth
3. **Storage**: Paired clients stored in `state->config->paired_clients` (persistent)

**Client Authentication Flow**:
- Initial pairing happens over HTTP (`/pair` endpoint)
- Subsequent requests use HTTPS with client certificate validation
- Wolf validates certificates against stored paired clients

### 3. Sessions (`events::StreamSession`)

**Structure**: Sessions represent active streaming connections between clients and apps:

```cpp
struct StreamSession {
  // Core session data
  moonlight::DisplayMode display_mode;   // Resolution, refresh rate, codec support
  int audio_channel_count;               // Audio configuration
  std::shared_ptr<App> app;              // The app being streamed
  std::string app_local_state_folder;    // App state directory

  // Encryption and networking
  std::string aes_key;                   // Encryption key for streams
  std::string aes_iv;                    // Initialization vector
  std::array<char, 16> rtp_secret_payload; // RTP authentication
  std::string rtsp_fake_ip;              // Fake IP for RTSP routing

  // Session identification
  uint64_t session_id;                   // Unique session ID
  std::string ip;                        // Client IP address
  unsigned short video_stream_port;      // RTP video port
  unsigned short audio_stream_port;      // RTP audio port
  unsigned short control_stream_port;    // Control stream port

  // Runtime state
  std::shared_ptr<EventBusType> event_bus;           // Event system
  immer::box<wolf::config::ClientSettings> client_settings; // Client config

  // Input devices (populated when session starts)
  std::optional<MouseTypes> mouse;
  std::optional<KeyboardTypes> keyboard;
  std::vector<JoypadTypes> joypads;
  std::optional<TouchScreenTypes> touch_screen;
  std::optional<PenTabletTypes> pen_tablet;

  // Wayland display (for compositor-based apps)
  std::optional<virtual_display::wl_state_ptr> wayland_display;
}
```

**Session Types**:

1. **Regular Sessions** (`auto_persistent_sessions = false`):
   - Created when client launches an app
   - Use client ID as session ID
   - Destroyed when client disconnects
   - Traditional Moonlight behavior

2. **Background/Persistent Sessions** (`auto_persistent_sessions = true`):
   - Created automatically when apps are added to Wolf
   - Use deterministic session ID (hash of app ID)
   - Persist between client connections
   - Allow session sharing/takeover by newest client

**Session Lifecycle**:
1. **Creation**: Via `/launch` (regular) or automatic (background)
2. **Storage**: Added to `state->running_sessions` (in-memory only)
3. **Streaming**: Video/audio pipelines activated
4. **Termination**: Via `/cancel` or client disconnect

### 4. State Management

**Global State** (`state::AppState`):
```cpp
struct AppState {
  immer::box<Config> config;                                    // Configuration
  immer::atom<immer::vector<events::StreamSession>> running_sessions; // Active sessions
  std::shared_ptr<EventBusType> event_bus;                     // Event system
  // ... other components
}
```

**Configuration** (`state::Config`):
```cpp
struct Config {
  bool auto_persistent_sessions;                    // Enable background sessions
  immer::atom<PairedClientList> paired_clients;    // Paired Moonlight clients
  immer::atom<AppList> apps;                        // Available applications
  // ... other settings
}
```

## Session Management Architecture

### Regular Session Flow (auto_persistent_sessions = false)

1. **Client Launch Request** → `/launch?appid=X`
2. **Session Creation** → `create_stream_session()` with client ID as session ID
3. **App Startup** → Runner starts app container/process
4. **Stream Setup** → Video/audio pipelines configured
5. **Client Disconnect** → Session destroyed, app stopped

### Background Session Flow (auto_persistent_sessions = true)

1. **App Addition** → `/api/v1/apps/add` with auto-start enabled
2. **Background Session Creation** → `create_stream_session()` with deterministic session ID (hash of app ID)
3. **Container Startup** → App container starts immediately
4. **Session Persistence** → Session remains in `running_sessions` waiting for clients
5. **Client Connection** → Client connects to existing session (no new session creation)
6. **Session Reuse** → Multiple clients can connect sequentially (latest takes over)
7. **Persistence** → Session survives client disconnects, container keeps running

### Session ID Strategy

- **Regular sessions**: `session_id = get_client_id(current_client)` (client certificate hash)
- **Background sessions**: `session_id = std::hash<std::string>{}(app.base.id)` (app ID hash)

This deterministic approach enables session reuse for background sessions.

## API Endpoints and Protocol

### HTTP Endpoints (Pairing, Public)
- `/serverinfo` - Server capabilities and status
- `/pair` - 4-phase Moonlight pairing protocol
- `/pin/` - PIN entry for pairing

### HTTPS Endpoints (Authenticated)
- `/applist` - List available applications
- `/launch` - Start streaming session
- `/resume` - Resume existing session
- `/cancel` - Stop streaming session
- `/appasset` - Serve app icons/assets

### Wolf API Endpoints (Unix Socket)
- `/api/v1/apps` - App management
- `/api/v1/sessions` - Session management
- `/api/v1/clients` - Client management

## Event System

Wolf uses an event-driven architecture with typed events:

**Key Events**:
- `StreamSession` - New session created, triggers app startup
- `VideoSession` / `AudioSession` - Stream configuration
- `StopStreamEvent` - Session termination
- `PairSignal` - Client pairing request

**Event Flow**:
1. Events fired via `event_bus->fire_event()`
2. Handlers registered for specific event types
3. Events trigger actions (app startup, stream setup, etc.)

## Personal Dev Environment Integration

**Helix Integration**:
- Personal Dev Environments use Wolf for streaming
- Each environment becomes a Wolf app (Docker-based)
- Background sessions enable persistent containers
- Latest client connecting gets stream to running container

**Configuration**:
- `auto_persistent_sessions = true` enables background behavior
- Apps auto-created when Personal Dev Environments start
- Containers persist between Moonlight connections

## Key Architectural Decisions

1. **Immutable State**: Uses `immer` library for thread-safe immutable data structures
2. **Event-Driven**: Loose coupling via event bus prevents tight dependencies
3. **Session Persistence**: Background sessions enable container reuse
4. **Deterministic IDs**: Hash-based session IDs enable predictable session reuse
5. **Protocol Compatibility**: Full Moonlight protocol support for client compatibility
6. **Container-First**: Docker integration for secure app isolation

## Critical Files

- `src/moonlight-server/events/events.hpp` - Core data structures
- `src/moonlight-server/state/sessions.hpp` - Session creation logic
- `src/moonlight-server/rest/endpoints.hpp` - Moonlight protocol endpoints
- `src/moonlight-server/rest/servers.cpp` - HTTP/HTTPS server setup
- `src/moonlight-server/api/endpoints.cpp` - Wolf API implementation
- `src/moonlight-server/state/configTOML.cpp` - Configuration management

## Current Issue Context

**Problem**: Background sessions not showing as "busy" in `/serverinfo` response
**Root Cause**: Session detection works, but `endpoints::serverinfo` may have issue processing app ID
**Status**: Session creation ✅, Session detection ✅, Response generation ❌