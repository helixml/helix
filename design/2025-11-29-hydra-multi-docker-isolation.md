# Hydra: Multi-Docker Isolation for Zed Agent Desktop Environments

**Date:** 2025-11-29
**Status:** Design
**Author:** Claude

## Problem Statement

The current Helix sandbox architecture uses Docker-in-Docker (DinD) where:
- One sandbox container runs Wolf and a single dockerd
- Wolf spawns multiple desktop containers (helix-sway) for different users/sessions
- All desktop containers share the same Docker daemon via `/var/run/docker.sock`

This creates several problems:
1. **No container isolation**: Users can see/manage each other's containers
2. **Security risk**: One user could interfere with another's development containers
3. **Resource contention**: All containers compete for the same Docker resources
4. **No per-user Docker contexts**: DevContainers, docker-compose projects collide

## Solution: Hydra Multi-Docker Architecture

Hydra is a Go daemon that runs inside the sandbox container and manages dedicated Docker daemon instances for each Zed agent desktop.

### Architecture Overview

```
┌─────────────────────────────────────────────────────────────────────────┐
│ HOST                                                                     │
│   /var/lib/docker/volumes/helix_helix-filestore/_data/                  │
│     └── hydra/                                                          │
│           ├── lobby-abc123/                                             │
│           │   ├── docker/  (isolated Docker data dir)                   │
│           │   └── docker.sock                                           │
│           └── lobby-def456/                                             │
│               ├── docker/                                               │
│               └── docker.sock                                           │
└──────────────────────────────────┬──────────────────────────────────────┘
                                   │ volume mount
                                   ▼
┌─────────────────────────────────────────────────────────────────────────┐
│ SANDBOX CONTAINER                                                        │
│                                                                          │
│  ┌─────────────────┐    ┌─────────────────────────────────────────────┐ │
│  │     Wolf        │    │               Hydra Daemon                  │ │
│  │                 │    │                                             │ │
│  │ Creates lobbies │───>│ /var/run/hydra/hydra.sock                   │ │
│  │ Mounts sockets  │    │                                             │ │
│  └────────┬────────┘    │ Manages per-lobby dockerd instances:        │ │
│           │             │  - lobby-abc123 → dockerd on :2375          │ │
│           │             │  - lobby-def456 → dockerd on :2376          │ │
│           │             └─────────────────────────────────────────────┘ │
│           │                                                              │
│  ┌────────┴────────────────────────────────────────────────────────────┐│
│  │ Primary dockerd (Wolf's Docker)                                     ││
│  │ /var/run/docker.sock                                                ││
│  │ Spawns helix-sway containers                                        ││
│  └─────────────────────────────────────────────────────────────────────┘│
│                                                                          │
│  ┌─────────────────────────────────────────────────────────────────────┐│
│  │ helix-sway container (lobby-abc123)                                 ││
│  │   Mounts: /var/run/hydra/lobby-abc123/docker.sock:/var/run/docker.sock│
│  │   User's Docker commands → isolated dockerd                          ││
│  └─────────────────────────────────────────────────────────────────────┘│
│                                                                          │
│  ┌─────────────────────────────────────────────────────────────────────┐│
│  │ helix-sway container (lobby-def456)                                 ││
│  │   Mounts: /var/run/hydra/lobby-def456/docker.sock:/var/run/docker.sock│
│  │   User's Docker commands → different isolated dockerd                ││
│  └─────────────────────────────────────────────────────────────────────┘│
└─────────────────────────────────────────────────────────────────────────┘
```

### Hydra API

Hydra exposes a Unix socket API at `/var/run/hydra/hydra.sock`:

#### Create Docker Instance
```
POST /api/v1/docker-instances
{
  "scope_type": "spectask",  // "spectask", "session", or "exploratory"
  "scope_id": "stask_abc123",
  "user_id": "usr_xxx",
  "max_containers": 10  // optional limit
}

Response:
{
  "scope_type": "spectask",
  "scope_id": "stask_abc123",
  "docker_socket": "/var/run/hydra/active/spectask-stask_abc123/docker.sock",
  "docker_host": "unix:///var/run/hydra/active/spectask-stask_abc123/docker.sock",
  "data_root": "/filestore/hydra/spectasks/stask_abc123/docker",
  "status": "running"
}
```

