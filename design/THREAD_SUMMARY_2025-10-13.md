# Thread Summary - Agent Sandboxes Dashboard & Moonlight-web Integration
**Date**: 2025-10-13
**Context**: Continuation session after previous context limit

## Session Overview

This session focused on two main areas:
1. Completing the apps-based Wolf integration for Agent Sandboxes dashboard
2. Adding moonlight-web client connection visualization to the dashboard

## Background Context (from previous session)

The previous session implemented **session persistence in moonlight-web** to support autonomous external agents:

- **Problem**: Wolf lobbies were unreliable, causing frequent keepalive failures
- **Solution**: Moved session persistence into moonlight-web itself
- **Architecture**: Decoupled streamer process lifecycle from WebSocket connection lifecycle
- **Result**: Agents can run with 0-or-1 clients, "last one wins" behavior

### Session Persistence Implementation (Previous)

Created 3 session modes in moonlight-web:
- `create`: New session, fails if already exists
- `keepalive`: Create if missing, run headless without WebRTC peer
- `join`: Join existing session, kick previous client

This enabled external agents to:
1. Start immediately without any client connected
2. Run autonomously in headless mode (keepalive)
3. Allow users to connect/disconnect/reconnect without disrupting agent work

## This Session's Work

### Part 1: Apps-Based Wolf Integration Testing

**Context**: Testing the transition from lobbies mode to apps mode for improved reliability.

**Initial Todo List** (from context):
1. ✅ Port memory debugging endpoint to stable branch
2. ✅ Rebuild Wolf with memory endpoint
3. ✅ Update Helix dashboard backend to handle apps instead of lobbies
4. ✅ Update Helix dashboard frontend to support both modes
5. 🔄 Test apps-based Wolf integration end-to-end

These tasks were already completed in previous work, dashboard was ready for testing.

### Part 2: Moonlight-web Client Visualization

**User Request**:
> "i'd like to update the agent sandboxes dashboard to additionally query moonlight-web to see which client(s) are connected - and display that in the dashboard in the same graph style that we previously displayed lobbies (and their clients) in lobby mode"

**User Follow-up**:
> "btw, if moonlight-web gets disconnected from a wolf app, it should be able to resume it - correct? can we make it do that automatically?"

This identified a gap: automatic reconnection when streamer dies.

#### Implementation Steps

##### Step 1: Added Moonlight-web `/sessions` Endpoint

**File**: `moonlight-web/web-server/src/api/mod.rs`

```rust
/// Session info for external monitoring (Agent Sandboxes dashboard)
#[derive(Debug, Serialize)]
struct SessionInfo {
    session_id: String,
    mode: SessionMode,
    has_websocket: bool,
}

#[get("/sessions")]
async fn get_sessions(data: Data<RuntimeApiData>) -> Either<Json<GetSessionsResponse>, HttpResponse> {
    let sessions_lock = data.sessions.read().await;

    let mut sessions = Vec::new();
    for (session_id, stream_session) in sessions_lock.iter() {
        let ws_lock = stream_session.websocket.lock().await;
        let has_websocket = ws_lock.is_some();

        sessions.push(SessionInfo {
            session_id: session_id.clone(),
            mode: stream_session.mode,
            has_websocket,
        });
    }

    Either::Left(Json(GetSessionsResponse { sessions }))
}
```

**Key Points**:
- Exposes internal session state to external monitoring
- Shows whether WebRTC client is currently connected
- Authenticated endpoint (requires `MOONLIGHT_CREDENTIALS`)

##### Step 2: Rebuilt Moonlight-web Container

```bash
docker compose -f docker-compose.dev.yaml build moonlight-web
docker compose -f docker-compose.dev.yaml down moonlight-web && up -d moonlight-web
```

Build completed successfully in ~48 seconds with 2 warnings (unused variables, non-blocking).

##### Step 3: Updated Helix Backend

