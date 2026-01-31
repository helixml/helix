# Fix Ubuntu Desktop Container Startup Issues

**Date:** 2025-12-05
**Status:** Ready for Implementation
**Author:** Claude (with Kai)

## Executive Summary

The Ubuntu (XFCE) desktop container exits immediately with exit code 1. This document provides step-by-step instructions to fix all identified issues. Sway and Zorin desktops work correctly, so we know the infrastructure is sound.

---

## Issue 1: Permission Denied on devilspie2 Config

### Symptom
Container logs show:
```
cp: cannot open '/etc/skel/.config/devilspie2/helix-tiling.lua' for reading: Permission denied
```

### Root Cause
The file `/etc/skel/.config/devilspie2/helix-tiling.lua` has `600` permissions (root read/write only). The `retro` user cannot read it during startup when trying to copy to `~/.config/devilspie2/`.

### Fix

**File:** `Dockerfile.ubuntu-helix`

**Location:** After line 132 (the COPY command for helix-tiling.lua)

**Current code (lines 131-133):**
```dockerfile
RUN mkdir -p /etc/skel/.config/devilspie2
COPY wolf/ubuntu-config/devilspie2/helix-tiling.lua /etc/skel/.config/devilspie2/helix-tiling.lua
```

**Add this line immediately after:**
```dockerfile
RUN chmod 644 /etc/skel/.config/devilspie2/helix-tiling.lua
```

**Result (lines 131-134):**
```dockerfile
RUN mkdir -p /etc/skel/.config/devilspie2
COPY wolf/ubuntu-config/devilspie2/helix-tiling.lua /etc/skel/.config/devilspie2/helix-tiling.lua
RUN chmod 644 /etc/skel/.config/devilspie2/helix-tiling.lua
```

---

## Issue 2: Wrong Startup Script (xorg.sh doesn't exist)

### Symptom
Container logs show:
```
GOW xorg script exists: NO
```

The startup script calls `exec /opt/gow/xorg.sh` but this file doesn't exist in the XFCE base image.

### Root Cause
The XFCE base image (`ghcr.io/games-on-whales/xfce:edge`) uses a different startup mechanism than Zorin:
- **Zorin** (community image): Has `/opt/gow/xorg.sh`
- **XFCE** (official GOW image): Has `/opt/gow/launch-comp.sh` with a `launcher()` function

### Fix

**File:** `wolf/ubuntu-config/startup-app.sh`

**Location:** End of file (lines 287-296)

**Current code:**
```bash
# ============================================================================
# XFCE Session Startup via GOW xorg.sh
# ============================================================================
# Launch XFCE via GOW's proven xorg.sh script
# This handles: Xwayland startup -> D-Bus -> XFCE session
# XFCE is simpler than GNOME and doesn't need systemd workarounds

echo "Launching XFCE via GOW xorg.sh..."
exec /opt/gow/xorg.sh
```

**Replace with:**
```bash
# ============================================================================
# XFCE Session Startup via GOW launch-comp.sh
# ============================================================================
# Launch XFCE via GOW's launcher() function from launch-comp.sh
# This handles: Xwayland startup -> D-Bus -> XFCE session
# The XFCE base image uses launch-comp.sh (not xorg.sh like Zorin)

echo "Launching XFCE via GOW launcher..."
source /opt/gow/launch-comp.sh
launcher
```

### Technical Note
The `launcher()` function in `/opt/gow/launch-comp.sh` does the following:
1. Sets up XDG_DATA_DIRS and XFCE config
2. Starts D-Bus via `sudo /opt/gow/startdbus`
3. Sets X11 environment variables (DISPLAY=:0, GDK_BACKEND=x11, etc.)
4. Runs `dbus-run-session -- bash -E -c "WAYLAND_DISPLAY=$REAL_WAYLAND_DISPLAY Xwayland :0 & sleep 2 && startxfce4"`

---

## Issue 3: Missing Home Directory Ownership Fix

### Symptom
Potential permission errors when creating files/symlinks in `/home/retro`.

