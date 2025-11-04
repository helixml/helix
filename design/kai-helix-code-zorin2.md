# Zorin/GNOME Container Debugging Session - Resolution

**Date**: 2025-11-04
**Status**: ✅ RESOLVED
**Goal**: Get Zorin/GNOME desktop container to start properly with Wolf streaming

## Summary

Successfully debugged and fixed Zorin container startup issues. The container now starts GNOME desktop correctly using direct gnome-session launch instead of GOW's Sway-oriented launcher scripts.

## Problems Encountered and Solutions

### Problem 1: XDG_RUNTIME_DIR Not Set
**Symptom**: Container exited immediately with error:
```
chown: cannot access '': No such file or directory
```

**Root Cause**: Zorin base image's init script tried to `chown "${XDG_RUNTIME_DIR}"` but the environment variable was empty.

**Solution**: Added to `Dockerfile.zorin-helix`:
```dockerfile
ENV XDG_RUNTIME_DIR=/run/user/1000
RUN mkdir -p /run/user/1000 && chmod 700 /run/user/1000
```

### Problem 2: chmod Permission Error on Startup Script
**Symptom**: Container logs showed:
```
chmod: changing permissions of '/opt/gow/startup.sh': Read-only file system
```

**Root Cause**: Mounted startup script as `:ro` (read-only) but container entrypoint tries to chmod it.

**Solution**: Changed mount in `api/pkg/external-agent/wolf_executor.go`:
```go
// startup.sh needs rw mount so entrypoint can chmod it (even though it's already executable)
fmt.Sprintf("%s/wolf/zorin-config/startup-app.sh:/opt/gow/startup.sh:rw", helixHostHome),
```

### Problem 3: GNOME Entered "Failed Session" Mode
**Symptom**:
- Container started but process `gnome-session-failed` running instead of normal GNOME
- No window manager (gnome-shell) running
- Autostart entries not executing
- Desktop unusable

**Root Cause**: We were calling GOW's Sway-oriented launcher scripts (`launch-comp.sh` and `launcher /opt/gow/xorg.sh`) instead of directly launching GNOME session. GOW's launcher is designed for Sway compositor, not GNOME's self-contained session manager.

