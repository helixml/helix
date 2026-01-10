# PipeWire DMA-BUF Format Negotiation

**Date:** 2026-01-08
**Status:** In Progress
**Goal:** Fix ~40fps cap on GNOME Ubuntu desktop streaming (should be 60fps)

## Problem Statement

GNOME ScreenCast via PipeWire delivers frames at ~40fps instead of 60fps. OBS achieves 60fps from the same source, proving the issue is in the consumer code (`pipewirezerocopysrc` GStreamer element).

Investigation revealed that despite CUDA mode initializing correctly, PipeWire was sending **SHM frames instead of DmaBuf frames**, causing the pipeline to use a slow fallback path instead of zero-copy GPU rendering.

## Architecture

```
GNOME 49 Mutter ScreenCast
    ↓ (PipeWire)
pipewirezerocopysrc (Rust/GStreamer)
    ↓ (DmaBuf or SHM)
EGLImage → CUDAImage → CUDA Buffer
    ↓
Wolf streaming pipeline
```

## Key Files

- `/prod/home/luke/pm/wolf/gst-pipewire-zerocopy/src/pipewire_stream.rs` - PipeWire stream handling
- `/prod/home/luke/pm/wolf/gst-pipewire-zerocopy/src/pipewiresrc/imp.rs` - GStreamer source element
- `/tmp/obs-studio-src/plugins/linux-pipewire/pipewire.c` - OBS reference implementation

## Experiment Log

### Attempt 1: Increase PipeWire buffer count (Prior session)
- Changed `mpsc::sync_channel(2)` to `mpsc::sync_channel(8)`
- **Result:** Did not fix the framerate issue

### Attempt 2: Add VIDEO_RATE property (Prior session)
- Added `VIDEO_RATE => "60/1"` to stream properties
- **Result:** Did not fix the framerate issue

### Attempt 3: Add MUTTER_DEBUG_KMS_THREAD_TYPE=user workaround (Prior session)
- Added environment variable to fix Mutter frame scheduling
- Reference: https://gitlab.gnome.org/GNOME/mutter/-/issues/3788
- **Result:** Helped with frame drops but didn't fix underlying DmaBuf vs SHM issue

### Attempt 4: Add buffer params for DMA-BUF (Today)
- Added `build_buffer_params()` function to request `SPA_DATA_DmaBuf | SPA_DATA_MemPtr` buffer types
- Added `update_params()` call in `param_changed` callback (like OBS does)
- **Result:** Still receiving SHM frames

### Attempt 5: Add format/modifier constraints (Today)
- Studied OBS's `build_format()` function which creates one pod per format
- OBS includes modifier enum with `MANDATORY | DONT_FIXATE` flags
- Implemented single format pod with BGRx + DRM_FORMAT_MOD_INVALID

**Format pod structure:**
```
- mediaType: Video
- mediaSubtype: Raw
- videoFormat: BGRx (5)
- modifier: DRM_FORMAT_MOD_INVALID with flags 0x18
- framerate: Range 0-360 fps
```

**Result:** Format negotiation fails with "no more input formats"

PipeWire debug shows:
- Consumer offers: BGRx with modifier (flags 0x18)
- Producer offers: BGRx with modifier, BGRA with modifier, BGRx (no mod), BGRA (no mod)
- Negotiation fails at intersection

### Attempt 6: Fix SPA format enum values (Today)
- SPA format IDs were wrong! Using hardcoded values instead of `VideoFormat::BGRA.as_raw()`
- Fixed: BGRA=12, RGBA=11, BGRx=8, RGBx=7 (NOT 2,4,5,6)
- **Result:** Format negotiation improved but stream stuck at Paused state

### Attempt 7: Fix infinite loop in param_changed (Today)
- Problem: Calling `update_params()` in param_changed triggers another param_changed
- Initial fix: Added static `AtomicBool` flag to only call update_params once
- Problem: Static flag shared across all Wolf sessions - first session sets it, others skip
- Fixed: Changed to per-stream `Arc<AtomicBool>` created in run_pipewire_loop
- **Result:** No more infinite loop, but stream still stuck at Paused

