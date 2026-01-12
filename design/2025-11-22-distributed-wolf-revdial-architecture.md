# Distributed Wolf Architecture with RevDial

**Date**: 2025-11-22
**Status**: Phase 1 Complete ✅ (DinD working), Phase 2 In Progress (RevDial)
**Problem**: Separate Wolf streaming infrastructure from control plane to support multiple remote Wolf instances

---

## ✅ VERIFIED WORKING: Docker-in-Docker (2025-11-22)

**Successfully tested on dev machine with NVIDIA RTX 2000 Ada:**

✅ **Wolf runs isolated dockerd** - No host socket dependency
✅ **Sandboxes run in Wolf's dockerd** - Desktop streaming works perfectly
✅ **GPU devices pass through** - Sway compositor renders with GPU acceleration
✅ **Docker commands work inside sandboxes** - Tested `docker ps`, `docker run hello-world`
✅ **Devcontainers run as siblings** - Only 2 nesting levels (not 3!)
✅ **K8s-ready architecture** - No host docker socket required

**Implementation**:
- Wolf image: dockerd + nvidia-container-toolkit + network creation
- Sandboxes: Bind-mount Wolf's `/var/run/docker.sock` (not host's!)
- Development: `/helix-dev/` mounts for hot-reloading in DinD
- Production: Files baked into helix-sway image, pulled from registry

**Network Fix** (critical discovery):
- Network isolation: Sandboxes can't reach `api:8080` (different Docker network)
- Git cloning fails, screenshots fail
- **Next**: Implement RevDial for sandbox ↔ API communication

**Repos**:
- Wolf: `feature/wolf-dind` branch (3 commits)
- Helix: `feature/wolf-dind` branch (4 commits)

---

## Current Architecture Limitations

1. **Wolf + Moonlight Web tightly coupled to API**: Wolf socket at `/var/run/wolf/wolf.sock`, Moonlight Web runs on same host
2. **Direct Docker network access**: API connects directly to sandbox containers (screenshot server, Sway Go daemons) over helix_default network
3. **Single Wolf instance**: No load balancing, no geographic distribution
4. **NAT traversal only for streaming**: Control plane traffic requires direct routing

## Proposed Architecture

### Multi-Wolf Infrastructure

**Wolf Deployment Model**: Wolf + Moonlight Web run together as a **paired service** on remote machines, separate from control plane.

**Scheduling Strategy**:
- Maintain list of available Wolf instances in database
- Round-robin OR least-loaded scheduling (fewest active sandboxes)
- Each Wolf instance reports health + capacity metrics via RevDial

**Components per Wolf instance**:
- Wolf container (GPU-accelerated streaming)
- Moonlight Web container (WebRTC gateway)
- RevDial client (persistent connection to API)

### RevDial Connection Architecture

**Core Principle**: Sandboxes and Wolf infrastructure make **outbound connections** to API, allowing deployment behind NAT.

#### RevDial Connections Required

1. **Wolf → API**: Wolf API endpoints (app management, session control)
2. **Moonlight Web → API**: Moonlight Web API + WebSocket proxy for browser connections
3. **Per-sandbox connections**:
   - Screenshot server (Go HTTP server on port 8080 in Sway container)
   - Sway daemon (file sync, clipboard, other IPC - **TODO: identify exact service**)

#### Network Flow

```
┌─────────────────────────────────────────────────────────────┐
│                     Helix Control Plane                      │
│  ┌────────────────────────────────────────────────────────┐  │
│  │ API Server (RevDial listener)                          │  │
│  │  - Accepts RevDial from Wolf instances                 │  │
│  │  - Accepts RevDial from each sandbox                   │  │
│  │  - Routes requests over RevDial tunnels                │  │
│  └─────────────▲────────────────────▲─────────────────────┘  │
│                │                    │                         │
└────────────────┼────────────────────┼─────────────────────────┘
                 │                    │
         (outbound RevDial)   (outbound RevDial)
                 │                    │
┌────────────────┼────────────────────┼─────────────────────────┐
│  Wolf Instance │(behind NAT)        │                         │
│  ┌─────────────┴──────────┐  ┌──────┴──────────────────────┐ │
│  │ Wolf Container         │  │ Moonlight Web               │ │
│  │  - RevDial client      │  │  - RevDial client           │ │
│  │  - Wolf API over RevDial│  │  - API over RevDial        │ │
│  │  - Manages sandboxes   │  │  - WebSocket proxy          │ │
│  └────────────────────────┘  └─────────────────────────────┘ │
│                                                                │
│  ┌───────────────────────────────────────────────────────┐    │
│  │ Sandbox Container (Sway)                              │    │
│  │  - RevDial client (per-sandbox connection)            │    │
│  │  - Screenshot server → API via RevDial                │    │
│  │  - Sway daemon → API via RevDial                      │    │
│  └───────────────────────────────────────────────────────┘    │
└────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────┐
│                     User Browser                             │
│  - Connects to API (HTTPS)                                   │
│  - API proxies WebSocket to Moonlight Web via RevDial        │
│  - WebRTC media: Browser ←──UDP (STUN)──→ Moonlight Web     │
└─────────────────────────────────────────────────────────────┘
```

