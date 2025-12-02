# Guide: Adding New Desktop Environments to Helix Code

This guide documents how to add a new desktop environment to the Helix Code sandbox system. It uses **Ubuntu** as the example, documenting lessons learned from implementing Sway and Zorin.

---

## Architecture Overview

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

## Ubuntu Version Decision

### Options Evaluated

| Version | Codename | Pros | Cons |
|---------|----------|------|------|
| Ubuntu 24.04 LTS | noble | Latest LTS, support until 2029 | No official GOW image |
| Ubuntu 22.04 LTS | jammy | What Zorin 18 uses | Older packages |
| Ubuntu 25.04 | plucky | Latest packages | Non-LTS, short support |

### Recommendation: Ubuntu 24.04 LTS (noble)

**Reasoning:**
- Current LTS with support until 2029
- Modern packages (kernel 6.8+)
- Can build custom GOW-based image like Zorin

**Base Image Options:**
1. `ghcr.io/games-on-whales/xfce:edge` - Simplest, already GOW-integrated, lightweight
2. Build custom from `ghcr.io/games-on-whales/base-app:edge` + Ubuntu GNOME packages
3. Find/create a third-party GOW-Ubuntu image (like mollomm1 did for Zorin)

---

## Files to Create for a New Desktop

### 1. Dockerfile.{desktop}-helix

**Location:** `Dockerfile.ubuntu-helix`

**Structure (copy from Dockerfile.zorin-helix):**

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
# ... desktop-specific setup
```

**Required Components (checklist):**

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

### 2. wolf/{desktop}-config/ Directory

**Location:** `wolf/ubuntu-config/`

**Required Files (copy from sway-config or zorin-config):**

| File | Purpose | Copy From |
|------|---------|-----------|
| `startup-app.sh` | GOW startup, launches desktop | Modify for desktop-specific launcher |
| `start-zed-helix.sh` | Zed setup, git cloning, workspace | Copy from sway-config (most complete) |
| `docker-wrapper.sh` | Hydra symlink resolution | Copy as-is from sway-config |
| `docker-compose-wrapper.sh` | Hydra compose resolution | Copy as-is from sway-config |
| `helix-specs-create.sh` | Design docs worktree | Copy as-is from sway-config |
| `16-add-docker-group.sh` | Runtime Docker group | Copy as-is from sway-config |

### 3. stack Script Updates

**Location:** `stack`

Add build command:
```bash
build-ubuntu)
    # Similar to build-sway and build-zorin
    docker build -f Dockerfile.ubuntu-helix -t helix-ubuntu:latest .
    # Export tarball, transfer to sandbox dockerd, etc.
    ;;
```

### 4. wolf_executor.go Updates

**Location:** `api/pkg/external-agent/wolf_executor.go`

Add new desktop type:
```go
const (
    DesktopSway   DesktopType = "sway"
    DesktopZorin  DesktopType = "zorin"
    DesktopUbuntu DesktopType = "ubuntu"  // Add new type
)
```

Add to parseDesktopType():
```go
case "ubuntu":
    return DesktopUbuntu
```

Add to computeZedImageFromVersion():
```go
case DesktopUbuntu:
    prefix = "helix-ubuntu"
```

> **Note:** Ubuntu is already defined in wolf_executor.go!

---

## startup-app.sh: Desktop-Specific Differences

### Sway (Wayland)
```bash
# Sources GOW's launch-comp.sh
source /opt/gow/launch-comp.sh
custom_launcher /usr/local/bin/start-zed-helix.sh
# Uses dbus-run-session -- sway --unsupported-gpu
```

### Zorin/GNOME (X11 via Xwayland)
```bash
# Uses GOW's xorg.sh script
exec /opt/gow/xorg.sh
# GNOME autostart entries launch apps
# IBus must be disabled (keyboard issues)
# Screensaver proxy must be disabled
```

### XFCE (X11 via Xwayland)
Would be similar to Zorin but simpler (XFCE doesn't need systemd workarounds)

---

## Required Features Checklist

Every desktop MUST implement these features:

### CRITICAL (API Communication)
- [ ] **RevDial Client** - Start before desktop for API ↔ sandbox communication
- [ ] **WORKSPACE_DIR Symlink** - `/home/retro/work` → `$WORKSPACE_DIR` for Hydra compatibility
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

## Build and Test Process

### 1. Commit Before Building
```bash
git add -A && git commit -m "feat(ubuntu): add Ubuntu desktop environment"
```

### 2. Build the Image
```bash
./stack build-ubuntu
```

### 3. Test
```bash
# Set environment variable
export HELIX_DESKTOP=ubuntu

# Restart API to pick up change
docker compose -f docker-compose.dev.yaml down api
docker compose -f docker-compose.dev.yaml up -d api

# Create external agent session
# Check logs for:
# - RevDial client connects
# - Repositories clone
# - helix-specs worktree created
# - Zed opens with multi-folder workspace
```

### 4. Test Docker Access
```bash
# Inside the desktop container
docker run hello-world
docker compose version
```

### 5. Test Clipboard
- Copy in Zed with Ctrl+C
- Paste in Firefox with Ctrl+V
- Verify bidirectional sync

---

## Common Issues and Solutions

### Issue: GNOME won't start
**Cause:** systemd-logind not available in containers
**Solution:** Disable login1 files (see Zorin Dockerfile line 28-30)

### Issue: Keyboard modifiers stuck
**Cause:** IBus input method framework interference
**Solution:** Disable IBus in startup-app.sh:
```bash
export GTK_IM_MODULE=gtk-im-context-simple
export QT_IM_MODULE=gtk-im-context-simple
export XMODIFIERS=@im=none
```

### Issue: Docker bind mounts fail with Hydra
**Cause:** Docker CLI resolves symlinks, Hydra uses different dockerd
**Solution:** Use docker-wrapper.sh to resolve symlinks before Docker

### Issue: Clipboard not syncing
**Cause:** Wrong clipboard tool for display system
**Solution:** Use `xclip` for X11, `wl-clipboard` for Wayland

### Issue: Screensaver notification spam
**Cause:** gsd-screensaver-proxy detects missing GDM
**Solution:** Disable via autostart override (see Zorin startup-app.sh)

---

## Sources

- [Games on Whales GitHub](https://github.com/games-on-whales/gow)
- [Wolf Streaming Server](https://github.com/games-on-whales/wolf)
- [x11docker - GUI apps in Docker](https://github.com/mviereck/x11docker)
- [Dockerfile-Ubuntu-Gnome](https://github.com/darkdragon-001/Dockerfile-Ubuntu-Gnome)
- [Running Wayland compositor in Docker](https://unix.stackexchange.com/questions/756867/running-a-wayland-compositor-inside-docker)
