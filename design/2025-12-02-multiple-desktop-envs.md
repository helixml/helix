# Multiple Desktop Environments: Unified Hot-Reload System

**Date:** 2025-12-02
**Status:** Implemented
**Author:** Claude (with Kai)

## Overview

This document describes the unified desktop image management system that supports multiple desktop environments (Sway, Zorin, Ubuntu) with hot-reload capability in development and consistent behavior in production.

### Goals

1. **Content-addressable versioning**: Use Docker image hashes instead of git commit hashes
2. **Generic desktop support**: Single set of functions for any desktop type
3. **Dynamic discovery**: Heartbeat discovers and reports all available desktops
4. **Unified behavior**: Same code path for production (pre-baked) and development (hot-reload)
5. **Easy extensibility**: Adding a new desktop requires only Dockerfile + config directory

---

## Architecture Overview

### Component Hierarchy

```
Host Docker
└── Sandbox Container (helix-sandbox)
    └── Wolf Streaming Server + Docker-in-Docker
        └── Desktop Container (helix-sway, helix-zorin, helix-ubuntu)
            └── Docker CLI (for AI agent use)
```

**Key components:**
- **Wolf:** Streaming server (Moonlight protocol) that creates virtual desktops on-demand
- **GOW (Games on Whales):** Base Docker images with display/audio infrastructure
- **Desktop Container:** The actual desktop environment (Sway, Zorin, Ubuntu)
- **Zed:** Editor with AI agent integration

### Data Flow

```
┌─────────────────────────────────────────────────────────────────────────┐
│ ./stack build-desktop <name>  (e.g., sway, zorin, ubuntu)               │
│   │                                                                     │
│   ├── docker build -f Dockerfile.<name>-helix -t helix-<name>:latest .  │
│   ├── IMAGE_HASH=$(docker images helix-<name>:latest --format '{{.ID}}' │
│   │               | sed 's/sha256://')                                  │
│   ├── docker save helix-<name>:latest > helix-<name>.tar                │
│   ├── echo "${IMAGE_HASH}" > helix-<name>.version                       │
│   │                                                                     │
│   └── transfer-desktop-to-sandbox <name>                                │
│       └── docker save helix-<name>:latest |                             │
│           docker exec -i helix-sandbox-1 docker load                    │
│           (image hash preserved automatically)                          │
└─────────────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────────────┐
│ Sandbox Container                                                       │
│   │                                                                     │
│   ├── Production: /opt/images/helix-<name>.version baked in at build    │
│   └── Development: bind-mounted from host (hot-reload)                  │
│                                                                         │
│   Files per desktop:                                                    │
│     /opt/images/helix-sway.version    → "a1b2c3d4e5f6"                  │
│     /opt/images/helix-zorin.version   → "d4e5f6g7h8i9"                  │
│     /opt/images/helix-ubuntu.version  → "j1k2l3m4n5o6"                  │
└─────────────────────────────────────────────────────────────────────────┘
                              │
                              ▼ (every 30s)
┌─────────────────────────────────────────────────────────────────────────┐
│ sandbox-heartbeat (Go binary)                                           │
│   │                                                                     │
│   ├── Scan: /opt/images/helix-*.version                                 │
│   └── Build map: {"sway": "a1b2c3...", "zorin": "d4e5f6...", ...}       │
└─────────────────────────────────────────────────────────────────────────┘
                              │
                              ▼ POST /api/v1/wolf-instances/{id}/heartbeat
┌─────────────────────────────────────────────────────────────────────────┐
│ API Server                                                              │
│   │                                                                     │
│   └── Store in WolfInstance.DesktopVersions (JSON map in DB)            │
└─────────────────────────────────────────────────────────────────────────┘
                              │
                              ▼ (when launching desktop)
┌─────────────────────────────────────────────────────────────────────────┐
│ wolf_executor.go                                                        │
│   │                                                                     │
│   ├── desktopType := getDesktopTypeFromEnv()  // "sway", "zorin", etc.  │
│   ├── version := wolfInstance.DesktopVersions[desktopType]              │
│   └── docker run ${version}  // Uses image hash directly                │
└─────────────────────────────────────────────────────────────────────────┘
```

### Why Docker Image Hashes?

| Aspect | Git Commit Hash | Docker Image Hash |
|--------|-----------------|-------------------|
| Requires git commit | Yes | No |
| Reflects actual content | No (stale if uncommitted) | Yes |
| Survives docker save/load | N/A (external) | Yes (intrinsic) |
| Works in production | Needs embedded file | Hash is part of image |
| Universal identifier | No | Yes |

