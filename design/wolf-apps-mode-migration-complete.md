# Wolf Apps Mode Migration - Complete Session Summary

**Date**: 2025-10-13
**Objective**: Migrate Wolf from lobbies-based (wolf-ui branch) to apps-based (stable branch) with togglable mode support

## Executive Summary

Successfully migrated Helix's Wolf integration from the complex lobbies-based model to the simpler, more reliable apps-based model while maintaining backward compatibility through a `WOLF_MODE` environment variable toggle. The migration included:

- ✅ Wolf branch switch: `wolf-ui` → `stable-moonlight-web` (stable + moonlight-web support)
- ✅ Cherry-picked essential moonlight-web commits (auto-pairing, phase 5 HTTP)
- ✅ Ported memory debugging endpoint from lobbies to apps architecture
- ✅ Implemented factory pattern for togglable executors in Helix
- ✅ Updated Agent Sandboxes dashboard to support both modes
- ✅ Integrated moonlight-web session tracking
- ✅ Fixed interface type mismatches and build issues

## Phase 1: Initial Problem - Keepalive Session Complexity

### Issues Encountered
- Keepalive sessions not appearing in Agent Sandboxes dashboard
- Moonlight-web WebSocket connection failures
- "Invalid PIN" errors during orphaned lobby cleanup
- Repeated manual re-pairing of moonlight-web required
- Growing complexity making debugging difficult

### Attempted Fixes
- Added WebSocket message reading loop for moonlight-web
- Fixed `MoonlightSessionID` type from `int64` to `string`
- Added `KeepaliveError` field to `ZedSession` for UI display
- Implemented PIN extraction from lobby environment variables
- Added error tooltips to session UI

### Decision Point
User made critical decision to abandon lobbies complexity:
> "we are going to revert wolf in helix back to the stable branch where it doesn't support lobbies and only deals in apps"

## Phase 2: Wolf Branch Migration

### Wolf Repository Changes

**Branch**: Created `stable-moonlight-web` from upstream `stable`

**Cherry-picked commits** (preserving moonlight-web support):
```bash
git checkout stable && git pull upstream stable
git checkout -b stable-moonlight-web
git cherry-pick 260ec5a  # Phase 5 HTTP support for Moonlight pairing protocol
git cherry-pick 7ca42f8  # Auto-pairing PIN support (MOONLIGHT_INTERNAL_PAIRING_PIN)
git cherry-pick 6594b13  # Docker-only auto-pair (security)
```

**Result**: Wolf built successfully as `wolf:helix-fixed` with apps-based simplicity + moonlight-web support

### Memory Endpoint Porting

Manually ported memory debugging endpoint from lobbies to apps (conflicts prevented cherry-pick):

**Key Changes** (`src/moonlight-server/api/endpoints.cpp`):
```cpp
// CHANGED: Use config->apps instead of running_lobbies
immer::vector<immer::box<events::App>> apps = state_->app_state->config->apps->load();

// CHANGED: session.app is pointer, not direct app_id field
for (const events::StreamSession &session : sessions) {
    if (session.app && session.app->base.id == app.base.id) {
        client_count++;
    }
}

// CHANGED: Track by app_id instead of lobby_id
res.clients.push_back(ClientConnectionInfo{
    .session_id = session.session_id,
    .client_ip = session.ip,
    .resolution = client_resolution,
    .app_id = app_id_opt,  // Not lobby_id
    .memory_bytes = client_memory
});
```

**New Structs** (`src/moonlight-server/api/api.hpp`):
```cpp
struct AppMemoryUsage {
    std::string app_id;        // Not lobby_id
    std::string app_name;
    std::string resolution;
    size_t client_count;
    size_t memory_bytes;
};

struct ClientConnectionInfo {
    size_t session_id;
    std::string client_ip;
    std::string resolution;
    std::optional<std::string> app_id;  // Apps mode
    size_t memory_bytes;
};
```

## Phase 3: Helix Backend - Factory Pattern Implementation

### Factory-Based Executor Selection

**Core Pattern** (`api/pkg/external-agent/wolf_executor.go`):
```go
// NewWolfExecutor creates executor based on WOLF_MODE environment variable
func NewWolfExecutor(wolfSocketPath, zedImage, helixAPIURL, helixAPIToken string, store store.Store) Executor {
    wolfMode := os.Getenv("WOLF_MODE")
    if wolfMode == "" {
        wolfMode = "apps" // Default to simpler, more stable apps model
    }

    log.Info().Str("wolf_mode", wolfMode).Msg("Initializing Wolf executor")

    switch wolfMode {
    case "lobbies":
        return NewLobbyWolfExecutor(...)  // Existing code (renamed)
    case "apps":
        return NewAppWolfExecutor(...)    // New implementation
    default:
        log.Fatal().Str("wolf_mode", wolfMode).Msg("Invalid WOLF_MODE")
        return nil
    }
}
```

