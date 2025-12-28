# KDE vs Sway Compositor Architecture Deep Dive

**Date:** 2025-12-28
**Status:** Investigation
**Problem:** KDE desktop shows either fullscreen windows without decorations OR black screen with cursor

## Executive Summary

The KDE and Sway desktop containers have fundamentally different Wayland compositor architectures. Sway uses a **single-socket** model where apps connect to the same socket as the compositor. KDE uses a **nested-socket** model where KWin creates a separate socket for client applications. This difference causes apps to bypass KWin when using the wrong socket.

## Architecture Diagrams

### Wolf → Sway (WORKING)

```
┌─────────────────────────────────────────────────────────────────┐
│ Wolf (gst-wayland-src)                                          │
│ Creates WAYLAND_DISPLAY=wayland-1 for container                 │
└───────────────────────────┬─────────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────────┐
│ Container Environment                                           │
│ WAYLAND_DISPLAY=wayland-1                                       │
│                                                                 │
│   ┌───────────────────────────────────────────────────────┐    │
│   │ Sway (nested compositor)                               │    │
│   │ - Connects to wayland-1 as parent                      │    │
│   │ - Does NOT create separate socket                      │    │
│   │ - Provides window management on wayland-1              │    │
│   │                                                        │    │
│   │   ┌─────────────────┐  ┌─────────────────┐            │    │
│   │   │ Zed             │  │ Kitty           │            │    │
│   │   │ WAYLAND_DISPLAY │  │ WAYLAND_DISPLAY │            │    │
│   │   │ = wayland-1     │  │ = wayland-1     │            │    │
│   │   └─────────────────┘  └─────────────────┘            │    │
│   │                                                        │    │
│   │   Apps use SAME socket as Sway → Window decorations ✓ │    │
│   └────────────────────────────────────────────────────────┘    │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

**Why Sway works:**
- Sway is a Wayland compositor that connects to the parent (wayland-1)
- Sway does NOT create a separate socket for clients
- Apps inherit `WAYLAND_DISPLAY=wayland-1` and connect to Sway through the same socket
- Sway handles window management (decorations, tiling) for apps on wayland-1

### Wolf → KDE (BROKEN)

```
┌─────────────────────────────────────────────────────────────────┐
│ Wolf (gst-wayland-src)                                          │
│ Creates WAYLAND_DISPLAY=wayland-1 for container                 │
└───────────────────────────┬─────────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────────┐
│ Container Environment                                           │
│ WAYLAND_DISPLAY=wayland-1                                       │
│                                                                 │
│   ┌────────────────────────────────────────────────────────┐   │
│   │ KWin (nested compositor)                                │   │
│   │ Connects to wayland-1 via --wayland-display             │   │
│   │ Creates NEW socket: wayland-0 for clients               │   │
│   │                                                         │   │
│   │   ┌─────────────────────────────────────────────────┐  │   │
│   │   │ wayland-0 socket (for KWin clients)              │  │   │
│   │   │                                                  │  │   │
│   │   │   ┌──────────────┐  ┌──────────────┐            │  │   │
│   │   │   │ Plasmashell  │  │ Dolphin      │            │  │   │
│   │   │   │ (SHOULD be   │  │ (SHOULD be   │            │  │   │
│   │   │   │ wayland-0)   │  │ wayland-0)   │            │  │   │
│   │   │   └──────────────┘  └──────────────┘            │  │   │
│   │   └──────────────────────────────────────────────────┘  │   │
│   └─────────────────────────────────────────────────────────┘   │
│                                                                 │
│   ┌─────────────────────────────────────────────────────────┐   │
│   │ BROKEN: Apps connecting to wayland-1 (parent)            │   │
│   │                                                          │   │
│   │   ┌──────────────────┐  ┌──────────────────┐            │   │
│   │   │ Zed              │  │ Kitty            │            │   │
│   │   │ WAYLAND_DISPLAY  │  │ WAYLAND_DISPLAY  │            │   │
│   │   │ = wayland-1 ✗    │  │ = wayland-1 ✗    │            │   │
│   │   └──────────────────┘  └──────────────────┘            │   │
│   │                                                          │   │
│   │   Apps bypass KWin → NO window decorations! ✗            │   │
│   └──────────────────────────────────────────────────────────┘   │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

