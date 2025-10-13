# Agent Sandboxes Dashboard Implementation Progress

## Summary

Building a comprehensive Wolf debugging dashboard for the Helix admin panel to monitor and debug:
- Memory usage issues
- Video corruption (GStreamer pipelines, multiple feeds)
- Input scaling issues (resolution-related)
- General Wolf streaming problems

## Main Request

Create "Agent Sandboxes" dashboard with:
1. Memory usage monitoring (process RSS, GStreamer buffers, per-lobby breakdown)
2. Lobby management (list, resolution settings, client connections)
3. Stream session monitoring (client connections, resolution comparison)
4. GStreamer pipeline visualization (producers, consumers, interpipe flow)
5. Real-time event stream (SSE from Wolf)

## Work Completed

### 1. Wolf Memory Endpoint (✅ COMPLETE)
**Location**: `/home/luke/pm/wolf/src/moonlight-server/api/`

**Files Modified**:
- `events/events.hpp`:
  - Moved `VideoSettings` and `AudioSettings` struct definitions before `Lobby`
  - Added `video_settings` field to `Lobby` structure
- `sessions/lobbies.cpp`: Updated lobby creation to store video_settings
- `api.hpp`: Added `SystemMemoryResponse`, `LobbyMemoryUsage`, and `ClientConnectionInfo` structures
- `endpoints.cpp`: Added `endpoint_SystemMemory()` implementation with:
  - Actual lobby resolution from `lobby.video_settings`
  - Iteration over all client connections (StreamSessions)
  - Lobby association detection for each client
- `unix_socket_server.cpp`: Registered `GET /api/v1/system/memory` endpoint

**Final Implementation**:
```cpp
struct SystemMemoryResponse {
  bool success = true;
  size_t process_rss_bytes;           // From /proc/self/status
  size_t gstreamer_buffer_bytes;      // Estimated
  size_t total_memory_bytes;          // Total Wolf memory
  std::vector<LobbyMemoryUsage> lobbies;  // Per-lobby breakdown
  std::vector<ClientConnectionInfo> clients; // All active clients
};

struct LobbyMemoryUsage {
  std::string lobby_id;
  std::string lobby_name;
  std::string resolution;              // Actual resolution from VideoSettings
  size_t client_count;                 // Connected clients
  size_t memory_bytes;                 // Estimated lobby memory
};

struct ClientConnectionInfo {
  size_t session_id;                   // Moonlight session ID
  std::string client_ip;               // Client IP address
  std::string resolution;              // Client video resolution
  std::optional<std::string> lobby_id; // Lobby if connected
  size_t memory_bytes;                 // Estimated client memory
};
```

**Tested & Working**:
```bash
$ docker compose -f docker-compose.dev.yaml exec api curl -s --unix-socket /var/run/wolf/wolf.sock http://localhost/api/v1/system/memory | jq '.'
{
  "success": true,
  "process_rss_bytes": "83226624",
  "gstreamer_buffer_bytes": "0",
  "total_memory_bytes": "83226624",
  "lobbies": [],
  "clients": []
}
```

**Key Improvements**:
1. ✅ **Actual lobby resolution**: Now retrieves actual resolution from `lobby.video_settings`
2. ✅ **Client connection iteration**: Iterates over all `running_sessions` to show individual clients
3. ✅ **Leak detection**: Shows which lobby each client is connected to (or none if orphaned)
4. ✅ **Memory breakdown**: Separate estimates for lobbies and individual clients

### 2. Endpoint Registration
- ✅ Added includes: `<fstream>`, `<sstream>` to endpoints.cpp
- ✅ Registered endpoint at `/api/v1/system/memory`
- ✅ Added OpenAPI schema documentation

## Current Build Error

```
ninja: error: 'src/moonlight-server/api/libmoonlight-server_api.a', needed by 'src/moonlight-server/wolf', missing and no known rule to make it
```

**Likely cause**: Syntax error or missing dependency in the C++ code preventing library compilation.

## Next Steps (Priority Order)

### Immediate Fixes Required

1. **Fix Wolf Build Error**
   - Debug the linking issue in Wolf build
   - Verify endpoint_SystemMemory implementation compiles
   - Check for missing includes or syntax errors

