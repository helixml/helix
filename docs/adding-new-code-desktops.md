# Adding New Desktop Environments

This guide explains how to add a new desktop environment to Helix's streaming sandbox system. We'll use Ubuntu Desktop as an example.

## Overview

Helix supports multiple desktop environments that run inside the sandbox container. Each desktop:

- Is built as a separate Docker image (`helix-<name>`)
- Gets exported as a tarball and embedded in the sandbox
- Uses content-addressable versioning (Docker image hash)
- Is automatically discovered by the heartbeat system

## Prerequisites

Before adding a new desktop, ensure you have:

1. A working Helix development environment (`./stack start`)
2. Understanding of Docker multi-stage builds
3. Familiarity with the target desktop environment (GNOME, KDE, XFCE, etc.)

## Step-by-Step Guide: Adding Ubuntu Desktop

### Step 1: Create the Dockerfile

Create `Dockerfile.ubuntu-helix` in the repository root:

```dockerfile
# Build stage for Go binaries
FROM golang:1.24 AS go-build-env

WORKDIR /app
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY api ./api
WORKDIR /app/api

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -buildvcs=false -ldflags "-s -w" -o /settings-sync-daemon ./cmd/settings-sync-daemon && \
    CGO_ENABLED=0 go build -buildvcs=false -ldflags "-s -w" -o /screenshot-server ./cmd/screenshot-server

# Main desktop image
# Use GOW base image or Ubuntu base with required packages
FROM ghcr.io/games-on-whales/ubuntu-desktop:edge
# Or: FROM ubuntu:24.04

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

# Copy Zed editor
RUN mkdir -p /zed-build
COPY zed-build/zed /zed-build/zed
RUN chmod +x /zed-build/zed

# Copy desktop-specific config
ADD wolf/ubuntu-config/start-zed-helix.sh /usr/local/bin/start-zed-helix.sh
RUN chmod +x /usr/local/bin/start-zed-helix.sh

ADD wolf/ubuntu-config/startup-app.sh /opt/gow/startup-app.sh
RUN chmod +x /opt/gow/startup-app.sh

# Copy wallpaper
COPY wolf/assets/images/helix_hero.png /usr/share/backgrounds/helix-hero.png
```

### Step 2: Create Configuration Directory

Create `wolf/ubuntu-config/` with the required scripts:

**wolf/ubuntu-config/startup-app.sh:**
```bash
#!/bin/bash
# Startup script for Ubuntu desktop

# Start screenshot server
/usr/local/bin/screenshot-server &

# Start settings sync daemon
/usr/local/bin/settings-sync-daemon &

# Launch the desktop session
exec /usr/bin/gnome-session
```

**wolf/ubuntu-config/start-zed-helix.sh:**
```bash
#!/bin/bash
# Launch Zed editor in Helix mode

export HELIX_MODE=1
exec /zed-build/zed "$@"
```

### Step 3: Add Build Function to Stack

Edit `stack` and add the Ubuntu build step to the `build-sandbox` function:

```bash
# Find the section after zorin build and before the sandbox build
# Add this new step:

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "📝 [4/6] Building helix-ubuntu and exporting tarball..."
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
build-desktop ubuntu
if [ ! -f helix-ubuntu.tar ]; then
    echo "❌ helix-ubuntu.tar not found after build-ubuntu - this shouldn't happen"
    rm -f helix-sway.tar helix-zorin.tar helix-ubuntu.tar
    exit 1
fi
echo "✅ Using helix-ubuntu.tar ($(du -h helix-ubuntu.tar | cut -f1)) version=$(cat helix-ubuntu.version)"
```

Also update the step numbers (from `/5` to `/6`) and update the intro message to include Ubuntu.

### Step 4: Update Dockerfile.sandbox

Add the Ubuntu tarball copy in `Dockerfile.sandbox`:

```dockerfile
# Find the section that copies desktop tarballs
# Add these lines:

# Copy Ubuntu desktop image tarball
COPY helix-ubuntu.tar* /opt/images/
COPY helix-ubuntu.version* /opt/images/
```

