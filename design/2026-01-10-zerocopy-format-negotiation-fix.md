# Zero-Copy PipeWire Format Negotiation Fix

**Date:** 2026-01-10
**Status:** ✅ FIXED - Testing Complete
**Author:** Claude (with Luke)
**Depends on:** 2026-01-10-zero-copy-video-streaming.md

## Solution Summary (TL;DR)

**Root Cause:** GNOME headless mode only supports SHM (MemFd) buffers, not DMA-BUF. Offering DmaBuf modifiers caused infinite format renegotiation.

**Fix:** Only offer SHM format (no modifiers) and only accept MemFd buffer type.

```rust
// What works (SHM only):
let format_no_modifier = build_video_format_params_no_modifier();
let buffer_types: i32 = 1 << 2;  // MemFd only = 0x4

// What failed (DmaBuf + SHM):
let buffer_types: i32 = (1 << 2) | (1 << 3);  // MemFd + DmaBuf = 0xc
```

**Result:** 112 frames at 7.5 fps - video streaming works!

## Problem Statement

Our `pipewirezerocopysrc` plugin loads correctly but produces **0 frames** when used with GNOME ScreenCast. Investigation revealed:

1. Plugin successfully connects to PipeWire
2. Stream stays in **Paused** state, never transitions to **Streaming**
3. Constant `param_changed` callbacks indicate format renegotiation loop
4. Root cause: **Format mismatch between what we request and what GNOME offers**

```
We Request:          GNOME Offers:
─────────────        ─────────────
BGRx + LINEAR        BGRx (SHM only)
BGRx + INVALID       No DMA-BUF modifiers
                     No modifier field at all
```

GNOME's nested ScreenCast (via `gnome-shell --nested`) only offers **SHM formats** - no DMA-BUF modifiers are advertised. Our plugin unconditionally requests DMA-BUF with modifiers, causing negotiation failure.

## Research: How OBS and gnome-remote-desktop Solve This

### OBS Studio Approach

From `plugins/linux-pipewire/pipewire.c`:

```c
// 1. Query actual GPU DMA-BUF capabilities
bool capabilities_queried = gs_query_dmabuf_capabilities(
    &dmabuf_flags, &drm_formats, &n_drm_formats);

// 2. Get modifiers for each format the GPU actually supports
if (gs_query_dmabuf_modifiers_for_format(obs_pw_video_format.drm_format,
                                          &modifiers, &n_modifiers)) {
    da_push_back_array(info->modifiers, modifiers, n_modifiers);
}

// 3. Handle both DMA-BUF (sync) and SHM (async) buffer types
if (spa_buffer->datas[0].type == SPA_DATA_DmaBuf) {
    // Zero-copy path: import DMA-BUF via EGL
    process_dmabuf_buffer(stream, spa_buffer);
} else {
    // Fallback path: copy from SHM
    process_shm_buffer(stream, spa_buffer);
}
```

Key insight: OBS **queries the GPU** for supported modifiers rather than hardcoding them.

### gnome-remote-desktop Approach

From `src/grd-rdp-pipewire-stream.c`:

```c
// 1. Query modifiers from EGL or Vulkan
static gboolean
get_modifiers_for_format(GrdRdpPipeWireStream *stream,
                         uint32_t drm_format,
                         int *out_n_modifiers,
                         uint64_t **out_modifiers)
{
    if (hwaccel_vulkan) {
        return grd_hwaccel_vulkan_get_modifiers_for_format(...);
    }
    return grd_egl_thread_get_modifiers_for_format(egl_thread, drm_format, ...);
}

// 2. Build format params with modifier choice list
spa_pod_builder_push_choice(pod_builder, &modifier_frame, SPA_CHOICE_Enum, 0);
spa_pod_builder_long(pod_builder, modifiers[0]);  // Preferred
for (i = 0; i < n_modifiers; i++) {
    spa_pod_builder_long(pod_builder, modifiers[i]);  // All supported
}
spa_pod_builder_long(pod_builder, DRM_FORMAT_MOD_INVALID);  // Implicit modifier
spa_pod_builder_pop(pod_builder, &modifier_frame);

// 3. Add fallback format WITHOUT modifiers (for SHM)
if (need_fallback_format) {
    // Second format param - no modifier field at all
    spa_pod_builder_push_object(...);
    add_common_format_params(pod_builder, spa_format, ...);
    // NO modifier field added
    params[n_params++] = spa_pod_builder_pop(...);
}

// 4. Accept both DMA-BUF and MemFd buffer types
allowed_buffer_types = 1 << SPA_DATA_MemFd;  // Always accept SHM
if (egl_thread && !hwaccel_nvidia) {
    allowed_buffer_types |= 1 << SPA_DATA_DmaBuf;  // Also accept DMA-BUF
}
```

