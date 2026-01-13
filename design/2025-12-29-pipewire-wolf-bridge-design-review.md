# PipeWire-Wolf Bridge: Deep Design Review

**Date:** 2025-12-29
**Author:** Claude
**Status:** Critical Issues Identified

## Executive Summary

After deep analysis, the current PipeWire bridge design has **fundamental architectural issues** that will prevent it from working. This document identifies these issues and proposes solutions.

## Critical Issues

### Issue 1: Portal Requires User Interaction

**Problem:** The XDG Desktop Portal screen-cast API is designed for **user-initiated** screen sharing. When you call `portal.start()`, it shows a dialog asking the user to select which screen to share.

```
Application → Portal → "Select a screen to share" dialog → User clicks → Stream starts
```

**In our headless container:** There's no user to click the dialog. The Portal will wait forever.

**Evidence:** From ashpd documentation:
> "The Start method will show a dialog to the user to select sources and confirm the screencast."

**Solutions:**
1. **Use GNOME's direct API instead of Portal** - `org.gnome.Mutter.ScreenCast` doesn't require user interaction
2. **Use `RecordVirtual` instead of `SelectSources`** - For headless mode, GNOME exposes virtual displays without selection
3. **Set `restore_token`** - Pre-authorize the session (requires initial user approval)

### Issue 2: xdg-desktop-portal Not Running in Container

**Problem:** The Portal requires `xdg-desktop-portal` daemon to be running. This is typically started by the desktop session, but in our Wolf container:

- Wolf creates a Wayland compositor
- Desktop environment starts
- But `xdg-desktop-portal` may not auto-start

**Solution:** Explicitly start xdg-desktop-portal in the startup script:
```bash
/usr/libexec/xdg-desktop-portal &
/usr/libexec/xdg-desktop-portal-gnome &  # or -kde, or -wlr
```

### Issue 3: D-Bus Session Bus in Container

**Problem:** Both Portal and GNOME's screen-cast API use the D-Bus session bus. In a container, the session bus may not exist or be incorrectly configured.

**Current state:** The startup script sets `DBUS_SESSION_BUS_ADDRESS` but we need to verify:
1. D-Bus daemon is running
2. Session bus socket exists
3. Environment variable is correctly set for all processes

**Solution:** Verify in startup script:
```bash
# Ensure D-Bus session bus
if [ -z "$DBUS_SESSION_BUS_ADDRESS" ]; then
    eval $(dbus-launch --sh-syntax)
    export DBUS_SESSION_BUS_ADDRESS
fi
```

### Issue 4: PipeWire Daemon Not Running

**Problem:** PipeWire requires the `pipewire` daemon to be running. Screen-cast creates a PipeWire stream, but if there's no PipeWire daemon, it fails.

**Solution:** Start PipeWire in the container:
```bash
pipewire &
pipewire-pulse &  # Optional, for audio
wireplumber &     # Session manager
```

### Issue 5: GNOME Headless Mode Virtual Display

**Problem:** When running `gnome-shell --headless`, GNOME doesn't automatically create a virtual display. The screen-cast API returns nothing to cast.

**Evidence:** From GNOME Remote Desktop docs:
> "For headless operation, gnome-remote-desktop creates a virtual display via GNOME's display configuration API."

**Solution:** Either:
1. Use `gnome-remote-desktop` which handles this automatically
2. Or explicitly create a virtual monitor via `org.gnome.Mutter.DisplayConfig.ApplyMonitorsConfig`

### Issue 6: Format Negotiation Between Desktop and Wolf

**Problem:** DMA-BUF requires compatible formats between producer (GNOME) and consumer (Wolf). If GNOME renders in format A but Wolf only supports format B, zero-copy fails.

**Current code:** We accept whatever format GNOME sends. But Wolf may reject it.

**Solution:**
1. Query Wolf's supported formats first (`zwp_linux_dmabuf_v1.format` events)
2. Request compatible format in PipeWire stream negotiation
3. Fall back to SHM if format negotiation fails

### Issue 7: Multi-Process Architecture Issues

**Problem:** The current design has multiple processes that need to coordinate:
1. Wolf (Wayland compositor)
2. gnome-shell (or KDE/Sway)
3. wolf-bridge (our bridge)
4. pipewire daemon
5. xdg-desktop-portal

**Startup order matters:**
```
1. D-Bus session bus ← Everything depends on this
2. PipeWire daemon ← Screen-cast needs this
3. Wolf ← Creates wayland-1
4. Desktop (GNOME/KDE/Sway) ← Connects to wayland-1
5. xdg-desktop-portal + backend ← Depends on desktop
6. wolf-bridge ← Needs all of the above
```

If any step fails or starts out of order, the whole system breaks.