### Attempt 8: Add Meta params like OBS (Today)
- Compared with OBS `on_param_changed_cb` - it sends multiple params:
  - ParamBuffers (dataType = DmaBuf | MemPtr)
  - ParamMeta for Header (REQUIRED for GNOME to complete negotiation)
  - ParamMeta for VideoCrop, Cursor, etc.
- We were only sending ParamBuffers - GNOME kept renegotiating forever
- Created `build_negotiation_params()` to return both Buffers + Header meta
- Updated param_changed to pass all params to update_params
- Added `set_active(true)` call after successful update_params
- **Result:** Format negotiation works, set_active succeeds, but stream never reaches Streaming state

### Attempt 9: Wrong SPA_PARAM constants investigation (2026-01-09)
- Noticed that GNOME keeps sending Format param_changed repeatedly
- Stream stuck in Paused state, never transitions to Streaming
- No frames received - keepalive timeout hits

**Investigation revealed wrong SPA_PARAM constant values:**

Initially tried 0x20000/0x10005 thinking they were offset-based, but these are WRONG.

Looking at actual PipeWire headers (spa/param/buffers.h):
```c
enum spa_param_buffers {
    SPA_PARAM_BUFFERS_START,      // = 0
    SPA_PARAM_BUFFERS_buffers,    // = 1
    SPA_PARAM_BUFFERS_blocks,     // = 2
    SPA_PARAM_BUFFERS_size,       // = 3
    SPA_PARAM_BUFFERS_stride,     // = 4
    SPA_PARAM_BUFFERS_align,      // = 5
    SPA_PARAM_BUFFERS_dataType,   // = 6
};

enum spa_param_meta {
    SPA_PARAM_META_START,         // = 0
    SPA_PARAM_META_type,          // = 1
    SPA_PARAM_META_size,          // = 2
};
```

**Correct values:**
- SPA_PARAM_META_type = 1
- SPA_PARAM_META_size = 2
- SPA_PARAM_BUFFERS_dataType = 6

**Fixed to use correct enum values (not offset-based):**
- **Result:** Testing in progress...

## OBS Reference Code

Key insight from OBS `build_format_params()`:

```c
// First, build pods WITH modifiers (DmaBuf capable)
for (size_t i = 0; i < format_info.num; i++) {
    if (format_info[i].modifiers.num == 0) continue;
    params[count++] = build_format(..., modifiers, modifier_count);
}

// Then, build pods WITHOUT modifiers (SHM fallback)
for (size_t i = 0; i < format_info.num; i++) {
    params[count++] = build_format(..., NULL, 0);
}
```

OBS also adds `DRM_FORMAT_MOD_INVALID` as implicit modifier if GPU supports it:
```c
if (dmabuf_flags & GS_DMABUF_FLAG_IMPLICIT_MODIFIERS_SUPPORTED) {
    uint64_t modifier_implicit = DRM_FORMAT_MOD_INVALID;
    da_push_back(info->modifiers, &modifier_implicit);
}
```

### Attempt 10: Mutter source code analysis (2026-01-09)

Cloned GNOME Mutter source to verify claims about linked sessions and DMA-BUF.

**Key findings from `/tmp/mutter/src/backends/meta-screen-cast-stream-src.c`:**

1. **`is-platform` flag has NO effect on DMA-BUF:**
   - `META_SCREEN_CAST_FLAG_IS_PLATFORM` is defined in `meta-screen-cast.h` (line 42)
   - It's SET in session creation (line 770) but **NEVER CHECKED** anywhere
   - My earlier claim that "is-platform forces SHM" was **INCORRECT**

2. **Linked sessions do NOT restrict DMA-BUF:**
   - Searched for `remote_desktop_session` in `meta-screen-cast-stream-src.c`: **0 matches**
   - `meta_screen_cast_query_modifiers()` queries Cogl renderer directly, no session filtering
   - Modifier availability is determined by GPU driver, not session type