Docker image hashes are content-addressable SHA256 hashes. They:
- Are computed from image layers and configuration
- Survive `docker save | docker load` operations unchanged
- Can be used directly as image references: `docker run a1b2c3d4e5f6`

---

## Display System: X11 vs Wayland

### The Complexity

Linux has two display systems:
- **X11:** Legacy, mature, broad compatibility (uses Xorg/Xwayland)
- **Wayland:** Modern, better performance, simpler architecture

**In GOW/Wolf containers:**
- GOW provides a custom Wayland compositor (`gst-wayland-display`)
- X11 applications run via **Xwayland** (X11 compatibility layer on Wayland)
- Pure Wayland apps connect directly to the compositor

### Existing Implementations

| Desktop | Base Image | Display | Clipboard | Screenshot |
|---------|-----------|---------|-----------|------------|
| Sway | `ghcr.io/games-on-whales/base-app:edge` | Native Wayland | `wl-clipboard` | `grim` |
| Zorin | `ghcr.io/mollomm1/gow-zorin-18:latest` | X11 via Xwayland | `xclip` | `scrot` |

### Recommendation for New Desktops

**Use X11 via Xwayland** (like Zorin) unless you have a specific need for native Wayland:
- GNOME has native Wayland support but is complex to configure in containers
- X11 is simpler and more reliable in GOW's nested compositor architecture
- Use `xclip`/`scrot` for clipboard/screenshot tools

---

## Adding a New Desktop: Step-by-Step Guide

### Prerequisites

Before adding a new desktop, ensure you have:
1. A working Helix development environment (`./stack start`)
2. Understanding of Docker multi-stage builds
3. Familiarity with the target desktop environment (GNOME, KDE, XFCE, etc.)

### Files to Create

| File | Purpose |
|------|---------|
| `Dockerfile.<name>-helix` | Build the desktop image |
| `wolf/<name>-config/` | Desktop-specific scripts |
| `stack` (build-sandbox) | Add build step |
| `wolf_executor.go` | Add type constant |

**No changes needed:**
- `Dockerfile.sandbox` - folder copy (`COPY sandbox-images/ /opt/images/`) handles all desktops automatically
- `docker-compose.dev.yaml` - folder mount (`./sandbox-images:/opt/images:ro`) handles all desktops automatically

### Step 1: Create the Dockerfile

Create `Dockerfile.<name>-helix` in the repository root:

```dockerfile
# ====================================================================
# Go build stage (IDENTICAL for all desktops)
# ====================================================================
FROM golang:1.24 AS go-build-env
WORKDIR /app
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY api ./api
WORKDIR /app/api
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -buildvcs=false -ldflags "-s -w" -o /settings-sync-daemon ./cmd/settings-sync-daemon && \
    CGO_ENABLED=0 go build -buildvcs=false -ldflags "-s -w" -o /screenshot-server ./cmd/screenshot-server && \
    CGO_ENABLED=0 go build -buildvcs=false -ldflags "-s -w" -o /revdial-client ./cmd/revdial-client

# ====================================================================
# Desktop stage
# ====================================================================
FROM ghcr.io/games-on-whales/xfce:edge   # Or custom base image

# Create runtime directory
RUN mkdir -p /run/user/1000 && chmod 700 /run/user/1000

# Install required packages
RUN apt-get update && \
    apt-get install -y \
    sudo \
    grim \
    firefox \
    && rm -rf /var/lib/apt/lists/*

# Setup sudo access for retro user
RUN echo "%sudo ALL=(ALL) NOPASSWD:ALL" >> /etc/sudoers && \
    echo "retro ALL=(ALL) NOPASSWD:ALL" >> /etc/sudoers

# Copy Go binaries
COPY --from=go-build-env /settings-sync-daemon /usr/local/bin/settings-sync-daemon
COPY --from=go-build-env /screenshot-server /usr/local/bin/screenshot-server
COPY --from=go-build-env /revdial-client /usr/local/bin/revdial-client

# Copy Zed editor
RUN mkdir -p /zed-build
COPY zed-build/zed /zed-build/zed
RUN chmod +x /zed-build/zed

# Copy desktop-specific config
ADD wolf/<name>-config/start-zed-helix.sh /usr/local/bin/start-zed-helix.sh
RUN chmod +x /usr/local/bin/start-zed-helix.sh

ADD wolf/<name>-config/startup-app.sh /opt/gow/startup-app.sh
RUN chmod +x /opt/gow/startup-app.sh

# Copy wallpaper
COPY wolf/assets/images/helix_hero.png /usr/share/backgrounds/helix-hero.png
```