## Revised Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         CORRECTED ARCHITECTURE                              │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  Wolf (wayland-1)                                                           │
│       ↑ wl_surface + zwp_linux_dmabuf_v1                                    │
│       │                                                                     │
│  wolf-bridge (Wayland client)                                               │
│       ↑ PipeWire stream (SPA_DATA_DmaBuf)                                   │
│       │                                                                     │
│  PipeWire Daemon ←── pipewire, wireplumber                                  │
│       ↑                                                                     │
│  Screen-cast Source:                                                        │
│   ├── GNOME: org.gnome.Mutter.ScreenCast.RecordVirtual (NO portal needed)  │
│   ├── KDE: org.kde.kwin.ScreenCast (direct)                                │
│   └── Sway: wlr-screencopy-unstable-v1 (Wayland protocol, not D-Bus)       │
│       ↑                                                                     │
│  Desktop Environment (headless)                                             │
│       ↑ Wayland (wayland-0 or internal)                                     │
│       │                                                                     │
│  Virtual Display                                                            │
│   ├── GNOME: Created via org.gnome.Mutter.DisplayConfig                    │
│   ├── KDE: kwin_wayland --virtual                                          │
│   └── Sway: output HEADLESS-1                                              │
│                                                                             │
│  Prerequisites: D-Bus session bus, PipeWire daemon                          │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

## Key Changes Needed

### 1. Don't Use Portal for Headless

Portal is for user-facing screen sharing. For headless:

```rust
// WRONG (current code)
let proxy = Screencast::new().await?;
let session = proxy.create_session().await?;
proxy.select_sources(&session, ...).await?;  // ← Shows dialog, waits for user
proxy.start(&session, ...).await?;

// CORRECT for headless GNOME
let screencast = GnomeScreenCast::new()?;  // Direct D-Bus, not Portal
let session = screencast.create_session()?;
let stream = session.record_virtual()?;    // ← No user interaction needed
let node_id = stream.pipewire_node_id();
```

### 2. Add Proper Startup Sequence

```bash
#!/bin/bash
# start-wolf-bridge.sh

# 1. D-Bus session bus
if [ -z "$DBUS_SESSION_BUS_ADDRESS" ]; then
    eval $(dbus-launch --sh-syntax)
fi

# 2. PipeWire
pipewire &
sleep 0.5
wireplumber &
sleep 0.5

# 3. Wait for Wolf's wayland-1
for i in {1..30}; do
    [ -S "${XDG_RUNTIME_DIR}/wayland-1" ] && break
    sleep 0.1
done

# 4. Start desktop (headless)
gnome-shell --headless &

# 5. Wait for GNOME to be ready
gdbus wait --session --timeout 30 org.gnome.Mutter.ScreenCast

# 6. Start bridge
wolf-bridge --display wayland-1
```

### 3. Desktop-Specific Implementations

Each desktop needs different handling:

| Desktop | Screen-Cast API | Virtual Display |
|---------|-----------------|-----------------|
| GNOME | `org.gnome.Mutter.ScreenCast.RecordVirtual` | Auto with `--headless` |
| KDE | `org.kde.kwin.ScreenCast` | `--virtual` flag |
| Sway | `wlr-screencopy-unstable-v1` (Wayland) | `output HEADLESS-1` |

**Note:** Sway doesn't use D-Bus for screen capture - it uses a Wayland protocol extension!

### 4. Format Negotiation

```rust
// Query Wolf's supported DMA-BUF formats first
let wolf_formats = wayland.get_dmabuf_formats();

// Request compatible format from PipeWire
let stream_params = build_params_with_formats(&wolf_formats);
pipewire.connect(node_id, stream_params);
```

## Sway: Different Approach Needed

Sway uses `wlr-screencopy-unstable-v1` Wayland protocol, not D-Bus/Portal:

```
┌─────────────────────────────────────────────────────────────────────────────┐
│  SWAY ARCHITECTURE (different from GNOME/KDE!)                              │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  Wolf (wayland-1)                                                           │
│       ↑ wl_surface                                                          │
│       │                                                                     │
│  wolf-bridge                                                                │
│       ↑                                                                     │
│  zwlr_screencopy_manager_v1 (Wayland protocol, not D-Bus!)                 │
│       ↑                                                                     │
│  Sway (nested in Wolf)                                                      │
│                                                                             │
│  OR: Use xdg-desktop-portal-wlr which converts wlr-screencopy to PipeWire  │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

For Sway, we have two options:
1. **Use wlr-screencopy directly** - Wayland extension, no PipeWire
2. **Use xdg-desktop-portal-wlr** - Converts to PipeWire, but needs user interaction

## Recommendation

1. **For GNOME:** Use `org.gnome.Mutter.ScreenCast.RecordVirtual` (already implemented in C version)
2. **For KDE:** Use `org.kde.kwin.ScreenCast` (similar to GNOME)
3. **For Sway:** Use `wlr-screencopy-unstable-v1` protocol directly (different implementation)

The Portal abstraction (`ashpd`) is **not suitable for headless screen capture**. Each desktop needs its own direct API.

## Action Items

1. [ ] Remove Portal dependency from Rust implementation
2. [ ] Implement GNOME direct D-Bus API (`org.gnome.Mutter.ScreenCast`)
3. [ ] Add startup script with proper daemon ordering
4. [ ] Add format negotiation between desktop and Wolf
5. [ ] For Sway: implement wlr-screencopy as alternative path
6. [ ] Test each desktop environment separately

## Conclusion

The current design conflates "user-facing screen sharing" (Portal) with "headless screen capture" (direct APIs). These are fundamentally different use cases requiring different implementations.

The C implementation with `screencast.c` (GNOME direct API) is closer to correct than the Rust Portal-based approach.
