# Multiple Desktop Environments: Unified Hot-Reload System

**Date:** 2025-12-02
**Status:** Plan
**Author:** Claude (with Kai)

## Overview

This document describes the implementation plan for a unified desktop image management system that supports multiple desktop environments (Sway, Zorin, Ubuntu) with hot-reload capability in development and consistent behavior in production.

## Goals

1. **Content-addressable versioning**: Use Docker image hashes instead of git commit hashes
2. **Generic desktop support**: Single set of functions for any desktop type
3. **Dynamic discovery**: Heartbeat discovers and reports all available desktops
4. **Unified behavior**: Same code path for production (pre-baked) and development (hot-reload)
5. **Easy extensibility**: Adding a new desktop requires only Dockerfile + config directory

## Architecture

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

## Implementation Plan

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
  echo "📦 Exporting ${DESKTOP_NAME} tarball..."
  docker save "${IMAGE_NAME}:latest" > "${IMAGE_NAME}.tar"
  echo "${IMAGE_HASH}" > "${IMAGE_NAME}.version"

  local TARBALL_SIZE=$(du -h "${IMAGE_NAME}.tar" | cut -f1)
  echo "✅ Tarball created: ${IMAGE_NAME}.tar ($TARBALL_SIZE) hash=${IMAGE_HASH}"

  # Transfer to running sandbox (hot-reload in development)
  transfer-desktop-to-sandbox "$DESKTOP_NAME"
}
```

#### 1.2 Create `transfer-desktop-to-sandbox` function

Replace `transfer-sway-to-sandbox` and `transfer-zorin-to-sandbox`:

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
    # Just log confirmation
    if [ -f "${IMAGE_NAME}.version" ]; then
      echo "✅ Version file ${IMAGE_NAME}.version contains: $(cat ${IMAGE_NAME}.version)"
    fi
  else
    echo "ℹ️  Could not transfer image to sandbox (container may be starting/restarting)"
  fi
}
```

#### 1.3 Create wrapper functions for backward compatibility

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

#### 1.4 Update help text

Add to the `help()` function:

```bash
echo "  build-desktop <name> - Build desktop container (sway, zorin, ubuntu)"
```

#### 1.5 Delete commented-out code

Remove lines ~967-1003 (the old commented-out transfer code).

---

### Phase 2: Docker Compose (Bind Mounts)

**File:** `docker-compose.dev.yaml`

Add bind mounts for all desktop version files to all sandbox services (sandbox, sandbox2, sandbox3):

```yaml
services:
  sandbox:
    volumes:
      # ... existing volumes ...
      # Desktop image tarballs and versions (bind-mounted for hot-reload)
      - ./helix-sway.tar:/opt/images/helix-sway.tar:ro
      - ./helix-sway.version:/opt/images/helix-sway.version:ro
      - ./helix-zorin.tar:/opt/images/helix-zorin.tar:ro
      - ./helix-zorin.version:/opt/images/helix-zorin.version:ro
      - ./helix-ubuntu.tar:/opt/images/helix-ubuntu.tar:ro
      - ./helix-ubuntu.version:/opt/images/helix-ubuntu.version:ro
```

**Note:** Files that don't exist will cause Docker to create empty files, which is fine - the heartbeat handles missing files gracefully.

---

### Phase 3: Sandbox Heartbeat (Dynamic Discovery)

**File:** `api/cmd/sandbox-heartbeat/main.go`

#### 3.1 Update HeartbeatRequest struct

Replace individual version fields with a map:

```go
// HeartbeatRequest is the request body sent to the API
type HeartbeatRequest struct {
    // Desktop image versions (content-addressable Docker image hashes)
    // Key: desktop name (e.g., "sway", "zorin", "ubuntu")
    // Value: image hash (e.g., "a1b2c3d4e5f6...")
    DesktopVersions       map[string]string    `json:"desktop_versions,omitempty"`
    DiskUsage             []DiskUsageMetric    `json:"disk_usage,omitempty"`
    ContainerUsage        []ContainerDiskUsage `json:"container_usage,omitempty"`
    PrivilegedModeEnabled bool                 `json:"privileged_mode_enabled,omitempty"`
    GPUVendor             string               `json:"gpu_vendor,omitempty"`
    RenderNode            string               `json:"render_node,omitempty"`
}
```