## Key Technical Differences

| Aspect | Sway | KDE/KWin |
|--------|------|----------|
| **Socket model** | Single socket (reuses parent) | Dual socket (creates nested) |
| **Client connection** | Apps connect to same socket as Sway | Apps should connect to KWin's socket |
| **Parent socket** | wayland-1 (used by both Sway and apps) | wayland-1 (KWin only) |
| **Client socket** | wayland-1 (same as parent) | wayland-0 (created by KWin) |
| **Environment inheritance** | Works correctly | Causes apps to bypass KWin |

## Root Cause Analysis

### How Wolf Sets Up Containers

Wolf executor sets `WAYLAND_DISPLAY=wayland-1` for all containers:

```go
// wolf_executor.go:1375
envVars := []string{
    ...
    "WAYLAND_DISPLAY=wayland-1",
}
```

This works for Sway because Sway doesn't create a separate socket - apps use wayland-1.

For KDE, this causes problems because:
1. KWin connects to wayland-1 as its parent (correct)
2. KWin creates wayland-0 for clients (correct)
3. Apps inherit WAYLAND_DISPLAY=wayland-1 (WRONG - should be wayland-0)
4. Apps connect to wayland-1, bypassing KWin (BUG)

### How KWin Creates the Nested Socket

The kwin_wayland_wrapper script passes the parent socket:

```bash
/usr/bin/kwin_wayland_wrapper \
    --width $GAMESCOPE_WIDTH \
    --height $GAMESCOPE_HEIGHT \
    --wayland-display $WAYLAND_DISPLAY \  # Parent socket (wayland-1)
    --xwayland \
    --no-lockscreen \
    $@
```

KWin then:
1. Connects to wayland-1 (the parent/Wolf compositor)
2. Creates wayland-0 for its own clients (default when parent is not wayland-0)
3. Starts plasmashell and other session components

### The Missing Link