### WebRTC Media Flow (NAT Traversal)

**Assumption**: STUN servers successfully establish WebRTC connection.

**Flow**:
1. User browser → API WebSocket endpoint (HTTPS)
2. API proxies WebSocket → Moonlight Web via RevDial tunnel
3. Moonlight Web negotiates WebRTC with browser (STUN for NAT traversal)
4. Media streams: Browser ←──UDP (WebRTC)──→ Moonlight Web (NAT-traversed)

**Public IP Requirement**: Only for **direct Moonlight client** connections (not browser-based). Browser-based streaming works behind NAT via WebRTC.

### RevDial Implementation Details

#### Wolf RevDial Client

**Location**: Wolf container starts RevDial client on boot
**Connection**: `wss://api.example.com/revdial/wolf/{wolf_instance_id}`
**Endpoints exposed**:
- `GET /api/v1/apps` → Wolf API
- `POST /api/v1/apps` → Wolf API
- `DELETE /api/v1/apps/{id}` → Wolf API
- All other Wolf HTTP endpoints

#### Moonlight Web RevDial Client

**Location**: Moonlight Web container starts RevDial client on boot
**Connection**: `wss://api.example.com/revdial/moonlight/{wolf_instance_id}`
**Endpoints exposed**:
- `GET /healthz` → Moonlight Web health check
- `WS /stream/{session_id}` → WebSocket proxy for browser connections

#### Per-Sandbox RevDial Client

**Location**: Sway container starts RevDial client on boot
**Connection**: `wss://api.example.com/revdial/sandbox/{sandbox_id}`
**Endpoints exposed** (HTTP server on port 9876 in Sway container):
- `GET /screenshot` → Capture screenshot using grim (Wayland screenshot tool)
- `GET /clipboard` → Retrieve clipboard content from sandbox
- `POST /clipboard` → Set clipboard content in sandbox

**Implementation**: HTTP server runs inside Sway container (part of Games-on-Whales image or custom build), not in Helix codebase.

### Database Schema Changes

```go
type WolfInstance struct {
    ID            string `gorm:"type:varchar(255);primaryKey"`
    HostAddress   string `gorm:"type:text"` // Display only (not routable)
    GPUVendor     string `gorm:"type:varchar(50)"` // "nvidia", "amd", "intel"
    GPUCount      int
    ActiveSessions int   // Updated by Wolf via RevDial
    MaxSessions    int   // Configured capacity
    LastHeartbeat  time.Time
    Status        string `gorm:"type:varchar(50)"` // "online", "offline", "degraded"
}

type Sandbox struct {
    // ... existing fields
    WolfInstanceID string `gorm:"type:varchar(255);index"` // Which Wolf instance runs this sandbox
    RevDialConnected bool // RevDial connection established
}
```

### API Changes

**New endpoints**:
- `POST /api/v1/wolf-instances` - Register Wolf instance
- `GET /api/v1/wolf-instances` - List Wolf instances (admin only)
- `DELETE /api/v1/wolf-instances/{id}` - Deregister Wolf instance
- `POST /api/v1/wolf-instances/{id}/heartbeat` - Wolf heartbeat (via RevDial)

**Modified behavior**:
- Sandbox creation: Select Wolf instance via scheduling algorithm
- Screenshot requests: Route via RevDial to sandbox's RevDial tunnel
- Wolf API calls: Route via RevDial to Wolf instance's RevDial tunnel

### Scheduling Algorithm