**Required Components Checklist:**

- [ ] Go build stage with all 3 binaries (settings-sync-daemon, screenshot-server, revdial-client)
- [ ] Base image selection (GOW-compatible)
- [ ] XDG_RUNTIME_DIR setup
- [ ] systemd-logind disable (if GNOME)
- [ ] Passwordless sudo for retro/user
- [ ] Screenshot tool: `scrot` (X11) or `grim` (Wayland)
- [ ] Firefox browser
- [ ] Docker CLI + docker-compose-plugin
- [ ] Docker wrappers (Hydra compatibility)
- [ ] Clipboard tools: `xclip`/`xsel` (X11) or `wl-clipboard` (Wayland)
- [ ] git + openssh-client
- [ ] Go binary COPY commands
- [ ] Zed binary COPY
- [ ] Startup scripts (startup-app.sh, start-zed-helix.sh)
- [ ] Helper scripts (helix-specs-create.sh)
- [ ] cont-init.d script (16-add-docker-group.sh)
- [ ] Background wallpaper

### Step 2: Create Configuration Directory

Create `wolf/<name>-config/` with the required scripts:

| File | Purpose | Copy From |
|------|---------|-----------|
| `startup-app.sh` | GOW startup, launches desktop | Modify for desktop-specific launcher |
| `start-zed-helix.sh` | Zed setup, git cloning, workspace | Copy from sway-config (most complete) |
| `docker-wrapper.sh` | Hydra symlink resolution | Copy as-is from sway-config |
| `docker-compose-wrapper.sh` | Hydra compose resolution | Copy as-is from sway-config |
| `helix-specs-create.sh` | Design docs worktree | Copy as-is from sway-config |
| `16-add-docker-group.sh` | Runtime Docker group | Copy as-is from sway-config |

**startup-app.sh differences by desktop:**

**Sway (Wayland):**
```bash
# Sources GOW's launch-comp.sh
source /opt/gow/launch-comp.sh
custom_launcher /usr/local/bin/start-zed-helix.sh
# Uses dbus-run-session -- sway --unsupported-gpu
```

**Zorin/GNOME (X11 via Xwayland):**
```bash
# Uses GOW's xorg.sh script
exec /opt/gow/xorg.sh
# GNOME autostart entries launch apps
# IBus must be disabled (keyboard issues)
# Screensaver proxy must be disabled
```

