# Agent Sandboxes Dashboard Redesign

## Problem

The `AgentSandboxes.tsx` admin dashboard was built for the old Wolf/Moonlight/WebRTC architecture:
- Displays apps/lobbies with interpipe connections
- Shows Wolf sessions and Moonlight WebRTC clients
- Uses `data.wolf_mode`, `data.moonlight_clients`, etc.

This infrastructure no longer exists. We now have:
- **Hydra**: Docker instance manager running inside sandbox
- **Dev Containers**: helix-sway/helix-ubuntu with direct WebSocket streaming
- **session_registry.go**: Tracks connected WebSocket clients per session

## New Architecture

```
Helix API
    ↓ (RevDial)
Sandbox Container
    └── Hydra (Docker instance manager)
            └── dockerd instances (for dev container isolation)
                    └── Dev Containers (helix-sway, helix-ubuntu)
                            └── Go streaming server (ws_stream.go)
                                    └── WebSocket clients
```

## Data Available from Hydra

### GET /api/v1/dev-containers
```go
type ListDevContainersResponse struct {
    Containers []DevContainerResponse `json:"containers"`
}

type DevContainerResponse struct {
    SessionID     string             `json:"session_id"`
    ContainerID   string             `json:"container_id"`
    ContainerName string             `json:"container_name"`
    Status        DevContainerStatus `json:"status"`  // starting, running, stopped, error
    IPAddress     string             `json:"ip_address,omitempty"`
    ContainerType DevContainerType   `json:"container_type"`  // sway, ubuntu, headless
    DesktopVersion string            `json:"desktop_version,omitempty"`
    GPUVendor      string            `json:"gpu_vendor,omitempty"`
    RenderNode     string            `json:"render_node,omitempty"`
}
```

### GET /api/v1/system-stats
```go
type SystemStatsResponse struct {
    GPUs             []GPUInfo `json:"gpus"`
    ActiveContainers int       `json:"active_containers"`
    ActiveSessions   int       `json:"active_sessions"`
}

type GPUInfo struct {
    Index       int    `json:"index"`
    Name        string `json:"name"`
    Vendor      string `json:"vendor"`  // nvidia, amd, intel
    MemoryTotal int64  `json:"memory_total_bytes"`
    MemoryUsed  int64  `json:"memory_used_bytes"`
    MemoryFree  int64  `json:"memory_free_bytes"`
    Utilization int    `json:"utilization_percent"`
    Temperature int    `json:"temperature_celsius"`
}
```

### WebSocket Clients (from session_registry.go)
```go
type ConnectedClient struct {
    ID        uint32    // Unique client ID within session
    UserID    string    // Helix user ID
    UserName  string    // Display name
    Color     string    // Assigned color
    LastSeen  time.Time
}
```

**New endpoint needed in desktop server**: `GET /clients`
Returns list of connected users for the session. This enables showing multi-player info in the dashboard.

## Implementation Plan

### Phase 1: Backend API Extension

**File: `api/pkg/server/agent_sandboxes_handlers.go`**

Update `AgentSandboxesDebugResponse` to include:
```go
type AgentSandboxesDebugResponse struct {
    Message      string                    `json:"message"`
    Sandboxes    []SandboxInfo             `json:"sandboxes"`
    GPUs         []hydra.GPUInfo           `json:"gpus,omitempty"`
    DevContainers []hydra.DevContainerResponse `json:"dev_containers,omitempty"`
}

type SandboxInfo struct {
    ID           string `json:"id"`
    Status       string `json:"status"`
    RunnerID     string `json:"runner_id"`  // For RevDial connection
}
```

The endpoint will:
1. List sandboxes from store
2. For each sandbox, create RevDialClient and query Hydra for:
   - `/api/v1/dev-containers`
   - `/api/v1/system-stats`
3. Aggregate and return data

**Note**: Need access to connman in the handler. Check how HydraExecutor gets it.

### Phase 2: Frontend Dashboard Redesign

**File: `frontend/src/components/admin/AgentSandboxes.tsx`**

Replace the 3-tier Wolf visualization with a simpler 2-tier view:

```
┌─────────────────────────────────────────────────────────────┐
│ GPU Stats (if available)                                     │
│ ┌─────────┐ ┌─────────┐ ┌─────────┐ ┌─────────┐             │
│ │ NVENC   │ │ Encoder │ │ GPU %   │ │ Temp    │             │
│ │ Sessions│ │ FPS     │ │         │ │         │             │
│ └─────────┘ └─────────┘ └─────────┘ └─────────┘             │
└─────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────┐
│ Dev Containers                                               │
│                                                              │
│  ┌──────────────────────────────────────────────────────┐   │
│  │ ses_abc123 | helix-ubuntu | ● Running | GPU: nvidia  │   │
│  │ Connected users: Luke (●), Alice (●), Bob (●)        │   │
│  └──────────────────────────────────────────────────────┘   │
│                                                              │
│  ┌──────────────────────────────────────────────────────┐   │
│  │ ses_def456 | helix-sway   | ● Running | GPU: nvidia  │   │
│  │ Connected users: Charlie (●)                          │   │
│  └──────────────────────────────────────────────────────┘   │
│                                                              │
│  ┌──────────────────────────────────────────────────────┐   │
│  │ ses_ghi789 | helix-ubuntu | ○ Starting | GPU: -      │   │
│  │ Connected users: (none)                               │   │
│  └──────────────────────────────────────────────────────┘   │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

### Phase 3: API Types Update

Regenerate API client after backend changes:
```bash
./stack update_openapi
```

## Files to Modify

### Backend - Desktop Server (runs in container)
1. `api/pkg/desktop/desktop.go` - Add `/clients` endpoint that returns connected users

### Backend - API Server
1. `api/pkg/server/agent_sandboxes_handlers.go` - Extend response, add Hydra + desktop queries
2. `api/pkg/server/swagger.yaml` - Update API spec (auto-generated from go docs)

### Frontend
1. `frontend/src/components/admin/AgentSandboxes.tsx` - Complete rewrite
2. `frontend/src/api/api.ts` - Regenerated

## Implementation Notes

1. **RevDial Access**: `HelixAPIServer` has `connman *connman.ConnectionManager` field. Use `apiServer.connman.Dial(ctx, runnerID)` to connect to sandbox, then create `hydra.NewRevDialClient()`.

2. **Runner ID**: Sandboxes register with runner ID format like `sandbox-{id}`. Check `external_agent_handlers.go` for patterns.

3. **Error Handling**: Hydra may be unreachable if sandbox is starting/stopped. Return partial data with error messages.

4. **WebSocket Clients**: The session_registry is in the desktop container, not accessible from API. For now, show container count, not individual clients.

## Verification

1. Start a sandbox with dev containers
2. Navigate to Admin > Agent Sandboxes
3. Verify:
   - GPU stats appear (if GPU available)
   - Dev containers listed with correct status
   - Container type (sway/ubuntu) displayed
   - Status updates on refresh
