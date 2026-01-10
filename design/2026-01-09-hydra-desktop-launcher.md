# Hydra Dev Container Launcher

**Date:** 2026-01-09
**Status:** Design
**Author:** Claude (with Luke)
**Depends on:** 2026-01-09-direct-video-streaming.md

## Overview

This design proposes extending Hydra to launch and manage dev containers (Zed+agent environments), replacing Wolf's container lifecycle management. This is step 2 of eliminating Wolf entirely (step 1 being Wolf-free video streaming).

**Terminology:** We call these "dev containers" rather than "desktop containers" because:
1. Future support for headless containers (no GUI, just agent)
2. Dev container better describes the purpose (coding environment)
3. Aligns with the industry term "dev container" (VS Code, etc.)

## Current Architecture

```
Helix API (wolf_executor.go)
    ↓ RevDial (CreateLobby request with container config)
Wolf (C++ in sandbox)
    ↓ Docker API
Desktop Container (helix-sway/helix-ubuntu)
```

**What Wolf does for container lifecycle:**
1. Receives lobby creation request with `MinimalWolfRunner` config
2. Parses Docker image, env vars, mounts, devices from config
3. Creates container via Docker API with extensive config:
   - GPU device passthrough (NVIDIA/AMD/Intel)
   - Device cgroup rules for hidraw/input
   - PipeWire socket sharing (for GNOME video capture)
   - Per-lobby Unix socket mounting
   - Fake udev for device hot-plugging
4. Monitors container health, handles stop/cleanup
5. Injects virtual input devices via fake-udev

**What Hydra already does:**
1. Manages isolated dockerd instances per session
2. Creates/destroys Docker instances via API
3. Bridges desktop to Hydra network (veth injection)
4. Runs DNS proxy for container name resolution
5. Connected to Helix API via RevDial

## Proposed Architecture

```
Helix API (hydra_executor.go - NEW)
    ↓ RevDial (CreateDevContainer request)
Hydra (Go in sandbox - EXTENDED)
    ↓ Docker API (Wolf's dockerd or isolated dockerd)
Desktop Container (helix-sway/helix-ubuntu)
```

**Key insight:** Hydra already runs in the sandbox, connected via RevDial, with Docker client ready. We just need to add the container launching logic.

## Implementation Plan

### Phase 1: Extend Hydra API

Add new endpoints to `api/pkg/hydra/server.go`:

```go
// POST /api/v1/dev-containers
// Creates and starts a dev container (Zed+agent environment)
api.HandleFunc("/dev-containers", s.handleCreateDevContainer).Methods("POST")

// DELETE /api/v1/dev-containers/{session_id}
// Stops and removes a dev container
api.HandleFunc("/dev-containers/{session_id}", s.handleDeleteDevContainer).Methods("DELETE")

// GET /api/v1/dev-containers/{session_id}
// Gets dev container status
api.HandleFunc("/dev-containers/{session_id}", s.handleGetDevContainer).Methods("GET")

// GET /api/v1/dev-containers
// Lists all dev containers
api.HandleFunc("/dev-containers", s.handleListDevContainers).Methods("GET")
```

### Phase 2: Dev Container Types

Add to `api/pkg/hydra/types.go`:

```go
// CreateDevContainerRequest creates a dev container (Zed+agent environment) for a session
type CreateDevContainerRequest struct {
    SessionID string `json:"session_id"`

    // Container configuration
    Image         string            `json:"image"`          // e.g., "helix-sway:latest"
    ContainerName string            `json:"container_name"` // e.g., "sway-external-ses_xxx"
    Hostname      string            `json:"hostname"`
    Env           []string          `json:"env"`            // KEY=value format
    Mounts        []MountConfig     `json:"mounts"`

    // Display settings (optional - headless containers omit these)
    DisplayWidth  int `json:"display_width,omitempty"`
    DisplayHeight int `json:"display_height,omitempty"`
    DisplayFPS    int `json:"display_fps,omitempty"`

    // Dev container type
    // - "sway": Sway compositor with Zed (current default)
    // - "ubuntu": GNOME with Zed
    // - "headless": No GUI, just agent (future)
    ContainerType string `json:"container_type"` // "sway", "ubuntu", "headless", etc.

    // GPU settings
    GPUVendor string `json:"gpu_vendor"` // "nvidia", "amd", "intel", ""

    // Docker socket to use (from Hydra isolation or default)
    DockerSocket string `json:"docker_socket,omitempty"`

    // User ID for SSH key mounting
    UserID string `json:"user_id,omitempty"`
}

type MountConfig struct {
    Source      string `json:"source"`
    Destination string `json:"destination"`
    ReadOnly    bool   `json:"readonly,omitempty"`
}

type DevContainerResponse struct {
    SessionID     string `json:"session_id"`
    ContainerID   string `json:"container_id"`
    ContainerName string `json:"container_name"`
    Status        string `json:"status"` // "starting", "running", "stopped", "error"
    Error         string `json:"error,omitempty"`

    // Network info for RevDial/screenshot-server connections
    IPAddress string `json:"ip_address,omitempty"`
}
```

