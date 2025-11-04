# Zorin Desktop Integration for Helix Code

**Author**: Kai (with Claude Code assistance)
**Status**: Implemented, Ready for Testing

## Problem Statement

Helix Code uses Sway as the default desktop compositor for Personal Dev Environments (PDEs) and External Agent sessions. While Sway is lightweight (~150MB memory) and efficient, it presents significant UX challenges:

- **Tiling window management is confusing**: Sway automatically tiles windows, which is unfamiliar to users expecting traditional overlapping windows
- **Steep learning curve**: Users need to learn Sway-specific keybindings and tiling concepts
- **Not truly interactive**: Sway is designed for tiling workflows, not traditional desktop interaction

**User requirement**: Add a more traditional desktop environment like Zorin OS/GNOME, while maintaining compatibility with Wolf (Moonlight streaming server) and GStreamer.

## Implementation Journey

This document chronicles the debugging process and fixes required to get Zorin/GNOME desktop working alongside Sway in Helix Code.

---

### Phase 1: Initial Multi-Desktop Support

**Goal**: Implement side-by-side support for Sway, XFCE, and GNOME/Zorin desktops.

#### Files Created

1. **`Dockerfile.zorin-helix`**
   - Based on `ghcr.io/mollomm1/gow-zorin-18:latest` (community Zorin OS 18 image)
   - Multi-stage build with Go binaries (screenshot-server, settings-sync-daemon)
   - Installs: Firefox, Docker CLI, grim, git, OnlyOffice, Ghostty, Zed
   - Copies GNOME config files to `/cfg/gnome/`
   - Copies startup scripts to `/opt/gow/`

2. **`wolf/gnome-config/` directory** (later renamed to `wolf/zorin-config/`)
   - `startup-app.sh` - GNOME initialization script
   - `start-zed-helix.sh` - Zed launcher with auto-restart loop
   - `dconf-settings.ini` - GNOME desktop configuration (dark theme, wallpaper, keybindings)

3. **`api/pkg/external-agent/wolf_executor.go`** updates
   - Added `DesktopType` enum (Sway, XFCE, Gnome)
   - Added `getDesktopImage()` function to map desktop types to Docker images
   - Added `parseDesktopType()` and `getDesktopTypeFromEnv()` helper functions
   - Updated `createSwayWolfApp()` to support desktop parameter

4. **`./stack` script** updates
   - Added `build-gnome` command (follows same pattern as `build-sway`)
   - Updated help messages to show all three desktop options

5. **`design/2025-11-04-kai-helix-code-multiple-desktops.md`**
   - Comprehensive design document for multi-desktop support

**Initial test**: Created GNOME container, but encountered errors.

---

### Phase 2: First Issue - Systemd Errors

**Problem**: Container logs showed systemd-related errors:

```
gnome-session-binary[235]: WARNING: Failed to upload environment to systemd
gnome-session-binary[235]: WARNING: Falling back to non-systemd startup procedure
gnome-session-binary[235]: WARNING: Could not get session id for session
```

**Initial hypothesis**: GNOME requires systemd for session management, but systemd isn't running in containers.

**Attempted fix**: Modified `startup-app.sh` to use `dbus-run-session -- gnome-session` (like Sway does with `dbus-run-session -- sway`).

**Result**: Still saw errors. The approach conflicted with how the Zorin image expected to be used.

---

### Phase 3: Second Issue - Sway Being Executed

**Problem**: Container logs showed Sway binary being executed:

```
[2025-11-04 13:07:44] [Sway] - Starting: `/opt/gow/xorg.sh`
00:00:00.001 [ERROR] [sway/main.c:62] !!! Proprietary Nvidia drivers are in use !!!
```

This was **very confusing** - we built a GNOME image, why is Sway running?

**Investigation 1**: Checked if our Dockerfile accidentally installed Sway.
- Result: No Sway installation in `Dockerfile.zorin-helix`

**Investigation 2**: Checked `helix-gnome:latest` image history:
```bash
docker history helix-gnome:latest --no-trunc | grep sway
# Found: "apt-get install -y sway xwayland..." from 9 days ago
```

**Discovery**: The `helix-gnome:latest` image was built 9 days ago with an old Dockerfile that installed Sway. Docker was using the cached old image!

**Fix attempt**: Removed old image and rebuilt:
```bash
docker rmi helix-gnome:latest
./stack build-gnome
```

**Investigation 3**: Checked if base Zorin image has Sway:
```bash
docker run --rm ghcr.io/mollomm1/gow-zorin-18:latest which sway
# Result: /usr/bin/sway
```

**Discovery**: The community Zorin image **includes both GNOME and Sway** to support multiple desktop environments!