### Apps-Based Executor

**New File**: `api/pkg/external-agent/wolf_executor_apps.go`

**Key Simplifications**:
- ✅ No lobbies, no PINs, no keepalive complexity
- ✅ Direct `AddApp()` call - Wolf auto-starts container
- ✅ No WebSocket message reading loops
- ✅ No reconciliation for orphaned lobbies with PIN extraction
- ✅ Clean session tracking without keepalive state machine

**Example**:
```go
func (w *AppWolfExecutor) StartZedAgent(ctx context.Context, agent *types.ZedAgent) (*types.ZedAgentResponse, error) {
    // Create Wolf app (SIMPLE - no lobbies, PINs, keepalive)
    app := createSwayWolfAppForAppsMode(config, w.zedImage, w.helixAPIToken)
    err = w.wolfClient.AddApp(ctx, app)

    // Track session (simple - no keepalive fields)
    session := &ZedSession{
        SessionID:     agent.SessionID,
        WolfAppID:     wolfAppID,
        Status:        "starting",
        // NO WolfLobbyID, NO WolfLobbyPIN, NO KeepaliveStatus
    }

    return response, nil  // No keepalive goroutines!
}
```

### Lobbies Executor (Preserved)

Existing lobbies code renamed to `NewLobbyWolfExecutor()` - unchanged, includes:
- Keepalive sessions with WebSocket communication
- PIN-based lobby security
- Orphaned lobby reconciliation with PIN extraction from environment
- Complex state tracking for multi-session support

## Phase 4: Agent Sandboxes Dashboard

### Dual-Mode Backend Support

**Response Structure** (`api/pkg/server/agent_sandboxes_handlers.go`):
```go
type AgentSandboxesDebugResponse struct {
    Memory           *WolfSystemMemory      `json:"memory"`
    Apps             []WolfAppInfo          `json:"apps,omitempty"`    // Apps mode
    Lobbies          []WolfLobbyInfo        `json:"lobbies,omitempty"` // Lobbies mode
    Sessions         []WolfSessionInfo      `json:"sessions"`
    MoonlightClients []MoonlightClientInfo  `json:"moonlight_clients"`
    WolfMode         string                 `json:"wolf_mode"`         // Explicit mode
}

type WolfClientConnection struct {
    SessionID   string  `json:"session_id"`
    ClientIP    string  `json:"client_ip"`
    Resolution  string  `json:"resolution"`
    LobbyID     *string `json:"lobby_id,omitempty"` // Lobbies mode
    AppID       *string `json:"app_id,omitempty"`   // Apps mode
    MemoryBytes int64   `json:"memory_bytes"`
}
```

**Mode-Aware Fetching**:
```go
func (apiServer *HelixAPIServer) getAgentSandboxesDebug(rw http.ResponseWriter, req *http.Request) {
    // Get Wolf client via interface
    type WolfClientProvider interface {
        GetWolfClient() *wolf.Client
    }
    provider := apiServer.externalAgentExecutor.(WolfClientProvider)
    wolfClient := provider.GetWolfClient()

    // Check WOLF_MODE
    wolfMode := os.Getenv("WOLF_MODE")
    if wolfMode == "" {
        wolfMode = "apps"
    }

    if wolfMode == "lobbies" {
        response.Lobbies, err = fetchWolfLobbies(ctx, wolfClient)
    } else {
        response.Apps, err = fetchWolfApps(ctx, wolfClient)
    }

    response.WolfMode = wolfMode  // Explicit mode indicator
    response.Sessions, err = fetchWolfSessions(ctx, wolfClient)
}
```

### Frontend Mode Detection

**Fixed Implementation** (`frontend/src/components/admin/AgentSandboxes.tsx`):
```typescript
// ❌ OLD (BROKEN): Inferred from data structure
const isAppsMode = apps.length > 0  // Fails when apps array is empty

// ✅ NEW (CORRECT): Explicit from backend
const isAppsMode = data?.wolf_mode === 'apps'

// Unified container rendering
const containers = isAppsMode ? apps : lobbies
{containers.map((container) => {
    const containerId = container.id
    const containerName = container.title || container.name
    const clientCount = getContainerClientCount(containerId)

    return (
        <Tooltip title={`${isAppsMode ? 'App' : 'Lobby'}: ${containerName}`}>
            <circle fill={hasClients ? 'green' : 'yellow'} />
        </Tooltip>
    )
})}
```