#### 3.2 Update sendHeartbeat function

Replace hardcoded version file reads with dynamic discovery:

```go
func sendHeartbeat(apiURL, runnerToken, wolfInstanceID string, privilegedModeEnabled bool) {
    // Discover all desktop versions dynamically
    // Scans /opt/images/helix-*.version files
    desktopVersions := discoverDesktopVersions()

    // ... rest of function unchanged ...

    req := HeartbeatRequest{
        DesktopVersions:       desktopVersions,
        DiskUsage:             diskUsage,
        ContainerUsage:        containerUsage,
        PrivilegedModeEnabled: privilegedModeEnabled,
        GPUVendor:             gpuVendor,
        RenderNode:            renderNode,
    }

    // ... send request ...
}

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

---

### Phase 4: API Types (Map-based Versions)

**File:** `api/pkg/types/wolf_instance.go`

#### 4.1 Update WolfHeartbeatRequest

```go
// WolfHeartbeatRequest is the request body for Wolf instance heartbeat
type WolfHeartbeatRequest struct {
    // Desktop image versions (content-addressable Docker image hashes)
    // Key: desktop name (e.g., "sway", "zorin", "ubuntu")
    // Value: image hash (e.g., "a1b2c3d4e5f6...")
    DesktopVersions       map[string]string    `json:"desktop_versions,omitempty"`
    DiskUsage             []DiskUsageMetric    `json:"disk_usage,omitempty"`
    ContainerUsage        []ContainerDiskUsage `json:"container_usage,omitempty"`
    PrivilegedModeEnabled bool                 `json:"privileged_mode_enabled,omitempty"`
    GPUVendor             string               `json:"gpu_vendor,omitempty"`
    RenderNode            string               `json:"render_node,omitempty"`
}
```

#### 4.2 Update WolfInstance model

```go
// WolfInstance represents a Wolf streaming instance
type WolfInstance struct {
    ID                    string    `gorm:"type:varchar(255);primaryKey" json:"id"`
    Name                  string    `gorm:"type:varchar(255);not null" json:"name"`
    Address               string    `gorm:"type:varchar(255);not null" json:"address"`
    Status                string    `gorm:"type:varchar(50);not null;default:'offline'" json:"status"`
    LastHeartbeat         time.Time `gorm:"index" json:"last_heartbeat"`
    ConnectedSandboxes    int       `gorm:"default:0" json:"connected_sandboxes"`
    MaxSandboxes          int       `gorm:"default:12" json:"max_sandboxes"`
    GPUType               string    `gorm:"type:varchar(100)" json:"gpu_type"`
    GPUVendor             string    `gorm:"type:varchar(100)" json:"gpu_vendor"`
    RenderNode            string    `gorm:"type:varchar(255)" json:"render_node"`
    // Desktop versions stored as JSON map
    // e.g., {"sway": "a1b2c3...", "zorin": "d4e5f6..."}
    DesktopVersionsJSON   string    `gorm:"type:text" json:"-"`
    DiskUsageJSON         string    `gorm:"type:text" json:"-"`
    DiskAlertLevel        string    `gorm:"type:varchar(20)" json:"disk_alert_level"`
    PrivilegedModeEnabled bool      `gorm:"default:false" json:"privileged_mode_enabled"`
    CreatedAt             time.Time `json:"created_at"`
    UpdatedAt             time.Time `json:"updated_at"`
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

#### 4.3 Update WolfInstanceResponse

```go
type WolfInstanceResponse struct {
    ID                    string               `json:"id"`
    Name                  string               `json:"name"`
    Address               string               `json:"address"`
    Status                string               `json:"status"`
    LastHeartbeat         time.Time            `json:"last_heartbeat"`
    ConnectedSandboxes    int                  `json:"connected_sandboxes"`
    MaxSandboxes          int                  `json:"max_sandboxes"`
    GPUType               string               `json:"gpu_type"`
    GPUVendor             string               `json:"gpu_vendor,omitempty"`
    RenderNode            string               `json:"render_node,omitempty"`
    DesktopVersions       map[string]string    `json:"desktop_versions,omitempty"`
    DiskUsage             []DiskUsageMetric    `json:"disk_usage,omitempty"`
    DiskAlertLevel        string               `json:"disk_alert_level,omitempty"`
    PrivilegedModeEnabled bool                 `json:"privileged_mode_enabled"`
    CreatedAt             time.Time            `json:"created_at"`
    UpdatedAt             time.Time            `json:"updated_at"`
}
```

#### 4.4 Update ToResponse method

```go
func (w *WolfInstance) ToResponse() *WolfInstanceResponse {
    resp := &WolfInstanceResponse{
        // ... existing fields ...
        DesktopVersions: w.GetDesktopVersions(),
    }
    // ... rest of method ...
    return resp
}
```

---

### Phase 5: Store (Save Desktop Versions)

**File:** `api/pkg/store/store_wolf_instance.go`

Update `UpdateWolfHeartbeat` to save the desktop versions map:

```go
func (s *PostgresStore) UpdateWolfHeartbeat(ctx context.Context, id string, req *types.WolfHeartbeatRequest) error {
    now := time.Now()
    updates := map[string]interface{}{
        "last_heartbeat": now,
        "updated_at":     now,
        "status":         types.WolfInstanceStatusOnline,
    }

    // Save desktop versions as JSON
    if req != nil && len(req.DesktopVersions) > 0 {
        versionsJSON, err := json.Marshal(req.DesktopVersions)
        if err == nil {
            updates["desktop_versions_json"] = string(versionsJSON)
        }
    }

    // ... rest of existing code for privileged_mode, gpu_vendor, etc. ...

    return s.gdb.WithContext(ctx).
        Model(&types.WolfInstance{}).
        Where("id = ?", id).
        Updates(updates).Error
}
```

---

### Phase 6: Wolf Executor (Use Desktop Version)

**File:** `api/pkg/external-agent/wolf_executor.go`

#### 6.1 Update computeZedImageFromVersion

The function already handles different desktop types. Update to use the map:

```go
// computeZedImageFromVersion returns the Docker image hash for the given desktop type
// The hash can be used directly as an image reference: docker run <hash>
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
    // No need for image name prefix, just the content-addressable hash
    return version
}
```

#### 6.2 Update StartZedAgent call site (around line 951)

```go
// Get the image hash for the selected desktop type
desktopType := getDesktopTypeFromEnv()
zedImage := w.computeZedImageFromVersion(desktopType, wolfInstance)

ZedImage: zedImage, // Docker image hash (e.g., "a1b2c3d4e5f6...")
```

---

### Phase 7: Dockerfile.sandbox (Restore Heartbeat Binary)

**File:** `Dockerfile.sandbox`

#### 7.1 Restore sandbox-heartbeat binary in init script

Replace the curl-based heartbeat loop (lines 631-652) with the Go binary:

```bash
# 08: Start Wolf instance heartbeat daemon (Go binary with disk space monitoring)
# NOTE: This script is SOURCED by entrypoint.sh, so use "return" not "exit"!
RUN cat > /etc/cont-init.d/08-start-wolf-heartbeat.sh << 'EOF'
#!/bin/bash
set -e

# Skip if no control plane configured (local dev mode)
# NOTE: Use "return" not "exit" - this script is sourced by entrypoint.sh!
if [ -z "$HELIX_API_URL" ] || [ -z "$RUNNER_TOKEN" ]; then
    echo "ℹ️  No HELIX_API_URL set, skipping Wolf heartbeat (local mode)"
    return 0
fi

WOLF_INSTANCE_ID=${WOLF_INSTANCE_ID:-local}
echo "💓 Starting Wolf heartbeat daemon for instance: $WOLF_INSTANCE_ID"

# Log discovered desktop versions
echo "📦 Discovering desktop versions..."
for f in /opt/images/helix-*.version; do
    if [ -f "$f" ]; then
        NAME=$(basename "$f" | sed 's/helix-//' | sed 's/.version//')
        VERSION=$(cat "$f")
        echo "   ${NAME}: ${VERSION}"
    fi
done

# Start the Go heartbeat daemon with auto-restart supervisor loop
# The daemon:
# - Dynamically discovers all desktop versions from /opt/images/helix-*.version
# - Monitors disk space on /var and /
# - Reports container disk usage
# - Sends heartbeat every 30 seconds
# The daemon can be safely killed and will automatically restart within 2 seconds
(
    while true; do
        echo "[$(date -Iseconds)] Starting heartbeat daemon..."
        /usr/local/bin/sandbox-heartbeat
        EXIT_CODE=$?
        echo "[$(date -Iseconds)] ⚠️  Heartbeat daemon exited with code $EXIT_CODE, restarting in 2s..."
        sleep 2
    done
) 2>&1 | sed -u 's/^/[HEARTBEAT] /' &

HEARTBEAT_PID=$!
echo "✅ Wolf heartbeat daemon started with auto-restart (wrapper PID: $HEARTBEAT_PID)"
EOF
RUN chmod +x /etc/cont-init.d/08-start-wolf-heartbeat.sh
```

#### 7.2 Update image loading logic (optional)

The existing image loading logic in `04-start-dockerd.sh` can remain as-is since it already handles versioned tags. However, with image hashes, the logic simplifies:

```bash
# Load helix-sway image (and any other desktop images)
for TARBALL in /opt/images/helix-*.tar; do
    if [ -f "$TARBALL" ]; then
        DESKTOP_NAME=$(basename "$TARBALL" | sed 's/helix-//' | sed 's/.tar//')
        VERSION_FILE="/opt/images/helix-${DESKTOP_NAME}.version"

        if [ -f "$VERSION_FILE" ]; then
            VERSION=$(cat "$VERSION_FILE")
            echo "📦 Loading helix-${DESKTOP_NAME} (hash: ${VERSION})..."
        else
            echo "📦 Loading helix-${DESKTOP_NAME} (no version file)..."
        fi

        if docker load -i "$TARBALL" 2>&1 | tee /tmp/docker-load-${DESKTOP_NAME}.log; then
            echo "✅ helix-${DESKTOP_NAME} loaded successfully"
        else
            echo "⚠️  Failed to load helix-${DESKTOP_NAME} tarball"
        fi
    fi
done
```

---

### Phase 8: Handler Logging (Optional Enhancement)

**File:** `api/pkg/server/wolf_instance_handlers.go`

Update the heartbeat handler logging to show all desktop versions:

```go
// Log versions if provided (helps debugging)
if len(req.DesktopVersions) > 0 {
    log.Debug().
        Str("wolf_id", id).
        Interface("desktop_versions", req.DesktopVersions).
        Msg("Wolf heartbeat received with desktop versions")
}
```

---

## Files to Modify Summary

| File | Changes |
|------|---------|
| `stack` | Create `build-desktop`, `transfer-desktop-to-sandbox`, wrapper functions, delete old code |
| `docker-compose.dev.yaml` | Add bind mounts for all desktop version files (3 sandbox services) |
| `api/cmd/sandbox-heartbeat/main.go` | Dynamic version discovery, map-based HeartbeatRequest |
| `api/pkg/types/wolf_instance.go` | Map-based versions in 3 structs, helper methods |
| `api/pkg/store/store_wolf_instance.go` | Save desktop versions JSON |
| `api/pkg/server/wolf_instance_handlers.go` | Update logging |
| `api/pkg/external-agent/wolf_executor.go` | Use map lookup for desktop version |
| `Dockerfile.sandbox` | Restore sandbox-heartbeat binary, optional: generic image loading |

---

## Adding a New Desktop (Future)

With this system in place, adding a new desktop (e.g., Ubuntu) requires only:

1. **Create Dockerfile:** `Dockerfile.ubuntu-helix`
2. **Create config directory:** `wolf/ubuntu-config/` with startup scripts
3. **Build:** `./stack build-desktop ubuntu`

Everything else is automatic:
- Tarball and version file created
- Transferred to sandbox (if running)
- Heartbeat discovers and reports it
- API stores it
- Wolf executor can use it

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

## Testing Checklist

- [ ] `./stack build-desktop sway` builds and creates tarball with image hash
- [ ] `./stack build-desktop zorin` builds and creates tarball with image hash
- [ ] Image hash survives `docker save | docker load` (verify hash is identical)
- [ ] Heartbeat discovers all version files dynamically
- [ ] API receives and stores desktop versions map
- [ ] Wolf executor uses correct image hash for selected desktop type
- [ ] Hot-reload: changing desktop code → rebuild → sandbox picks up new hash
- [ ] Production: pre-baked images work identically