**Option 1: Least-Loaded**
```go
func selectWolfInstance(instances []WolfInstance) *WolfInstance {
    var best *WolfInstance
    lowestLoad := math.MaxFloat64
    for _, inst := range instances {
        if inst.Status != "online" { continue }
        load := float64(inst.ActiveSessions) / float64(inst.MaxSessions)
        if load < lowestLoad {
            lowestLoad = load
            best = &inst
        }
    }
    return best
}
```

**Option 2: Round-Robin** (simpler, start with this)

### Migration Path

1. **Phase 1**: Add RevDial support to Wolf, Moonlight Web, Sway containers (keep local socket support)
2. **Phase 2**: Add Wolf instance registry to API, scheduling logic
3. **Phase 3**: Deploy first remote Wolf instance, test RevDial connections
4. **Phase 4**: Deprecate local Wolf socket, require RevDial for all Wolf instances

---

# Docker-in-Docker Architecture Change

**Date**: 2025-11-22
**Status**: Design
**Problem**: Remove Docker socket dependency from Wolf without triple-nesting Docker

## Constraints

**OverlayFS Stacking Limit**: Kernel hard limit of **2 nested levels**. Triple nesting fails:
```
overlayfs: maximum fs stacking depth exceeded
```

**Why Triple-Nesting Required**:
- Level 1: Wolf container on host (overlay)
- Level 2: Docker-in-Docker inside Wolf (overlay on overlay) ✅
- Level 3: Agent sandboxes inside DinD (overlay on overlay on overlay) ❌
- Level 4: Docker in sandboxes for devcontainers ❌

## Solution: Docker Socket Bind-Mount (Simple DinD)

**Key Insight**: We don't need 3 nesting levels - devcontainers can run as **siblings** to sandboxes!

**Architecture**:
```
Host
├── Wolf Container (runs dockerd inside, privileged)
│   ├── Sandbox1 (mounts /var/run/docker.sock from Wolf)
│   ├── Sandbox2 (mounts /var/run/docker.sock from Wolf)
│   ├── Devcontainer-A (created by Sandbox1 via docker.sock - SIBLING to Sandbox1!)
│   └── Devcontainer-B (created by Sandbox2 via docker.sock - SIBLING to Sandbox2!)
└── Other containers (API, etc.)
```

**How it works**:
1. Wolf runs privileged with dockerd inside
2. Sandboxes bind-mount Wolf's docker socket: `-v /var/run/docker.sock:/var/run/docker.sock`
3. When sandbox runs `docker run`, it creates containers in Wolf's dockerd (not nested inside sandbox)
4. Devcontainers run as **siblings** to sandboxes (both at level 2)

**Nesting levels**: Only **2** (not 3!)
- Level 1: Host → Wolf container
- Level 2: Wolf's dockerd → Sandboxes AND Devcontainers (as siblings)

**Advantages**:
✅ No Sysbox required (simpler, more maintainable)
✅ No overlayfs nesting limit hit (only 2 levels)
✅ NVIDIA runtime works normally (Wolf uses `runtime: nvidia`, sandboxes create siblings with same runtime)
✅ Proven pattern (Docker-in-Docker has used this for years)
✅ No GPU library bind-mount complexity

**Security Consideration** (punted for later):
- Multiple sandboxes share Wolf's docker socket → can see/kill each other's containers
- **Phase 1**: Simple bind-mount (accept cross-tenant visibility)
- **Phase 2**: Docker API proxy with namespace isolation (filter by labels/container name prefixes)

### Deployment Model

**Wolf Container** (NO host docker socket mount!):
```yaml
wolf:
  image: wolf-with-dockerd:latest
  privileged: true  # Required for dockerd inside Wolf
  runtime: nvidia   # NVIDIA runtime works at host level!
  volumes:
    - wolf-docker:/var/lib/docker  # Wolf's dockerd storage (isolated from host)
    # NO host docker socket mount! Wolf runs its own dockerd
  devices:
    - /dev/dri
    - /dev/nvidia0
    - /dev/nvidiactl
    - /dev/nvidia-uvm
```

**CRITICAL**: Wolf does NOT mount host's `/var/run/docker.sock`. This works in K8s, bare metal, anywhere.