**Result**: Still saw `[Sway] - Starting` errors in new containers.

---

### Phase 4: Root Cause Discovery - RUN_SWAY=1 Set Unconditionally

**Critical observation**: Even though we set `HELIX_DESKTOP=gnome`, containers still had `RUN_SWAY=1` in their environment.

**Investigation**: Checked container environment:
```bash
docker exec <container-id> env | grep RUN_SWAY
# Result: RUN_SWAY=1
```

**Code audit**: Found `RUN_SWAY=1` being set in THREE locations:

1. **`createSwayWolfApp()` (wolf_executor.go:154-157)** - ✅ CORRECT
   ```go
   if desktop == DesktopSway {
       env = append(env, "RUN_SWAY=1")
   }
   ```
   This code correctly only sets RUN_SWAY for Sway desktop.

2. **`recreateWolfAppForInstance()` (wolf_executor.go:1672)** - ❌ WRONG
   ```go
   env := []string{
       "GOW_REQUIRED_DEVICES=/dev/input/* /dev/dri/* /dev/nvidia*",
       "RUN_SWAY=1", // Always sets this, ignores HELIX_DESKTOP!
   ```
   This legacy code always set `RUN_SWAY=1` regardless of desktop type.

3. **`createSwayWolfAppForAppsMode()` (wolf_executor_apps.go:1158)** - ❌ WRONG
   ```go
   env := []string{
       "GOW_REQUIRED_DEVICES=/dev/input/* /dev/dri/* /dev/nvidia*",
       "RUN_SWAY=1", // Always sets this, doesn't support desktop selection!
   ```
   This legacy code also always set `RUN_SWAY=1`.

**Why this broke GNOME**: The GOW launcher function checks `if [ -n "$RUN_SWAY" ]` and if set, launches Sway instead of executing the provided command. Our startup-app.sh was never executed!

---

### Phase 5: Second Root Cause - Wrong Mount Path

**Critical observation**: None of our echo statements from `startup-app.sh` appeared in container logs:
- No "Starting Helix Personal Dev Environment with GNOME/Zorin..."
- No "✅ Zed state symlinks created"
- No "✅ GNOME autostart entries created"

**Investigation**: Checked what startup script GOW actually calls.

**Discovery**: The Zorin base image has a default `/opt/gow/startup.sh`:
```bash
#!/bin/bash
source /opt/gow/launch-comp.sh
launcher /opt/gow/xorg.sh
```

**Our mistake**: We mounted our script to `/opt/gow/startup-app.sh`, but the Zorin image calls `/opt/gow/startup.sh` (no `-app` suffix).

**Naming convention difference**:
- **Games-on-Whales images** (XFCE, Sway): Call `/opt/gow/startup-app.sh`
- **Community Zorin image**: Calls `/opt/gow/startup.sh`

This explains why Sway worked but GNOME didn't - different base images use different conventions!

---

## Final Solution

### Fix 1: Correct Mount Path

**Changed in `wolf_executor.go` (line 211)**:
```go
case DesktopGnome:
    mounts = append(mounts,
        // Changed from: /opt/gow/startup-app.sh
        fmt.Sprintf("%s/wolf/zorin-config/startup-app.sh:/opt/gow/startup.sh:ro", helixHostHome),
        // Now mounts to /opt/gow/startup.sh (what Zorin expects)
```

**Changed in `Dockerfile.zorin-helix` (line 95)**:
```dockerfile
# Before: ADD wolf/gnome-config/startup-app.sh /opt/gow/startup-app.sh
# After:
ADD wolf/zorin-config/startup-app.sh /opt/gow/startup.sh
RUN chmod +x /opt/gow/startup.sh
```

### Fix 2: Update `recreateWolfAppForInstance()` (wolf_executor.go:1662-1770)

**Changes**:
1. Read desktop type via `getDesktopTypeFromEnv()`
2. Only add `RUN_SWAY=1` if desktop is Sway
3. Use correct desktop image via `getDesktopImage(desktop)`
4. Mount desktop-specific config files in dev mode

**Key code**:
```go
// Determine desktop type from environment variable
desktop := getDesktopTypeFromEnv()

// Only add RUN_SWAY for Sway desktop
if desktop == DesktopSway {
    env = append(env, "RUN_SWAY=1")
}

// Mount desktop-specific config files
switch desktop {
case DesktopSway:
    // Mount sway-config/startup-app.sh to /opt/gow/startup-app.sh
case DesktopZorin:
    // Mount zorin-config/startup-app.sh to /opt/gow/startup.sh (different path!)
```

### Fix 3: Update `createSwayWolfAppForAppsMode()` (wolf_executor_apps.go:1155-1228)

