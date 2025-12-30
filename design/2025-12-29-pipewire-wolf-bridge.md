# PipeWire-Wolf Bridge: GPU-Accelerated GNOME Streaming

**Date:** 2025-12-29
**Status:** In Progress - pipewiresrc implementation needed
**Author:** Claude

## Executive Summary

This document explores a pure Wayland approach for running GNOME Shell inside Wolf's streaming architecture. Instead of XWayland (which adds X11 overhead), we can use **GNOME Remote Desktop's PipeWire screen-cast** and bridge it to Wolf's Wayland compositor.

**Key insight:** GNOME 49's `--devkit` mode (Mutter SDK) provides this bridge out of the box! The `mutter-devkit` viewer reads GNOME's screen via ScreenCast D-Bus API and renders to any Wayland display - including Wolf's.

## Desktop-Specific Approach

| Desktop | Video Source | Reason |
|---------|--------------|--------|
| **GNOME** | pipewiresrc (direct) | GNOME 49 removed --nested, mutter-devkit has fullscreen issues on Smithay |
| **KDE** | Nested Wayland | KWin supports nested mode, working correctly |
| **Sway** | Nested Wayland | wlroots supports nested mode, working correctly |

**GNOME-only pipewiresrc:** The pipewiresrc direct approach is only needed for GNOME because:
1. GNOME 49 removed `--nested` mode
2. The replacement `--devkit` mode spawns mutter-devkit which doesn't properly fullscreen on Wolf's Smithay compositor
3. Going direct from PipeWire to Wolf's encoder bypasses the fullscreen window problem entirely

**Sway/KDE keep nested:** These desktops support nested Wayland mode which works correctly with Wolf's compositor.

## GNOME 49 Mutter SDK Solution (Recommended)

GNOME 49 introduced the **Mutter SDK** (`--devkit` flag) which eliminates the need for a custom bridge:

```
┌─────────────────────────────────────────────────────────────────────────────┐
│  GNOME 49 Mutter SDK Architecture                                           │
│  ================================                                           │
│                                                                             │
│  Wolf(wayland-11)                                                           │
│       ↑                                                                     │
│       │ Wayland surface (fullscreen)                                        │
│       │                                                                     │
│  ┌────┴─────────────────┐                                                   │
│  │  mutter-devkit       │ ← SDK viewer, spawned by gnome-shell --devkit    │
│  │  (built-in bridge!)  │                                                   │
│  └────┬─────────────────┘                                                   │
│       │                                                                     │
│       │ ScreenCast D-Bus API + PipeWire                                     │
│       │                                                                     │
│  ┌────┴─────────────────┐     ┌──────────────────┐                         │
│  │  GNOME Shell         │────→│  Client apps     │                         │
│  │  (creates wayland-0) │     │  (Zed, nautilus) │                         │
│  └──────────────────────┘     └──────────────────┘                         │
│                                    ↓                                        │
│                              wayland-0 (Mutter socket)                      │
│                                                                             │
│  Key: mutter-devkit inherits WAYLAND_DISPLAY from gnome-shell's parent     │
│       Set WAYLAND_DISPLAY=wayland-11 (Wolf) before running --devkit        │
│       Client apps must use WAYLAND_DISPLAY=wayland-0 explicitly            │
└─────────────────────────────────────────────────────────────────────────────┘
```

### How to Use

```bash
# Save Wolf's display
export WOLF_WAYLAND_DISPLAY="${WAYLAND_DISPLAY:-wayland-1}"

# Keep WAYLAND_DISPLAY pointing to Wolf - mutter-devkit will output here
# DO NOT set WAYLAND_DISPLAY=wayland-0 before running gnome-shell!

# Start GNOME Shell in devkit mode
gnome-shell --devkit  # Creates wayland-0, spawns mutter-devkit

# Launch client apps with explicit wayland-0
WAYLAND_DISPLAY=wayland-0 zed
```

### What mutter-devkit Does