### Phase 3: Container Creation Logic

Add `api/pkg/hydra/devcontainer.go`:

```go
package hydra

import (
    "context"
    "fmt"
    "os"
    "path/filepath"

    "github.com/docker/docker/api/types/container"
    "github.com/docker/docker/api/types/mount"
    "github.com/docker/docker/api/types/network"
    "github.com/docker/docker/client"
)

// DevContainerManager manages dev container lifecycle (Zed+agent environments)
type DevContainerManager struct {
    docker   *client.Client
    manager  *Manager // Parent Hydra manager for network access

    // Track active dev containers
    containers map[string]*DevContainer
    mu         sync.RWMutex
}

type DevContainer struct {
    SessionID     string
    ContainerID   string
    ContainerName string
    Status        string
    IPAddress     string
    ContainerType string // "sway", "ubuntu", "headless"
    CreatedAt     time.Time
}

// CreateDevContainer creates and starts a dev container
func (dm *DevContainerManager) CreateDevContainer(ctx context.Context, req *CreateDevContainerRequest) (*DevContainerResponse, error) {
    // Build container configuration
    containerConfig := &container.Config{
        Image:    req.Image,
        Hostname: req.Hostname,
        Env:      dm.buildEnv(req),
    }

    hostConfig := &container.HostConfig{
        NetworkMode: "helix_default",
        IpcMode:     "host",
        Privileged:  false,
        CapAdd:      []string{"SYS_ADMIN", "SYS_NICE", "SYS_PTRACE", "NET_RAW", "MKNOD", "NET_ADMIN"},
        SecurityOpt: []string{"seccomp=unconfined", "apparmor=unconfined"},
        Mounts:      dm.buildMounts(req),
        Resources: container.Resources{
            DeviceCgroupRules: dm.getDeviceCgroupRules(),
            Ulimits: []*container.Ulimit{
                {Name: "nofile", Soft: 65536, Hard: 65536},
            },
        },
    }

    // Add GPU configuration based on vendor
    dm.configureGPU(hostConfig, req.GPUVendor)

    // Create container
    resp, err := dm.docker.ContainerCreate(ctx, containerConfig, hostConfig, nil, nil, req.ContainerName)
    if err != nil {
        return nil, fmt.Errorf("failed to create container: %w", err)
    }

    // Start container
    if err := dm.docker.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
        // Cleanup on failure
        dm.docker.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})
        return nil, fmt.Errorf("failed to start container: %w", err)
    }

    // Get container IP
    inspect, err := dm.docker.ContainerInspect(ctx, resp.ID)
    if err != nil {
        return nil, fmt.Errorf("failed to inspect container: %w", err)
    }

    ipAddress := ""
    if inspect.NetworkSettings != nil && inspect.NetworkSettings.Networks != nil {
        if net, ok := inspect.NetworkSettings.Networks["helix_default"]; ok {
            ipAddress = net.IPAddress
        }
    }

    // Track container
    dc := &DevContainer{
        SessionID:     req.SessionID,
        ContainerID:   resp.ID,
        ContainerName: req.ContainerName,
        Status:        "running",
        IPAddress:     ipAddress,
        CreatedAt:     time.Now(),
    }
    dm.mu.Lock()
    dm.containers[req.SessionID] = dc
    dm.mu.Unlock()

    return &DevContainerResponse{
        SessionID:     req.SessionID,
        ContainerID:   resp.ID,
        ContainerName: req.ContainerName,
        Status:        "running",
        IPAddress:     ipAddress,
    }, nil
}

// configureGPU adds GPU-specific Docker configuration
func (dm *DevContainerManager) configureGPU(hostConfig *container.HostConfig, vendor string) {
    switch vendor {
    case "nvidia":
        // NVIDIA: use nvidia-container-runtime
        hostConfig.Runtime = "nvidia"
        hostConfig.DeviceRequests = []container.DeviceRequest{
            {
                DeviceIDs:    []string{"all"},
                Capabilities: [][]string{{"gpu"}},
            },
        }
    case "amd":
        // AMD: mount /dev/kfd and /dev/dri/*
        hostConfig.Devices = append(hostConfig.Devices,
            container.DeviceMapping{PathOnHost: "/dev/kfd", PathInContainer: "/dev/kfd", CgroupPermissions: "rwm"},
        )
        // DRI devices are handled via GOW_REQUIRED_DEVICES env var
    case "intel":
        // Intel: mount /dev/dri/* (handled via GOW_REQUIRED_DEVICES)
    default:
        // Software rendering - no special config needed
    }
}

// getDeviceCgroupRules returns cgroup rules for hidraw and input devices
func (dm *DevContainerManager) getDeviceCgroupRules() []string {
    // Read major numbers from /proc/devices
    hidrawMajor := dm.getDeviceMajor("hidraw")
    inputMajor := dm.getDeviceMajor("input")

    var rules []string
    if hidrawMajor != "" {
        rules = append(rules, fmt.Sprintf("c %s:* rwm", hidrawMajor))
    }
    if inputMajor != "" {
        rules = append(rules, fmt.Sprintf("c %s:* rwm", inputMajor))
    }
    return rules
}
```