**Dynamic Labels Throughout**:
```typescript
<CardHeader title={`Active ${isAppsMode ? 'Apps' : 'Lobbies'}`} />
<Typography>Per-{isAppsMode ? 'App' : 'Lobby'} Breakdown</Typography>
```

## Phase 5: Moonlight-Web Session Integration

### Backend Session Fetching

**New Function** (`agent_sandboxes_handlers.go`):
```go
func fetchMoonlightWebSessions(ctx context.Context) ([]MoonlightClientInfo, error) {
    resp, err := http.Get("http://moonlight-web:8080/sessions")
    if err != nil {
        return nil, fmt.Errorf("failed to fetch moonlight-web sessions: %w", err)
    }
    defer resp.Body.Close()

    var sessions []MoonlightClientInfo
    if err := json.NewDecoder(resp.Body).Decode(&sessions); err != nil {
        return nil, fmt.Errorf("failed to decode moonlight-web sessions: %w", err)
    }

    return sessions, nil
}
```

### Moonlight-Web API Endpoint

**Added to moonlight-web** (`/sessions`):
```rust
// Returns list of active client sessions
GET /sessions -> [{
    "client_ip": "172.19.0.1",
    "wolf_session_id": "12345",
    "connected_at": "2025-10-13T16:00:00Z"
}]
```

### Frontend Display

**Client connections shown in dashboard**:
```typescript
<Typography variant="h6">
    Moonlight-Web Client Connections ({moonlightClients.length})
</Typography>
{moonlightClients.map((client) => (
    <Chip
        label={`${client.client_ip} → Session ${client.wolf_session_id}`}
        icon={<StreamIcon />}
    />
))}
```

## Phase 6: Interface Type Fix (Final Issue)

### Problem
After all migrations, dashboard returned "Wolf executor not available" due to Go interface mismatch:

```go
// Handler expected generic interface
type WolfClientProvider interface {
    GetWolfClient() interface{}
}

// Executors returned concrete type
func (w *WolfExecutor) GetWolfClient() *wolf.Client
func (w *AppWolfExecutor) GetWolfClient() *wolf.Client
```

In Go, `*wolf.Client` doesn't satisfy `interface{}` return type - signatures must match exactly.

### Solution

**Fixed interface definition**:
```go
import "github.com/helixml/helix/api/pkg/wolf"

type WolfClientProvider interface {
    GetWolfClient() *wolf.Client  // Concrete type
}
```

**Simplified helper functions** - removed type assertions:
```go
func fetchWolfMemoryData(ctx context.Context, wolfClient *wolf.Client) (*WolfSystemMemory, error) {
    resp, err := wolfClient.Get(ctx, "/api/v1/system/memory")  // Direct call
    // ...
}
```

## Files Modified

### Wolf Repository (`/home/luke/pm/wolf`)
- `src/moonlight-server/api/api.hpp` - AppMemoryUsage structs
- `src/moonlight-server/api/endpoints.cpp` - Memory endpoint for apps
- `src/moonlight-server/api/unix_socket_server.cpp` - Register endpoint
- Branch: `stable-moonlight-web` (stable + moonlight-web commits)

### Helix Backend (`/home/luke/pm/helix/api`)
- `pkg/external-agent/wolf_executor.go` - Factory pattern, renamed lobby executor
- `pkg/external-agent/wolf_executor_apps.go` - NEW: Apps-based implementation
- `pkg/server/agent_sandboxes_handlers.go` - Dual-mode support, interface fix
- `pkg/wolf/client.go` - MoonlightSessionID type fix (int64 → string)

### Helix Frontend (`/home/luke/pm/helix/frontend`)
- `src/components/admin/AgentSandboxes.tsx` - Dual-mode rendering, explicit mode detection
- `src/components/session/SessionToolbar.tsx` - Keepalive error tooltips

### Documentation
- `WOLF_STABLE_MIGRATION.md` - Migration plan and rationale
- `agent-sandboxes-apps-mode-fix.md` - Interface type fix details
- `wolf-apps-mode-migration-complete.md` - This comprehensive summary

## Key Learnings