**Changes**:
1. Read desktop type from `config.Desktop` parameter
2. Only add `RUN_SWAY=1` if desktop is Sway
3. Mount desktop-specific config files based on desktop type

**Key code**:
```go
// Determine desktop type (use from config if set, otherwise default to Sway)
desktop := config.Desktop
if desktop == "" {
    desktop = DesktopSway
}

// Add desktop-specific environment variables
if desktop == DesktopSway {
    env = append(env, "RUN_SWAY=1")
}
// XFCE and Zorin don't need special flags - GOW detects them automatically
```

### Fix 4: Defensive Programming in startup-app.sh

**Added safeguard** (line 140):
```bash
# CRITICAL: Unset RUN_SWAY to prevent GOW launcher from starting Sway
# The base Zorin image includes both GNOME and Sway
# GOW's launcher checks "if [ -n $RUN_SWAY ]" and starts Sway if set
unset RUN_SWAY

source /opt/gow/launch-comp.sh
launcher /opt/gow/xorg.sh
```

This provides defense-in-depth in case the environment variable is set incorrectly.

---

## Technical Details

### How GOW Launcher Works

The Games-on-Whales launcher function (in `/opt/gow/launch-comp.sh`):

```bash
function launcher() {
  if [ -n "$RUN_SWAY" ]; then
    gow_log "[Sway] - Starting: \`$@\`"
    # ... starts Sway compositor
    dbus-run-session -- sway --unsupported-gpu
  else
    gow_log "[exec] Starting: $@"
    exec $@
  fi
}
```

**Key insight**: The launcher checks `if [ -n "$RUN_SWAY" ]` - if the variable is set to ANY value (even empty string), it launches Sway. Only when the variable is completely unset does it execute the provided command.

### Why Zorin Image Uses Different Path

The community Zorin image (`ghcr.io/mollomm1/gow-zorin-18:latest`) includes a default `/opt/gow/startup.sh` that calls `launcher /opt/gow/xorg.sh`. The official Games-on-Whales images call `/opt/gow/startup-app.sh` instead.

**Naming convention summary**:
- **Official GOW images**: `/opt/gow/startup-app.sh`
- **Community Zorin image**: `/opt/gow/startup.sh`

This difference is likely because the Zorin image maintainer followed a different convention or predates the `-app` suffix standardization.

### Desktop Type Detection Flow

```
User sets HELIX_DESKTOP=gnome environment variable
           ↓
API reads via getDesktopTypeFromEnv()
           ↓
Returns DesktopGnome (or DesktopZorin after rename)
           ↓
createSwayWolfApp() only adds RUN_SWAY=1 for DesktopSway
           ↓
Container starts without RUN_SWAY set
           ↓
GOW launcher sees RUN_SWAY unset, executes provided command
           ↓
Our startup.sh runs and starts GNOME via /opt/gow/xorg.sh
```

---

## Files Modified

### 1. `Dockerfile.zorin-helix`
- **Line 87**: Changed path from `wolf/gnome-config/` to `wolf/zorin-config/`
- **Line 95**: Changed mount from `/opt/gow/startup-app.sh` to `/opt/gow/startup.sh`
- Added comment explaining Zorin image naming convention

### 2. `api/pkg/external-agent/wolf_executor.go`
- **Line 211**: Fixed GNOME mount path to `/opt/gow/startup.sh`
- **Lines 1662-1770**: Updated `recreateWolfAppForInstance()` to:
  - Read desktop type via `getDesktopTypeFromEnv()`
  - Only set `RUN_SWAY=1` for Sway desktop
  - Use correct desktop image and mounts

### 3. `api/pkg/external-agent/wolf_executor_apps.go`
- **Lines 1155-1228**: Updated `createSwayWolfAppForAppsMode()` to:
  - Accept desktop type from `config.Desktop` parameter
  - Only set `RUN_SWAY=1` for Sway desktop
  - Mount desktop-specific config files

### 4. `wolf/gnome-config/` → `wolf/zorin-config/`
- Renamed directory to reflect it's Zorin-specific (not generic GNOME)
- Updated references in Dockerfile and wolf_executor.go

### 5. `wolf/zorin-config/startup-app.sh`
- **Line 140**: Added `unset RUN_SWAY` safeguard
- Added comments explaining the fix

---

## Testing Checklist

After applying fixes, verify:

### 1. Environment Variables
```bash
docker exec <gnome-container-id> env | grep RUN_SWAY
# Expected: Empty (no output) for Zorin containers
# Expected: RUN_SWAY=1 for Sway containers
```

