# KDE vs Sway Compositor Architecture Deep Dive

**Date:** 2025-12-28
**Status:** Resolved
**Problem:** KDE desktop shows black screen with cursor

## Executive Summary

KDE required three fixes to work properly in our nested Wayland architecture:

1. **KWin version bug** - Plasma 6.3 (Ubuntu 25.04) has a nested compositor bug. Fixed by upgrading to Plasma 6.5.4 via Ubuntu 25.10 + Kubuntu Backports PPA.

2. **Socket configuration** - KWin creates wayland-0 for clients but apps inherited wayland-1 from Wolf. Fixed by setting `WAYLAND_DISPLAY=wayland-0` in the KDE session startup.

3. **Missing kitty terminal** - Zed startup script uses kitty for visible output. Missing kitty caused 60s timeout before Zed launched.

## Root Causes and Fixes

### Root Cause 1: KWin Nested Compositor Bug (PRIMARY)

**Symptom:** Black screen with cursor, no desktop visible.

**Cause:** Plasma 6.3 (Ubuntu 25.04 default) has a bug in KWin's nested Wayland compositor mode. This was confirmed by GOW developers who saw the same issue.

**Fix:** Upgrade to Plasma 6.5.4 using Ubuntu 25.10 + Kubuntu Backports PPA.

```dockerfile
# Dockerfile.kde-helix
FROM ubuntu:25.10

# Add Kubuntu Backports PPA for Plasma 6.5
RUN add-apt-repository -y ppa:kubuntu-ppa/backports
RUN apt-get update && apt-get install -y kde-plasma-desktop
```

**Verification:**
```bash
kwin_wayland --version  # Should show 6.5.4
plasmashell --version   # Should show 6.5.4
```

### Root Cause 2: Wayland Socket Misconfiguration

**Symptom:** Apps appear fullscreen without window decorations (before KWin version fix).

**Cause:** KWin creates wayland-0 for client apps, but Wolf sets `WAYLAND_DISPLAY=wayland-1` for the container. Apps inherited wayland-1 and bypassed KWin.

**Fix:** Set `WAYLAND_DISPLAY=wayland-0` for the entire KDE session in `wolf/kde-config/startup-app.sh`:

```bash
# Inside gow_start_kde heredoc script:
export WAYLAND_DISPLAY=wayland-0
```

**Why this works:** The `kwin_wayland_wrapper` script is created with a non-quoted heredoc (`<<EOF`), so `$WAYLAND_DISPLAY` (wayland-1) is captured at script creation time. KWin connects to wayland-1, but all child processes get wayland-0.

### Root Cause 3: Missing Kitty Terminal

**Symptom:** Zed doesn't start for 60 seconds, then appears.

**Cause:** `start-zed-helix.sh` uses kitty to show visible terminal output during startup. Ubuntu 25.10 base image doesn't include kitty.

**Fix:** Add kitty to `Dockerfile.kde-helix`:

```dockerfile
RUN apt-get install -y kitty
```

### Additional Issue: Screenshot Popups

**Symptom:** Spectacle (KDE screenshot tool) pops up every few seconds, making the navbar drop down.

**Cause:** Our screenshot-server used spectacle for KDE screenshots. Even with `-b` (background) flag, spectacle briefly shows a window.

**Fix:** Use grim on Wolf's outer compositor (wayland-1) instead of spectacle on KWin (wayland-0). Wolf uses Cage/wlroots which supports `wlr-screencopy` protocol.

```go
// api/cmd/screenshot-server/main.go
func captureScreenshotKDE(format string, quality int) ([]byte, string, error) {
    // Use grim on wayland-1 (Wolf's outer compositor)
    // Wolf uses Cage/wlroots which supports wlr-screencopy
    waylandSockets := []string{"wayland-1"}
    // ... grim capture logic
}
```

### Additional Issue: Display Scaling

**Symptom:** KDE display settings panel doesn't change scaling.

**Cause:** Nested Wayland limitation - KDE settings change KWin's output, but Wolf captures from its outer compositor. KWin can't change Wolf's output resolution.

**Fix:** Use environment-based scaling. Pass `HELIX_DISPLAY_SCALE` env var from Wolf executor, startup script sets Qt/GDK scaling vars:

```bash
# wolf/kde-config/startup-app.sh
if [ -n "$HELIX_DISPLAY_SCALE" ] && [ "$HELIX_DISPLAY_SCALE" != "1" ]; then
    export QT_SCALE_FACTOR=$HELIX_DISPLAY_SCALE
    export GDK_SCALE=$HELIX_DISPLAY_SCALE
    export PLASMA_USE_QT_SCALING=1
fi
```

## Architecture Explanation

### Wolf → Sway (Single Socket Model)

```
┌─────────────────────────────────────────────────────────────────┐
│ Wolf (Cage/wlroots)                                             │
│ Creates WAYLAND_DISPLAY=wayland-1                               │
└───────────────────────────┬─────────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────────┐
│ Sway (nested compositor)                                        │
│ - Connects to wayland-1 as parent                               │
│ - Does NOT create separate socket                               │
│ - Apps use wayland-1 directly                                   │
│                                                                 │
│   ┌─────────────┐  ┌─────────────┐                              │
│   │ Zed         │  │ Kitty       │  ← All use wayland-1         │
│   └─────────────┘  └─────────────┘                              │
└─────────────────────────────────────────────────────────────────┘
```

### Wolf → KDE (Dual Socket Model)

```
┌─────────────────────────────────────────────────────────────────┐
│ Wolf (Cage/wlroots)                                             │
│ Creates WAYLAND_DISPLAY=wayland-1                               │
│ grim captures from here for screenshots                         │
└───────────────────────────┬─────────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────────┐
│ KWin (nested compositor)                                        │
│ - Connects to wayland-1 as parent                               │
│ - Creates wayland-0 for client apps                             │
│                                                                 │
│   ┌─────────────────────────────────────────────────────────┐   │
│   │ wayland-0 (KWin's client socket)                        │   │
│   │                                                         │   │
│   │   ┌─────────────┐  ┌─────────────┐  ┌─────────────┐     │   │
│   │   │ Plasmashell │  │ Dolphin     │  │ Zed         │     │   │
│   │   └─────────────┘  └─────────────┘  └─────────────┘     │   │
│   │                                                         │   │
│   │   All apps use wayland-0 → KWin provides decorations    │   │
│   └─────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────┘
```

**Key difference:** Sway reuses the parent socket; KWin creates a separate client socket.

## Files Modified

1. **Dockerfile.kde-helix** - Ubuntu 25.10 base, Kubuntu Backports PPA, kitty terminal
2. **wolf/kde-config/startup-app.sh** - WAYLAND_DISPLAY=wayland-0, scaling env vars
3. **api/cmd/screenshot-server/main.go** - Use grim on wayland-1 for KDE
4. **api/pkg/types/types.go** - Add DisplayScale field to ZedAgent
5. **api/pkg/external-agent/wolf_executor.go** - Pass HELIX_DISPLAY_SCALE if user specifies

## Testing

```bash
# Build new KDE image
./stack build-sway

# Start a NEW session (existing containers use old image)

# Verify inside container:
kwin_wayland --version    # Should show 6.5.4
ls $XDG_RUNTIME_DIR/wayland-*  # Should show wayland-0 and wayland-1
```

## References

- [GOW dev-kde branch](https://github.com/games-on-whales/gow/tree/dev-kde/apps/kde)
- [Kubuntu Backports PPA](https://launchpad.net/~kubuntu-ppa/+archive/ubuntu/backports)