### Phase 4: HydraExecutor in Helix API

Create `api/pkg/external-agent/hydra_executor.go`:

```go
package external_agent

import (
    "context"
    "fmt"
    "time"

    "github.com/helixml/helix/api/pkg/hydra"
    "github.com/helixml/helix/api/pkg/store"
    "github.com/helixml/helix/api/pkg/types"
)

// HydraExecutor implements Executor using Hydra for dev containers
// This is an alternative to WolfExecutor that bypasses Wolf entirely
type HydraExecutor struct {
    store         store.Store
    sessions      map[string]*ZedSession
    mutex         sync.RWMutex

    // Helix configuration
    zedImage      string
    helixAPIURL   string
    helixAPIToken string

    // RevDial connection manager
    connman connmanInterface

    // Feature flag for side-by-side testing
    enabled bool
}

// StartDevContainer implements Executor using Hydra instead of Wolf
func (h *HydraExecutor) StartDevContainer(ctx context.Context, agent *types.ZedAgent) (*types.ZedAgentResponse, error) {
    // Get Hydra client via RevDial
    // Hydra runner ID follows pattern: hydra-{sandbox_id}
    sandboxID := agent.SandboxID
    if sandboxID == "" {
        // Fall back to session ID for backward compatibility
        sandboxID = agent.SessionID
    }
    hydraRunnerID := fmt.Sprintf("hydra-%s", sandboxID)
    hydraClient := hydra.NewRevDialClient(h.connman, hydraRunnerID)

    // Build container configuration
    // This mirrors the logic in createDesktopWolfApp but sends to Hydra
    containerType := parseContainerType(agent.DesktopType) // "sway", "ubuntu", "headless"
    containerName := fmt.Sprintf("%s-external-%s", containerType, strings.TrimPrefix(agent.SessionID, "ses_"))

    req := &hydra.CreateDevContainerRequest{
        SessionID:     agent.SessionID,
        Image:         h.getDevContainerImage(containerType, agent),
        ContainerName: containerName,
        Hostname:      containerName,
        Env:           h.buildEnvVars(agent, containerType),
        Mounts:        h.buildMounts(agent),
        DisplayWidth:  agent.DisplayWidth,
        DisplayHeight: agent.DisplayHeight,
        DisplayFPS:    agent.DisplayRefreshRate,
        ContainerType: string(containerType),
        GPUVendor:     h.detectGPUVendor(),
        UserID:        agent.UserID,
    }

    // If Hydra isolation is enabled, create isolated dockerd first
    if h.hydraEnabled {
        dockerInstance, err := hydraClient.CreateDockerInstance(ctx, &hydra.CreateDockerInstanceRequest{
            ScopeType:     hydra.ScopeTypeSession,
            ScopeID:       agent.SessionID,
            UserID:        agent.UserID,
            UseHostDocker: agent.UseHostDocker,
        })
        if err != nil {
            return nil, fmt.Errorf("failed to create isolated Docker instance: %w", err)
        }
        req.DockerSocket = dockerInstance.DockerSocket
    }

    // Create dev container via Hydra
    resp, err := hydraClient.CreateDevContainer(ctx, req)
    if err != nil {
        return nil, fmt.Errorf("failed to create dev container via Hydra: %w", err)
    }

    // Track session
    session := &ZedSession{
        SessionID:     agent.SessionID,
        HelixSessionID: agent.HelixSessionID,
        UserID:        agent.UserID,
        Status:        "running",
        StartTime:     time.Now(),
        LastAccess:    time.Now(),
        ContainerName: resp.ContainerName,
        ContainerID:   resp.ContainerID,
    }
    h.mutex.Lock()
    h.sessions[agent.SessionID] = session
    h.mutex.Unlock()

    return &types.ZedAgentResponse{
        SessionID:     agent.SessionID,
        ScreenshotURL: fmt.Sprintf("/api/v1/sessions/%s/screenshot", agent.SessionID),
        StreamURL:     fmt.Sprintf("/api/v1/sessions/%s/stream", agent.SessionID),
        Status:        "running",
        ContainerName: resp.ContainerName,
        ContainerIP:   resp.IPAddress,
    }, nil
}
```

### Phase 5: Feature Flag & Side-by-Side