**Sandbox Creation** (inside Wolf's dockerd):
```go
// Wolf creates sandbox with bind-mounted docker socket (Wolf's socket, NOT host socket!)
containerConfig := &container.Config{
    Image: "ghcr.io/games-on-whales/xfce:edge",
    Env: []string{
        "GOW_REQUIRED_DEVICES=/dev/input/* /dev/dri/* /dev/nvidia*",
    },
}

hostConfig := &container.HostConfig{
    Binds: []string{
        // Mount WOLF's /var/run/docker.sock into sandbox
        // This is Wolf's nested dockerd socket, NOT the host's socket
        // Works in K8s, bare metal, anywhere - fully isolated from host
        "/var/run/docker.sock:/var/run/docker.sock",
    },
    // GPU devices already available (Wolf's dockerd has access via privileged mode)
}
```

**Path clarification**:
- Host: May or may not have docker socket (K8s uses containerd)
- Wolf container: Has `/var/run/docker.sock` from its own dockerd (isolated)
- Sandbox: Mounts Wolf's `/var/run/docker.sock` (not host's!)

**Inside Sandbox** (developer running devcontainer):
```bash
# Sandbox has /var/run/docker.sock mounted
docker run -it ubuntu:24.04 bash  # Creates SIBLING container in Wolf's dockerd
docker-compose up                 # All containers run as siblings
```

**Container Hierarchy** (actual runtime):
```
Host dockerd:
└── Wolf container (privileged, has nested dockerd)

Wolf's nested dockerd:
├── Sandbox1 container
├── Sandbox2 container
├── Devcontainer-A container (created by Sandbox1, sibling to Sandbox1)
└── Devcontainer-B container (created by Sandbox2, sibling to Sandbox2)
```

### Wolf Dockerfile Changes

**Add dockerd to Wolf image**:
```dockerfile
# Install Docker inside Wolf container
RUN curl -fsSL https://get.docker.com | sh

# Entrypoint script starts dockerd before Wolf
COPY wolf-entrypoint.sh /usr/local/bin/
ENTRYPOINT ["/usr/local/bin/wolf-entrypoint.sh"]
```

**wolf-entrypoint.sh**:
```bash
#!/bin/bash
set -e

# Start dockerd in background (Wolf's isolated dockerd)
dockerd --host=unix:///var/run/docker.sock &

# Wait for dockerd to be ready
until docker info >/dev/null 2>&1; do
    echo "Waiting for Wolf's dockerd to start..."
    sleep 0.5
done

echo "Wolf's dockerd ready"

# Start Wolf
exec /usr/local/bin/wolf "$@"
```

### Original Sysbox Requirements (DEPRECATED)

**Kernel**: >= 5.0 (Ubuntu 20.04+, already met)
**Features**:
- User namespaces (CONFIG_USER_NS=y)
- Cgroups v2 (Ubuntu 22.04+ default)
- seccomp, capabilities (standard)

**Installation** (per Wolf host):
```bash
# Install Sysbox
wget https://downloads.nestybox.com/sysbox/releases/v0.6.4/sysbox-ce_0.6.4-0.linux_amd64.deb
dpkg -i sysbox-ce_0.6.4-0.linux_amd64.deb
systemctl enable sysbox --now
```

**Docker config** (`/etc/docker/daemon.json`):
```json
{
  "runtimes": {
    "sysbox-runc": {
      "path": "/usr/bin/sysbox-runc"
    }
  }
}
```

### Wolf Container Changes

**docker-compose.yaml**:
```yaml
wolf:
  runtime: sysbox-runc  # Changed from: nvidia
  cap_add:
    - SYS_ADMIN  # Required for Sysbox to setup user namespaces
  environment:
    - DOCKER_HOST=unix:///var/run/docker.sock  # Wolf's own dockerd socket
  volumes:
    # Remove host Docker socket mount (no longer needed)
    # - /var/run/docker.sock:/var/run/docker.sock:rw
```

**Dockerfile.wolf changes**:
```dockerfile
# Install Docker inside Wolf container
RUN curl -fsSL https://get.docker.com | sh

# Start dockerd via entrypoint
ENTRYPOINT ["/usr/local/bin/wolf-entrypoint.sh"]
```

**wolf-entrypoint.sh**:
```bash
#!/bin/bash
# Start dockerd in background
dockerd --host=unix:///var/run/docker.sock &
# Wait for dockerd to be ready
while ! docker info >/dev/null 2>&1; do sleep 0.1; done
# Start Wolf
exec /usr/local/bin/wolf "$@"
```