The issue is that `startplasma-wayland` starts session components (plasmashell, etc.) as children of the KDE session, but:
- These components SHOULD use wayland-0 (KWin's socket)
- If they inherit wayland-1 from the environment, they bypass KWin

In a normal KDE installation:
- KWin runs directly on hardware, creating wayland-0
- WAYLAND_DISPLAY is set to wayland-0 by the display manager
- All apps connect to wayland-0

In our nested setup:
- KWin runs inside a container, connecting to wayland-1 (parent)
- KWin creates wayland-0 for its clients
- But WAYLAND_DISPLAY=wayland-1 is already set by Wolf
- Apps use wayland-1 and bypass KWin

## GOW Reference Implementation

The GOW dev-kde branch startup script is simple:

```bash
#!/bin/bash -e
source /opt/gow/bash-lib/utils.sh

# Create wrapper to pass parent socket to kwin
cat <<EOF > $XDG_RUNTIME_DIR/nested_kde/kwin_wayland_wrapper
#!/bin/sh
/usr/bin/kwin_wayland_wrapper --width $GAMESCOPE_WIDTH --height $GAMESCOPE_HEIGHT --wayland-display $WAYLAND_DISPLAY --xwayland --no-lockscreen \$@
EOF

dbus-run-session -- bash -c "pipewire & startplasma-wayland"
```

**Notable:**
1. GOW does NOT explicitly set `WAYLAND_DISPLAY=wayland-0` for apps
2. GOW sets `XDG_RUNTIME_DIR=/tmp/.X11-unix` in the Dockerfile
3. No special handling for app socket selection

**Question:** How does GOW KDE work if it doesn't set WAYLAND_DISPLAY for apps?

**Hypothesis:** Either:
1. `startplasma-wayland` handles setting WAYLAND_DISPLAY for session components
2. KDE D-Bus session discovery works differently
3. GOW has the same bug and apps run fullscreen without decorations

## Observed Symptoms

### Before Fix (WAYLAND_DISPLAY=wayland-1 for all)
- Apps appear fullscreen without window decorations
- KDE-style mouse cursor visible (KWin is rendering)
- Apps functional but no title bars, panels, etc.

**Why:** Apps connect to wayland-1 (parent), bypassing KWin's window management.

### After Fix (WAYLAND_DISPLAY=wayland-0 for Zed only)
- Black screen with mouse cursor only
- No visible apps or desktop

**Why:** Unknown - need to investigate. Possibilities:
1. wayland-0 doesn't exist or has wrong permissions
2. Timing issue - Zed connects before KWin creates socket
3. KWin isn't creating wayland-0 as expected
4. Other KDE components (plasmashell) also need wayland-0

## Investigation Plan

### Step 1: Verify Socket Creation
```bash
# Inside running container
ls -la $XDG_RUNTIME_DIR/wayland-*
# Expected: wayland-0 (KWin) and wayland-1 (Wolf)
```

### Step 2: Check Process Socket Connections
```bash
# What socket is each process using?
for pid in $(pgrep -x kwin_wayland); do
    cat /proc/$pid/environ | tr '\0' '\n' | grep WAYLAND
done
for pid in $(pgrep -x plasmashell); do
    cat /proc/$pid/environ | tr '\0' '\n' | grep WAYLAND
done
```

### Step 3: Verify KWin Is Creating Client Socket
```bash
# Check KWin command line
ps aux | grep kwin_wayland
# Should show --wayland-display wayland-1
```

### Step 4: Test Manual Socket Override
```bash
# Inside container, after KDE starts
export WAYLAND_DISPLAY=wayland-0
dolphin  # Should have decorations
```

## Implemented Solution

### Option A: Set WAYLAND_DISPLAY=wayland-0 in gow_start_kde ✅ IMPLEMENTED

The key insight is that `kwin_wayland_wrapper` is created using a heredoc **without** single quotes (`<<EOF` not `<<'EOF'`), which means `$WAYLAND_DISPLAY` is expanded at script creation time. This captures `wayland-1` in the wrapper script.

This allows us to safely change `WAYLAND_DISPLAY` to `wayland-0` for the entire KDE session without breaking KWin's connection to the parent compositor.

**Changes made to `wolf/kde-config/startup-app.sh`:**

```bash
# Inside gow_start_kde script:

# Set KDE environment variables
export XDG_CURRENT_DESKTOP=KDE
export KDE_SESSION_VERSION=6
export DESKTOP_SESSION=plasma

# CRITICAL: Set WAYLAND_DISPLAY for KDE session to use KWin's client socket
# - Wolf creates wayland-1 as the parent compositor for video streaming
# - KWin connects to wayland-1 as its parent (via --wayland-display in kwin_wayland_wrapper)
# - KWin creates wayland-0 for its client applications (default nested socket name)
# - All KDE apps (plasmashell, dolphin, Zed) must connect to wayland-0 for window decorations
# - The kwin_wayland_wrapper already captured wayland-1 in the heredoc, so KWin still
#   connects to the correct parent even though we're changing WAYLAND_DISPLAY here
export WAYLAND_DISPLAY=wayland-0

# ... rest of startup
startplasma-wayland
```

**Why this works:**

1. `kwin_wayland_wrapper` contains hardcoded `--wayland-display wayland-1` (captured during heredoc expansion)
2. KWin connects to `wayland-1` (parent) and creates `wayland-0` (for clients)
3. `WAYLAND_DISPLAY=wayland-0` is inherited by `startplasma-wayland` and all session components
4. `plasmashell`, `dolphin`, `Zed`, etc. all connect to `wayland-0` → KWin provides window decorations

**Why the previous fix failed (black screen):**

The previous fix only set `WAYLAND_DISPLAY=wayland-0` inside the Zed subshell. This meant:
- Zed tried to connect to wayland-0 ✓
- But plasmashell and other KDE components used wayland-1 ✗
- The desktop never rendered properly because KDE session components bypassed KWin

### Alternative Options (Not Implemented)

#### Option B: Let KWin Handle Environment
KDE should theoretically set `WAYLAND_DISPLAY` for session components, but Wolf's pre-set environment variable overrides this.

#### Option C: Use KWin's --socket Option
Explicitly specify `--socket wayland-0` to KWin. Not needed since wayland-0 is the default.

#### Option D: Use Cage as Intermediate Compositor
Would add unnecessary complexity.

## Files Modified

1. **wolf/kde-config/startup-app.sh** - KDE startup script (FIXED)
   - Added `export WAYLAND_DISPLAY=wayland-0` for entire KDE session
   - Removed redundant per-app WAYLAND_DISPLAY override in Zed subshell
2. **wolf_executor.go:1375** - Sets WAYLAND_DISPLAY=wayland-1 for containers (unchanged, correct)

## Testing

To verify the fix works:

1. Build new KDE image: `./stack build-sway` (uses Dockerfile.kde-helix)
2. Start a new KDE session (existing sessions won't pick up the change)
3. Verify window decorations appear on Zed and other apps
4. Check socket creation: `ls -la /run/user/*/wayland-*` should show both wayland-0 and wayland-1

## Known Issue: KWin Version Bug (from GOW developers)

**UPDATE 2025-12-28**: The GOW developers (ABeltramo, lewq) are experiencing the **exact same black screen issue** when running KDE through Wolf.

From the GOW Discord (2024-12-28):

> **ABeltramo**: "I've pushed where I was in that PR. I think it was working when running it under my host KDE Wayland sessions but it was a black screen when running thru Wolf IIRC"
>
> **lewq**: "ah, yeah I'm seeing a black screen too"
>
> **ABeltramo**: "I've asked him already and we came to the conclusion that it might be just an outdated KDE version in the base Ubuntu image. Yet another reason to try out a bleeding edge base image like Arch (which is what Nestri uses)"

**Key insight**: DatHorse got KDE working, but he was using **Arch Linux** with bleeding-edge KDE/KWin.

### Implications

1. The black screen may be caused by a **KWin nested compositor bug** in Ubuntu-packaged versions
2. This bug appears to be **fixed in newer KWin versions** (Arch has latest)
3. The socket configuration fix (`WAYLAND_DISPLAY=wayland-0`) is still architecturally correct
4. But it won't help if KWin itself has a bug preventing nested compositing

### Possible Solutions

1. **Wait for Ubuntu to update KDE** - unlikely to happen quickly
2. **Use Arch-based image** - Nestri uses this approach, but "hardcore for enterprise"
3. **Try Ubuntu 25.10** - might have newer KDE
4. **Build KWin from source** - complex but could work
5. **Use KDE Neon PPA** - bleeding edge KDE on Ubuntu base

### Version Check

To check which KDE/KWin version is installed:
```bash
kwin_wayland --version
plasmashell --version
```

### Solution Implemented: Ubuntu 25.10 + Kubuntu Backports

**UPDATE 2025-12-28 20:00 UTC**: Successfully rebuilt KDE image with Plasma 6.5.4!

Changes to `Dockerfile.kde-helix`:
1. Changed base from `ghcr.io/games-on-whales/base-app:edge` (Ubuntu 25.04) to `ubuntu:25.10`
2. Replicated GOW base-app functionality (gosu, entrypoint, init scripts)
3. Added Kubuntu Backports PPA with pin priority 1001
4. Fixed Docker repo to use `$(lsb_release -cs)` instead of hardcoded "plucky"

**Installed versions in new image:**
```
kwin-wayland:      6.5.4-0ubuntu3~ubuntu25.10~ppa1
plasma-desktop:    6.5.4-0ubuntu1~ubuntu25.10~ppa2
```

This matches the version DatHorse had when he got KDE working on Arch.

## Debugging Commands

If issues persist, run these inside the container:

```bash
# Verify socket creation
ls -la $XDG_RUNTIME_DIR/wayland-*

# Check what socket each process uses
for proc in kwin_wayland plasmashell zed; do
    pids=$(pgrep -x "$proc" 2>/dev/null)
    for pid in $pids; do
        echo "=== $proc (PID $pid) ==="
        cat /proc/$pid/environ | tr '\0' '\n' | grep WAYLAND
    done
done

# Check KWin command line
ps aux | grep kwin_wayland
```

## References

- [KWin Wayland Nested Compositor Documentation](https://invent.kde.org/plasma/kwin/-/blob/master/src/wayland/compositor.cpp)
- [GOW dev-kde branch](https://github.com/games-on-whales/gow/tree/dev-kde/apps/kde)
- [Wolf gst-wayland-src](https://gstreamer.freedesktop.org/documentation/waylandsink/index.html)