**XFCE (X11 via Xwayland):**
Would be similar to Zorin but simpler (XFCE doesn't need systemd workarounds)

### Step 3: Update Stack Script

Add build command wrapper to `stack`:

```bash
function build-<name>() {
    build-desktop <name>
}
```

Update `build-sandbox` to include the new desktop:

```bash
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "📝 [X/Y] Building helix-<name> and exporting tarball..."
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
build-desktop <name>
if [ ! -f sandbox-images/helix-<name>.tar ]; then
    echo "❌ sandbox-images/helix-<name>.tar not found after build - this shouldn't happen"
    rm -f sandbox-images/helix-*.tar
    exit 1
fi
echo "✅ Using sandbox-images/helix-<name>.tar ($(du -h sandbox-images/helix-<name>.tar | cut -f1)) version=$(cat sandbox-images/helix-<name>.version)"
```

### Step 4: Add Desktop Type Constant

In `api/pkg/external-agent/wolf_executor.go`:

```go
const (
    DesktopSway   DesktopType = "sway"
    DesktopZorin  DesktopType = "zorin"
    DesktopUbuntu DesktopType = "ubuntu"  // Add new type
)
```

Add to `parseDesktopType()`:
```go
case "<name>":
    return Desktop<Name>
```

Add to `computeZedImageFromVersion()`:
```go
case Desktop<Name>:
    prefix = "helix-<name>"
```

---

## Required Features Checklist

Every desktop MUST implement these features:

### CRITICAL (API Communication)
- [ ] **RevDial Client** - Start before desktop for API <-> sandbox communication
- [ ] **WORKSPACE_DIR Symlink** - `/home/retro/work` -> `$WORKSPACE_DIR` for Hydra compatibility
- [ ] **Git HTTP Credentials** - Configure `~/.git-credentials` with USER_API_TOKEN
- [ ] **Repository Cloning** - Clone from HELIX_REPOSITORIES env var

### IMPORTANT (Functionality)
- [ ] **Docker CLI + Wrappers** - Allow AI agents to run containers
- [ ] **Docker Group Setup** - Runtime group membership (16-add-docker-group.sh)
- [ ] **helix-specs Worktree** - Design docs branch setup
- [ ] **Project Startup Script** - Execute `.helix/startup.sh` in terminal
- [ ] **Multi-folder Zed Workspace** - Open primary repo + specs + other repos
- [ ] **Clipboard Keybindings** - Ctrl+C/V use system clipboard
- [ ] **Comprehensive Logging** - Output to `~/.helix-startup.log`

### NICE-TO-HAVE
- [ ] **Telemetry Firewall** - Block AI agent phone-home (configured in sandbox, not desktop)
- [ ] **HiDPI Scaling** - 200% scaling for streaming

---

## Environment Variables

These are passed from Wolf to the desktop container:

| Variable | Purpose | Example |
|----------|---------|---------|
| `HELIX_SESSION_ID` | Session identifier | `abc123` |
| `HELIX_API_BASE_URL` | API server URL | `http://api:8080` |
| `USER_API_TOKEN` | User's API token for RBAC | `hlx_...` |
| `HELIX_REPOSITORIES` | Repos to clone | `id1:name1:code,id2:name2:internal` |
| `HELIX_PRIMARY_REPO_NAME` | Primary repo name | `my-project` |
| `WORKSPACE_DIR` | Actual workspace path | `/filestore/workspaces/...` |
| `GIT_USER_NAME` | Git commit name | `Helix Agent` |
| `GIT_USER_EMAIL` | Git commit email | `agent@helix.ml` |

---

## Implementation Details

### Phase 1: Stack Functions (Generic Desktop Build/Transfer)

**File:** `stack`

#### 1.1 Create `build-desktop` function

Replace `build-sway` and `build-zorin` with a single generic function:

```bash
function build-desktop() {
  local DESKTOP_NAME="$1"

  if [ -z "$DESKTOP_NAME" ]; then
    echo "Usage: ./stack build-desktop <name>"
    echo "Available: sway, zorin, ubuntu"
    exit 1
  fi

  local DOCKERFILE="Dockerfile.${DESKTOP_NAME}-helix"
  local IMAGE_NAME="helix-${DESKTOP_NAME}"
  local CONFIG_DIR="wolf/${DESKTOP_NAME}-config"

  # Validate Dockerfile exists
  if [ ! -f "$DOCKERFILE" ]; then
    echo "❌ Dockerfile not found: $DOCKERFILE"
    exit 1
  fi

  echo "🖥️  Building ${DESKTOP_NAME} desktop container..."

  # Build Zed if needed (for desktops that include Zed)
  if [ ! -f "./zed-build/zed" ]; then
    echo "❌ Zed binary not found. Building in release mode first..."
    if ! build-zed release; then
      echo "❌ Failed to build Zed binary"
      exit 1
    fi
  fi

  # Build the desktop image
  echo "🔨 Building ${IMAGE_NAME}:latest..."
  docker build -f "$DOCKERFILE" -t "${IMAGE_NAME}:latest" .

  if [ $? -ne 0 ]; then
    echo "❌ Failed to build ${DESKTOP_NAME} container"
    exit 1
  fi

  # Get Docker image hash (content-addressable, survives save/load)
  local IMAGE_HASH=$(docker images "${IMAGE_NAME}:latest" --format '{{.ID}}' | sed 's/sha256://')

  echo "✅ ${DESKTOP_NAME} container built successfully"
  echo "📦 Image hash: ${IMAGE_HASH}"

  # Export tarball for embedding in sandbox
  mkdir -p sandbox-images
  echo "📦 Exporting ${DESKTOP_NAME} tarball..."
  docker save "${IMAGE_NAME}:latest" > "sandbox-images/${IMAGE_NAME}.tar"
  echo "${IMAGE_HASH}" > "sandbox-images/${IMAGE_NAME}.version"

  local TARBALL_SIZE=$(du -h "sandbox-images/${IMAGE_NAME}.tar" | cut -f1)
  echo "✅ Tarball created: sandbox-images/${IMAGE_NAME}.tar ($TARBALL_SIZE) hash=${IMAGE_HASH}"

  # Transfer to running sandbox (hot-reload in development)
  transfer-desktop-to-sandbox "$DESKTOP_NAME"
}
```

#### 1.2 Create `transfer-desktop-to-sandbox` function

```bash
function transfer-desktop-to-sandbox() {
  local DESKTOP_NAME="$1"
  local IMAGE_NAME="helix-${DESKTOP_NAME}"

  if [ -z "$DESKTOP_NAME" ]; then
    echo "Usage: transfer-desktop-to-sandbox <name>"
    return 1
  fi

  # Check if sandbox container is running
  if ! docker compose -f docker-compose.dev.yaml ps sandbox | grep -q "Up"; then
    echo "ℹ️  Sandbox container not running, skipping image transfer"
    return 0
  fi

  # Check if image exists on host
  if ! docker images "${IMAGE_NAME}:latest" -q | grep -q .; then
    echo "⚠️  ${IMAGE_NAME}:latest not found on host, skipping transfer"
    return 0
  fi

  # Get image hash
  local IMAGE_HASH=$(docker images "${IMAGE_NAME}:latest" --format '{{.ID}}' | sed 's/sha256://')

  echo "📦 Transferring ${IMAGE_NAME}:latest to sandbox's dockerd..."
  if docker save "${IMAGE_NAME}:latest" | docker exec -i helix-sandbox-1 docker load 2>/dev/null; then
    echo "✅ ${IMAGE_NAME}:latest transferred to sandbox's dockerd"
    echo "📦 Image hash: ${IMAGE_HASH} (preserved through transfer)"

    # Version file is bind-mounted in dev mode, so it's already updated
    if [ -f "sandbox-images/${IMAGE_NAME}.version" ]; then
      echo "✅ Version file sandbox-images/${IMAGE_NAME}.version contains: $(cat sandbox-images/${IMAGE_NAME}.version)"
    fi
  else
    echo "ℹ️  Could not transfer image to sandbox (container may be starting/restarting)"
  fi
}
```

#### 1.3 Wrapper functions for backward compatibility

```bash
function build-sway() {
  build-desktop sway
}

function build-zorin() {
  build-desktop zorin
}

function build-ubuntu() {
  build-desktop ubuntu
}
```

### Phase 2: Docker Compose (Bind Mounts)

**File:** `docker-compose.dev.yaml`

Bind mount the entire sandbox-images folder:

```yaml
services:
  sandbox:
    volumes:
      # ... existing volumes ...
      # Desktop image tarballs and versions (bind-mounted for hot-reload)
      - ./sandbox-images:/opt/images:ro
```

### Phase 3: Sandbox Heartbeat (Dynamic Discovery)

**File:** `api/cmd/sandbox-heartbeat/main.go`

```go
// discoverDesktopVersions scans for all desktop version files
// and returns a map of desktop name -> image hash
func discoverDesktopVersions() map[string]string {
    versions := make(map[string]string)

    // Scan for all version files matching pattern
    files, err := filepath.Glob("/opt/images/helix-*.version")
    if err != nil {
        log.Warn().Err(err).Msg("Failed to scan for desktop version files")
        return versions
    }

    for _, file := range files {
        // Extract desktop name from filename
        // e.g., "/opt/images/helix-sway.version" -> "sway"
        base := filepath.Base(file)                    // "helix-sway.version"
        name := strings.TrimPrefix(base, "helix-")     // "sway.version"
        name = strings.TrimSuffix(name, ".version")    // "sway"

        // Read version (image hash)
        data, err := os.ReadFile(file)
        if err != nil {
            log.Warn().Err(err).Str("file", file).Msg("Failed to read version file")
            continue
        }

        version := string(bytes.TrimSpace(data))
        if version != "" {
            versions[name] = version
            log.Debug().
                Str("desktop", name).
                Str("version", version).
                Msg("Discovered desktop version")
        }
    }

    return versions
}
```

### Phase 4: API Types (Map-based Versions)

**File:** `api/pkg/types/wolf_instance.go`

```go
// WolfInstance represents a Wolf streaming instance
type WolfInstance struct {
    // ... existing fields ...
    // Desktop versions stored as JSON map
    // e.g., {"sway": "a1b2c3...", "zorin": "d4e5f6..."}
    DesktopVersionsJSON   string    `gorm:"type:text" json:"-"`
}

// GetDesktopVersions parses the JSON and returns the map
func (w *WolfInstance) GetDesktopVersions() map[string]string {
    if w.DesktopVersionsJSON == "" {
        return nil
    }
    var versions map[string]string
    if err := json.Unmarshal([]byte(w.DesktopVersionsJSON), &versions); err != nil {
        return nil
    }
    return versions
}

// GetDesktopVersion returns the version for a specific desktop type
func (w *WolfInstance) GetDesktopVersion(desktopType string) string {
    versions := w.GetDesktopVersions()
    if versions == nil {
        return ""
    }
    return versions[desktopType]
}
```

### Phase 5: Store (Save Desktop Versions)

**File:** `api/pkg/store/store_wolf_instance.go`

```go
func (s *PostgresStore) UpdateWolfHeartbeat(ctx context.Context, id string, req *types.WolfHeartbeatRequest) error {
    updates := map[string]interface{}{
        "last_heartbeat": time.Now(),
        "status":         types.WolfInstanceStatusOnline,
    }

    // Save desktop versions as JSON
    if req != nil && len(req.DesktopVersions) > 0 {
        versionsJSON, err := json.Marshal(req.DesktopVersions)
        if err == nil {
            updates["desktop_versions_json"] = string(versionsJSON)
        }
    }

    // ... rest of existing code ...
}
```

### Phase 6: Wolf Executor (Use Desktop Version)

**File:** `api/pkg/external-agent/wolf_executor.go`

```go
// computeZedImageFromVersion returns the Docker image hash for the given desktop type
func (w *WolfExecutor) computeZedImageFromVersion(desktopType DesktopType, wolfInstance *types.WolfInstance) string {
    if wolfInstance == nil {
        return "" // Fall back to default w.zedImage
    }

    // Get version (image hash) for this desktop type
    version := wolfInstance.GetDesktopVersion(string(desktopType))
    if version == "" {
        return "" // Fall back to default w.zedImage
    }

    // Return the image hash directly - Docker can run images by hash
    return version
}
```

---

## Build and Test Process

### 1. Commit Before Building

```bash
git add -A && git commit -m "feat(<name>): add <name> desktop environment"
```

### 2. Build the Image

```bash
# Build just the desktop image
./stack build-desktop <name>

# Or build everything including the sandbox
./stack build-sandbox
```

### 3. Verify Version File

```bash
cat sandbox-images/helix-<name>.version
# Should output a 12-character Docker image hash like: a1b2c3d4e5f6
```

### 4. Check Heartbeat Discovery

```bash
# View sandbox heartbeat logs
docker compose -f docker-compose.dev.yaml logs sandbox | grep -i desktop

# Check wolf instance in API
curl -s http://localhost:8080/api/v1/wolf/instances | jq '.[0].desktop_versions'
```

Expected output:
```json
{
  "sway": "e3047008385c",
  "zorin": "2954a89ed294",
  "ubuntu": "a1b2c3d4e5f6"
}
```

### 5. Test Desktop Launch

```bash
# Set environment variable
export HELIX_DESKTOP=<name>

# Restart API to pick up change
docker compose -f docker-compose.dev.yaml down api
docker compose -f docker-compose.dev.yaml up -d api

# Create external agent session and verify
```

### 6. Test Docker Access

Inside the desktop container:
```bash
docker run hello-world
docker compose version
```

### 7. Test Clipboard

- Copy in Zed with Ctrl+C
- Paste in Firefox with Ctrl+V
- Verify bidirectional sync

### Testing Checklist

- [ ] `./stack build-desktop sway` builds and creates tarball with image hash
- [ ] `./stack build-desktop zorin` builds and creates tarball with image hash
- [ ] Image hash survives `docker save | docker load` (verify hash is identical)
- [ ] Heartbeat discovers all version files dynamically
- [ ] API receives and stores desktop versions map
- [ ] Wolf executor uses correct image hash for selected desktop type
- [ ] Hot-reload: changing desktop code -> rebuild -> sandbox picks up new hash
- [ ] Production: pre-baked images work identically

---

## Debugging & Troubleshooting

### Quick Reference: Debug a Failed Desktop Container

**If a user says "I launched a {sway|zorin|ubuntu} container and it failed", run these commands:**

```bash
# Step 1: Get sandbox container ID
SANDBOX_ID=$(docker ps | grep "helix-sandbox" | awk '{print $1}')

# Step 2: Find the most recent container for that desktop type
# Replace "ubuntu" with "sway" or "zorin" as needed
DESKTOP_TYPE="ubuntu"
CONTAINER_ID=$(docker exec $SANDBOX_ID docker ps -a --format '{{.ID}} {{.Image}}' | grep "helix-${DESKTOP_TYPE}" | head -1 | awk '{print $1}')

# Step 3: Get the logs
docker exec $SANDBOX_ID docker logs $CONTAINER_ID
```

**One-liner for each desktop type:**

```bash
# Ubuntu
docker exec $(docker ps | grep "helix-sandbox" | awk '{print $1}') docker logs $(docker exec $(docker ps | grep "helix-sandbox" | awk '{print $1}') docker ps -a --format '{{.ID}} {{.Image}}' | grep "helix-ubuntu" | head -1 | awk '{print $1}')

# Sway
docker exec $(docker ps | grep "helix-sandbox" | awk '{print $1}') docker logs $(docker exec $(docker ps | grep "helix-sandbox" | awk '{print $1}') docker ps -a --format '{{.ID}} {{.Image}}' | grep "helix-sway" | head -1 | awk '{print $1}')

# Zorin
docker exec $(docker ps | grep "helix-sandbox" | awk '{print $1}') docker logs $(docker exec $(docker ps | grep "helix-sandbox" | awk '{print $1}') docker ps -a --format '{{.ID}} {{.Image}}' | grep "helix-zorin" | head -1 | awk '{print $1}')
```

---

### Keeping Dead Containers for Debugging

By default, Wolf removes containers after they exit (production behavior). To preserve exited containers for debugging:

```bash
# In your .env file or export before starting:
export HELIX_KEEP_DEAD_CONTAINERS=true

# Restart the sandbox to apply:
docker compose -f docker-compose.dev.yaml down sandbox
docker compose -f docker-compose.dev.yaml up -d sandbox
```

When enabled, you'll see this in sandbox logs:
```
🔧 Debug mode: HELIX_KEEP_DEAD_CONTAINERS=true - dead containers will be preserved
```

---

### Step-by-Step Debugging Guide

#### 1. Get the Sandbox Container ID

```bash
SANDBOX_ID=$(docker ps | grep "helix-sandbox" | awk '{print $1}')
echo "Sandbox container: $SANDBOX_ID"
```

#### 2. List All Desktop Containers

```bash
# List all containers (including exited) inside the sandbox
docker exec $SANDBOX_ID docker ps -a

# Expected output - image names show desktop type:
# CONTAINER ID   IMAGE                                       COMMAND           STATUS
# c90a0a7b7bac   ghcr.io/games-on-whales/pulseaudio:master   "/entrypoint.sh"  Up
# b8b8ba03c012   helix-sway:4d368a6b01e9                     "/entrypoint.sh"  Exited (1)
# 08f0484c1ab7   helix-ubuntu:abc123def456                   "/entrypoint.sh"  Exited (255)
# a1b2c3d4e5f6   helix-zorin:3976eb50ff70                    "/entrypoint.sh"  Up
```

#### 3. Find Containers by Desktop Type

```bash
# Find all Ubuntu containers
docker exec $SANDBOX_ID docker ps -a --format '{{.ID}} {{.Image}} {{.Status}}' | grep "helix-ubuntu"

# Find all Sway containers
docker exec $SANDBOX_ID docker ps -a --format '{{.ID}} {{.Image}} {{.Status}}' | grep "helix-sway"

# Find all Zorin containers
docker exec $SANDBOX_ID docker ps -a --format '{{.ID}} {{.Image}} {{.Status}}' | grep "helix-zorin"

# Get just the most recent container ID for a desktop type
CONTAINER_ID=$(docker exec $SANDBOX_ID docker ps -a --format '{{.ID}} {{.Image}}' | grep "helix-ubuntu" | head -1 | awk '{print $1}')
echo "Most recent Ubuntu container: $CONTAINER_ID"
```

#### 4. Get Container Logs

```bash
# Full logs
docker exec $SANDBOX_ID docker logs $CONTAINER_ID

# Last 100 lines
docker exec $SANDBOX_ID docker logs --tail 100 $CONTAINER_ID

# Last 100 lines with timestamps
docker exec $SANDBOX_ID docker logs --tail 100 --timestamps $CONTAINER_ID

# Follow logs (for running containers)
docker exec $SANDBOX_ID docker logs -f $CONTAINER_ID
```

#### 5. Inspect Container Configuration

```bash
# Full container config
docker exec $SANDBOX_ID docker inspect $CONTAINER_ID

# Just environment variables (pipe to jq for readability)
docker exec $SANDBOX_ID docker inspect --format '{{json .Config.Env}}' $CONTAINER_ID | jq .

# Just mounts
docker exec $SANDBOX_ID docker inspect --format '{{json .Mounts}}' $CONTAINER_ID | jq .

# Exit code
docker exec $SANDBOX_ID docker inspect --format '{{.State.ExitCode}}' $CONTAINER_ID

# Error message (if any)
docker exec $SANDBOX_ID docker inspect --format '{{.State.Error}}' $CONTAINER_ID
```

#### 6. Check Available Desktop Images

```bash
# List all desktop images and their tags
docker exec $SANDBOX_ID docker images | grep helix-

# Expected output showing both :latest and :hash tags:
# REPOSITORY     TAG              IMAGE ID       CREATED        SIZE
# helix-sway     4d368a6b01e9     4d368a6b01e9   2 days ago     2.1GB
# helix-sway     latest           4d368a6b01e9   2 days ago     2.1GB
# helix-ubuntu   abc123def456     abc123def456   1 hour ago     2.5GB
# helix-ubuntu   latest           abc123def456   1 hour ago     2.5GB
```

#### 7. Interactive Shell in the Sandbox

```bash
# Get a shell inside the sandbox container
docker exec -it $SANDBOX_ID bash

# From there you can run docker commands directly:
docker ps -a
docker logs <container_id>
docker images
```

---

### Image Naming Convention

Desktop container images follow this naming pattern:

```
helix-{desktop}:{image_hash}
```

| Desktop | Image Name Example |
|---------|-------------------|
| Sway | `helix-sway:4d368a6b01e9` |
| Zorin | `helix-zorin:3976eb50ff70` |
| Ubuntu | `helix-ubuntu:abc123def456` |

The image hash is a 12-character Docker image ID that uniquely identifies the build. This allows you to:
- Identify which desktop type a container is running
- Match containers to specific image builds
- Grep for desktop type in `docker ps` output

### Common Issues and Solutions

#### Issue: GNOME won't start
**Cause:** systemd-logind not available in containers
**Solution:** Disable login1 files (see Zorin Dockerfile line 28-30)

#### Issue: Keyboard modifiers stuck
**Cause:** IBus input method framework interference
**Solution:** Disable IBus in startup-app.sh:
```bash
export GTK_IM_MODULE=gtk-im-context-simple
export QT_IM_MODULE=gtk-im-context-simple
export XMODIFIERS=@im=none
```

#### Issue: Docker bind mounts fail with Hydra
**Cause:** Docker CLI resolves symlinks, Hydra uses different dockerd
**Solution:** Use docker-wrapper.sh to resolve symlinks before Docker

#### Issue: Clipboard not syncing
**Cause:** Wrong clipboard tool for display system
**Solution:** Use `xclip` for X11, `wl-clipboard` for Wayland

#### Issue: Screensaver notification spam
**Cause:** gsd-screensaver-proxy detects missing GDM
**Solution:** Disable via autostart override (see Zorin startup-app.sh)

#### Build Fails: "helix-<name>.tar: Is a directory"

Docker created an empty directory when bind mount source didn't exist:
```bash
sudo rm -rf sandbox-images/helix-<name>.tar sandbox-images/helix-<name>.version
./stack build-sandbox
```

#### Desktop Not Appearing in Heartbeat

1. Check the version file exists in sandbox:
   ```bash
   docker exec sandbox-1 ls -la /opt/images/helix-*.version
   ```

2. Check heartbeat logs:
   ```bash
   docker exec sandbox-1 cat /var/log/sandbox-heartbeat.log
   ```

#### Image Not Loading in Inner Docker

1. Check tarball was copied:
   ```bash
   docker exec sandbox-1 ls -la /opt/images/helix-<name>.tar
   ```

2. Check inner Docker loaded it:
   ```bash
   docker exec sandbox-1 docker images helix-<name>
   ```

---

## Migration Notes

### Backward Compatibility

The API should accept both old format (individual fields) and new format (map) during transition:

```go
// In UpdateWolfHeartbeat, handle both formats:
if req.DesktopVersions != nil {
    // New format: use map directly
    updates["desktop_versions_json"] = marshal(req.DesktopVersions)
} else if req.SwayVersion != "" || req.ZorinVersion != "" {
    // Old format: convert to map
    versions := map[string]string{}
    if req.SwayVersion != "" {
        versions["sway"] = req.SwayVersion
    }
    if req.ZorinVersion != "" {
        versions["zorin"] = req.ZorinVersion
    }
    updates["desktop_versions_json"] = marshal(versions)
}
```

### Database Migration

GORM AutoMigrate will handle adding the new `desktop_versions_json` column. The old `sway_version` column can be kept for a transition period, then removed.

---

## References

- [Games on Whales GitHub](https://github.com/games-on-whales/gow)
- [Wolf Streaming Server](https://github.com/games-on-whales/wolf)
- [x11docker - GUI apps in Docker](https://github.com/mviereck/x11docker)
- [Dockerfile-Ubuntu-Gnome](https://github.com/darkdragon-001/Dockerfile-Ubuntu-Gnome)
- [Running Wayland compositor in Docker](https://unix.stackexchange.com/questions/756867/running-a-wayland-compositor-inside-docker)