3. **What actually determines SHM vs DMA-BUF (line 1668-1673):**
   ```c
   prop_modifier = spa_pod_find_prop(format, NULL, SPA_FORMAT_VIDEO_modifier);
   if (prop_modifier)
     buffer_types = 1 << SPA_DATA_DmaBuf;
   else
     buffer_types = 1 << SPA_DATA_MemFd;
   ```
   - If the negotiated format has modifiers → DMA-BUF
   - If no modifiers → MemFd (SHM)

4. **Format negotiation is consumer-driven:**
   - Consumer (Wolf's pipewiresrc) advertises supported formats/modifiers
   - Mutter intersects with its capabilities
   - If intersection includes modifiers → DMA-BUF is used

**Root cause hypothesis update:**
The issue is NOT a Mutter limitation. The issue is that Wolf's `pipewirezerocopysrc` is either:
- Not properly advertising modifier support in format negotiation, OR
- Advertising incompatible modifiers that Mutter can't intersect with, OR
- Some other PipeWire client-side issue

**Observed behavior:**
- Linked session (node 44): Format negotiated as BGRx, modifier=0x0 (linear = SHM fallback)
- Standalone session (node 47/51): Format negotiated as BGRx with NVIDIA modifiers

This difference is likely due to the TIMING of session creation or the ORDER of modifier preference, not a hard limitation on linked sessions.

### Attempt 11: Rebuild and test with set_active(true) fix (2026-01-09)

Committed changes to Wolf branch `feature/pipewire-set-active-fix`:
- Added `pw_stream_set_active(true)` call after `update_params()` (following OBS pattern)
- Added 30-second first-frame timeout for fail-fast behavior
- Updated Cargo.toml version to force rebuild

Committed changes to Helix branch `feature/sway-ubuntu-25.10`:
- Removed `is-platform=true` from RecordMonitor options (no effect, but clean up)
- Deleted dead `start-pipewire-screencast.sh` script
- Updated comments to reflect accurate Mutter behavior

**Status:** Testing pending - sandbox rebuilt, need to create test session.

### Attempt 12: Cross-container PipeWire authorization analysis (2026-01-09)

Deep analysis of OBS vs Wolf implementation revealed the root cause: **portal FD authorization**.

**How OBS connects to PipeWire (authorized):**
```c
// 1. Create D-Bus session with XDG Portal
// 2. Get authorized FD via OpenPipeWireRemote()
g_dbus_proxy_call_with_unix_fd_list(portal_proxy, "OpenPipeWireRemote", ...);

// 3. Connect using the authorized FD
obs_pw->core = pw_context_connect_fd(context, authorized_fd, NULL, 0);

// 4. Core sync to wait for registry (CRITICAL)
obs_pw->sync_id = pw_core_sync(obs_pw->core, PW_ID_CORE, seq);
pw_thread_loop_wait(obs_pw->thread_loop);
```

**How Wolf connects to PipeWire (unauthorized):**
```rust
// 1. Uses Mutter's native D-Bus API (not portal)
// 2. Gets node ID from PipeWireStreamAdded signal
// 3. Connects via socket (PIPEWIRE_REMOTE env var)
let core = context.connect(None)?;

// 4. NO core sync
// 5. Tries to consume stream from outside container
```

**Key differences:**
1. OBS uses XDG Portal's `OpenPipeWireRemote()` - this returns an authorized FD
2. Wolf uses direct socket connection via `PIPEWIRE_REMOTE` environment variable
3. The portal FD carries authorization context that allows stream linking
4. Cross-container socket access doesn't have this authorization

**Evidence:**
- Linked session format shows linear modifier (0x0) = SHM fallback
- Standalone session shows NVIDIA modifiers = DMA-BUF works
- Stream connects but stays in Paused state forever
- WirePlumber linking doesn't complete for unauthorized streams

**Root cause:** PipeWire's linking mechanism requires either:
1. Portal-authorized FD (what OBS does)
2. Same-process/container access (what standalone sessions have)

Wolf's cross-container socket access has neither.

### Proposed Solutions

**Option A: In-container PipeWire capture (Recommended)**
Move PipeWire stream consumption inside the Ubuntu container:

```
Ubuntu Container                    Wolf (sandbox)
├── GNOME ScreenCast (producer)     ├── Receives H.264 video
│       │                           │       ↑
│       ↓ (PipeWire link)           │       │
├── GStreamer pipewiresrc           │   Shared memory
│       │                           │   or socket
│       ↓ (encode)                  │       ↑
├── H.264 encoder ─────────────────────────┘
```

This bypasses cross-container PipeWire issues entirely.

**Option B: Portal FD passthrough**
Have screenshot-server call `OpenPipeWireRemote()` and somehow pass the FD to Wolf.
Challenge: FD passing between containers requires SCM_RIGHTS over Unix socket.

**Option C: PipeWire proxy node**
Create a PipeWire producer inside container that re-exports ScreenCast as new node.
Challenge: Complex PipeWire programming, duplicates effort.

### Implementation Plan: Option A (In-container capture via SHM)

Use GStreamer's shared memory sink/source to forward frames from container to Wolf.

**In Ubuntu container (screenshot-server):**
```bash
gst-launch-1.0 pipewiresrc path=<node_id> ! \
  video/x-raw,format=BGRx ! \
  shmsink socket-path=/wolf-state/video-shm wait-for-connection=true
```

**In Wolf (sandbox):**
```
shmsrc socket-path=/wolf-state/video-shm ! video/x-raw,format=BGRx ! <nvenc pipeline>
```

The `runner_state_folder` (/wolf-state/agent-xxx) is already shared between containers.

**Changes required:**
1. screenshot-server: Add background GStreamer process for video forwarding
2. Wolf: Add shmsrc-based video producer option as alternative to pipewiresrc
3. API: Report SHM path instead of node_id for GNOME mode

**Pros:**
- Minimal code changes
- Uses existing GStreamer knowledge
- SHM socket path already in shared volume

**Cons:**
- Extra memory copy (not true zero-copy)
- Additional latency from GStreamer pipeline
- Needs process management for gst-launch

**Alternative: Direct DMA-BUF FD forwarding**

For true zero-copy, forward DMA-BUF file descriptors via Unix socket:
1. Go captures via cgo + GStreamer appsink
2. Extracts DMA-BUF fd from buffer
3. Sends fd via SCM_RIGHTS to Wolf's Unix socket
4. Wolf imports fd directly into CUDA

This is more complex but preserves GPU zero-copy.

### The Actual "Error": Silent Authorization Failure (2026-01-09)

**Important clarification:** There is NO explicit "permission denied" error. The failure is silent:

**Observed symptoms:**
1. PipeWire stream successfully created with correct node ID
2. Format negotiation completes (BGRx with linear modifier)
3. `pw_stream_set_active(true)` returns success
4. Stream state transitions: Unconnected → Connecting → **Paused** (stops here)
5. Stream NEVER transitions to **Streaming** state
6. No frames are ever delivered
7. After 30 seconds, our timeout fires and logs: "No frames received - stream keepalive failed"

**Why this happens:**

PipeWire streams require proper authorization to link producer→consumer. When OBS runs inside the same session as GNOME:

```
OBS Process                       GNOME Mutter
    │                                 │
    ├── OpenPipeWireRemote() ───────►│
    │◄──────── authorized FD ────────┤
    │                                 │
    ├── pw_context_connect_fd(fd) ──►│ PipeWire daemon
    │                                 │ "ah, this FD was authorized
    │◄───── link completes ──────────┤  by portal, allow streaming"
```

When Wolf runs in a different container:

```
Wolf (sandbox container)          Ubuntu Container (GNOME)
    │                                 │
    │   PIPEWIRE_REMOTE=socket ──────►│
    │   pw_context_connect(socket) ──►│ PipeWire daemon
    │                                 │ "socket connection from
    │◄───── Paused state ────────────┤  external process, no portal
    │       (link incomplete)         │  authorization, block linking"
```

The stream enters Paused state and waits forever because:
1. The consumer (Wolf) connected via socket without portal authorization
2. PipeWire's security model prevents linking to protected streams
3. Mutter's ScreenCast stream is protected (created via RemoteDesktop portal)

**Evidence from debug logs:**
```
[PIPEWIRE] Stream state changed from Connecting to Paused
[PIPEWIRE] Format negotiation complete: BGRx, modifier=0x0
[PIPEWIRE] Called set_active(true) - waiting for Streaming state...
... (30 seconds of silence) ...
[PIPEWIRE] ERROR: No frames received after 30s - stream keepalive failed
```

The lack of any explicit error message makes this difficult to debug. PipeWire doesn't log "authorization failed" - it just silently refuses to complete the link.

### SHM Solution: Performance Analysis (2026-01-09)

**Question: Can we achieve 4K@60fps with CPU memory copies?**

**Frame size calculation:**
- 4K resolution: 3840 × 2160 = 8,294,400 pixels
- BGRx format: 4 bytes/pixel
- Frame size: 3840 × 2160 × 4 = **31.6 MB per frame**
- At 60fps: 31.6 × 60 = **1,899 MB/s ≈ 1.9 GB/s**

**Memory bandwidth on modern systems:**
- DDR4-2400: ~19 GB/s (single channel), ~38 GB/s (dual channel)
- DDR5-4800: ~38 GB/s (single channel), ~76 GB/s (dual channel)
- PCIe 3.0 x16: ~16 GB/s (GPU↔CPU)
- PCIe 4.0 x16: ~32 GB/s (GPU↔CPU)

**Data flow with SHM solution:**

```
Ubuntu Container                           Wolf (sandbox)
┌─────────────────────────────────────┐   ┌─────────────────────────────────┐
│                                     │   │                                 │
│  Mutter ScreenCast                  │   │                                 │
│       │ (DMA-BUF, GPU memory)       │   │                                 │
│       ▼                             │   │                                 │
│  pipewiresrc (inside container)     │   │                                 │
│       │ negotiates DMA-BUF          │   │                                 │
│       │ with Mutter directly        │   │                                 │
│       ▼                             │   │                                 │
│  videoconvert (if needed)           │   │                                 │
│       │                             │   │                                 │
│       ▼ [COPY 1: GPU→CPU]           │   │                                 │
│  shmsink ──────────────────────────────►│ shmsrc                          │
│       (Unix socket, CPU memory)     │   │       │                         │
│                                     │   │       ▼ [COPY 2: CPU→GPU]       │
│                                     │   │  cudaupload                     │
│                                     │   │       │                         │
│                                     │   │       ▼                         │
│                                     │   │  NVENC (GPU encoding)           │
│                                     │   │       │                         │
│                                     │   │       ▼                         │
│                                     │   │  H.264/HEVC stream to client    │
└─────────────────────────────────────┘   └─────────────────────────────────┘
```

**Latency impact:**
- GPU→CPU download: ~1-2ms for 32MB with PCIe 3.0
- Unix socket IPC: <1ms (kernel optimized, same-machine)
- CPU→GPU upload: ~1-2ms

**Total added latency: ~3-5ms** (well under one frame time of 16.67ms)

**Conclusion: YES, 4K@60fps is achievable with this design.**

The bandwidth requirements (1.9 GB/s) are well within modern system capabilities:
- Memory bandwidth: 10-20x headroom
- PCIe bandwidth: 8-16x headroom

The latency addition (~4ms) is acceptable for cloud gaming/remote desktop.

**Comparison with true zero-copy (DMA-BUF forwarding):**

If we were to implement DMA-BUF fd forwarding via SCM_RIGHTS:
- Latency: <1ms (no memory copies)
- Complexity: Much higher (custom protocol, cgo, fd management)
- Risk: DMA-BUF fds may not be valid across container boundaries

The SHM solution trades ~4ms latency for simplicity and reliability.

## Commits

- `e618b39` (wolf) feat(pipewiresrc): add pw_stream_set_active(true) + 30s first-frame timeout
- `6ae887de5` (helix) refactor(desktop): remove is-platform=true and dead script