#### Delete Docker Instance (stops dockerd, preserves data)
```
DELETE /api/v1/docker-instances/{scope_type}/{scope_id}

Response:
{
  "scope_type": "spectask",
  "scope_id": "stask_abc123",
  "status": "stopped",
  "containers_stopped": 3,
  "data_preserved": true
}
```

#### Get Docker Instance Status
```
GET /api/v1/docker-instances/{scope_type}/{scope_id}

Response:
{
  "scope_type": "spectask",
  "scope_id": "stask_abc123",
  "status": "running",  // or "stopped" (data exists but dockerd not running)
  "container_count": 2,
  "uptime_seconds": 3600,
  "docker_socket": "/var/run/hydra/active/spectask-stask_abc123/docker.sock",
  "data_root": "/filestore/hydra/spectasks/stask_abc123/docker",
  "data_size_bytes": 1073741824
}
```

#### List All Docker Instances
```
GET /api/v1/docker-instances

Response:
{
  "instances": [
    {
      "scope_type": "spectask",
      "scope_id": "stask_abc123",
      "status": "running",
      "container_count": 2
    },
    {
      "scope_type": "session",
      "scope_id": "ses_def456",
      "status": "stopped",
      "container_count": 0
    }
  ]
}
```

#### Purge Docker Instance Data (stops dockerd AND deletes data)
```
DELETE /api/v1/docker-instances/{scope_type}/{scope_id}/data

Response:
{
  "scope_type": "spectask",
  "scope_id": "stask_abc123",
  "status": "purged",
  "data_deleted_bytes": 1073741824
}
```

### Integration with Wolf Executor

The Wolf executor in `api/pkg/external-agent/wolf_executor.go` will be modified to:

1. **Before creating lobby**: Call Hydra to create/resume a dedicated dockerd for the scope
2. **When creating lobby**: Mount the scope-specific socket instead of shared socket
3. **When stopping lobby**: Call Hydra to stop the dockerd (data preserved for resume)

```go
// wolf_executor.go changes

// Determine scope from agent request
var scopeType, scopeID string
if agent.SpecTaskID != "" {
    scopeType = "spectask"
    scopeID = agent.SpecTaskID
} else if agent.ProjectID != "" {
    scopeType = "exploratory"
    scopeID = agent.SessionID
} else {
    scopeType = "session"
    scopeID = agent.SessionID
}

// Before creating lobby - create or resume dockerd for this scope
hydraClient := hydra.NewRevDialClient(w.connman, "hydra")
dockerInstance, err := hydraClient.CreateDockerInstance(ctx, &hydra.CreateRequest{
    ScopeType: scopeType,
    ScopeID:   scopeID,
    UserID:    agent.UserID,
})
if err != nil {
    return nil, fmt.Errorf("failed to create isolated docker instance: %w", err)
}

// In createSwayWolfApp, change mount from:
"/var/run/docker.sock:/var/run/docker.sock"
// To:
fmt.Sprintf("%s:/var/run/docker.sock", dockerInstance.DockerSocket)

// When stopping lobby - stop dockerd but preserve data
hydraClient.DeleteDockerInstance(ctx, scopeType, scopeID)
```

**Resume behavior**: When a SpecTask/session is resumed:
- Hydra starts a new dockerd process
- The dockerd reconnects to the existing data directory
- Previously pulled images and created volumes are available
- Running containers are NOT restored (they died with the previous dockerd)

### Hydra Implementation Details

#### Directory Structure

Docker state persists by **scope** (SpecTask, session, or exploratory session), not by lobby:

```
/var/run/hydra/
├── hydra.sock              # Hydra API socket
├── hydra.pid               # Hydra PID file
└── active/                 # Runtime state (ephemeral)
    ├── spectask-{id}/
    │   └── docker.sock     # Active Docker socket
    ├── session-{id}/
    │   └── docker.sock
    └── exploratory-{id}/
        └── docker.sock

/filestore/hydra/           # Persistent storage (survives sandbox restart)
├── spectasks/
│   └── {spectask_id}/
│       └── docker/         # Docker data-root (images, containers, volumes)
├── sessions/
│   └── {session_id}/
│       └── docker/
└── exploratory/
    └── {session_id}/
        └── docker/
```

**Persistence guarantees:**
- SpecTask Docker state persists across lobby restarts and sandbox restarts
- Session Docker state persists across lobby restarts and sandbox restarts
- Exploratory session Docker state persists similarly
- When a lobby is stopped and restarted, the dockerd reconnects to the same data directory
- Container state (running containers) is lost on sandbox restart, but images/volumes persist

#### dockerd Configuration Per Instance
Each per-lobby dockerd is started with:
```bash
dockerd \
  --host=unix:///var/run/hydra/lobby-{id}/docker.sock \
  --data-root=/filestore/hydra/lobby-{id}/docker \
  --exec-root=/var/run/hydra/lobby-{id}/exec \
  --pidfile=/var/run/hydra/lobby-{id}/docker.pid \
  --config-file=/etc/hydra/lobby-{id}/daemon.json \
  --iptables=false \
  --ip-masq=false \
  --bridge=none
```

Note: `--iptables=false --ip-masq=false --bridge=none` because:
- Network isolation handled by outer Docker
- Each inner dockerd uses host networking of its container
- Avoids iptables conflicts between multiple dockerd instances

