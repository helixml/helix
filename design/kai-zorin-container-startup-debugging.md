# Zorin Container Startup Debugging Log

**Author**: Kai (with Claude Code assistance)
**Date**: 2025-11-04
**Status**: In Progress - Container exits silently, no output from startup script

## Problem Statement

Zorin/GNOME containers are being created via Wolf lobbies but exit immediately with no visible output from the startup script. The container's init scripts run successfully, but the main startup script (`/opt/gow/startup.sh`) produces no output before the container exits.

## Background Context

We are adding multi-desktop support to Helix Code, allowing users to choose between:
- **Sway** (working) - Lightweight tiling compositor
- **XFCE** (untested) - Traditional desktop
- **Zorin/GNOME** (broken) - Full-featured desktop

The Zorin implementation is based on:
- **Base image**: `ghcr.io/mollomm1/gow-zorin-18:latest` (community Zorin 18 image)
- **Startup pattern**: Copied from working Sway implementation
- **Key difference**: Different base image (community vs official GOW)

## Investigation Timeline

### Issue #1: Wrong Mount Path (FIXED ✅)
**Problem**: Startup script was mounted to `/opt/gow/startup-app.sh` but Zorin image expects `/opt/gow/startup.sh`

**Evidence**:
- Sway/XFCE use: `/opt/gow/startup-app.sh` (official GOW convention)
- Zorin uses: `/opt/gow/startup.sh` (community image convention)

**Fix**: Changed mount in `wolf_executor.go` line 214:
```go
case DesktopZorin:
    mounts = append(mounts,
        fmt.Sprintf("%s/wolf/zorin-config/startup-app.sh:/opt/gow/startup.sh:ro", helixHostHome),
```

**Result**: Script is now mounted correctly but still exits

### Issue #2: Read-Only Filesystem chmod Error (FIXED ✅)
**Problem**: Container entrypoint tries to `chmod +x /opt/gow/startup.sh`, but we mount it as read-only (`:ro`)

**Evidence from Wolf logs**:
```
chmod: changing permissions of '/opt/gow/startup.sh': Read-only file system
```

**Fix**: Made host file executable so container's chmod becomes no-op:
```bash
chmod +x wolf/zorin-config/startup-app.sh
chmod +x wolf/xfce-config/startup-app.sh
```

**Result**: chmod error gone, but script still exits silently

### Issue #3: Script Exits Silently (CURRENT ISSUE ❌)
**Problem**: NO output from startup script - not even the first `echo` statement

**Evidence from Wolf logs** (session at 14:25:54):
```
[2025-11-04 14:25:54] [ /etc/cont-init.d/10-setup_user.sh: executing... ]
[2025-11-04 14:25:54] **** Configure default user ****
[2025-11-04 14:25:54] Setting default user uid=1000(retro) gid=1000(retro)
...
[2025-11-04 14:25:54] Launching the container's startup script as user 'retro'
[ERROR] [GSTREAMER] Pipeline error: Internal data stream error.
[INFO] [LOBBY] stopping lobby 733b12d1-3622-455f-a4d8-a24d490f4f59
```

**Observations**:
- ✅ Init scripts run successfully
- ✅ Entrypoint says "Launching the container's startup script as user 'retro'"
- ❌ **NO output from script** - first line is `echo "Starting Helix..."` but it never prints
- ❌ GStreamer pipeline error appears immediately
- ❌ Lobby stops within 1 second

**Testing in isolation**:
```bash
docker run --rm -v /home/kai/projects/helix/wolf/zorin-config/startup-app.sh:/test.sh:ro \
  --entrypoint bash helix-zorin:latest -c "bash /test.sh 2>&1 | head -10"

# Output:
Starting Helix Personal Dev Environment with GNOME/Zorin...
Created symlink: /usr/local/bin/zed -> /zed-build/zed
chown: invalid user: 'retro:retro'
```

**Key Finding**: When run directly as bash (not as user 'retro'), script DOES produce output!

## Critical Hypothesis: User 'retro' May Not Exist

**Evidence**:
```bash
# When trying to run as retro user:
error: failed switching to "retro": unable to find user retro: no matching entries in passwd file

# When testing chown:
chown: invalid user: 'retro:retro'
```

**Theory**:
- Sway base image (`ghcr.io/games-on-whales/sway:edge`) - official GOW image, has 'retro' user preconfigured
- Zorin base image (`ghcr.io/mollomm1/gow-zorin-18:latest`) - community image, may not have 'retro' user
- Entrypoint tries `exec gosu "${UNAME}" /opt/gow/startup.sh` where UNAME='retro'
- If 'retro' user doesn't exist, gosu fails silently or exec fails before script can output anything

## Files Modified

1. **wolf/zorin-config/startup-app.sh** - Fixed chown to use absolute path:
   ```bash
   sudo chown retro:retro /home/retro/work  # was: sudo chown retro:retro work
   ```

2. **wolf/zorin-config/startup-app.sh** - Made executable:
   ```bash
   chmod +x wolf/zorin-config/startup-app.sh
   ```

3. **wolf/xfce-config/startup-app.sh** - Made executable:
   ```bash
   chmod +x wolf/xfce-config/startup-app.sh
   ```