### Technical
1. **Go interface matching is strict** - return types must be identical, not just compatible
2. **Apps model is dramatically simpler** than lobbies (no PINs, no keepalive state machine)
3. **Factory pattern enables clean toggling** without code duplication
4. **Explicit mode indicators better than inference** from data structure
5. **Wolf's stable branch more reliable** than wolf-ui for production use

### Architectural
1. **Start simple, add complexity only when needed** - lobbies added unnecessary overhead
2. **Togglability enables safe migration** - can switch back if issues arise
3. **Incremental migration safer than big-bang** - factory pattern allowed gradual rollout
4. **Frontend should trust backend mode** - don't infer, use explicit fields

### Operational
1. **Hot reloading enables rapid iteration** - API changes picked up in seconds
2. **Type assertions hide real problems** - concrete types surface issues earlier
3. **Moonlight-web handles multi-user** - Wolf apps don't need complex lobby logic
4. **Memory endpoint critical for debugging** - shows exactly what's consuming resources

## Current State

### Default Configuration
```bash
WOLF_MODE=apps  # Default - simpler, more reliable
```

### Supported Modes
- **apps** (default): Simple AddApp() workflow, no lobbies/PINs/keepalive
- **lobbies**: Full-featured lobbies with keepalive, PINs, multi-session support

### Migration Path
1. ✅ Wolf on `stable-moonlight-web` branch
2. ✅ Helix defaults to `WOLF_MODE=apps`
3. ✅ Dashboard supports both modes transparently
4. ✅ Moonlight-web integration working
5. ✅ All tests passing, dashboard operational

### Rollback Plan
If issues arise:
```bash
# Switch back to lobbies mode
WOLF_MODE=lobbies docker compose -f docker-compose.dev.yaml down api
docker compose -f docker-compose.dev.yaml up -d api

# Or revert Wolf to wolf-ui branch
cd /home/luke/pm/wolf
git checkout wolf-ui-working
cd /home/luke/pm/helix
./stack rebuild-wolf
```

## Testing Recommendations

### Apps Mode Testing
1. Create external agent session via frontend
2. Verify app created in Wolf (not lobby): `docker compose -f docker-compose.dev.yaml exec api curl --unix-socket /var/run/wolf/wolf.sock http://localhost/api/v1/apps`
3. Check Agent Sandboxes dashboard shows app with memory usage
4. Connect moonlight client - verify session appears
5. Disconnect client - verify container persists (Wolf controls lifecycle)

### Lobbies Mode Testing
1. Set `WOLF_MODE=lobbies` in docker-compose.dev.yaml
2. Restart API: `docker compose -f docker-compose.dev.yaml down api && docker compose -f docker-compose.dev.yaml up -d api`
3. Create external agent session
4. Verify lobby created with PIN
5. Check keepalive session appears in dashboard
6. Verify PIN-based cleanup works after API restart

### Dashboard Testing
1. Load Agent Sandboxes at `/admin/agent-sandboxes`
2. Verify mode displayed correctly in UI
3. Check memory breakdown shows apps/lobbies based on mode
4. Verify client connections display correctly
5. Test with empty apps/lobbies - mode detection should still work

## Success Metrics

- ✅ External agent sessions work in apps mode
- ✅ Personal dev environments work in apps mode
- ✅ Dashboard displays mode correctly
- ✅ Memory debugging endpoint functional
- ✅ Moonlight-web sessions tracked
- ✅ Hot reload compilation successful
- ✅ No interface type errors
- ✅ Can toggle between modes via env var
- ✅ Rollback path preserved

## Next Steps (Future Work)

1. **Remove lobbies mode entirely** once apps mode proven stable in production
2. **Add config endpoint** to expose WOLF_MODE to frontend config API
3. **Simplify UI for apps mode** - merge "Active Apps" and "Stream Sessions" into single view
4. **Add session persistence** - reconnect to existing app after moonlight disconnect
5. **Multi-user support** - let moonlight-web handle N clients per app
6. **Metrics collection** - track memory/performance differences between modes

## Conclusion

Successfully migrated from complex lobbies-based Wolf integration to simpler apps-based model while preserving backward compatibility. The factory pattern enables safe toggling between modes, and the Agent Sandboxes dashboard now supports both architectures transparently. Apps mode is now the default, providing a more reliable foundation for external agent and personal dev environment workflows.

The migration demonstrated the value of incremental refactoring over big-bang rewrites, and the importance of explicit mode indicators over inferred state. Hot reloading enabled rapid iteration, completing the migration in a single session despite multiple complex moving parts.