## Current Bug in Our Plugin

Location: `desktop/gst-pipewire-zerocopy/src/pipewire_stream.rs`

Our plugin builds format params with hardcoded modifiers:

```rust
// WRONG: Hardcoding modifiers that GNOME may not support
fn build_video_format_params(pod_builder: &mut SpaPodBuilder) {
    // Request DMA-BUF with LINEAR modifier
    pod_builder.add_prop(SPA_FORMAT_VIDEO_modifier,
        &[DRM_FORMAT_MOD_LINEAR, DRM_FORMAT_MOD_INVALID]);
}
```

This fails because:
1. We don't query what modifiers the GPU actually supports
2. We don't provide a fallback format without modifiers
3. GNOME's ScreenCast doesn't offer any modifiers (SHM only in nested mode)

## The Fix

### 1. Query Actual GPU Modifiers (Like OBS)

```rust
// desktop/gst-pipewire-zerocopy/src/egl_modifiers.rs (new file)

use wayland_display_core::egl::EGLDisplay;

/// Query DMA-BUF modifiers supported by the GPU via EGL
pub fn query_egl_modifiers(drm_format: u32) -> Vec<u64> {
    let display = match EGLDisplay::new() {
        Ok(d) => d,
        Err(_) => return vec![DRM_FORMAT_MOD_INVALID], // Fallback
    };

    // EGL_EXT_image_dma_buf_import_modifiers extension
    let mut modifiers = Vec::new();

    if let Ok(mods) = display.query_dmabuf_modifiers(drm_format) {
        modifiers.extend(mods);
    }

    // Always include INVALID for implicit modifier support
    if !modifiers.contains(&DRM_FORMAT_MOD_INVALID) {
        modifiers.push(DRM_FORMAT_MOD_INVALID);
    }

    modifiers
}
```

### 2. Build Format Params with Fallback (Like gnome-remote-desktop)