The `*` glob pattern ensures the build doesn't fail if the file doesn't exist (for backward compatibility).

### Step 5: Add Bind Mount for Development

In `docker-compose.dev.yaml`, add the bind mount for hot-reload in development:

```yaml
services:
  sandbox:
    volumes:
      # ... existing mounts ...
      - ./helix-ubuntu.version:/opt/images/helix-ubuntu.version:ro
```

This allows version updates without rebuilding the entire sandbox container.

### Step 6: Add Desktop Type Constant

In `api/pkg/external-agent/wolf_executor.go`, add the constant:

```go
const (
    DesktopSway   DesktopType = "sway"
    DesktopZorin  DesktopType = "zorin"
    DesktopUbuntu DesktopType = "ubuntu"  // Add this line
)
```

## Testing Your New Desktop

### 1. Build the Desktop Image

```bash
# Build just the desktop image
./stack build-desktop ubuntu

# Or build everything including the sandbox
./stack build-sandbox
```

### 2. Verify Version File

```bash
cat helix-ubuntu.version
# Should output a 12-character Docker image hash like: a1b2c3d4e5f6
```

### 3. Check Heartbeat Discovery

After starting the sandbox, check that the API receives the desktop version:

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

### 4. Test Desktop Launch

Create a session with your new desktop type and verify it streams correctly.

## How It Works

### Content-Addressable Versioning

Each desktop image version is determined by its Docker image hash:

```bash
docker images helix-ubuntu:latest --format '{{.ID}}'
# Output: sha256:a1b2c3d4e5f6...

# This hash is stored in the .version file (first 12 chars)
echo "a1b2c3d4e5f6" > helix-ubuntu.version
```

This approach ensures:
- Version survives `docker save` / `docker load` operations
- Identical images produce identical versions
- No dependency on git history

### Dynamic Discovery

The sandbox-heartbeat binary discovers all available desktops at runtime:

```go
files, _ := filepath.Glob("/opt/images/helix-*.version")
for _, file := range files {
    name := extractDesktopName(file)  // "ubuntu" from "helix-ubuntu.version"
    version := readFile(file)          // "a1b2c3d4e5f6"
    versions[name] = version
}
```

This means:
- No code changes needed in heartbeat for new desktops
- Desktops can be added/removed without API changes
- Version map is sent to control plane every heartbeat

### Database Storage

Desktop versions are stored as JSON in the `desktop_versions_json` column:

```json
{"sway": "e3047008385c", "zorin": "2954a89ed294", "ubuntu": "a1b2c3d4e5f6"}
```

Helper methods provide type-safe access:

```go
versions := wolfInstance.GetDesktopVersions()
ubuntuVersion := wolfInstance.GetDesktopVersion("ubuntu")
```

## Troubleshooting

### Build Fails: "helix-ubuntu.tar: Is a directory"

Docker created an empty directory when bind mount source didn't exist:

```bash
sudo rm -rf helix-ubuntu.tar helix-ubuntu.version
./stack build-sandbox
```

### Desktop Not Appearing in Heartbeat

1. Check the version file exists in sandbox:
   ```bash
   docker exec sandbox-1 ls -la /opt/images/helix-*.version
   ```

2. Check heartbeat logs:
   ```bash
   docker exec sandbox-1 cat /var/log/sandbox-heartbeat.log
   ```

### Image Not Loading in Inner Docker

1. Check tarball was copied:
   ```bash
   docker exec sandbox-1 ls -la /opt/images/helix-ubuntu.tar
   ```

2. Check inner Docker loaded it:
   ```bash
   docker exec sandbox-1 docker images helix-ubuntu
   ```

## Summary

Adding a new desktop requires:

| File | Purpose |
|------|---------|
| `Dockerfile.<name>-helix` | Build the desktop image |
| `wolf/<name>-config/` | Desktop-specific scripts |
| `stack` (build-sandbox) | Add build step |
| `Dockerfile.sandbox` | Copy tarball and version |
| `docker-compose.dev.yaml` | Bind mount for dev |
| `wolf_executor.go` | Add type constant |

The heartbeat system automatically discovers new desktops - no additional API changes required.