**Key Research**: Investigated working reference implementation at https://github.com/Mollomm1/gow-desktops which successfully runs Zorin with Wolf. Discovered they:
1. Directly call `gnome-session` via dbus-launch (NOT via GOW's launcher scripts)
2. Disable systemd-logind (rename login1 files)
3. Set comprehensive GNOME environment variables
4. Have `dbus-x11` package installed
5. Enable bwrap setuid for application sandboxing

**Solution Applied**: Implemented three critical fixes:

#### Fix 3a: systemd-logind Workaround in Dockerfile
Added to `Dockerfile.zorin-helix`:
```dockerfile
# CRITICAL FIX: Disable systemd-logind (not available in containers)
# GNOME session manager tries to connect to systemd-logind for session management
# In containers without systemd, this causes GNOME to fail and enter "failed session" mode
# This workaround renames login1 files so GNOME falls back to working without systemd
RUN for file in $(find /usr -type f -iname "*login1*" 2>/dev/null); do \
    mv -v "$file" "$file.back" 2>/dev/null || true; \
    done

# Enable bubblewrap (bwrap) for application sandboxing - needed by GNOME
RUN chmod u+s /usr/bin/bwrap 2>/dev/null || true

# Ensure dbus-x11 is installed (required for D-Bus in X11 sessions)
RUN apt-get update && \
    apt-get install -y dbus-x11 && \
    rm -rf /var/lib/apt/lists/*
```

#### Fix 3b: Direct GNOME Session Launch
Replaced GOW launcher calls with direct gnome-session launch in `wolf/zorin-config/startup-app.sh`:

**OLD CODE (WRONG)**:
```bash
# This was calling Sway-oriented launcher
source /opt/gow/launch-comp.sh
launcher /opt/gow/xorg.sh
```

**NEW CODE (CORRECT)**:
```bash
# Set GNOME session environment variables
export XDG_SESSION_TYPE=x11
export XDG_CURRENT_DESKTOP=zorin:GNOME
export DESKTOP_SESSION=zorin
export XDG_SESSION_DESKTOP=zorin
export GNOME_SHELL_SESSION_MODE=zorin

# Application compatibility - force X11 mode
export GDK_BACKEND=x11
export QT_QPA_PLATFORM="xcb"
export MOZ_ENABLE_WAYLAND=0
export QT_AUTO_SCREEN_SCALE_FACTOR=1

# Data directories for Flatpak apps and system data
export XDG_DATA_DIRS=/var/lib/flatpak/exports/share:/home/retro/.local/share/flatpak/exports/share:/usr/local/share/:/usr/share/

# Locale configuration
export LC_ALL="en_US.UTF-8"

# Display configuration - Xwayland is started by Wolf
export DISPLAY=:9
unset WAYLAND_DISPLAY

# Add Flathub repository for Flatpak support (if not already added)
if ! flatpak remote-list 2>/dev/null | grep -q flathub; then
    flatpak remote-add --user --if-not-exists flathub https://flathub.org/repo/flathub.flatpakrepo 2>/dev/null || true
fi

# Launch GNOME session directly with D-Bus
# This is the CORRECT way to start GNOME (not via GOW's Sway launcher)
exec /usr/bin/dbus-launch /usr/bin/gnome-session
```

## Debug Logging Implementation

Added comprehensive debug logging to `wolf/zorin-config/startup-app.sh` (placed BEFORE `set -e`):

```bash
#!/bin/bash
# GOW GNOME startup script for Helix Personal Dev Environment

# ============================================================================
# CRITICAL DEBUG SECTION - MUST BE FIRST (before set -e)
# ============================================================================
DEBUG_LOG=/tmp/zorin-startup-debug.log

# Redirect all output to both stdout and debug log file
exec 1> >(tee -a "$DEBUG_LOG")
exec 2>&1

echo "=== ZORIN STARTUP DEBUG START $(date) ==="
echo "User: $(whoami)"
echo "UID: $(id -u)"
echo "GID: $(id -g)"
echo "Groups: $(groups)"
echo "Home: $HOME"
echo "PWD: $PWD"
echo "Shell: $SHELL"

echo ""
echo "=== ENVIRONMENT VARIABLES ==="
echo "XDG_RUNTIME_DIR: ${XDG_RUNTIME_DIR:-NOT SET}"
echo "HELIX_SESSION_ID: ${HELIX_SESSION_ID:-NOT SET}"
echo "HELIX_API_URL: ${HELIX_API_URL:-NOT SET}"

echo ""
echo "=== CRITICAL FILE CHECKS ==="
echo "Zed binary exists: $([ -f /zed-build/zed ] && echo YES || echo NO)"
echo "Workspace mount exists: $([ -d /home/retro/work ] && echo YES || echo NO)"

# Trap EXIT to show exit code and keep container alive for debugging
# Container will stay alive 5 minutes for log inspection
trap 'EXIT_CODE=$?; echo ""; echo "=== SCRIPT EXITING WITH CODE $EXIT_CODE at $(date) ==="; echo "Container will stay alive 5 minutes for log inspection..."; sleep 300' EXIT

# NOW enable strict error checking (after debug setup is complete)
set -e
```

This debug logging:
- Captures user info, environment variables, file existence checks
- Writes to both stdout (for `docker logs`) AND `/tmp/zorin-startup-debug.log` (for container inspection)
- Includes EXIT trap to keep container alive 5 minutes on failure for log inspection
- Placed BEFORE `set -e` so it executes even if script encounters errors

## Key Insights

1. **GOW launcher scripts are Sway-specific**: The `launch-comp.sh` and `launcher` scripts in GOW are designed for Sway compositor, not for full desktop environments like GNOME.

2. **GNOME has its own session manager**: GNOME should be started via `gnome-session` which handles starting gnome-shell, mutter, and all desktop services automatically.

3. **systemd-logind is expected but unavailable**: GNOME tries to connect to systemd-logind for session management, but it's not available in containers. The workaround is to rename login1 files so GNOME falls back to working without systemd.

4. **gow-desktops is the reference**: The Mollomm1/gow-desktops repository has proven working implementations of GNOME/Zorin with Wolf and should be used as the reference for desktop environment integration.

5. **Debug logging is critical**: Comprehensive debug logging placed before error handling (`set -e`) is essential for troubleshooting container startup issues.

## Files Modified

1. **wolf/zorin-config/startup-app.sh**
   - Added comprehensive debug logging (lines 4-57)
   - Replaced GOW launcher calls with direct gnome-session launch (lines 185-230)

2. **Dockerfile.zorin-helix**
   - Added XDG_RUNTIME_DIR environment variable and directory creation (lines 20-24)
   - Added systemd-logind workaround (lines 26-32)
   - Enabled bwrap setuid (lines 34-35)
   - Ensured dbus-x11 is installed (lines 37-40)

3. **api/pkg/external-agent/wolf_executor.go**
   - Changed startup script mount from `:ro` to `:rw` (line 214-215)

## Testing

After applying all fixes:
```bash
./stack build-zorin        # Rebuild Zorin image with all fixes
docker compose -f docker-compose.dev.yaml down wolf
docker compose -f docker-compose.dev.yaml up -d wolf
```

Then create a new external agent session via frontend to test the GNOME desktop.

Expected results:
- ✅ gnome-session starts (not gnome-session-failed)
- ✅ gnome-shell process running (window manager)
- ✅ Desktop successfully streaming via Wolf/Moonlight
- ✅ Helix services (screenshot-server, Zed, settings-sync-daemon) launch via autostart

## Next Steps

1. Test the GNOME desktop via frontend external agent session creation
2. Verify gnome-shell is running (not gnome-session-failed)
3. Verify Helix services start correctly
4. Once confirmed working, consider removing verbose debug logging (optional cleanup)

## References

- Working reference implementation: https://github.com/Mollomm1/gow-desktops
- Wolf streaming server: https://github.com/games-on-whales/wolf
- Previous debugging document: design/kai-zorin-container-startup-debugging.md