#### Resource Limits
Each per-lobby dockerd can be configured with:
- Max containers (via Hydra enforcement)
- Storage quota (via Docker's `--storage-opt dm.basesize=10G`)
- Memory limits (inherited from helix-sway container)

### Lifecycle Management

#### Lobby Creation Flow
```
1. Wolf executor receives StartZedAgent request
2. Wolf executor calls Hydra: POST /docker-instances {lobby_id}
3. Hydra:
   a. Creates directory /filestore/hydra/lobby-{id}/docker/
   b. Generates daemon.json with NVIDIA runtime
   c. Starts dockerd with isolated config
   d. Waits for socket to be ready
   e. Returns socket path
4. Wolf executor creates lobby with mount: {socket_path}:/var/run/docker.sock
5. helix-sway container starts with isolated Docker
```

#### Lobby Shutdown Flow
```
1. Wolf executor receives StopZedAgent request
2. Wolf executor calls Hydra: DELETE /docker-instances/{lobby_id}
3. Hydra:
   a. Lists and stops all containers in this dockerd
   b. Sends SIGTERM to dockerd
   c. Waits for graceful shutdown (timeout 30s)
   d. If needed, SIGKILL
   e. Cleans up /var/run/hydra/lobby-{id}/
   f. Optionally preserves /filestore/hydra/lobby-{id}/ for container image cache
4. Wolf executor proceeds with Wolf lobby teardown
```

#### Orphan Cleanup
Hydra runs a background goroutine that:
- Every 5 minutes, lists running dockerd instances
- Checks if corresponding Wolf lobby still exists
- Cleans up orphaned dockerd instances (lobby was deleted without proper cleanup)

### Communication with Hydra

The Wolf executor runs in the API container, but Hydra runs in the sandbox container. Communication happens via RevDial:

```go
// In API container (wolf_executor.go)
hydraConn, err := w.connman.Dial(ctx, "hydra")
// Send HTTP request over RevDial tunnel to Hydra's Unix socket
```

Hydra registers itself with RevDial on startup, just like the sandbox's revdial-client.

### Security Considerations

1. **Socket permissions**: Per-lobby sockets owned by root:docker, mode 0660
2. **No cross-lobby access**: Each helix-sway only sees its own socket
3. **NVIDIA runtime**: Each dockerd configured with NVIDIA runtime for GPU access
4. **Network isolation**: Inner containers use host networking of their parent (helix-sway)
5. **Resource limits**: Hydra can enforce container count limits per lobby

### Files to Create/Modify

#### New Files
```
api/cmd/hydra/main.go              # Hydra daemon entry point
api/pkg/hydra/server.go            # HTTP server and API handlers
api/pkg/hydra/dockerd_manager.go   # Manages dockerd lifecycle
api/pkg/hydra/types.go             # API request/response types
api/pkg/hydra/client.go            # Client for API consumers
```

#### Modified Files
```
Dockerfile.sandbox                  # Add hydra build and startup
api/pkg/external-agent/wolf_executor.go  # Integrate Hydra client
```

### Testing Plan

1. **Unit tests**: Mock dockerd process management
2. **Integration tests**:
   - Create multiple lobbies, verify socket isolation
   - Run `docker ps` in each lobby, verify no cross-visibility
   - Stop lobby, verify dockerd cleanup
3. **Stress tests**:
   - Create/destroy 10 lobbies rapidly
   - Verify no socket/PID file leaks

### Rollout Plan

1. **Phase 1**: Implement Hydra daemon with basic create/delete
2. **Phase 2**: Integrate with Wolf executor
3. **Phase 3**: Add RevDial communication
4. **Phase 4**: Add monitoring and cleanup loops
5. **Phase 5**: Production rollout with feature flag

### Metrics and Observability

Hydra exposes metrics at `/metrics`:
- `hydra_dockerd_instances_total`: Number of running dockerd instances
- `hydra_dockerd_create_duration_seconds`: Time to start a new dockerd
- `hydra_dockerd_stop_duration_seconds`: Time to stop a dockerd
- `hydra_containers_per_instance`: Histogram of containers per dockerd

### Privileged Mode (Host Docker Access)

For developing Helix inside Helix, we need a special privileged mode that bypasses Hydra's isolated dockerd and provides direct access to the host's Docker daemon.

#### Use Case
- Developing Helix itself using Helix agents
- Running `./stack start`, `docker compose`, etc. that need to access host containers
- Full Docker access for infrastructure work

#### Implementation

1. **Environment Variable Control**
   The sandbox container must opt-in to privileged mode via environment variable:
   ```bash
   HYDRA_PRIVILEGED_MODE_ENABLED=true
   ```

2. **API Endpoint**
   ```
   GET /api/v1/privileged-mode/status

   Response:
   {
     "enabled": true,
     "host_docker_socket": "/var/run/docker.sock",
     "description": "Host Docker access is available for Helix development"
   }
   ```

3. **Wolf Executor Integration**
   When creating a lobby, Wolf executor:
   - Checks if `use_host_docker=true` in the request
   - Queries Hydra's privileged mode status
   - If enabled, mounts `/var/run/docker.sock` instead of scope-specific socket

4. **UI Flow**
   - When starting an external agent, UI shows "Use host Docker" checkbox
   - Checkbox only visible when sandbox reports `privileged_mode.enabled=true`
   - User must explicitly request it per-session

5. **Security Considerations**
   - Privileged mode is NEVER enabled by default
   - Only the sandbox operator can enable it via environment variable
   - Even when enabled, users must explicitly request it per-session
   - Audit logs track all privileged mode sessions

#### Example Session Flow

```
1. User clicks "New SpecTask" in UI
2. UI queries sandbox capabilities (via session metadata)
3. If privileged mode enabled, shows "Use host Docker (for Helix development)" checkbox
4. User checks the box (or not)
5. API includes `use_host_docker: true` in CreateDockerInstance request
6. Hydra returns host socket path instead of creating isolated dockerd
7. Wolf mounts /var/run/docker.sock directly
```

### Open Questions

1. **Storage cleanup**: Should we auto-delete Docker data when lobby is deleted, or keep for caching?
   - Proposed: Keep for 24h, then cleanup unused

2. **Image preloading**: Should each dockerd have common images pre-loaded?
   - Proposed: No, let users pull as needed (storage efficient)

3. **GPU sharing**: How do multiple dockerds share GPU?
   - Answer: NVIDIA runtime handles this - each container can use GPU via runtime