### Sandbox Creation Changes

**Before** (direct Docker socket):
```go
cli, err := client.NewClientWithOpts(client.FromEnv)
container, err := cli.ContainerCreate(ctx, config, hostConfig, nil, nil, sandboxID)
```

**After** (same code, different socket):
- Wolf's Docker client connects to Wolf's own dockerd (inside Wolf container)
- Sandboxes run inside Wolf's dockerd, isolated from host
- No code changes required (Docker API remains identical)

### GPU Access with Sysbox

**Challenge**: GPU devices must be accessible inside Sysbox containers AND nested sandbox containers.

**GPU Requirements by Component**:
- **Wolf**: Hardware-accelerated video encoding (NVENC/VAAPI/AMD VCE) - requires GPU devices
- **Sway containers**: Compositor rendering via KMS/DRM - **requires `/dev/dri`** (fails without it)
- **grim (screenshot)**: Uses Wayland screencopy protocol - no direct GPU access needed (works via Wayland socket)

**CRITICAL: No NVIDIA Runtime Inside Wolf**

Sysbox does NOT support the `nvidia` runtime inside nested Docker. We use **device pass-through only**:

```yaml
wolf:
  runtime: sysbox-runc  # NOT nvidia runtime!
  devices:
    - /dev/dri         # Required: Sway compositor needs this for KMS/DRM
    - /dev/nvidia0     # NVIDIA GPUs (for Wolf video encoding)
    - /dev/nvidiactl
    - /dev/nvidia-uvm
    - /dev/nvidia-modeset
    - /dev/kfd         # AMD ROCm (if AMD GPU)
```

**Inside Wolf's dockerd**:
- NO `nvidia` runtime configured (not supported by Sysbox)
- Sandboxes created with explicit `--device` flags:
```bash
# Inside Wolf's dockerd
docker run --device /dev/dri --device /dev/nvidia0 --device /dev/nvidiactl ...
```

**Games-on-Whales** (Sway base image) automatically mounts devices specified in `GOW_REQUIRED_DEVICES`:
```go
env := []string{
    "GOW_REQUIRED_DEVICES=/dev/input/* /dev/dri/* /dev/nvidia*",  // NVIDIA
    // OR
    "GOW_REQUIRED_DEVICES=/dev/input/* /dev/dri/* /dev/kfd",      // AMD
}
```

**Critical Constraints**:
- Sway compositor **will not start** without `/dev/dri` access: `"Failed to open any DRM device"`
- NVIDIA libraries (CUDA, NVENC) accessed via device files, NOT via `nvidia-docker` runtime
- Wolf's dockerd runs vanilla Docker (no nvidia-container-toolkit inside Wolf)

### Migration Path

1. **Phase 1**: Install Sysbox on dev Wolf host, test Wolf container with `--runtime=sysbox-runc`
2. **Phase 2**: Add dockerd startup to Wolf entrypoint, verify sandboxes launch
3. **Phase 3**: Test GPU access inside sandboxes (screenshot server, streaming)
4. **Phase 4**: Update install.sh to install Sysbox on Wolf hosts
5. **Phase 5**: Deploy to production, monitor for issues

### Limitations

**No Triple-Nesting**: Sandboxes can run Docker (devcontainers), but devcontainers **cannot run Docker** (kernel limit).
**Sysbox Host Requirement**: Every Wolf host must install Sysbox runtime.
**Not in Containers**: Sysbox cannot run inside a container (must be host-level runtime).

### Devcontainer Support Confirmation

**REQUIREMENT**: Sandboxes MUST run Docker for image builds, DevOps stacks, development environments.

**Sysbox Solution**: Supports **exactly 2 levels** of Docker-in-Docker (the maximum before hitting kernel limit):
- **Level 1**: Wolf runs dockerd → creates sandboxes
- **Level 2**: Sandbox runs dockerd → creates devcontainers ✅

**What works**:
```bash
# Inside sandbox (Sway container running in Wolf's dockerd)
docker run -it --rm ubuntu:24.04 bash  # Devcontainer ✅
docker build -t myapp .                # Image builds ✅
docker-compose up                      # DevOps stacks ✅
```