### Root Cause
Wolf mounts `/wolf-state/agent-xxx` to `/home/retro`, which may be owned by `ubuntu:ubuntu` instead of `retro:retro`. Zorin's startup script has this fix, but Ubuntu's doesn't.

### Fix

**File:** `wolf/ubuntu-config/startup-app.sh`

**Location:** After the debug section (after line 42, before "Workspace Directory Setup")

**Find this section (around line 42-43):**
```bash
echo ""
echo "=== UBUNTU/XFCE STARTUP BEGINS ==="
echo "Starting Helix Ubuntu/XFCE environment..."
```

**Add this block immediately after:**
```bash
# ============================================================================
# CRITICAL: Fix home directory ownership FIRST
# ============================================================================
# Wolf mounts /wolf-state/agent-xxx:/home/retro which may be owned by ubuntu:ubuntu
# We need write permission to /home/retro before creating any symlinks or files
echo "Fixing /home/retro ownership..."
sudo chown retro:retro /home/retro
echo "✅ /home/retro ownership fixed"
```

**Result:**
```bash
echo ""
echo "=== UBUNTU/XFCE STARTUP BEGINS ==="
echo "Starting Helix Ubuntu/XFCE environment..."

# ============================================================================
# CRITICAL: Fix home directory ownership FIRST
# ============================================================================
# Wolf mounts /wolf-state/agent-xxx:/home/retro which may be owned by ubuntu:ubuntu
# We need write permission to /home/retro before creating any symlinks or files
echo "Fixing /home/retro ownership..."
sudo chown retro:retro /home/retro
echo "✅ /home/retro ownership fixed"

# ============================================================================
# Workspace Directory Setup (Hydra Compatibility)
# ============================================================================
```

---

## Issue 4: Container Names Don't Show Desktop Type

### Symptom
`docker ps` inside the sandbox shows containers with names like `zed-external-01kbpycak...` which doesn't indicate if it's Sway, Zorin, or Ubuntu.

### Root Cause
The container hostname is hardcoded to use `zed-external-` prefix regardless of desktop type.

### Fix

**File:** `api/pkg/external-agent/wolf_executor.go`

**There are 3 locations to change:**

#### Location 1: Line ~653

**Current code:**
```go
containerHostname := fmt.Sprintf("zed-external-%s", sessionIDPart)
```

**Change to:**
```go
containerHostname := fmt.Sprintf("%s-external-%s", getDesktopTypeFromEnv(), sessionIDPart)
```

#### Location 2: Line ~1758

**Current code:**
```go
containerHostname := fmt.Sprintf("zed-external-%s", sessionIDPart)
```

**Change to:**
```go
containerHostname := fmt.Sprintf("%s-external-%s", getDesktopTypeFromEnv(), sessionIDPart)
```

#### Location 3: Line ~2225 (in comment, update for accuracy)

**Current comment:**
```go
// Container name format: zed-external-{session_id_without_ses_}_{lobby_id}
```

**Change to:**
```go
// Container name format: {desktop_type}-external-{session_id_without_ses_}_{lobby_id}
```

### Result
After this change, `docker ps` will show:
- `ubuntu-external-01kbpycak...` for Ubuntu containers
- `zorin-external-01kbpycak...` for Zorin containers
- `sway-external-01kbpycak...` for Sway containers

---

## Complete File Change Summary

### File 1: `Dockerfile.ubuntu-helix`

Add one line after line 132:
```dockerfile
RUN chmod 644 /etc/skel/.config/devilspie2/helix-tiling.lua
```

### File 2: `wolf/ubuntu-config/startup-app.sh`

Two changes:

1. **Add home directory ownership fix** after line ~43 (after "UBUNTU/XFCE STARTUP BEGINS"):
```bash
# ============================================================================
# CRITICAL: Fix home directory ownership FIRST
# ============================================================================
# Wolf mounts /wolf-state/agent-xxx:/home/retro which may be owned by ubuntu:ubuntu
# We need write permission to /home/retro before creating any symlinks or files
echo "Fixing /home/retro ownership..."
sudo chown retro:retro /home/retro
echo "✅ /home/retro ownership fixed"
```