## Current State

- ✅ Correct mount path: `/opt/gow/startup.sh`
- ✅ Script is executable
- ✅ Script has correct syntax
- ✅ Script works when run directly
- ❌ Script exits silently when run via entrypoint as 'retro' user
- ❌ No output captured in Wolf logs
- ❌ Container stops immediately

## Proposed Debugging Strategy

### Step 1: Verify User Existence in Base Images

Compare user setup in working Sway vs broken Zorin:

```bash
# Check Sway base image
docker run --rm --entrypoint bash ghcr.io/games-on-whales/sway:edge -c "grep retro /etc/passwd"

# Check Zorin base image
docker run --rm --entrypoint bash ghcr.io/mollomm1/gow-zorin-18:latest -c "grep retro /etc/passwd"

# Check our built Zorin image
docker run --rm --entrypoint bash helix-zorin:latest -c "grep retro /etc/passwd"
```

### Step 2: Add Comprehensive Debug Logging

Add debug wrapper to `wolf/zorin-config/startup-app.sh` at the very top:

```bash
#!/bin/bash

# CRITICAL DEBUG: Write to file AND stdout before set -e
echo "=== ZORIN STARTUP DEBUG START ===" | tee /tmp/zorin-debug.log
echo "User: $(whoami)" | tee -a /tmp/zorin-debug.log
echo "Home: $HOME" | tee -a /tmp/zorin-debug.log
echo "Time: $(date)" | tee -a /tmp/zorin-debug.log

# Redirect all output to both stdout and debug file
exec 1> >(tee -a /tmp/zorin-debug.log)
exec 2>&1

# Trap EXIT to show us exit code and keep container alive
trap 'echo "SCRIPT EXITING WITH CODE $? at $(date)" | tee -a /tmp/zorin-debug.log; sleep 300' EXIT

# NOW set -e (after debug setup)
set -e

echo "Starting Helix Personal Dev Environment with GNOME/Zorin..."
# ... rest of script
```

**Why this works**:
1. Writes debug info BEFORE `set -e` (so it won't exit on first error)
2. Uses `tee` to write to both stdout (Wolf captures) AND file (we can inspect)
3. EXIT trap keeps container alive for 5 minutes even if script fails
4. Shows us WHO the script is running as
5. Captures exact exit code

### Step 3: Check Debug Logs After Container Exit

If container still exits, we can:

1. **Check Wolf logs** for our debug output:
   ```bash
   docker compose -f docker-compose.dev.yaml logs wolf 2>&1 | strings | grep "ZORIN STARTUP DEBUG"
   ```

2. **Quickly inspect the stopped container** (before Wolf removes it):
   ```bash
   # Get container ID from recent logs
   docker ps -a --filter "name=zed-external" --format "{{.ID}}" | head -1

   # Read the debug log file
   docker cp <container-id>:/tmp/zorin-debug.log ./zorin-debug.log
   cat ./zorin-debug.log
   ```

3. **Container will stay alive for 5 minutes** due to EXIT trap, giving us time to inspect

### Step 4: Fix Root Cause Based on Findings

**If retro user doesn't exist**:
- Option A: Add retro user creation to Dockerfile.zorin-helix
- Option B: Modify entrypoint to run as root if retro doesn't exist
- Option C: Change UNAME environment variable

**If script has other issues**:
- Debug logs will show exactly where it fails
- We'll see the actual error messages and exit code

## How Wolf Captures Container Logs

Wolf logs show container output in the line following `[DOCKER] Container logs:`. This is our primary debugging output. Example:

```
[37;1m14:25:54.869055226 DEBUG | [DOCKER] Container logs:
[2025-11-04 14:25:54] [ /etc/cont-init.d/10-setup_user.sh: executing... ]
...
O[2025-11-04 14:25:54] Launching the container's startup script as user 'retro'
```

The init scripts' output IS captured, so if our startup script had any output, it would appear here too.

## Key Learnings

1. **Community vs Official Images**: Community images (like Zorin) may have different user setups than official GOW images
2. **Silent Failures**: Scripts can fail silently when exec/gosu fails - need aggressive debug logging
3. **Wolf Log Inspection**: Wolf logs are the best way to debug container startup since containers are removed quickly
4. **Read-Only Mounts**: Scripts must be executable on host when mounted as `:ro`
5. **Mount Path Conventions**: Different base images may use different conventions (startup.sh vs startup-app.sh)

## Next Session Checklist

When we continue debugging:

- [ ] Run user verification commands (Step 1)
- [ ] Add debug logging wrapper (Step 2)
- [ ] Create new external agent session
- [ ] Capture Wolf logs with debug output
- [ ] Inspect debug log file if container stays alive
- [ ] Fix root cause based on findings
- [ ] Test that Zorin desktop actually loads
- [ ] Remove debug logging once working

## References

- Design doc: `design/kai-helix-code-zorin.md` - Full Zorin implementation details
- Wolf executor: `api/pkg/external-agent/wolf_executor.go` - Desktop type handling
- Startup script: `wolf/zorin-config/startup-app.sh` - Main startup logic
- Base image: https://github.com/mollomm1/gow-zorin-18 (community maintained)
