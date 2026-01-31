# GNOME Shell in Wolf Containers - Investigation

**Date:** 2025-12-29
**Status:** IN PROGRESS - Testing XWayland + GNOME X11 session approach
**Author:** Claude

## Executive Summary

**GNOME 49 removed nested Wayland support** - the `--nested` flag was X11-based and removed in commit `871ded20f`.

**Current Approach:** XWayland as Wayland client to Wolf, running GNOME as X11 session:
```
Wolf(wayland-1) → XWayland(Wayland client) → GNOME Session(X11)
```

**Alternative Solutions (if XWayland doesn't work):**

1. **PipeWire-to-Wayland Bridge** (~1,500 lines of C/Rust)
   - Run GNOME in headless mode → consume PipeWire stream → render to Wolf's Wayland

2. **Mutter Wayland Backend Patch** (~6,000+ lines of C)
   - Add a pure Wayland nested backend to Mutter

## Wolf Architecture Background

Wolf streaming architecture:
- Wolf creates `wayland-1` as the parent Wayland compositor (using gst-wayland-display, a Smithay-based micro compositor)
- Desktop environments connect to Wolf's Wayland socket as **clients**
- Wolf composites all client windows and streams via GStreamer → Moonlight
- This works with: Sway, KDE Plasma, Weston, any compositor that can run in nested mode

## Investigation

### 1. Initial Problem: logind Required

Running GNOME Shell fails with:
```
Failed to setup: Failed to find any matching session
```

Created a mock logind D-Bus service (`mock-logind.py`) to satisfy GNOME Shell's session tracking requirements. This is still useful and works correctly.

### 2. The Real Problem: Nested Mode Removed in GNOME 49

After mock logind was working, we tried running GNOME Shell:

**Attempt 1: `--devkit` mode**
```bash
gnome-shell --devkit --wayland-display=wayland-1
```
Result: Black screen with cursor. Investigation revealed `--devkit` runs in **headless mode** with PipeWire screen-cast - it does NOT render to the parent Wayland compositor.

**Attempt 2: `--nested` mode (as worked in GNOME 47)**
```bash
gnome-shell --nested --wayland-display=wayland-1
```
Result: `Failed to configure: Unknown option --nested`

**Attempt 3: Just `--wayland`**
```bash
gnome-shell --wayland --wayland-display=wayland-1
```
Result: `Failed to setup: No GPUs found` - GNOME Shell defaults to native DRM/KMS backend which requires direct GPU access.

### 3. Mutter 49 Source Code Analysis

Cloned and analyzed `/tmp/mutter` (Mutter 49.1):

**src/backends/ directory structure:**
```
src/backends/
├── native/     # DRM/KMS backend (direct GPU access)
└── [no wayland/ directory - it was removed]
```

**meta_context_main_create_backend() in src/core/meta-context-main.c:**
```c
static MetaBackend *
meta_context_main_create_backend (MetaContext *context, GError **error)
{
  MetaContextMain *context_main = META_CONTEXT_MAIN (context);

  if (context_main->options.headless ||
      context_main->options.devkit)
    return create_headless_backend (context, error);

  return create_native_backend (context, error);
}
```

**Only TWO backends exist in Mutter 49:**
1. **headless** - Virtual displays, PipeWire screen-cast streaming
2. **native** - Direct DRM/KMS GPU access

The nested Wayland backend was **completely removed**. There is no way to make GNOME Shell render to a parent Wayland compositor.

### 4. Official Confirmation

From [GNOME 49 Release Notes](https://release.gnome.org/49/developers/):
> "When developing extensions, previously it was possible to run a nested session to test out your plugin via `dbus-run-session -- gnome-shell --nested --wayland`. However, as of GNOME 49.beta1, the option was removed."

The replacement is the **Mutter Development Kit (MDK)** which:
- Uses headless mode with PipeWire streaming
- Launches a separate GTK4 viewer application to display the stream
- Is designed for GNOME Shell extension development, not production use

## Solution Options (Pure Wayland, No XWayland)

### Option 1: PipeWire-to-Wayland Bridge (RECOMMENDED)

**Architecture:**
```
┌──────────────────────────────────────────────────────────────────┐
│  Wolf (gst-wayland-display - Wayland compositor)                 │
│    │                                                             │
│    └── gnome-wolf-bridge (Wayland client)                        │
│          │                                                       │
│          ├── Receives PipeWire stream from GNOME headless        │
│          ├── Renders frames to xdg_toplevel surface              │
│          └── Forwards input via EIS (Emulated Input Subsystem)   │
│                                                                  │
│  GNOME Shell (headless mode)                                     │
│    │                                                             │
│    ├── Renders to virtual display                                │
│    ├── Screen-casts via PipeWire                                 │
│    └── Receives input via EIS                                    │
└──────────────────────────────────────────────────────────────────┘
```

**How It Works:**
1. Run GNOME Shell in **headless mode** (`gnome-shell --headless`)
2. Create a **gnome-wolf-bridge** program that:
   - Connects to Wolf's wayland-1 as a Wayland client
   - Creates an xdg_toplevel surface
   - Subscribes to GNOME's screen-cast D-Bus API
   - Receives PipeWire video stream from GNOME
   - Renders frames directly to the Wayland surface (GPU DMA-BUF)
   - Forwards input events from Wolf → GNOME via EIS

**Advantages:**
- Reuses GNOME's existing headless + MDK infrastructure
- ~1,500 lines of code (can be based on MDK's mdk-stream.c)
- No Mutter modifications needed
- Zero-copy rendering possible with DMA-BUF passthrough

**Implementation Effort:**
- Modify MDK viewer code to output to Wayland instead of GTK4
- Use wayland-client + wayland-egl instead of GDK
- Forward Wolf's wl_seat input to EIS

**Reference Code:**
- `/tmp/mutter/mdk/mdk-stream.c` - PipeWire stream handling (35KB)
- `/tmp/mutter/mdk/mdk-pipewire.c` - PipeWire connection (8KB)
- `/tmp/mutter/mdk/mdk-ei.c` - EIS input handling (6KB)
- `/tmp/wlroots/backend/wayland/` - Wayland client backend (reference)

---

### Option 2: Mutter Wayland Backend Patch

**Architecture:**
```
┌──────────────────────────────────────────────────────────────────┐
│  Wolf (gst-wayland-display - Wayland compositor)                 │
│    │                                                             │
│    └── GNOME Shell (Wayland client via patched Mutter)           │
│          │                                                       │
│          └── GTK apps connect to GNOME's nested Wayland          │
└──────────────────────────────────────────────────────────────────┘
```

**What Was Removed:**

The old `--nested` flag was actually X11-based (src/backends/x11/nested/), not Wayland.
GNOME never had a pure Wayland nested backend - only X11 nested.

Commit `871ded20f` removed:
- `src/backends/x11/` - ~90 files, ~15,000 lines
- X11-based nested compositor functionality
- The `--nested` command line flag

**What Would Be Needed:**

To create a **true Wayland nested backend**, we'd need to create:
```
src/backends/wayland/
├── meta-backend-wayland.c     (~800 lines)
├── meta-backend-wayland.h
├── meta-clutter-backend-wayland.c (~400 lines)
├── meta-monitor-manager-wayland.c (~600 lines)
├── meta-renderer-wayland.c    (~500 lines)
├── meta-seat-wayland.c        (~1,200 lines)
├── meta-output-wayland.c      (~400 lines)
├── meta-crtc-wayland.c        (~200 lines)
└── wayland-protocols/         (generated code)
```

**Required Wayland Protocols:**
- wl_compositor, wl_surface, wl_region
- xdg_wm_base, xdg_surface, xdg_toplevel
- zwp_linux_dmabuf_v1 (GPU buffer sharing)
- wl_seat, wl_keyboard, wl_pointer, wl_touch
- wp_presentation_time (frame timing)
- zwp_relative_pointer_v1 (for games)

**Implementation Reference:**
- `/tmp/wlroots/backend/wayland/` - Complete reference implementation
  - `backend.c` (~745 lines) - Main backend, registry handling
  - `output.c` (~800 lines) - xdg_toplevel outputs
  - `seat.c` (~600 lines) - Input handling

**Advantages:**
- Native integration with Mutter
- No additional streaming latency
- Single process architecture

**Disadvantages:**
- ~6,000+ lines of C code
- Requires maintaining a Mutter fork
- Must be updated for each GNOME release
- Mutter's internal APIs change frequently

---

### Option 3: GTK Apps in Sway/KDE (WORKS TODAY)

Run GTK apps directly in Sway or KDE without GNOME Shell:
- ✅ Works today with no modifications
- ✅ Full Wayland native support
- ❌ Loses GNOME Shell features (dash, extensions, etc.)
- ❌ Different user experience

---

### Comparison

| Aspect | PipeWire Bridge | Mutter Patch | Sway/KDE |
|--------|----------------|--------------|----------|
| Effort | ~1,500 LOC | ~6,000+ LOC | 0 |
| Latency | 1-2 frames | None | None |
| Maintenance | Low | High (Mutter fork) | None |
| GNOME Features | Full | Full | None |
| Upstream Friendly | Yes | Unlikely | N/A |

## PipeWire Bridge Implementation Plan

### Core Components

1. **Wayland Surface Setup** (wayland-client)
   ```c
   // Connect to Wolf's wayland-1
   struct wl_display *display = wl_display_connect("wayland-1");
   struct wl_registry *registry = wl_display_get_registry(display);
   // Bind to wl_compositor, xdg_wm_base, zwp_linux_dmabuf_v1

   // Create xdg_toplevel surface
   struct wl_surface *surface = wl_compositor_create_surface(compositor);
   struct xdg_surface *xdg_surface = xdg_wm_base_get_xdg_surface(xdg_wm_base, surface);
   struct xdg_toplevel *toplevel = xdg_surface_get_toplevel(xdg_surface);
   xdg_toplevel_set_title(toplevel, "GNOME Desktop");
   xdg_toplevel_set_fullscreen(toplevel, NULL);
   ```

2. **Screen-Cast D-Bus Session** (based on MDK)
   ```c
   // Call org.gnome.Mutter.ScreenCast.CreateSession
   // Get PipeWire node ID from session
   // Subscribe to stream
   ```

3. **PipeWire Stream Consumer** (from mdk-stream.c)
   ```c
   // Connect to PipeWire, create stream
   // Handle on_process callback for each frame
   // Get DMA-BUF or SHM buffer from PipeWire
   ```

4. **Frame Rendering** (zero-copy DMA-BUF)
   ```c
   // If PipeWire gives us DMA-BUF:
   // Create wl_buffer from DMA-BUF via zwp_linux_dmabuf_v1
   // Attach to surface, commit

   // If SHM fallback:
   // Copy pixels to wl_shm_pool buffer
   ```

5. **Input Forwarding** (EIS - Emulated Input Subsystem)
   ```c
   // Receive wl_keyboard/wl_pointer events from Wolf
   // Forward to GNOME via libei
   // Uses org.freedesktop.RemoteDesktop D-Bus interface
   ```

### File Structure

```
gnome-wolf-bridge/
├── main.c              - Entry point, event loop
├── wayland-output.c    - Wayland surface management
├── pipewire-stream.c   - PipeWire stream consumer (from MDK)
├── screencast-dbus.c   - D-Bus screen-cast session
├── eis-input.c         - Input forwarding via EIS
└── meson.build
```

### Dependencies

- wayland-client
- wayland-egl (optional, for EGL rendering)
- libpipewire-0.3
- libei (Emulated Input)
- gio-2.0 (D-Bus)
- libdrm (DMA-BUF)

## Files Created (Still Useful for Mock logind)

The mock logind service is still useful for GNOME headless mode:

1. **wolf/ubuntu-config/mock-logind.py** - Mock logind D-Bus service
   - Provides session tracking for GNOME applications
   - Enables screen locking, power management inhibitors
   - Required by GNOME Shell even in headless mode

2. **wolf/ubuntu-config/startup-app.sh** - Ubuntu startup script
   - Needs modification for PipeWire bridge approach

## Key Technical Insights

1. **GNOME 49 removed nested Wayland backend entirely** - Only native (DRM/KMS) and headless (virtual) backends remain
2. **--devkit is NOT nested mode** - It runs headlessly with PipeWire, doesn't render to parent Wayland
3. **Mock logind still works** - Useful for GNOME applications even without GNOME Shell
4. **Wolf's architecture is incompatible** - Wolf expects clients to connect to its Wayland socket; GNOME 49 can't do this
5. **KDE and Sway still work** - They maintain nested Wayland backends

## gst-wayland-display Protocols (Wolf's Compositor)

For reference, Wolf's Smithay-based compositor implements these Wayland protocols:

| Protocol | Purpose | Required By |
|----------|---------|-------------|
| wl_compositor v6 | Surface creation/management | All clients |
| xdg_shell | Window management | Desktop apps |
| wl_shm | CPU-side buffer sharing | Fallback rendering |
| zwp_linux_dmabuf | GPU buffer sharing | Hardware acceleration |
| wp_viewporter | Surface scaling/cropping | Video players |
| wp_presentation_time | Frame timing | Smooth video |
| zwp_relative_pointer | Relative mouse motion | Games, 3D apps |
| zwp_pointer_constraints | Mouse lock/confine | Games, 3D apps |
| wp_single_pixel_buffer | Solid color surfaces | KDE (added in PR #24) |
| wl_data_device | Clipboard/drag-drop | Desktop apps |
| xdg_output | Multi-monitor info | Desktop environments |
| wl_seat | Input devices | All input |
| wl_drm | Legacy Mesa buffer sharing | Older apps |

These protocols support KDE, Sway, and most Wayland applications. The issue is not missing protocols - it's that GNOME 49 fundamentally cannot run as a Wayland client.

## References

- [GNOME 49 nested sessions removed](https://discourse.gnome.org/t/gnome-49-nested-sessions-no-longer-possible/30987) - Community discussion confirming removal
- [GNOME 49 Release Notes](https://release.gnome.org/49/developers/) - Official MDK announcement
- [X11 Session Removal FAQ](https://blogs.gnome.org/alatiera/2025/06/23/x11-session-removal-faq/) - GNOME's direction away from X11
- [Fedora Wayland-Only GNOME](https://fedoraproject.org/wiki/Changes/WaylandOnlyGNOME) - Distribution changes
- [gst-wayland-display PR #24](https://github.com/games-on-whales/gst-wayland-display/pull/24) - KDE fix (single pixel buffer protocol)
- [org.freedesktop.login1 D-Bus interface](https://www.freedesktop.org/software/systemd/man/org.freedesktop.login1.html) - logind specification
- [GNOME Shell source - loginManager.js](https://gitlab.gnome.org/GNOME/gnome-shell) - Session tracking code
- [Mutter source](https://gitlab.gnome.org/GNOME/mutter) - Compositor implementation