2. **Replace the end of the file** (lines ~287-296):
```bash
# ============================================================================
# XFCE Session Startup via GOW launch-comp.sh
# ============================================================================
# Launch XFCE via GOW's launcher() function from launch-comp.sh
# This handles: Xwayland startup -> D-Bus -> XFCE session
# The XFCE base image uses launch-comp.sh (not xorg.sh like Zorin)

echo "Launching XFCE via GOW launcher..."
source /opt/gow/launch-comp.sh
launcher
```

### File 3: `api/pkg/external-agent/wolf_executor.go`

Change container hostname format in 3 locations (lines ~653, ~1758, ~2225):

From:
```go
containerHostname := fmt.Sprintf("zed-external-%s", sessionIDPart)
```

To:
```go
containerHostname := fmt.Sprintf("%s-external-%s", getDesktopTypeFromEnv(), sessionIDPart)
```

---

## Testing Instructions

After making all changes:

1. **Rebuild the Ubuntu desktop image:**
   ```bash
   ./stack build-desktop ubuntu
   ```

2. **Launch an Ubuntu container** from the Helix UI (External Agents or SpecTask with HELIX_DESKTOP=ubuntu)

3. **Verify the container stays running:**
   ```bash
   SANDBOX_ID=$(docker ps | grep "helix-sandbox" | awk '{print $1}')
   docker exec $SANDBOX_ID docker ps -a
   ```

   Expected: Container should show `Up X minutes` not `Exited (1)`

4. **Verify container name includes desktop type:**
   ```bash
   docker exec $SANDBOX_ID docker ps -a --format '{{.Names}}'
   ```

   Expected: Names like `ubuntu-external-...` instead of `zed-external-...`

5. **If container still exits, check logs:**
   ```bash
   CONTAINER_ID=$(docker exec $SANDBOX_ID docker ps -a --format '{{.ID}}' | head -1)
   docker exec $SANDBOX_ID docker logs $CONTAINER_ID
   ```

---

## Debugging Reference

### Quick Commands for Debug

```bash
# Get sandbox container ID
SANDBOX_ID=$(docker ps | grep "helix-sandbox" | awk '{print $1}')

# List all desktop containers
docker exec $SANDBOX_ID docker ps -a

# Get logs from most recent Ubuntu container
docker exec $SANDBOX_ID docker logs $(docker exec $SANDBOX_ID docker ps -a --format '{{.ID}} {{.Image}}' | grep ubuntu | head -1 | awk '{print $1}')

# Check what image reference was used
docker exec $SANDBOX_ID docker inspect $(docker exec $SANDBOX_ID docker ps -a -q | head -1) --format '{{.Config.Image}}'
```

### Expected Container Log Output (After Fix)

```
=== UBUNTU/XFCE STARTUP DEBUG START ... ===
User: retro
UID: 1000
...
=== UBUNTU/XFCE STARTUP BEGINS ===
Starting Helix Ubuntu/XFCE environment...
Fixing /home/retro ownership...
✅ /home/retro ownership fixed
[Workspace] Setting up workspace symlink: /home/retro/work -> ...
...
Setting up devilspie2 window positioning...
✅ Devilspie2 config copied to ~/.config/devilspie2/
...
Launching XFCE via GOW launcher...
```

---

## Lower Priority Issues (Monitor Only)

These issues may or may not cause problems. Monitor after the main fixes are applied:

1. **XFCE Autostart Keys:** Ubuntu uses `X-GNOME-Autostart-Delay=X` which is GNOME-specific. XFCE may ignore these, causing all services to start simultaneously. If race conditions occur, add `sleep` commands in wrapper scripts.

2. **XDG_RUNTIME_DIR:** Dockerfile sets `/run/user/1000` but GOW overrides to `/tmp/sockets`. This should be fine since GOW manages it.

---

## References

- Design doc: `design/2025-12-02-multiple-desktop-envs.md`
- XFCE base image: `ghcr.io/games-on-whales/xfce:edge`
- XFCE launcher script: `/opt/gow/launch-comp.sh` (inside container)
- Zorin startup for comparison: `wolf/zorin-config/startup-app.sh`