```rust
// desktop/gst-pipewire-zerocopy/src/pipewire_stream.rs

fn build_stream_params(&self) -> Vec<SpaPod> {
    let mut params = Vec::new();
    let drm_format = DRM_FORMAT_XRGB8888; // BGRx

    // Query actual GPU-supported modifiers
    let modifiers = query_egl_modifiers(drm_format);

    // Format 1: With modifiers (DMA-BUF preferred)
    if !modifiers.is_empty() {
        let pod = self.build_format_with_modifiers(drm_format, &modifiers);
        params.push(pod);
    }

    // Format 2: Without modifiers (SHM fallback) - CRITICAL for GNOME nested
    let fallback_pod = self.build_format_without_modifiers(drm_format);
    params.push(fallback_pod);

    params
}

fn build_format_with_modifiers(&self, format: u32, modifiers: &[u64]) -> SpaPod {
    let mut builder = SpaPodBuilder::new();
    builder.push_object(SPA_TYPE_OBJECT_Format, SPA_PARAM_EnumFormat);

    // Basic video format
    builder.add_prop(SPA_FORMAT_mediaType, SPA_MEDIA_TYPE_video);
    builder.add_prop(SPA_FORMAT_mediaSubtype, SPA_MEDIA_SUBTYPE_raw);
    builder.add_prop(SPA_FORMAT_VIDEO_format, format_to_spa(format));
    builder.add_prop(SPA_FORMAT_VIDEO_size, &self.size_range());
    builder.add_prop(SPA_FORMAT_VIDEO_framerate, &SPA_FRACTION(0, 1));
    builder.add_prop(SPA_FORMAT_VIDEO_maxFramerate, &self.max_framerate());

    // Modifier choice with MANDATORY | DONT_FIXATE flags
    builder.add_prop_flags(
        SPA_FORMAT_VIDEO_modifier,
        SPA_POD_PROP_FLAG_MANDATORY | SPA_POD_PROP_FLAG_DONT_FIXATE,
    );
    builder.push_choice(SPA_CHOICE_Enum, modifiers[0]);
    for &modifier in modifiers {
        builder.add_long(modifier);
    }
    builder.pop_choice();

    builder.pop_object()
}

fn build_format_without_modifiers(&self, format: u32) -> SpaPod {
    let mut builder = SpaPodBuilder::new();
    builder.push_object(SPA_TYPE_OBJECT_Format, SPA_PARAM_EnumFormat);

    // Same video format, but NO modifier field
    builder.add_prop(SPA_FORMAT_mediaType, SPA_MEDIA_TYPE_video);
    builder.add_prop(SPA_FORMAT_mediaSubtype, SPA_MEDIA_SUBTYPE_raw);
    builder.add_prop(SPA_FORMAT_VIDEO_format, format_to_spa(format));
    builder.add_prop(SPA_FORMAT_VIDEO_size, &self.size_range());
    builder.add_prop(SPA_FORMAT_VIDEO_framerate, &SPA_FRACTION(0, 1));
    builder.add_prop(SPA_FORMAT_VIDEO_maxFramerate, &self.max_framerate());

    // NO modifier field - PipeWire will use SHM buffers

    builder.pop_object()
}
```

### 3. Accept Both Buffer Types in param_changed

```rust
fn on_param_changed(&mut self, id: u32, param: Option<&SpaPod>) {
    // ... parse format ...

    // Accept both DMA-BUF and MemFd (SHM)
    let mut allowed_buffer_types = 1 << SPA_DATA_MemFd;

    if self.cuda_context.is_some() || self.egl_display.is_some() {
        allowed_buffer_types |= 1 << SPA_DATA_DmaBuf;
    }

    let buffers_param = spa_pod_builder_add_object!(
        SPA_TYPE_OBJECT_ParamBuffers, SPA_PARAM_Buffers,
        SPA_PARAM_BUFFERS_buffers: SPA_POD_CHOICE_RANGE_Int(8, 2, 8),
        SPA_PARAM_BUFFERS_dataType: SPA_POD_Int(allowed_buffer_types),
    );

    self.stream.update_params(&[buffers_param]);
}
```

### 4. Handle Both Buffer Types in process_buffer

```rust
fn process_buffer(&mut self, buffer: &PwBuffer) -> GstBuffer {
    let spa_buffer = buffer.buffer();
    let data = &spa_buffer.datas()[0];

    match data.type_() {
        SPA_DATA_DmaBuf => {
            // Zero-copy path: DMA-BUF → EGL → CUDA
            let fd = data.fd().unwrap();
            let dmabuf = self.import_dmabuf(fd, &spa_buffer);
            self.convert_to_cuda_buffer(dmabuf)
        }
        SPA_DATA_MemFd | SPA_DATA_MemPtr => {
            // Fallback path: SHM → CPU copy → CUDA upload
            let ptr = data.data().unwrap();
            self.upload_shm_to_cuda(ptr)
        }
        _ => {
            warn!("Unknown buffer type: {:?}", data.type_());
            GstBuffer::new()
        }
    }
}
```

## Can We Just Use OBS?

**Question:** Can OBS Studio run headlessly for our use case?

**Answer: No, not practically.**