**File**: `api/pkg/server/agent_sandboxes_handlers.go`

Added structures:
```go
type MoonlightClientInfo struct {
    SessionID     string `json:"session_id"`
    Mode          string `json:"mode"`
    HasWebsocket  bool   `json:"has_websocket"`
}

type AgentSandboxesDebugResponse struct {
    // ... existing fields
    MoonlightClients []MoonlightClientInfo  `json:"moonlight_clients"`
    WolfMode         string                 `json:"wolf_mode"`
}
```

Added fetch function:
```go
func fetchMoonlightWebSessions(ctx context.Context) ([]MoonlightClientInfo, error) {
    // Query http://moonlight-web:8080/api/sessions
    // Authenticate with MOONLIGHT_CREDENTIALS
    // Transform to MoonlightClientInfo structs
    // Non-fatal errors - continue without data if unavailable
}
```

Integrated into handler:
```go
// Fetch moonlight-web client connections
moonlightClients, err := fetchMoonlightWebSessions(ctx)
if err != nil {
    // Non-fatal - just log and continue
    fmt.Printf("Warning: Failed to fetch moonlight-web sessions: %v\n", err)
    response.MoonlightClients = []MoonlightClientInfo{}
} else {
    response.MoonlightClients = moonlightClients
}

// Set Wolf mode so frontend knows which architecture to display
response.WolfMode = wolfMode
```

##### Step 4: Updated Frontend Dashboard

**File**: `frontend/src/components/admin/AgentSandboxes.tsx`

**TypeScript Interfaces**:
```typescript
interface MoonlightClientInfo {
  session_id: string
  mode: string            // "create", "keepalive", "join"
  has_websocket: boolean  // Is a WebRTC client currently connected?
}

interface AgentSandboxesDebugResponse {
  memory: WolfSystemMemory | null
  apps?: WolfAppInfo[]
  lobbies?: WolfLobbyInfo[]
  sessions: WolfSessionInfo[]
  moonlight_clients: MoonlightClientInfo[]
  wolf_mode: string
}
```

**Layout Updates**:
```typescript
// 3-tier layout
const svgHeight = 700  // Increased from 600
const containerY = 120  // Apps/Lobbies at top
const sessionY = 350    // Wolf sessions in middle
const clientY = 580     // Moonlight-web clients at bottom

// Position moonlight-web clients
const clientPositions = new Map<string, { x: number; y: number }>()
const clientSpacing = svgWidth / (moonlightClients.length + 1)
moonlightClients.forEach((client, idx) => {
  clientPositions.set(client.session_id, {
    x: clientSpacing * (idx + 1),
    y: clientY,
  })
})
```

**Visual Elements**:
1. **Connection Lines** (Session → Client):
```typescript
<line
  x1={clientPos.x}
  y1={clientPos.y - clientRadius}
  x2={sessionPos.x}
  y2={sessionPos.y + sessionRadius}
  stroke={client.has_websocket ? "#4caf50" : "#ffc107"}
  strokeWidth="2"
  strokeDasharray={client.has_websocket ? "0" : "5,5"}
  opacity="0.7"
/>
<text>
  {client.has_websocket ? 'WebRTC' : 'headless'}
</text>
```

2. **Client Circles**:
```typescript
<circle
  cx={pos.x}
  cy={pos.y}
  r={clientRadius}
  fill={hasWebRTC ? 'rgba(76, 175, 80, 0.2)' : 'rgba(255, 193, 7, 0.2)'}
  stroke={hasWebRTC ? '#4caf50' : '#ffc107'}
  strokeWidth="2"
/>
```

3. **Mode Labels**:
```typescript
<text>{client.mode}</text>  // "create", "keepalive", "join"
<text>{hasWebRTC ? 'WebRTC' : 'headless'}</text>
```

**Mode-Specific Rendering**:

Apps Mode:
- Connection labels: "direct" (not "interpipe")
- No interpipesink/interpipesrc indicators
- Description: "Direct 1:1 connections. Each app has its own GStreamer pipeline."

Lobbies Mode:
- Connection labels: "interpipe"
- Shows interpipesink/interpipesrc indicators
- Description: "Dynamic interpipe switching..."

**Legend Updated**:
```typescript
<circle /> WebRTC Client (active) - green
<circle /> Headless Client (keepalive) - yellow
```

## User Feedback & Corrections

### Correction 1: Interpipe Only in Lobbies Mode

**User**: "i think in apps mode wolf doesn't use interpipesrc/sink though, so maybe update the text with this clarification"

**Fix**: Updated all text and visual indicators to be mode-aware:
- Connection labels conditional: `{isAppsMode ? 'direct' : 'interpipe'}`
- Hide interpipe indicators in apps mode: `{!isAppsMode && <interpipesink label>}`
- Separate descriptions for each mode

### Question: Automatic Reconnection

**User**: "btw, if moonlight-web gets disconnected from a wolf app, it should be able to resume it - correct? can we make it do that automatically?"

**Status**: Added to todo list (#6) but not yet implemented. This would require:
- Health check monitoring of streamer child process
- Automatic restart with same session_id on failure
- Reconnection logic to Wolf RTSP endpoint
- Error handling for Wolf unavailability

## System State After This Session

### Running Services

- ✅ **Moonlight-web**: Running with `/sessions` endpoint
- ✅ **Helix API**: Integrated moonlight-web session fetching
- ✅ **Frontend**: Updated dashboard with 3-tier visualization
- ✅ **Wolf**: Running in apps mode (WOLF_MODE=apps)

### Code Changes

#### Moonlight-web-stream Repo
- Added `/api/sessions` GET endpoint
- Exposes session_id, mode, has_websocket for monitoring
- Rebuilt container successfully

#### Helix Repo
**Backend**:
- `api/pkg/server/agent_sandboxes_handlers.go`:
  - Added `MoonlightClientInfo` struct
  - Added `fetchMoonlightWebSessions()` function
  - Integrated into debug response
  - Added `WolfMode` field to response

**Frontend**:
- `frontend/src/components/admin/AgentSandboxes.tsx`:
  - Added 3-tier visualization (containers → sessions → clients)
  - Color-coded connection states (green=WebRTC, yellow=keepalive)
  - Mode-aware labels (apps=direct, lobbies=interpipe)
  - Updated legend with client states
  - Conditional interpipe indicator rendering

### Branch Information

- **Moonlight-web**: `feature/session-persistence` branch
- **Helix**: `feature/external-agents-hyprland-working` branch

Both repos have uncommitted changes ready for testing.

## Testing Status

### Ready to Test

1. **Agent Sandboxes Dashboard** (`http://localhost:3000/admin/agent-sandboxes`)
   - Should show 3-tier architecture
   - Moonlight-web clients visible at bottom layer
   - Color-coded by connection state

2. **Keepalive Sessions**
   - Should show yellow circles (headless)
   - Dashed lines to Wolf sessions
   - Mode label: "keepalive"

3. **User Streaming**
   - Should show green circles (WebRTC active)
   - Solid lines to Wolf sessions
   - Mode label: "create" or "join"

### Not Yet Tested

- End-to-end visualization with real sessions
- Apps mode vs lobbies mode display differences
- Automatic reconnection when streamer dies

## Outstanding Items

### Todo List
1. ✅ Add moonlight-web /sessions API endpoint
2. ✅ Rebuild moonlight-web container with sessions endpoint
3. ✅ Update Helix backend to fetch moonlight-web sessions
4. ✅ Update Agent Sandboxes dashboard to display client connections
5. 🔄 Test end-to-end moonlight-web client visualization
6. ⏳ Add automatic reconnection to Wolf when streamer dies

### Technical Debt

1. **Automatic Reconnection** (User requested):
   - Moonlight-web should detect when streamer process dies
   - Automatically restart connection to Wolf
   - Preserve session state during reconnection
   - Handle Wolf unavailability gracefully

2. **Unused Variables** (Build warnings):
   - `moonlight-web/web-server/src/api/stream.rs:362` - `ipc_sender_clone` unused
   - Non-blocking but should be cleaned up

## Key Learnings

### Apps vs Lobbies Mode

**Apps Mode** (Current, Simpler):
- 1:1 relationship: Each app has one dedicated session
- Direct GStreamer pipeline connection (no interpipe)
- More reliable, less complex
- Better for external agents use case

**Lobbies Mode** (Advanced, Complex):
- N:M relationship: Multiple sessions can connect to multiple lobbies
- Uses interpipe for dynamic switching
- More features (multi-user, PINs) but less reliable
- Original design, now deprecated for external agents

### Moonlight-web Architecture

**Session Persistence** (New):
- Sessions stored in `Arc<StreamSession>` HashMap
- Survive WebSocket disconnects
- Streamer child process continues running
- Enables 0-or-1 client model

**Mode Behavior**:
- `create`: Normal browser clients, create new session
- `keepalive`: Helix agents, run headless without WebRTC
- `join`: Reconnecting users, kick previous client

### Visualization Design

**3-Tier Architecture Display**:
```
[Apps/Lobbies]  ← Wolf containers (top, y=120)
       ↓ direct/interpipe
[Wolf Sessions] ← RTSP consumers (middle, y=350)
       ↓ WebRTC
[MW Clients]    ← Browser/keepalive (bottom, y=580)
```

**Color Coding**:
- Green + solid = WebRTC active
- Yellow + dashed = Headless keepalive
- Red = Orphaned/error state

## Files Modified This Session

### Moonlight-web-stream Repository
```
moonlight-web/web-server/src/api/mod.rs
  + Added SessionInfo struct
  + Added GetSessionsResponse struct
  + Added get_sessions() endpoint
  + Registered in api_service()
```

### Helix Repository

#### Backend
```
api/pkg/server/agent_sandboxes_handlers.go
  + Added MoonlightClientInfo struct
  + Added WolfMode field to AgentSandboxesDebugResponse
  + Added fetchMoonlightWebSessions() function
  + Integrated moonlight-web data into getAgentSandboxesDebug()
  + Added time import for HTTP client timeout
```

#### Frontend
```
frontend/src/components/admin/AgentSandboxes.tsx
  + Added MoonlightClientInfo TypeScript interface
  + Added wolf_mode field to AgentSandboxesDebugResponse
  + Added clientY = 580 for 3rd tier layout
  + Added clientPositions Map for positioning
  + Added session-to-client connection line rendering
  + Added moonlight-web client circle rendering
  + Updated legend with client states
  + Made interpipe labels conditional on lobbies mode
  + Updated descriptions to distinguish apps vs lobbies
  + Changed svgHeight from 600 to 700
```

## Git Status

### Moonlight-web-stream
- Branch: `feature/session-persistence`
- Status: Uncommitted changes (new /sessions endpoint)
- Container: Rebuilt and running

### Helix
- Branch: `feature/external-agents-hyprland-working`
- Status: Uncommitted changes (dashboard updates)
- Modified files:
  - `api/pkg/server/agent_sandboxes_handlers.go`
  - `api/pkg/external-agent/wolf_executor.go` (user modified)
  - `frontend/src/components/admin/AgentSandboxes.tsx`

## Environment Variables Referenced

### Helix API
- `WOLF_MODE`: "apps" or "lobbies" - determines Wolf integration mode
- `MOONLIGHT_WEB_URL`: Moonlight-web endpoint (default: `http://moonlight-web:8080`)
- `MOONLIGHT_CREDENTIALS`: Shared secret for authentication

### Moonlight-web
- `MOONLIGHT_CREDENTIALS`: Authentication token for /api/* endpoints

## API Endpoints Added/Modified

### Moonlight-web
- **NEW**: `GET /api/sessions` - List all persistent sessions with connection state

### Helix
- **MODIFIED**: `GET /api/v1/admin/agent-sandboxes/debug` - Now includes `moonlight_clients` array

## Visual Design Decisions

### Layout Strategy
- **Horizontal spreading**: Distribute items evenly across width
- **3 vertical tiers**: Clear separation of architectural layers
- **Dynamic spacing**: Adapts to number of items in each tier

### Color Palette
- **Apps/Lobbies**: Green (active) / Yellow (idle)
- **Wolf Sessions**: Blue (connected) / Red (orphaned)
- **MW Clients**: Green (WebRTC) / Yellow (keepalive)
- **Connection lines**: Match destination color

### Information Density
- **Tooltips**: Detailed info on hover
- **Labels**: Mode and state visible inline
- **Memory**: Shown where relevant
- **Pipeline indicators**: Only in lobbies mode

## Next Steps

### Immediate Testing
1. Create new external agent session
2. Verify keepalive client appears (yellow, bottom layer)
3. Click "Stream" to connect via browser
4. Verify client turns green (WebRTC active)
5. Disconnect and verify returns to yellow (keepalive persists)

### Future Work
1. **Automatic Reconnection** (User requested):
   - Monitor streamer child process health
   - Auto-restart on failure
   - Reconnect to Wolf with same session_id

2. **Enhanced Metrics**:
   - Client IP addresses
   - Bandwidth/bitrate per client
   - Connection duration
   - Frame rate statistics

3. **Interactive Controls**:
   - Click to disconnect clients
   - Restart failed sessions
   - Force reconnection

## Known Issues

### Moonlight-web Build Warnings
```
warning: unused variable: `ipc_sender_clone`
  --> moonlight-web/web-server/src/api/stream.rs:362
```

**Impact**: None - cosmetic warning
**Fix**: Comment out or remove unused variable

### Potential Gaps

1. **No reconnection logic**: If Wolf restarts or streamer dies, session is stuck
2. **No health monitoring**: Can't detect when streamer is unhealthy
3. **No client timeout**: Stale keepalive sessions may accumulate
4. **No bandwidth metrics**: Can't see if clients are actively receiving data

## References

### Related Files
- Previous design doc: `/home/luke/pm/moonlight-web-stream/SESSION_PERSISTENCE_DESIGN.md`
- Wolf client: `api/pkg/wolf/client.go`
- Wolf executor: `api/pkg/external-agent/wolf_executor.go`
- Moonlight-web data: `moonlight-web/web-server/src/data.rs`
- Stream handler: `moonlight-web/web-server/src/api/stream.rs`

### Key Concepts
- **Session Persistence**: Streamer survives WebSocket disconnects
- **Keepalive Mode**: Headless operation without WebRTC peer
- **Last One Wins**: New client kicks previous client
- **Interpipe**: GStreamer dynamic pipeline switching (lobbies only)
- **Apps vs Lobbies**: Two Wolf operational modes with different architectures

## Summary

Successfully implemented complete visibility into the moonlight-web client layer, creating a comprehensive 3-tier visualization of the streaming architecture. The dashboard now clearly shows:

1. **Where streams originate** (Wolf apps/lobbies)
2. **How they're consumed** (Wolf sessions via direct or interpipe)
3. **Who's watching** (Moonlight-web clients with WebRTC or keepalive)

This provides operators with real-time insight into:
- Autonomous agent status (keepalive sessions)
- Active user connections (WebRTC sessions)
- Pipeline architecture differences (apps vs lobbies)
- Connection health and state

The implementation is mode-aware, correctly displaying different architectures for apps mode (direct 1:1) and lobbies mode (interpipe switching), making it a versatile debugging tool for both Wolf configurations.