Add environment variable to switch between executors:

```go
// In api/pkg/server/external_agent_handlers.go or config

// EXECUTOR_MODE controls which executor to use:
// - "wolf" (default): Use WolfExecutor (current behavior)
// - "hydra": Use HydraExecutor (Wolf-free)
// - "hybrid": Use HydraExecutor with Wolf fallback
executorMode := os.Getenv("EXECUTOR_MODE")

switch executorMode {
case "hydra":
    executor = NewHydraExecutor(...)
case "hybrid":
    executor = NewHybridExecutor(
        primary: NewHydraExecutor(...),
        fallback: NewWolfExecutor(...),
    )
default:
    executor = NewWolfExecutor(...)
}
```

## What We Port from Wolf

| Wolf Feature | Hydra Implementation |
|--------------|---------------------|
| Container creation | `DevContainerManager.CreateDevContainer()` |
| GPU passthrough | `configureGPU()` - NVIDIA/AMD/Intel detection |
| Device cgroup rules | `getDeviceCgroupRules()` - hidraw/input major numbers |
| PipeWire socket | Mount `/run/user/1000` for GNOME ScreenCast |
| Per-lobby socket | Mount Hydra socket at `/var/run/hydra/session.sock` |
| Container networking | Use `helix_default` network + bridge to Hydra |
| Container stop | `DevContainerManager.DeleteDevContainer()` |

## What We DON'T Need to Port

| Wolf Feature | Why Not Needed |
|--------------|----------------|
| Fake udev | Virtual input devices handled by screenshot-server D-Bus |
| Video encoding | Handled by screenshot-server (ws_stream.go) |
| Audio encoding | Future: handled by screenshot-server |
| Moonlight protocol | Replaced by WebSocket streaming |
| Session management | Helix API handles this |

## Files to Create/Modify

### New Files
- `api/pkg/hydra/devcontainer.go` - Dev container management
- `api/pkg/external-agent/hydra_executor.go` - HydraExecutor implementation

### Modified Files
- `api/pkg/hydra/server.go` - Add dev container endpoints
- `api/pkg/hydra/types.go` - Add request/response types
- `api/pkg/hydra/client.go` - Add client methods for new endpoints
- `api/pkg/server/external_agent_handlers.go` - Executor selection logic

## Migration Path

1. **Phase 1: Implement HydraExecutor** (this design)
   - Add dev container APIs to Hydra
   - Create HydraExecutor alongside WolfExecutor
   - Test with `EXECUTOR_MODE=hydra`

2. **Phase 2: Parallel Testing**
   - Run both executors in production (A/B test)
   - Monitor for issues, compare resource usage
   - Keep Wolf as fallback

3. **Phase 3: Wolf Deprecation**
   - Make Hydra the default (`EXECUTOR_MODE=hydra`)
   - Wolf only for explicit fallback
   - Document migration path for operators

4. **Phase 4: Wolf Removal**
   - Remove WolfExecutor code
   - Remove Wolf from Dockerfile.sandbox
   - Remove Moonlight-Web from Dockerfile.sandbox
   - Shrink sandbox image by ~1GB

## Benefits

| Aspect | Current (Wolf) | Proposed (Hydra) |
|--------|---------------|------------------|
| Language | C++ | Go |
| Codebase size | ~50K lines Wolf | ~2K lines Hydra extension |
| Build time | ~10 min Wolf | ~30 sec Go |
| Debug tooling | GDB, core dumps | pprof, delve |
| Dependency management | CMake, vcpkg | Go modules |
| Memory safety | Manual | Automatic |
| Maintainability | Requires C++ expertise | Standard Go |

## Testing Plan

1. **Unit tests** for Hydra dev container management
2. **Integration tests** with real Docker API
3. **E2E tests** comparing Wolf vs Hydra container creation
4. **Performance tests** measuring startup latency
5. **GPU tests** on NVIDIA, AMD, Intel hardware

## Open Questions

1. **Container naming:** Keep Wolf's `{type}-external-{session_id}_{lobby_id}` format or simplify?
   - Recommendation: Simplify to `{type}-{session_id}` since we don't have lobbies

2. **Health monitoring:** Who watches container health?
   - Recommendation: Hydra monitors via Docker events, reports to Helix API

3. **Log streaming:** How do we get container logs?
   - Recommendation: RevDial-based log streaming from Hydra

4. **Headless containers:** How do we handle containers without display?
   - Recommendation: Omit display env vars, skip video streaming setup
   - Agent communicates via RevDial only (no screenshot-server)

## Conclusion

Extending Hydra to launch dev containers is the logical next step after Wolf-free video streaming. Hydra already has the infrastructure (RevDial, Docker client, network management) - we just need to add the container creation logic ported from Wolf.

The side-by-side testing approach minimizes risk while allowing gradual migration away from Wolf.