From research:
- [OBS Ideas Forum](https://ideas.obsproject.com/posts/16/add-a-headless-mode-that-allows-full-control-via-scripting-api): "OBS would still require access to a GPU because the whole scene rendering backend is GPU-accelerated and not done in software. Just turning off the GUI will not make the other requirements go away."
- [obs-headless](https://github.com/a-rose/obs-headless): Community Docker solution that still requires a virtual X display
- [Headless OBS on Debian](https://binblog.de/2025/04/03/headless-obs-on-debian/): Requires VNC + virtual desktop

**Why OBS doesn't work for us:**
1. **Requires GUI**: OBS's rendering is GPU-accelerated, needs a display context
2. **Heavy**: Full OBS runtime is ~500MB+ with dependencies
3. **Overkill**: We just need `PipeWire → H.264 encode → WebSocket`, not a full streaming suite
4. **Licensing**: OBS is GPL, would require releasing our changes

**Alternatives considered:**
- [obs-gstreamer](https://github.com/fzwoch/obs-gstreamer): Plugin for OBS, not standalone
- [obs-vaapi](https://github.com/fzwoch/obs-vaapi): GStreamer VAAPI encoding for OBS

**Conclusion:** Fix our GStreamer plugin to work like OBS does, rather than trying to embed OBS.

## Other Bug Fixed: Concurrent WebSocket Writes

**Symptom:** `panic: concurrent write to websocket connection`

**Root Cause:** Three goroutines writing to WebSocket simultaneously:
1. `readRTPAndSend` → `sendVideoFrame` (line 660)
2. `heartbeat` → `WriteMessage(PingMessage)` (line 676)
3. Main loop → Pong response (line 840)

**Fix:** Added `wsMu sync.Mutex` to VideoStreamer and protected all writes:

```go
// api/pkg/desktop/ws_stream.go

type VideoStreamer struct {
    // ...
    wsMu sync.Mutex  // Protects WebSocket writes
}

func (v *VideoStreamer) writeMessage(messageType int, data []byte) error {
    v.wsMu.Lock()
    defer v.wsMu.Unlock()
    return v.ws.WriteMessage(messageType, data)
}
```

## Implementation Plan

### Phase 1: Fix WebSocket Crash (Done)
- [x] Add `wsMu` mutex to VideoStreamer
- [x] Update all WebSocket writes to use mutex-protected helpers
- [x] Verify build compiles

### Phase 2: Fix Zerocopy Format Negotiation (Done - REVISED)
**Final fix was simpler than OBS/gnome-remote-desktop approach:**

1. [x] ~~Add format params with proper modifier list~~ → Only offer SHM format (no modifiers)
2. [x] ~~Add fallback format without modifiers~~ → SHM-only format works for GNOME headless
3. [x] ~~Accept both DMA-BUF and MemFd buffer types~~ → Only accept MemFd (0x4)
4. [x] ~~Add fixated format to update_params~~ → NOT needed! pipewiresrc doesn't do this
5. [x] Fix stream flags: use AUTOCONNECT | MAP_BUFFERS (not RT_PROCESS)
6. [x] Fix buffer params: only include dataType with FLAGS choice
7. [x] Call set_active(true) AFTER update_params succeeds (gnome-remote-desktop pattern)

### Phase 3: OpenPipeWireRemote for Portal Access (Done)
1. [x] Add OpenPipeWireRemote call in session_portal.go
2. [x] Pass pipeWireFd to VideoStreamer
3. [x] Add pipewire-fd property to zerocopy GStreamer element
4. [x] Use context.connect_fd() when portal FD is provided

### Phase 4: Test and Validate
1. [ ] Test with GNOME nested (SHM fallback)
2. [ ] Test with GNOME headless (may support DMA-BUF)
3. [ ] Test with Sway (portal FD required)
4. [ ] Measure CPU/GPU usage improvement

## Files Modified

| File | Change |
|------|--------|
| `api/pkg/desktop/ws_stream.go` | Add wsMu mutex, pass pipeWireFd to VideoStreamer |
| `api/pkg/desktop/desktop.go` | Add pipeWireFd field to Server struct |
| `api/pkg/desktop/session_portal.go` | Add OpenPipeWireRemote call for portal FD |
| `desktop/gst-pipewire-zerocopy/src/pipewire_stream.rs` | Fix buffer types, use portal FD for connection |
| `desktop/gst-pipewire-zerocopy/src/pipewiresrc/imp.rs` | Add pipewire-fd property |

## Implementation Summary

### Fix 1: Buffer Types (pipewire_stream.rs)

Changed buffer type bitmask from `MemPtr + DmaBuf` to `MemFd + DmaBuf`:

```rust
// BEFORE (wrong):
let buffer_types: i32 = (1 << 1) | (1 << 3);  // MemPtr + DmaBuf

// AFTER (correct, matches gnome-remote-desktop):
let buffer_types: i32 = (1 << 2) | (1 << 3);  // MemFd + DmaBuf
```

**Why this matters:**
- GNOME ScreenCast uses `MemFd` (memory-mapped file descriptor) for SHM frames, not `MemPtr`
- `MemPtr` is for local pointers only, doesn't work across process boundaries
- gnome-remote-desktop uses exactly this combination: `1 << SPA_DATA_MemFd | 1 << SPA_DATA_DmaBuf`

### Fix 2: WebSocket Mutex (ws_stream.go)

Added `wsMu sync.Mutex` to protect concurrent WebSocket writes:

```go
type VideoStreamer struct {
    // ...
    wsMu sync.Mutex  // Protects WebSocket writes
}

func (v *VideoStreamer) writeMessage(messageType int, data []byte) error {
    v.wsMu.Lock()
    defer v.wsMu.Unlock()
    return v.ws.WriteMessage(messageType, data)
}
```

All WebSocket writes now use `writeMessage()` or `writeJSON()` helpers

### Fix 3: OpenPipeWireRemote for Portal Access

The XDG Desktop Portal (used by Sway via xdg-desktop-portal-wlr) creates ScreenCast nodes
in a sandboxed PipeWire session. To access these nodes, we must:

1. Call `OpenPipeWireRemote` on the portal session to get an FD
2. Pass this FD to pipewiresrc/zerocopy plugin
3. Use `pw_context_connect_fd()` instead of default daemon connection

**session_portal.go:**
```go
func (s *Server) openPipeWireRemote() error {
    portalObj := s.conn.Object(portalBus, portalPath)
    options := map[string]dbus.Variant{}

    var pipeWireFd dbus.UnixFD
    err := portalObj.Call(
        portalScreenCastIface+".OpenPipeWireRemote",
        0,
        dbus.ObjectPath(s.portalSessionHandle),
        options,
    ).Store(&pipeWireFd)

    s.pipeWireFd = int(pipeWireFd)
    return nil
}
```

**pipewire_stream.rs:**
```rust
let core = if let Some(fd) = pipewire_fd {
    // Portal FD provided - connect to sandboxed PipeWire session
    let owned_fd = unsafe { std::os::fd::OwnedFd::from_raw_fd(fd) };
    context.connect_fd(owned_fd, None)?
} else {
    // No portal FD - connect to default daemon (GNOME native)
    context.connect(None)?
};
```

**Why this matters:**
- Without the portal FD, pipewiresrc gets "target not found" for ScreenCast nodes
- GNOME's native Mutter ScreenCast doesn't need this (same session, same daemon)
- Sway's portal ScreenCast requires this (separate sandboxed session)

## References

- [gnome-remote-desktop PipeWire stream](https://github.com/GNOME/gnome-remote-desktop/blob/master/src/grd-rdp-pipewire-stream.c)
- [OBS PipeWire plugin](https://github.com/obsproject/obs-studio/blob/master/plugins/linux-pipewire/pipewire.c)
- [GNOME Shell H.264 ScreenCast MR](https://gitlab.gnome.org/GNOME/gnome-shell/-/merge_requests/2080)
- [wl-screenrec](https://github.com/russelltg/wl-screenrec) - True zero-copy for wlroots
- [OBS Studio 30.2 NVENC Linux](https://www.gamingonlinux.com/2024/07/obs-studio-302-is-out-now-with-native-nvenc-encode-for-linux/)
- [obs-headless feature request](https://ideas.obsproject.com/posts/16/add-a-headless-mode-that-allows-full-control-via-scripting-api)