### 2. Startup Script Execution
```bash
docker logs <gnome-container-id> | head -50
# Expected to see:
# - "Starting Helix Personal Dev Environment with GNOME/Zorin..."
# - "✅ Zed state symlinks created"
# - "✅ GNOME autostart entries created"
# - "Starting GNOME via Zorin's default startup mechanism..."

# Should NOT see:
# - "[Sway] - Starting: `/opt/gow/xorg.sh`"
# - "[ERROR] [sway/main.c:62]"
```

### 3. GNOME Desktop Starts
- Connect via Moonlight client
- Verify GNOME desktop loads (not Sway tiling)
- Verify dark theme applied
- Verify Helix wallpaper set
- Verify traditional overlapping windows

### 4. Zed Integration
- Verify Zed launches automatically after ~5 seconds
- Verify Zed connects to Helix WebSocket
- Verify settings-sync-daemon running
- Verify screenshot-server running

### 5. Multiple Desktop Types
- Create Sway container: Should see `[Sway] - Starting`
- Create XFCE container: Should start XFCE desktop
- Create Zorin container: Should start GNOME desktop
- All three should coexist without conflicts

---

## Lessons Learned

### 1. Community Images May Have Different Conventions
The official Games-on-Whales images call `/opt/gow/startup-app.sh`, but the community Zorin image calls `/opt/gow/startup.sh`. Always check the base image's expected conventions before assuming they match official images.

**Takeaway**: When using community images, inspect their startup flow and naming conventions. Don't assume they follow the same patterns as official images.

### 2. Environment Variables Affect Control Flow
Setting `RUN_SWAY=1` changes the GOW launcher's behavior entirely. Even an empty string counts as "set" in bash's `[ -n "$VAR" ]` check.

**Takeaway**: Be very careful about unconditionally setting environment variables that affect control flow. Only set them when the specific behavior is desired.

### 3. Docker Image Caching Can Hide Issues
Our old `helix-gnome:latest` image from 9 days ago had Sway installed, masking the real issue (wrong mount path + RUN_SWAY=1).

**Takeaway**: When troubleshooting, always verify you're testing the latest image build. Use `docker history` to inspect image layers and their age.

### 4. Legacy Code Requires Desktop-Aware Updates
The initial implementation added desktop support to `createSwayWolfApp()`, but two other functions (`recreateWolfAppForInstance` and `createSwayWolfAppForAppsMode`) still had hardcoded Sway assumptions.

**Takeaway**: When adding multi-variant support, audit ALL code paths that create/configure instances, not just the primary creation function.

### 5. Base Images May Include Multiple Desktops
The Zorin image includes both GNOME and Sway to support multiple use cases. This is flexible but requires explicit desktop selection via environment variables.

**Takeaway**: Don't assume base images only have one desktop. Check what's installed and how to select between options.

### 6. Defensive Programming Is Valuable
Adding `unset RUN_SWAY` in the startup script provides defense-in-depth, catching cases where the variable might be set incorrectly.

**Takeaway**: When dealing with external systems (GOW launcher), add defensive checks even if your code "should" be correct. Environment variables can leak in from unexpected places.

---

## Future Enhancements

### 1. Per-User Desktop Preference
Store desktop preference in user profile:
```go
type User struct {
    PreferredDesktop string `json:"preferred_desktop"`
}
```

### 2. Per-Session Desktop Selection
Allow specifying desktop when creating PDE or external agent:
```typescript
createPersonalDevEnvironment({
    name: "My PDE",
    desktop: "zorin",  // or "xfce", "sway"
})
```

### 3. Desktop Metrics and Recommendations
Track which desktops users prefer and which have best performance/reliability.

### 4. Additional Desktop Options
- KDE Plasma (modern, feature-rich)
- MATE (lightweight GNOME fork)
- Cinnamon (Linux Mint desktop)

---

## Summary

Getting Zorin/GNOME desktop working required:

1. ✅ **Correct mount path**: `/opt/gow/startup.sh` (not `/opt/gow/startup-app.sh`)
2. ✅ **Desktop-aware RUN_SWAY**: Only set for Sway, not for other desktops
3. ✅ **Update legacy code**: Fix `recreateWolfAppForInstance` and `createSwayWolfAppForAppsMode`
4. ✅ **Defensive programming**: `unset RUN_SWAY` in startup script
5. ✅ **Clear naming**: Renamed `gnome-config` to `zorin-config` to reflect specificity

The key insight was understanding that the Zorin image uses different naming conventions than official GOW images, and that `RUN_SWAY=1` was being set unconditionally in legacy code paths.

After these fixes, users can now choose between:
- **Sway** - Lightweight tiling compositor (150MB)
- **XFCE** - Traditional desktop (250MB)
- **Zorin/GNOME** - Full-featured desktop (500MB)

All three work correctly with Wolf/GStreamer streaming and maintain full Zed integration.