2. **Enhance Memory Endpoint** (User Request)
   - Get actual lobby resolution from `VideoSettings` (not placeholders)
   - Add client connection iteration (iterate `running_sessions`)
   - Show per-client memory estimates to detect leaks

### Implementation Roadmap

3. **Create Helix API Proxy** (`/api/v1/admin/agent-sandboxes/debug`)
   ```go
   // In api/pkg/server/admin_wolf.go
   func (s *HelixAPIServer) getAgentSandboxesDebug(rw http.ResponseWriter, r *http.Request) {
       // Proxy to Wolf: GET unix://wolf.sock/api/v1/system/memory
       // Proxy to Wolf: GET unix://wolf.sock/api/v1/lobbies
       // Proxy to Wolf: GET unix://wolf.sock/api/v1/sessions
       // Combine and return
   }
   ```

4. **Create Helix SSE Proxy** (`/api/v1/admin/agent-sandboxes/events`)
   ```go
   // SSE streaming proxy to Wolf /api/v1/events
   // Keep connection open, forward events to browser
   ```

5. **Build React Dashboard Component**
   - Location: `frontend/src/pages/Admin/AgentSandboxes.tsx`
   - 5 panels: Memory, Lobbies, Sessions, Pipeline Viz, Events
   - Use React Query for data fetching
   - SSE EventSource for real-time events

6. **Add to Admin Navigation**
   - Update `frontend/src/pages/Admin/Admin.tsx`
   - Add "Agent Sandboxes" tab

## Technical Architecture

### Resolution Flow (Already Understood)
1. **Lobby Resolution**: IMMUTABLE, set at creation (e.g., 2360x1640@120)
2. **Client Resolution**: Requested via `/launch?mode=1280x800x60`
3. **Wolf Transcoding**: Transcodes lobby output to match client request
4. **Display Mode**: Stored in `StreamSession.display_mode`

### Data Structures

**Lobby** (events.hpp):
```cpp
struct Lobby {
  std::string id;
  std::string name;
  // Need to add:
  // VideoSettings video_settings;  // Store actual resolution!

  std::shared_ptr<immer::atom<immer::vector<...>>> connected_sessions;
  std::shared_ptr<immer::atom<virtual_display::wl_state_ptr>> wayland_display;
  std::shared_ptr<immer::atom<std::shared_ptr<audio::VSink>>> audio_sink;
};
```

**StreamSession** (events.hpp):
```cpp
struct StreamSession {
  moonlight::DisplayMode display_mode;  // Client resolution
  std::size_t session_id;
  std::string ip;
  // ... has wayland_display, audio_sink, mouse, keyboard, joypads
};
```

### GStreamer Pipeline Architecture
```
Lobby (Fixed Resolution)
  └─> Video Producer (interpipesink)
        └─> interpipesrc (per client)
              └─> Transcode (to client resolution)
                    └─> Encoder (H264/HEVC/AV1)
                          └─> RTP
                                └─> Client
```

## Wolf API Endpoints Available

- `GET /api/v1/lobbies` - List all lobbies
- `GET /api/v1/sessions` - List all stream sessions
- `GET /api/v1/apps` - List all apps
- `GET /api/v1/events` - SSE event stream
- `GET /api/v1/system/memory` - **NEW** (needs build fix)

## Files to Work On Next

1. `/home/luke/pm/wolf/src/moonlight-server/api/endpoints.cpp` - Fix memory endpoint
2. `/home/luke/pm/wolf/src/moonlight-server/events/events.hpp` - Add VideoSettings to Lobby
3. `/home/luke/pm/helix/api/pkg/server/admin_wolf.go` - Create proxy endpoints
4. `/home/luke/pm/helix/frontend/src/pages/Admin/AgentSandboxes.tsx` - Build dashboard

## Key Debugging Use Cases

1. **Memory Leaks**: Per-lobby memory + client count shows if lobbies aren't cleaning up
2. **Video Corruption**: Pipeline visualization shows if multiple producers feed same consumer
3. **Resolution Issues**: Shows lobby resolution vs client resolution vs scale factor
4. **Connection Leaks**: Client iteration reveals orphaned connections

## References

- Wolf source: `/home/luke/pm/wolf/`
- Helix Wolf executor: `/home/luke/pm/helix/api/pkg/external-agent/wolf_executor.go`
- Previous context: Conversation focused on Wolf streaming resolution architecture