**What doesn't work**:
```bash
# Inside devcontainer (running in sandbox's dockerd)
docker run ...  # ❌ Fails (would be level 3)
```

**Limitation**: Devcontainers **cannot run Docker** (would require level 3). This is acceptable for development environments (rare to need Docker-in-Docker-in-Docker).

### Kubernetes Deployment

**Requirement**: Support running Wolf stack in Kubernetes.

**Solution**: Sysbox supports Kubernetes via custom runtime class.

#### K8s Node Setup

**Install Sysbox on K8s nodes**:
```bash
# On each K8s node that will run Wolf pods
wget https://downloads.nestybox.com/sysbox/releases/v0.6.4/sysbox-ce_0.6.4-0.linux_amd64.deb
dpkg -i sysbox-ce_0.6.4-0.linux_amd64.deb
systemctl enable sysbox --now
```

**Configure containerd** (`/etc/containerd/config.toml`):
```toml
[plugins."io.containerd.grpc.v1.cri".containerd.runtimes.sysbox-runc]
  runtime_type = "io.containerd.runc.v2"
  pod_annotations = ["io.kubernetes.cri.untrusted-workload"]
  [plugins."io.containerd.grpc.v1.cri".containerd.runtimes.sysbox-runc.options]
    BinaryName = "/usr/bin/sysbox-runc"
```

**Create RuntimeClass**:
```yaml
apiVersion: node.k8s.io/v1
kind: RuntimeClass
metadata:
  name: sysbox-runc
handler: sysbox-runc
```

#### Wolf Pod Definition

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: wolf-instance-1
  labels:
    app: wolf
spec:
  runtimeClassName: sysbox-runc  # Use Sysbox runtime
  containers:
  - name: wolf
    image: registry.helixml.tech/helix/wolf:latest
    securityContext:
      capabilities:
        add:
        - SYS_ADMIN  # Required for Sysbox
    env:
    - name: SANDBOX_INSTANCE_ID
      value: "wolf-instance-1"
    - name: API_ENDPOINT
      value: "https://api.helixml.tech"
    volumeMounts:
    - name: wolf-storage
      mountPath: /var/lib/docker  # Wolf's dockerd storage
    resources:
      limits:
        nvidia.com/gpu: 1  # GPU device plugin
  - name: moonlight-web
    image: registry.helixml.tech/helix/moonlight-web:latest
    ports:
    - containerPort: 8080
      name: http
    - containerPort: 40000-40100
      protocol: UDP
      name: webrtc
  volumes:
  - name: wolf-storage
    emptyDir: {}  # Ephemeral storage (see filesystem section)
```

**GPU Access in K8s**:
- Use NVIDIA GPU Operator or AMD GPU device plugin
- GPU devices automatically passed to Sysbox containers
- Works same as bare metal Docker

#### K8s Service Discovery

**Wolf instances register via RevDial** (same as bare metal):
- Pod starts → Wolf container connects RevDial to API
- API maintains list of available Wolf pods
- Scheduling uses K8s pod labels for affinity/anti-affinity

**Advantages**:
- K8s handles pod lifecycle, restarts
- Can use K8s node selectors for GPU-equipped nodes
- HPA for Wolf instances based on load

### Filesystem & Data Persistence

**Current Model**: Bind-mount Helix filestore (`/filestore`) from host into sandboxes.

**Problem with Remote Wolf**: Wolf instances on different machines can't access shared `/filestore`.

#### Proposed Model: Ephemeral Sandbox Storage

**Assumption**: Sandbox data is **session-scoped** (ephemeral), not persistent across sandbox restarts.

**Implementation**:
- **Wolf's dockerd storage**: Persistent volume (survives Wolf restart)
- **Sandbox containers**: Use Wolf's dockerd storage (ephemeral, dies with container)
- **No /filestore mount**: Sandboxes rely on local disk only

**Data Flow**:
```
User uploads file via API
  ↓
API stores in object storage (S3, Minio)
  ↓
Sandbox downloads from object storage URL (via RevDial proxy)
  ↓
Sandbox processes file locally (ephemeral storage)
  ↓
Sandbox uploads result to object storage (via RevDial proxy)
  ↓