Per the [Phoronix article](https://www.phoronix.com/news/GNOME-49-Mutter-SDK):
- The SDK adds a "viewer" that connects to the mutter instance
- Creates a virtual monitor via ScreenCast D-Bus API
- Sends input to the GNOME session
- Renders the screen content to its WAYLAND_DISPLAY

**No custom GStreamer bridge needed** - mutter-devkit IS the bridge.

---

## Original Analysis (Pre-GNOME 49 / Alternative Approaches)

## Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│  Current XWayland Approach                                                  │
│  ========================                                                   │
│                                                                             │
│  Wolf(wayland-1) ─→ XWayland(Wayland client) ─→ GNOME Session(X11)         │
│                                                                             │
│  Problems:                                                                  │
│  - X11 overhead and protocol translation                                    │
│  - Older protocol, less feature-rich                                        │
│  - No direct GPU buffer sharing (requires copy through X11)                 │
└─────────────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────────────┐
│  Proposed PipeWire Bridge Approach                                          │
│  =================================                                          │
│                                                                             │
│  Wolf(wayland-1)                                                            │
│       ↑                                                                     │
│       │ wl_surface + zwp_linux_dmabuf_v1                                    │
│       │                                                                     │
│  ┌────┴─────────────────┐                                                   │
│  │  gnome-wolf-bridge   │←── GPU DMA-BUF frames (zero-copy!)               │
│  │  (Wayland client)    │                                                   │
│  └────┬─────────────────┘                                                   │
│       │                                                                     │
│       │ PipeWire stream (dmabuf)                                            │
│       │                                                                     │
│  ┌────┴─────────────────┐                                                   │
│  │  GNOME Shell         │ ← gnome-shell --headless                          │
│  │  (headless mode)     │                                                   │
│  └──────────────────────┘                                                   │
│                                                                             │
│  Advantages:                                                                │
│  - Zero-copy GPU buffers via DMA-BUF                                        │
│  - Pure Wayland, no X11 overhead                                            │
│  - Uses GNOME's existing screen-cast infrastructure                         │
└─────────────────────────────────────────────────────────────────────────────┘
```

## Can GPU Frames Go Directly via PipeWire? YES!

**PipeWire supports DMA-BUF (GPU memory) directly.** No CPU copy required.

### How It Works

1. **GNOME Shell headless mode** renders to a virtual GPU display
2. **Screen-cast API** (`org.gnome.Mutter.ScreenCast`) exposes the framebuffer
3. **PipeWire** streams frames as either:
   - `SPA_DATA_DmaBuf` - GPU memory buffers (zero-copy!)
   - `SPA_DATA_MemPtr` - CPU memory (fallback)

4. **gnome-wolf-bridge** receives DMA-BUF frames from PipeWire
5. **Wolf's `zwp_linux_dmabuf_v1`** accepts DMA-BUF directly for rendering

**The entire path can be zero-copy GPU memory** when using DMA-BUF.

### PipeWire DMA-BUF Evidence

From Mutter's MDK (Mutter Development Kit) source code:

```c
// mdk/mdk-stream.c - PipeWire stream handling
static void on_process(void *data) {
    struct pw_buffer *pw_buffer = pw_stream_dequeue_buffer(stream);
    struct spa_buffer *buffer = pw_buffer->buffer;

    if (buffer->datas[0].type == SPA_DATA_DmaBuf) {
        // GPU DMA-BUF - can be used directly!
        int dmabuf_fd = buffer->datas[0].fd;
        uint32_t offset = buffer->datas[0].chunk->offset;
        uint32_t stride = buffer->datas[0].chunk->stride;
        // Import into EGL/Vulkan and render to Wayland surface
    }
}
```

### GNOME Remote Desktop Uses This

**gnome-remote-desktop** (RDP/VNC for GNOME) uses exactly this approach:

```
GNOME Shell (headless)
    → Screen-cast (org.gnome.Mutter.ScreenCast)
        → PipeWire (DMA-BUF)
            → gnome-remote-desktop
                → RDP/VNC encode
```

We're doing the same thing, but outputting to Wolf's Wayland instead of RDP/VNC.

## Wolf's DMA-BUF Support

Wolf's gst-wayland-display (Smithay-based compositor) implements `zwp_linux_dmabuf_v1`:

| Protocol | Version | Purpose |
|----------|---------|---------|
| `zwp_linux_dmabuf_v1` | v3+ | GPU buffer import |
| `wl_drm` | v2 | Legacy DRM buffer (Mesa fallback) |

**From gst-wayland-display source:**
```rust
// The compositor can import DMA-BUF directly
// No CPU copy needed when client provides dmabuf
```

## Implementation Components

### 1. gnome-wolf-bridge (~1,200 lines)

A small program that:
1. Connects to Wolf's `wayland-1` as a Wayland client
2. Creates an `xdg_toplevel` surface (fullscreen)
3. Calls GNOME's `org.gnome.Mutter.ScreenCast` D-Bus API
4. Receives PipeWire stream (preferring DMA-BUF)
5. Imports DMA-BUF into `wl_buffer` via `zwp_linux_dmabuf_v1`
6. Attaches buffer to surface and commits

### 2. Input Forwarding via EIS

GNOME headless uses **EIS (Emulated Input Subsystem)** for input:

```
Wolf (wl_seat input)
    → gnome-wolf-bridge
        → libei
            → GNOME Shell (org.freedesktop.RemoteDesktop)
```

### 3. File Structure

```
gnome-wolf-bridge/
├── main.c              (~200 lines) - Entry point, event loop
├── wayland-client.c    (~300 lines) - Wolf connection, surface management
├── screencast-dbus.c   (~200 lines) - D-Bus screen-cast session
├── pipewire-stream.c   (~350 lines) - PipeWire consumer (based on MDK)
├── dmabuf-import.c     (~100 lines) - DMA-BUF → wl_buffer conversion
├── eis-input.c         (~150 lines) - Input forwarding
└── meson.build
```

### 4. Dependencies

```
# Build dependencies
wayland-client          # Wolf connection
wayland-egl             # EGL for dmabuf import (optional)
libpipewire-0.3         # PipeWire stream handling
libei                   # Input forwarding to GNOME
gio-2.0                 # D-Bus for screen-cast API
libdrm                  # DMA-BUF handling

# Runtime: GNOME packages
gnome-shell
gnome-remote-desktop    # Optional, but has required D-Bus interfaces
```

## Zero-Copy Path Analysis

### Best Case: Full Zero-Copy

```
1. GNOME Shell renders frame (GPU)
2. Mutter exports DMA-BUF handle (no copy)
3. PipeWire passes DMA-BUF fd (no copy)
4. gnome-wolf-bridge imports DMA-BUF (no copy)
5. Wolf composites DMA-BUF to output (no copy)
```

**Latency: ~1 frame** (the PipeWire queue adds one frame of latency)

### Fallback: SHM Copy

If DMA-BUF isn't available (e.g., software renderer):

```
1. GNOME Shell renders frame (CPU/software)
2. Mutter copies to SHM buffer
3. PipeWire passes SHM memory
4. gnome-wolf-bridge copies to wl_shm_pool
5. Wolf composites
```

**Latency: ~1 frame + copy overhead**

## Reference Code

### MDK (Mutter Development Kit)

The MDK already implements most of what we need:

- **mdk/mdk-stream.c** (35KB) - PipeWire stream handling, DMA-BUF import
- **mdk/mdk-pipewire.c** (8KB) - PipeWire connection setup
- **mdk/mdk-ei.c** (6KB) - EIS input forwarding
- **mdk/mdk-screencast.c** - Screen-cast D-Bus session

**Key difference:** MDK outputs to GTK4 window, we output to Wayland surface.

### wlroots Wayland Backend

**wlroots/backend/wayland/** shows how to be a Wayland client:

- **backend.c** (~745 lines) - Registry, compositor binding
- **output.c** (~800 lines) - xdg_toplevel surface management
- **seat.c** (~600 lines) - Input handling

## Comparison: XWayland vs PipeWire Bridge

| Aspect | XWayland | PipeWire Bridge |
|--------|----------|-----------------|
| **GPU Copy** | May require copy through X11 | Zero-copy DMA-BUF possible |
| **Latency** | X11 protocol overhead | ~1 frame (PipeWire queue) |
| **Complexity** | Simple (XWayland is battle-tested) | ~1,200 LOC bridge |
| **Maintenance** | None (standard components) | Small bridge to maintain |
| **GNOME Features** | Full (X11 session) | Full (headless mode) |
| **Future Proof** | X11 is deprecated | Pure Wayland, GNOME's direction |

## Implementation Effort

**Estimated: 2-3 days of focused development**

### Day 1: Core Structure
- Wayland client connection to Wolf
- xdg_toplevel surface creation
- Screen-cast D-Bus session setup

### Day 2: PipeWire Integration
- PipeWire stream consumer (adapted from MDK)
- DMA-BUF import to wl_buffer
- Frame timing and synchronization

### Day 3: Input + Polish
- EIS input forwarding
- Error handling and recovery
- Testing and debugging

## Startup Sequence

```bash
# 1. Start GNOME Shell in headless mode
gnome-shell --headless --wayland &

# 2. Wait for GNOME to be ready
gdbus wait --session --timeout 30 org.gnome.Mutter.ScreenCast

# 3. Start the bridge
WAYLAND_DISPLAY=wayland-1 gnome-wolf-bridge &
# Bridge connects to Wolf, creates surface, starts screen-cast

# 4. GNOME Shell now appears in Wolf!
```

## Risks and Mitigations

### Risk: DMA-BUF Format Mismatch
**Problem:** GNOME's DMA-BUF format might not match Wolf's supported formats.
**Mitigation:** Query Wolf's `zwp_linux_dmabuf_v1` for supported formats, negotiate in PipeWire.

### Risk: Multi-GPU Setup
**Problem:** GNOME renders on GPU A, Wolf on GPU B - DMA-BUF can't cross GPU boundaries.
**Mitigation:** Fall back to SHM copy path (still works, just slower).

### Risk: Headless Mode Stability
**Problem:** GNOME headless is less tested than full session.
**Mitigation:** gnome-remote-desktop uses headless extensively, it's production-ready.

## DMA-BUF Verification (2025-12-29)

All components in the pipeline have been verified to support DMA-BUF zero-copy:

### 1. Wolf (gst-wayland-display) ✅

Wolf's Wayland compositor is built on Smithay and fully implements `zwp_linux_dmabuf_v1`:

```rust
// wayland-display-core/src/wayland/handlers/dmabuf.rs
impl DmabufHandler for State {
    fn dmabuf_imported(&mut self, _global: &DmabufGlobal, dmabuf: Dmabuf, notifier: ImportNotifier) {
        if self.renderer.import_dmabuf(&dmabuf, None).is_ok() {
            let _ = notifier.successful::<State>();
        } else {
            notifier.failed();
        }
    }
}
```

**Source:** `/pm/gst-wayland-display/wayland-display-core/src/wayland/handlers/dmabuf.rs`

### 2. GNOME/Mutter ✅

Mutter has supported DMA-BUF screen-cast since GNOME 42:
- [MR #1939: Announce dmabuf support via pipewire](https://gitlab.gnome.org/GNOME/mutter/-/merge_requests/1939)
- GNOME 47 added hardware encoding support for screen recording

### 3. KDE/KWin ✅

KDE supports DMA-BUF via xdg-desktop-portal-kde:
- Uses PipeWire with DMA-BUF for zero-copy screen capture
- [Phabricator T12863: Use PipeWire for screen casting](https://phabricator.kde.org/T12863)

### 4. Sway/wlroots ✅

Sway supports DMA-BUF via xdg-desktop-portal-wlr:
- [GitHub: xdg-desktop-portal-wlr](https://github.com/emersion/xdg-desktop-portal-wlr)
- Debug logs show "linux_dmabuf_feedback_format_table" and "linux_dmabuf_feedback_tranche_formats"

## Implementation

### C Implementation (Prototype)

The C bridge is in `/helix/gnome-wolf-bridge/`:

- **main.c** - Entry point and event loop
- **wayland-client.c** - Wolf Wayland connection, DMA-BUF/SHM buffer handling
- **screencast.c** - GNOME-specific D-Bus screen-cast session
- **portal-screencast.c** - XDG Desktop Portal (universal: GNOME/KDE/Sway)
- **pipewire-stream.c** - PipeWire stream consumer
- **eis-input.c** - Optional EIS input forwarding
- **meson.build** - Build configuration

```bash
cd gnome-wolf-bridge
meson setup build
meson compile -C build
```

### Rust Implementation (Production)

The Rust bridge is in `/helix/wolf-bridge-rs/`:

- **src/main.rs** - Entry point and async event loop
- **src/wayland.rs** - Wolf Wayland connection using wayland-client crate
- **src/portal.rs** - XDG Desktop Portal using ashpd crate
- **src/pipewire_stream.rs** - PipeWire stream using pipewire-rs crate

Key advantages of Rust implementation:
- Memory safety (no manual free/malloc)
- Uses `ashpd` crate for XDG Portal (works on GNOME, KDE, Sway out of the box)
- Integrates better with Wolf's Rust codebase (gst-wayland-display is Rust)
- Could potentially be merged into Wolf as an optional feature

```bash
cd wolf-bridge-rs
cargo build --release
```

### Startup Script

See `wolf/ubuntu-config/start-gnome-headless.sh` for a complete startup sequence.

## Input Handling

Input forwarding uses **EIS (Emulated Input Subsystem)** for headless GNOME:

```
Wolf (wl_seat input)
    → wolf-bridge (receives Wayland input events)
        → libei (EIS client library)
            → GNOME Shell (org.freedesktop.RemoteDesktop / EIS portal)
```

### How EIS Works

1. **GNOME Shell** exposes an EIS socket when running with remote desktop support
2. **wolf-bridge** connects as an EIS client via libei
3. **Keyboard/mouse events** from Wolf's `wl_seat` are translated to EIS events
4. **GNOME Shell** injects these events as if they came from real input devices

### Implementation

The C implementation has `eis-input.c` for this. In Rust, use the `reis` crate.

Key D-Bus interface: `org.freedesktop.RemoteDesktop.Session.ConnectToEIS()`

## Audio Handling

Audio is **separate from video** and handled automatically by Wolf:

```
GNOME Shell (application audio)
    → PipeWire / pipewire-pulse
        → Wolf's audio capture
            → GStreamer encoding → Moonlight client
```

### Why No Bridge Needed

1. **GNOME outputs audio to PipeWire** - The headless session uses pipewire-pulse
2. **Wolf captures audio directly** - Wolf's gstreamer pipeline already captures from PulseAudio/PipeWire
3. **Same container** - Both GNOME and Wolf run in the same container, sharing audio

### Audio Dependencies

```bash
# Install in container
pipewire
pipewire-pulse
wireplumber
```

Wolf auto-detects PipeWire and captures audio without additional configuration.

## Q&A: Design Decisions

### Q: Doesn't this add NVIDIA zero-copy complexity like Wolf had?

Wolf's gst-wayland-display went to extraordinary lengths for NVIDIA DMA-BUF support. However, PipeWire screen-cast is battle-tested by:
- **OBS Studio** - Professional streaming software
- **Discord/Slack** - Screen sharing in production
- **GNOME Remote Desktop** - RDP/VNC for GNOME
- **Firefox WebRTC** - Browser screen sharing

The DMA-BUF path through PipeWire is **more mature** than nested Wayland compositor support.

### Q: Aren't we adding another layer of complexity?

No - we're **replacing** poorly-supported nested Wayland with well-supported PipeWire:

| Approach | Support Status |
|----------|---------------|
| Nested Wayland (`--nested`) | Removed in GNOME 49 |
| PipeWire screen-cast | Production use in OBS, Discord, browsers |

The video path complexity is similar, just using different (better supported) plumbing.

### Q: Why not use GStreamer's waylandsink directly instead of custom Rust bridge?

**Answer:** We ARE using GStreamer! The bridge is just a shell script:
```bash
gst-launch-1.0 pipewiresrc path=$NODE_ID ! waylandsink
```

No custom code needed - just orchestration of existing tools.

## Future Exploration: Eliminate Wolf + Moonlight Entirely?

### The Idea

PipeWire + WebRTC could potentially replace Wolf + Moonlight:

```
Current architecture:
  GNOME → PipeWire → Wolf → Moonlight protocol → moonlight-web → Browser

Proposed simplified architecture:
  GNOME → PipeWire → GStreamer WebRTC → Browser (direct)
```

### What We'd Need

| Component | Current | Proposed |
|-----------|---------|----------|
| **Video** | Wolf compositor + Moonlight encoding | `pipewiresrc` → GStreamer WebRTC → browser |
| **Audio** | Wolf audio capture + Moonlight | PipeWire → GStreamer → WebRTC |
| **Input** | Moonlight → Wolf → Wayland | Browser → WebSocket → libei → GNOME |

### Benefits

- **Eliminate Wolf** - No custom Wayland compositor
- **Eliminate Moonlight protocol** - Standard WebRTC
- **Simpler deployment** - Fewer moving parts
- **Browser-native** - No moonlight-web-stream binary

### Challenges

1. **Hardware encoding** - Wolf uses VAAPI/NVENC. GStreamer WebRTC needs same.
2. **Latency** - Moonlight is optimized for low-latency gaming. WebRTC may have higher latency.
3. **Input latency** - WebSocket → libei adds round-trips vs Moonlight's direct input.
4. **Existing investment** - Wolf + moonlight-web-stream already work.

### Existing Project: Selkies-GStreamer

[Selkies-GStreamer](https://github.com/selkies-project/selkies) already does this!

**What it is:**
- Open-source GStreamer → WebRTC streaming platform
- Started by Google engineers, now maintained by UCSD
- Designed for containers/Kubernetes (no special devices needed)
- GPU-accelerated encoding (NVENC for NVIDIA, VAAPI for AMD/Intel)

**Technical stack:**
- `gst-python` for GStreamer bindings
- `webrtcbin` for WebRTC output
- Opus codec for audio
- Python signaling server + HTML5 web interface

**Relevance to Helix:**
- Could potentially **replace Wolf + moonlight-web-stream entirely**
- Already handles GPU encoding, WebRTC, input
- Would need integration with GNOME headless + EIS input

### Reuse Wolf's Existing GStreamer Pipelines

Wolf already has battle-tested GStreamer encoding pipelines for VAAPI/NVENC.
We could literally swap the output sink:

```
# Wolf's existing pipeline (simplified):
source → vaapih264enc → moonlight-encoder → network

# Modified for WebRTC (same encoding!):
pipewiresrc → vaapih264enc → webrtcbin → browser
```

**What this means:**
- **Zero new encoding work** - Reuse Wolf's GPU encoder code
- **Just change output** - From Moonlight protocol to WebRTC
- **Input via signaling** - WebRTC data channels → libei
- **Audio unchanged** - PipeWire → Opus → WebRTC

### Research Needed

- [ ] Evaluate Selkies-GStreamer as Wolf replacement
- [ ] Compare latency: Selkies vs Moonlight protocol
- [ ] Integration with GNOME headless mode
- [ ] EIS input handling in Selkies (or add it)

### Verdict

Worth exploring as a **v2 architecture** but the Wolf + PipeWire bridge approach should work for v1.

## PipeWire Direct to Wolf Encoder (GNOME Only)

> **STATUS: Implementation In Progress**
>
> This approach is being implemented for GNOME desktops only.
> Sway and KDE continue using nested Wayland mode with gst-wayland-display.

**Key Insight:** For GNOME (where nested Wayland was removed), we feed PipeWire directly into Wolf's encoder pipeline, **bypassing the fullscreen window problem with mutter-devkit**.

**Per-Lobby Toggle:** Wolf will support a per-lobby setting to choose between:
- `pipewiresrc` - For GNOME desktops (PipeWire node ID from container)
- `wlroot-src` - For Sway/KDE (nested Wayland mode, current default)

### Current vs Proposed Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│  Current Architecture (with gst-wayland-display)                            │
│  ==============================================                             │
│                                                                             │
│  GNOME (--devkit) → PipeWire → pipewiresrc → waylandsink                   │
│                                                    ↓                        │
│                                  gst-wayland-display (Smithay compositor)   │
│                                                    ↓                        │
│                                  Wolf video capture → encoder → Moonlight   │
│                                                                             │
│  Problems:                                                                  │
│  - Extra compositor layer (Smithay/gst-wayland-display)                     │
│  - Video path: GNOME → PipeWire → Wayland → capture → encode                │
│  - ~10K lines of Rust compositor code to maintain                           │
└─────────────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────────────┐
│  Proposed Architecture (PipeWire → Wolf encoder directly)                   │
│  ========================================================                   │
│                                                                             │
│  GNOME (--devkit) → PipeWire → pipewiresrc → Wolf encoder → Moonlight      │
│                                                    ↑                        │
│                                    Wolf manages GPU detection               │
│                                    Wolf selects VAAPI/NVENC/software        │
│                                                                             │
│  Benefits:                                                                  │
│  - No compositor needed for headless desktops                               │
│  - Direct path: GNOME → PipeWire → encode                                   │
│  - Wolf still handles all the complex stuff:                                │
│    - Container orchestration                                                │
│    - Lobby/pairing management                                               │
│    - GPU detection and encoder selection                                    │
│    - Moonlight protocol                                                     │
└─────────────────────────────────────────────────────────────────────────────┘
```

### What This Eliminates

| Component | Lines of Code | Purpose |
|-----------|---------------|---------|
| gst-wayland-display | ~10K Rust | Smithay Wayland compositor |
| wayland-display-core | ~5K Rust | DMA-BUF handling, surface management |
| wlroot-src | ~2K | Video capture from compositor |

**Total eliminated: ~17K lines of complex Rust code**

### What Wolf Still Does

Wolf continues to handle all the orchestration complexity:

1. **Container Management**
   - Start GNOME containers with GPU passthrough
   - Mount workspace directories
   - Configure environment (resolution, zoom, etc.)

2. **GPU Detection & Encoder Selection**
   - Detect NVIDIA (NVENC), AMD (VAAPI), Intel (QSV)
   - Select optimal encoder based on GPU type
   - Handle fallback to software encoding

3. **Lobby & Pairing**
   - Generate pairing PINs
   - Manage Moonlight client connections
   - Handle multiple sessions

4. **Moonlight Protocol**
   - Encode video frames
   - Handle input events
   - Stream to browser/client

### GStreamer Pipeline Change

Wolf's video pipeline becomes simpler:

```
# Before (with compositor):
wlroot-src → videoconvert → vaapih264enc → moonlight-sink

# After (PipeWire direct):
pipewiresrc path=N → videoconvert → vaapih264enc → moonlight-sink
                ↑
        Node ID from GNOME ScreenCast D-Bus
```

**Key:** The encoder chain (`vaapih264enc`, `nvh264enc`, etc.) stays exactly the same.

### Input Handling Without Compositor

With gst-wayland-display:
```
Moonlight input → Wolf → wl_seat → client apps
```

Without compositor (headless desktops):
```
Moonlight input → Wolf → input subsystem → desktop → apps
```

**Input subsystem varies by desktop:**

| Desktop | Input Method | Library |
|---------|-------------|---------|
| GNOME | EIS (Emulated Input Subsystem) | libei |
| KDE | EIS (KWin 6.0+) | libei |
| Sway | uinput or wtype | linux kernel uinput |

**EIS is a freedesktop.org standard** (not GNOME-specific). KDE added full support in 2024.

For Sway/wlroots, we can use `/dev/uinput` directly or the `wtype` tool, which doesn't require EIS.

### Implementation Steps (GNOME Only)

#### Phase 1: Wolf Config Changes

1. **Add `video_source_mode` option to App config**
   ```toml
   [[apps]]
   title = "GNOME Desktop"
   start_virtual_compositor = false
   video_source_mode = "pipewire"  # NEW: "wayland" (default) or "pipewire"
   ```

2. **Add pipewiresrc producer pipeline**
   - When `video_source_mode = "pipewire"`:
     - Don't start waylanddisplaysrc
     - Start pipewiresrc with node ID from container
   - Pipeline: `pipewiresrc path={node_id} ! interpipesink name={session_id}_video`

3. **PipeWire socket sharing**
   - Mount container's PipeWire socket into Wolf's namespace
   - Or use PipeWire remote protocol over TCP

#### Phase 2: Container Protocol

4. **Container reports PipeWire node ID**
   - Container runs GNOME with ScreenCast session (using gnome-screencast-screenshot.sh approach)
   - Container writes node ID to shared file or calls Wolf API
   - Wolf reads node ID and starts pipewiresrc

5. **Lifecycle management**
   - Wolf waits for node ID before starting encoder pipeline
   - Container cleanup closes ScreenCast session

#### Phase 3: Input Forwarding (Future)

6. **Add libei input forwarding**
   - Wolf receives Moonlight input events
   - Forward to container via EIS socket

**Note:** Sway/KDE continue using `start_virtual_compositor = true` (waylanddisplaysrc path).

### GPU Considerations

**NVIDIA Zero-Copy:**
- Wolf already handles NVIDIA DMA-BUF complexity
- PipeWire to NVENC path is battle-tested (OBS uses this)
- No additional work needed

**AMD/Intel:**
- VAAPI encoder works with PipeWire DMA-BUF
- Same path as NVIDIA, just different encoder element

**Software Fallback:**
- If no GPU encoder, use `x264enc` or `vp9enc`
- PipeWire provides SHM buffers automatically

### Advantages Over Current Approach

| Aspect | Current (waylandsink bridge) | Proposed (pipewiresrc direct) |
|--------|------------------------------|-------------------------------|
| Video path | GNOME → PipeWire → Wayland → capture | GNOME → PipeWire → encode |
| Compositor | Required (gst-wayland-display) | Not needed |
| Code to maintain | ~17K lines | ~500 lines (EIS integration) |
| DMA-BUF handling | In compositor | In PipeWire/encoder |
| Latency | +1 frame (compositor) | None |

## Conclusion

The PipeWire bridge approach is **verified to support zero-copy DMA-BUF** across all components:
- Wolf → DmabufHandler in gst-wayland-display
- GNOME → Mutter screen-cast with DMA-BUF (since GNOME 42)
- KDE → xdg-desktop-portal-kde with PipeWire DMA-BUF
- Sway → xdg-desktop-portal-wlr with DMA-BUF

**Fallback:** When DMA-BUF isn't available (e.g., software rendering, NVIDIA issues), SHM path with CPU copy is used automatically.

## GNOME 49 Screenshot D-Bus API Change

**BREAKING CHANGE:** GNOME 49 changed the argument order for `org.gnome.Shell.Screenshot.Screenshot`:

| Version | Signature | Argument Order |
|---------|-----------|----------------|
| GNOME 48 and earlier | `(sbb)` → `(bs)` | `filename, include_cursor, flash` |
| GNOME 49+ | `(bbs)` → `(bs)` | `include_cursor, flash, filename` |

The fix in `api/cmd/screenshot-server/main.go` reorders the arguments:

```go
// OLD (GNOME 48 and earlier):
obj.Call("org.gnome.Shell.Screenshot.Screenshot", 0,
    filename,  // s - first
    true,      // b - second
    false,     // b - third
)

// NEW (GNOME 49+):
obj.Call("org.gnome.Shell.Screenshot.Screenshot", 0,
    true,      // b - first (include_cursor)
    false,     // b - second (flash)
    filename,  // s - third (now last)
)
```

**Discovery:** The D-Bus interface file at `/usr/share/dbus-1/interfaces/org.gnome.Shell.Screenshot.xml` in the container shows the new signature.

## GNOME 49 Screenshot Security & ScreenCast Fallback

**GNOME 49 has stricter security** - even registering as `org.gnome.Screenshot` on D-Bus doesn't bypass the allowlist check. The Screenshot D-Bus API returns "Screenshot is not allowed" for programmatic access in headless containers.

**Solution:** Use the ScreenCast API (same as mutter-devkit uses) for screenshots:

```
screenshot-server → org.gnome.Shell.Screenshot (blocked)
                  ↓ fallback
screenshot-server → /usr/local/bin/gnome-screencast-screenshot.sh
                  → org.gnome.Mutter.ScreenCast (NOT blocked)
                  → PipeWire stream → GStreamer pipewiresrc → PNG
```

The helper script `gnome-screencast-screenshot.sh`:
1. Creates a ScreenCast session via D-Bus
2. Records the current monitor (or virtual display in headless)
3. Captures one frame via GStreamer pipewiresrc
4. Encodes as PNG and returns

## mutter-devkit Fullscreen Behavior

**Yes, mutter-devkit runs fullscreen by default.** The SDK viewer creates a fullscreen surface on its inherited `WAYLAND_DISPLAY` (Wolf's compositor).

The fullscreen behavior is inherent to how mutter-devkit works:
- It's designed to mirror the entire GNOME session
- No windowed mode is needed since it's a 1:1 display bridge
- The surface matches the GNOME session resolution

## libei Input Forwarding Compatibility

| Desktop | libei Support | Alternative |
|---------|---------------|-------------|
| **GNOME** (Mutter) | ✅ Full support | - |
| **KDE** (KWin 6.0+) | ✅ Full support (added 2024) | - |
| **Sway/wlroots** | ❌ Not supported | `/dev/uinput` or `wtype` |
| **Hyprland** | ❌ Not supported | `/dev/uinput` |

**libei** (Emulated Input Subsystem) is a freedesktop.org standard, but wlroots-based compositors haven't adopted it yet.

**For Sway/wlroots input forwarding:**
- Use `/dev/uinput` kernel interface directly
- Or spawn `wtype` commands for keyboard input
- Or use `ydotool` which works without X11

## V2 Architecture: PipeWire Direct to Wolf Encoder

**Implementation Effort:** Moderate (~2-3 days focused work)

The V2 architecture eliminates gst-wayland-display by feeding PipeWire directly into Wolf's encoder:

```
Current:  GNOME → PipeWire → mutter-devkit → Wolf Compositor → Encoder
V2:       GNOME → PipeWire → pipewiresrc → Wolf Encoder (directly)
```

**Changes required:**
1. Replace `wlroot-src` with `pipewiresrc` in Wolf's GStreamer pipeline
2. Pass PipeWire node ID from container to Wolf
3. Add libei/uinput input forwarding (Wolf → container)
4. Remove gst-wayland-display dependency for headless desktops

**Benefits:**
- Eliminates ~17K lines of Smithay compositor code
- Removes one frame of latency (no compositor pass)
- Simpler DMA-BUF path (PipeWire handles it)

**Risks:**
- Need to manage PipeWire node IDs across container boundary
- Input forwarding adds complexity
- May need per-desktop input handling (libei vs uinput)

## References

- [GNOME Remote Desktop](https://gitlab.gnome.org/GNOME/gnome-remote-desktop) - Uses same PipeWire approach
- [Mutter Development Kit](https://gitlab.gnome.org/GNOME/mutter/-/tree/main/mdk) - Reference implementation
- [PipeWire DMA-BUF](https://docs.pipewire.org/page_dma_buf.html) - DMA-BUF support docs
- [libei](https://gitlab.freedesktop.org/libinput/libei) - Input subsystem for headless
- [zwp_linux_dmabuf_v1](https://wayland.app/protocols/linux-dmabuf-v1) - Wayland DMA-BUF protocol
- [XDG RemoteDesktop Portal](https://flatpak.github.io/xdg-desktop-portal/docs/doc-org.freedesktop.portal.RemoteDesktop.html) - Combined video+input portal
- [liboeffis](https://libinput.pages.freedesktop.org/libei/api/group__liboeffis.html) - XDG RemoteDesktop portal wrapper for libei

---

## Roadmap: Use RemoteDesktop Portal for Combined Video + Input (2025-12-30)

### Current Approach vs Recommended Approach

**Current approach (ScreenCast + inputtino):**
```
Video:  ScreenCast portal → PipeWire → pipewirezerocopysrc → Wolf
Input:  Wolf → inputtino (kernel evdev) → fake-udev → container
```

**Problem:** Input via inputtino/fake-udev has been flaky. Wolf's input stack has numerous issues.

**Recommended approach (RemoteDesktop portal):**
```
Video:  RemoteDesktop portal → ScreenCast.SelectSources → PipeWire → pipewirezerocopysrc → Wolf
Input:  Wolf → ConnectToEIS → libei → GNOME Shell (via RemoteDesktop portal)
```

### Why RemoteDesktop Portal is Better

1. **Unified session** - One D-Bus session handles both video and input
2. **Battle-tested** - Used by gnome-remote-desktop, Chrome Remote Desktop, RustDesk
3. **Coordinate mapping** - `mapping_id` property correlates PipeWire streams with libei regions
4. **No fake devices** - Input goes directly to compositor via EIS, not kernel evdev

### Implementation Steps

1. **Replace ScreenCast with RemoteDesktop session:**
   ```python
   # Instead of:
   session = ScreenCast.CreateSession()

   # Do:
   session = RemoteDesktop.CreateSession()
   ScreenCast.SelectSources(session)  # Video on same session
   RemoteDesktop.Start(session)
   ```

2. **Get video stream (same as before):**
   ```python
   fd = ScreenCast.OpenPipeWireRemote(session)
   # Pass node_id to Wolf's pipewirezerocopysrc
   ```

3. **Get input via EIS instead of inputtino:**
   ```python
   eis_fd = RemoteDesktop.ConnectToEIS(session)
   # Connect libei sender to this fd
   # Forward Wolf's input events via libei
   ```

4. **Coordinate mapping:**
   - Get `mapping_id` from PipeWire stream properties
   - Match with libei device regions for correct absolute positioning

### Container-Side Changes

The container startup script needs to:
1. Create RemoteDesktop session (not just ScreenCast)
2. Report both PipeWire node_id AND EIS socket to Wolf
3. Possibly run a small daemon that forwards EIS events

### Wolf-Side Changes

1. Accept EIS socket path in addition to PipeWire node_id
2. Replace inputtino with libei sender
3. Forward keyboard/mouse events via libei instead of fake evdev

### References

- [XDG RemoteDesktop Portal docs](https://flatpak.github.io/xdg-desktop-portal/docs/doc-org.freedesktop.portal.RemoteDesktop.html)
- [ConnectToEIS method](https://flatpak.github.io/xdg-desktop-portal/docs/doc-org.freedesktop.portal.RemoteDesktop.html) - Returns fd for libei
- [liboeffis](https://libinput.pages.freedesktop.org/libei/api/group__liboeffis.html) - Helper library for RemoteDesktop + libei

---

## CRITICAL: Test on Virtualized AMD GPUs

**This is a critical roadmap item.**

All PipeWire/DMA-BUF work MUST be tested on virtualized AMD GPUs before considering complete. Reasons:

1. **Different DMA-BUF behavior** - AMD's amdgpu driver handles DMA-BUF differently than NVIDIA
2. **SR-IOV virtualization** - AMD GPUs with SR-IOV may have different memory sharing semantics
3. **Mesa vs proprietary** - AMD uses Mesa, NVIDIA uses proprietary drivers
4. **GBM allocation** - AMD uses GBM for buffer allocation, NVIDIA uses EGLStreams (legacy) or GBM

### Test Matrix

| GPU Type | Virtualization | Priority |
|----------|----------------|----------|
| AMD Radeon (bare metal) | None | High |
| AMD Radeon (SR-IOV) | KVM/QEMU | **Critical** |
| AMD Radeon (MxGPU) | VMware/Citrix | Medium |
| Intel Arc (bare metal) | None | Medium |
| Intel (SR-IOV) | KVM/QEMU | Medium |

### What to Test

1. **DMA-BUF export** - Can PipeWire export DMA-BUF from AMD GPU?
2. **Zero-copy path** - Does pipewirezerocopysrc achieve zero-copy on AMD?
3. **VA-API encoding** - Does vapostproc + vaapih264enc work correctly?
4. **Multi-GPU** - What happens if AMD render GPU differs from Wolf's GPU?
5. **Memory pressure** - Does DMA-BUF sharing work under memory pressure?

### Known Differences from NVIDIA

- AMD doesn't need CUDA context sharing (uses VA-API instead)
- AMD uses `video/x-raw(memory:DMABuf)` caps, not `video/x-raw(memory:CUDAMemory)`
- AMD may need `vapostproc` element instead of `cudaupload`
- AMD SR-IOV guests may have limited DMA-BUF modifier support