API retrieves result from object storage
```

**Advantages**:
- No shared filesystem required
- Wolf instances fully independent
- Scales to multi-region deployments

**Filesystem Requirements**:
- **Wolf host**: >= 100GB for dockerd storage (images, container layers)
- **Per sandbox**: ~10-20GB ephemeral storage (varies by workload)
- **No special filesystem**: ext4, xfs, overlay2 (standard Docker storage drivers)

**K8s Persistent Volumes**:
```yaml
volumes:
- name: wolf-storage
  persistentVolumeClaim:
    claimName: wolf-pvc  # 500GB PVC for Wolf's dockerd
```

**Bare Metal**:
```yaml
volumes:
- name: wolf-storage
  hostPath:
    path: /var/lib/wolf-docker  # Dedicated directory per Wolf instance
    type: DirectoryOrCreate
```

### Alternative: Firecracker (if Sysbox insufficient)

If Sysbox doesn't meet requirements (e.g., need Docker-in-Docker-in-Docker for level 3):
- **Firecracker microVMs**: Lightweight VMs (not containers), no nesting limits
- **Tradeoff**: Higher overhead than Sysbox, but full kernel isolation
- **Use case**: If devcontainers MUST run Docker (extremely rare requirement)

**Recommendation**: Start with Sysbox, migrate to Firecracker only if proven necessary.

---

## Open Questions

1. **RevDial connection pooling**: Should we reuse RevDial connections or create per-request connections?
2. **Wolf instance failure handling**: What happens to active sandboxes if Wolf instance goes offline? (Likely: active sessions disconnect, need migration strategy)
3. **Geographic distribution**: Do we need latency-aware scheduling for Wolf instances? (Probably yes for multi-region deployments)
4. **Sysbox GPU compatibility**: Does Sysbox work with NVIDIA runtime (`nvidia-docker`) or only device pass-through? (Needs testing, likely device pass-through only)
5. **RevDial WebSocket proxy**: How to efficiently proxy browser WebSocket connections through RevDial? (May need dedicated proxy goroutine per session)

## Implementation Plan

### Recommended Phasing Strategy

Given the complexity, we'll tackle in **3 parallel tracks** that can be worked on independently:

#### Track 1: Sysbox Foundation (CRITICAL PATH - Start First)
**Goal**: Remove Docker socket dependency from Wolf

**Why critical**: Blocks all other work, affects both bare metal and K8s deployments

**Steps**:
1. **Week 1**: Sysbox bare metal POC
   - Install Sysbox on dev Wolf host
   - Modify Wolf Dockerfile: install dockerd, add entrypoint script
   - Test Wolf with `--runtime=sysbox-runc`
   - Verify GPU pass-through: `/dev/dri`, `/dev/nvidia*` (or `/dev/kfd` for AMD)
   - Create test sandbox inside Wolf's dockerd
   - **Success criteria**: Screenshot server works, Sway renders correctly

2. **Week 2**: Sysbox production hardening
   - Update install.sh to install Sysbox automatically (Ubuntu/Debian/Fedora)
   - Test on AMD GPU system (verify `/dev/kfd` pass-through)
   - Document Sysbox installation for manual setups
   - Load test: 10+ concurrent sandboxes in Wolf
   - **Success criteria**: Stable multi-sandbox operation, no GPU access errors

#### Track 2: RevDial Infrastructure (Can Start in Parallel)
**Goal**: Enable remote Wolf instances behind NAT

**Dependencies**: None initially, but needs Track 1 complete for full integration

**Steps**:
1. **Week 1-2**: RevDial proof of concept
   - Add RevDial client to Wolf container (startup connection to API)
   - Add RevDial listener to API server (accept Wolf connections)
   - Test basic connectivity: API → RevDial → Wolf API calls
   - **Success criteria**: Can create Wolf app via RevDial tunnel

2. **Week 3**: Multi-instance support
   - Add Wolf instance registry (database: `WolfInstance` table)
   - Create Wolf instance CRUD endpoints (register, heartbeat, deregister)
   - Implement round-robin scheduling algorithm
   - **Success criteria**: API can route to multiple Wolf instances

3. **Week 4**: Sandbox RevDial tunnels
   - Modify Sway image: include RevDial client binary
   - Start sandbox RevDial on container boot (before Sway)
   - Route screenshot/clipboard requests via RevDial
   - Test end-to-end: Browser → API → RevDial → Sandbox HTTP server
   - **Success criteria**: Screenshots work with remote Wolf instance

4. **Week 5**: Moonlight Web RevDial
   - Add RevDial client to Moonlight Web container
   - Implement WebSocket proxy over RevDial
   - Test browser streaming: Browser → API → RevDial → Moonlight Web → WebRTC
   - **Success criteria**: Full streaming session works with remote Wolf

#### Track 3: Kubernetes Support (Can Start After Track 1 Week 1)
**Goal**: Deploy Wolf in K8s clusters

**Dependencies**: Needs Sysbox basics from Track 1, can proceed in parallel with Track 2

**Steps**:
1. **Week 2-3**: K8s RuntimeClass setup
   - Document K8s node setup (install Sysbox on nodes)
   - Create RuntimeClass YAML definition
   - Test Wolf pod with `runtimeClassName: sysbox-runc`
   - Verify GPU access via NVIDIA GPU Operator / AMD device plugin
   - **Success criteria**: Wolf pod starts, sandboxes work in K8s

2. **Week 4**: K8s integration with RevDial
   - Wolf pod connects RevDial to API on startup
   - Test pod lifecycle: restart, scheduling, GPU affinity
   - Document K8s deployment manifests (Deployment, Service, PVC)
   - **Success criteria**: K8s Wolf instances register and accept sessions

### Which Major Part to Do First?

**Two major components**:
1. **DinD Wolf (Simple bind-mount)**: Remove host Docker socket dependency
2. **RevDial**: Split Wolf/Moonlight Web to separate hardware

**RECOMMENDATION: Do DinD Wolf First** (Updated with simpler approach!)

**Why**:
- **Critical path blocker**: RevDial can't be production-deployed without DinD (breaks K8s)
- **Much simpler than expected**: No Sysbox, just privileged container + dockerd
- **De-risks architecture early**: Validate 2-level nesting works with GPU before investing in RevDial
- **Enables both bare metal and K8s**: Once DinD works, both deployment models become viable
- **Smaller scope**: 1-2 days (!) vs 5+ weeks for full RevDial
- **Lower complexity**: Just add dockerd to Wolf image + bind-mount socket in sandboxes

**Updated approach eliminates Sysbox complexity**:
- ❌ OLD: Sysbox runtime (complex, GPU compatibility unknown)
- ✅ NEW: Privileged container with dockerd (proven, simple)
- Devcontainers run as siblings (not nested) → only 2 nesting levels

**RevDial without DinD**:
- Can prototype RevDial with current Wolf (mount host Docker socket)
- But can't deploy to production (security risk, breaks K8s)

**DinD without RevDial**:
- Immediately deployable (Wolf on same machine as API)
- Unblocks K8s deployments
- Unblocks restricted environments (no host Docker socket access)
- RevDial can be added later without touching DinD code

**Decision**: Start Track 1 (DinD Wolf) - should take 1-2 days, not weeks!

### Quick Win Path (Minimum Viable)

If you need something working ASAP, focus on **Track 1 only**:

**1-2 Days** (updated with simpler approach!):
- Day 1: Add dockerd to Wolf Dockerfile, create entrypoint script, test locally
- Day 2: Update sandbox creation to bind-mount socket, test devcontainers
- **Result**: Wolf no longer needs host Docker socket, can run in K8s!

Then add Track 2 (RevDial) later when remote Wolf instances become a priority.

### Full Feature Timeline

**Parallel execution** (Teams A, B, C working simultaneously):
- **Weeks 1-2**: Track 1 complete (Sysbox), Track 2 POC (RevDial basics), Track 3 pending
- **Weeks 3-4**: Track 2 multi-instance + sandbox RevDial, Track 3 K8s setup
- **Week 5**: Track 2 Moonlight Web RevDial, Track 3 K8s + RevDial integration
- **Week 6**: Integration testing, bug fixes, documentation

**Total**: 6 weeks for full distributed Wolf architecture (with parallel teams)
**Solo**: 10-12 weeks (sequential implementation)

### Risk Mitigation

**Sysbox GPU compatibility unknown**: Test ASAP (Track 1 Week 1). If fails, pivot to Firecracker (adds 2-3 weeks).

**RevDial WebSocket proxy complexity**: If browser streaming breaks, add Track 2.5 (dedicated WebSocket reverse proxy, 1 week).

**K8s Sysbox installation friction**: If enterprise K8s clusters block Sysbox, provide DaemonSet installer (adds 1 week to Track 3).
