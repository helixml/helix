# Zero-Copy GPU Video Streaming

**Date:** 2026-01-10
**Status:** Design
**Author:** Claude (with Luke)
**Depends on:** 2026-01-09-direct-video-streaming.md

## Problem Statement

The current video streaming path in desktop containers uses SHM (shared memory), which is NOT zero-copy:

```
Current Path (2 CPU copies):
PipeWire ScreenCast → SHM → pipewiresrc → shmsink → shmsrc → cudaupload → nvh264enc
                      ^                           ^
                      Copy 1                      Copy 2
```

True zero-copy means the GPU frame data never touches the CPU after initial capture:

```
True Zero-Copy (0 CPU copies):
PipeWire ScreenCast → DMA-BUF → EGL Image → CUDA → nvh264enc
                      ↑         ↑           ↑
                      GPU       GPU         GPU
                      memory    memory      memory
```

## How OBS Does It

OBS achieves zero-copy on NVIDIA GPUs using EGL interop:

1. **PipeWire delivers DMA-BUF**: GNOME ScreenCast exports frames as DMA-BUF file descriptors
2. **EGL imports DMA-BUF**: `eglCreateImageKHR` with `EGL_LINUX_DMA_BUF_EXT` creates an EGLImage
3. **CUDA imports EGL texture**: `cuGraphicsEGLRegisterImage` registers the EGLImage for CUDA
4. **NVENC encodes directly**: CUDA buffer passed to nvh264enc without CPU copy

Key insight: NVIDIA doesn't support standard `DRM_IOCTL_PRIME_FD_TO_HANDLE` for DMA-BUF import. You MUST go through EGL.

## Option A: Native GStreamer (Ubuntu 25.10)

Ubuntu 25.10 ships with GStreamer 1.24+ which may have native DMA-BUF → CUDA support. This is the preferred option if available.

### GStreamer 1.24 Elements to Test

```bash
# Check available CUDA elements
gst-inspect-1.0 cuda

# Key elements for zero-copy:
gst-inspect-1.0 pipewiresrc     # Should support DMA-BUF output
gst-inspect-1.0 cudaupload      # Can it accept DMA-BUF directly?
gst-inspect-1.0 glupload        # Alternative: GL → CUDA path
gst-inspect-1.0 cudaconvert     # GPU-side format conversion
```

### Native Zero-Copy Pipeline (if supported)

```
pipewiresrc → video/x-raw(memory:DMABuf) → glupload → gldownload(memory:CUDAMemory) → nvh264enc
```

Or if `cudaupload` supports DMA-BUF directly:

```
pipewiresrc → video/x-raw(memory:DMABuf) → cudaupload → nvh264enc
```

### Testing Native Support

Run these tests in the Ubuntu 25.10 desktop container:

```bash
# Test 1: Check if pipewiresrc outputs DMA-BUF
gst-launch-1.0 pipewiresrc path=<node_id> ! fakesink -v 2>&1 | grep -i dmabuf

# Test 2: Check if cudaupload can accept DMA-BUF
gst-launch-1.0 videotestsrc ! video/x-raw ! glupload ! gldownload ! cudaupload ! fakesink

# Test 3: Full pipeline test (if pipewiresrc + CUDA works)
gst-launch-1.0 pipewiresrc path=<node_id> ! \
    video/x-raw\(memory:DMABuf\) ! \
    cudaupload ! \
    cudaconvertscale ! video/x-raw\(memory:CUDAMemory\),format=NV12 ! \
    nvh264enc ! fakesink
```

### Native GStreamer Pipeline (Go code)

If native support works:

```go
pipeline := fmt.Sprintf(
    "pipewiresrc path=%d ! video/x-raw(memory:DMABuf) " +
    "! cudaupload ! cudaconvertscale " +
    "! video/x-raw(memory:CUDAMemory),format=NV12,width=%d,height=%d " +
    "! nvh264enc preset=low-latency-hq rc-mode=cbr bitrate=%d gop-size=60 " +
    "! appsink name=videosink emit-signals=true max-buffers=2 drop=true sync=false",
    nodeID, width, height, bitrate,
)
```

### Advantages of Native GStreamer
- No custom Rust plugin to maintain
- Uses upstream-tested code paths
- Easier to update (apt upgrade)
- Better long-term support

### Limitations
- May not support GNOME 49+ damage-based ScreenCast keepalive
- Less control over EGL/CUDA interop details
- Untested - need to verify in Ubuntu 25.10

---

## Option B: pipewirezerocopysrc Plugin

Wolf already has a Rust GStreamer plugin that implements true zero-copy:

**Location:** `~/pm/wolf/gst-pipewire-zerocopy/`

**Architecture:**
```
pipewirezerocopysrc (GStreamer source element)
    ↓ Uses
waylanddisplaycore (Battle-tested CUDA/EGL code from gst-wayland-display)
    ↓ Provides
EGLImage::from() + CUDAImage::from() + CUDABufferPool
    ↓ Output
video/x-raw(memory:CUDAMemory) → nvh264enc → WebSocket
```

**Key Features:**
- Connects to PipeWire ScreenCast portal
- Receives DMA-BUF frames directly
- Converts to CUDA buffers using EGL interop
- Outputs `video/x-raw(memory:CUDAMemory)` for hardware encoding
- Handles GNOME 49+ damage-based ScreenCast (keepalive mechanism)
- Fallback to SHM if CUDA unavailable

---

## Comparison: Native vs Plugin

| Aspect | Option A (Native GStreamer) | Option B (pipewirezerocopysrc) |
|--------|----------------------------|-------------------------------|
| **Maintenance** | None - upstream maintained | Must maintain Rust plugin |
| **Build time** | None | ~2-3 min Rust compilation |
| **Container size** | Smaller | +50-100MB for Rust toolchain |
| **GNOME 49 keepalive** | Unlikely | Built-in (100ms resend) |
| **EGL/CUDA control** | Limited | Full control |
| **Debugging** | Harder (upstream code) | Easier (our code) |
| **Risk** | May not work on NVIDIA | Battle-tested in Wolf |

### Recommendation

1. **First**: Test Option A (native GStreamer) in Ubuntu 25.10
2. **If fails**: Implement Option B (pipewirezerocopysrc)
3. **Long term**: Monitor GStreamer upstream for native improvements

---

## Integration Plan

### Step 0: Test Native GStreamer Support

Before implementing either option, test native support:

```bash
# Start sandbox and desktop
./stack start

# Get a session ID
/tmp/helix spectask list

# Exec into the desktop container
docker compose exec -T sandbox docker exec -it <container_name> bash

# Run tests from Option A above
gst-inspect-1.0 cuda
gst-inspect-1.0 pipewiresrc 2>&1 | grep -i dmabuf
```

If native works, skip to "Native Integration". Otherwise, proceed with Plugin Integration.

---

### Option A Integration: Native GStreamer

If native DMA-BUF → CUDA works, update ws_stream.go:

```go
// In api/pkg/desktop/ws_stream.go
func buildZeroCopyPipeline(nodeID uint32, width, height, fps, bitrate int) string {
    return fmt.Sprintf(
        "pipewiresrc path=%d ! video/x-raw(memory:DMABuf) " +
        "! cudaupload ! cudaconvertscale " +
        "! video/x-raw(memory:CUDAMemory),format=NV12,width=%d,height=%d " +
        "! nvh264enc preset=low-latency-hq rc-mode=cbr bitrate=%d gop-size=60 " +
        "! appsink name=videosink emit-signals=true max-buffers=2 drop=true sync=false",
        nodeID, width, height, bitrate,
    )
}
```

---

### Option B Integration: pipewirezerocopysrc Plugin

### Phase 1: Copy Plugin to Helix Repository

Copy the GStreamer plugin source from Wolf to Helix:

```bash
# Create plugin directory in desktop codebase
mkdir -p desktop/gst-pipewire-zerocopy

# Copy source files
cp -r ~/pm/wolf/gst-pipewire-zerocopy/src desktop/gst-pipewire-zerocopy/
cp ~/pm/wolf/gst-pipewire-zerocopy/Cargo.toml desktop/gst-pipewire-zerocopy/
cp ~/pm/wolf/gst-pipewire-zerocopy/build.rs desktop/gst-pipewire-zerocopy/
```

### Phase 2: Update Desktop Container Dockerfiles

Modify `Dockerfile.sway-helix` and `Dockerfile.ubuntu-helix` to build and install the plugin:

```dockerfile
# Build dependencies for the GStreamer plugin
RUN apt-get update && apt-get install -y \
    libgstreamer1.0-dev \
    libgstreamer-plugins-base1.0-dev \
    libpipewire-0.3-dev \
    libspa-0.2-dev \
    libegl1-mesa-dev \
    libdrm-dev \
    && rm -rf /var/lib/apt/lists/*

# Build the zero-copy GStreamer plugin
COPY desktop/gst-pipewire-zerocopy /build/gst-pipewire-zerocopy
RUN cd /build/gst-pipewire-zerocopy && \
    cargo build --release && \
    cp target/release/libgstpipewirezerocopy.so /usr/lib/x86_64-linux-gnu/gstreamer-1.0/

# Verify plugin is loadable
RUN gst-inspect-1.0 pipewirezerocopysrc || echo "Plugin will be available at runtime with CUDA"
```

### Phase 3: Update Screenshot Server Pipeline

Modify `api/pkg/desktop/ws_stream.go` to use the new plugin:

**Current pipeline (SHM path):**
```go
pipeline := fmt.Sprintf(
    "pipewiresrc path=%d ! video/x-raw ! shmsink socket-path=/tmp/helix-video-%s shm-size=67108864 wait-for-connection=false sync=false",
    nodeID, sessionID,
)
// ... separate shmsrc → cudaupload pipeline
```

**New pipeline (zero-copy path):**
```go
pipeline := fmt.Sprintf(
    "pipewirezerocopysrc pipewire-node-id=%d output-mode=cuda keepalive-time=100 " +
    "! video/x-raw(memory:CUDAMemory),format=BGRA,width=%d,height=%d,framerate=%d/1 " +
    "! cudaconvertscale ! video/x-raw(memory:CUDAMemory),format=NV12 " +
    "! nvh264enc preset=low-latency-hq rc-mode=cbr bitrate=%d gop-size=60 " +
    "! video/x-h264,profile=high " +
    "! appsink name=videosink emit-signals=true max-buffers=2 drop=true sync=false",
    nodeID, width, height, fps, bitrate,
)
```

### Phase 4: Fallback Logic

The plugin supports automatic fallback to system memory if CUDA isn't available:

```go
// Try zero-copy first, fallback to SHM if it fails
pipeline := buildZeroCopyPipeline(nodeID, width, height, fps, bitrate)
if err := launchPipeline(pipeline); err != nil {
    log.Warn().Err(err).Msg("Zero-copy pipeline failed, falling back to SHM")
    pipeline = buildSHMPipeline(nodeID, width, height, fps, bitrate)
    if err := launchPipeline(pipeline); err != nil {
        return fmt.Errorf("all pipelines failed: %w", err)
    }
}
```

## Plugin Properties

The `pipewirezerocopysrc` element exposes these properties:

| Property | Type | Default | Description |
|----------|------|---------|-------------|
| `pipewire-node-id` | uint | 0 | PipeWire node ID from ScreenCast portal |
| `render-node` | string | `/dev/dri/renderD128` | DRM render node for EGL |
| `output-mode` | string | `auto` | `auto`, `cuda`, `dmabuf`, or `system` |
| `cuda-device-id` | int | -1 | CUDA device ID (-1 = auto) |
| `keepalive-time` | uint | 100 | Resend last frame interval (ms), for GNOME damage-based ScreenCast |
| `resend-last` | bool | false | Resend last buffer on EOS |

## Dependencies

The plugin depends on:

1. **waylanddisplaycore** - Battle-tested CUDA/EGL code from gst-wayland-display
   - Git: `https://github.com/games-on-whales/gst-wayland-display`
   - Provides: `CUDAContext`, `CUDAImage`, `EGLImage`, `CUDABufferPool`

2. **smithay** - Wayland compositor library
   - Git: `https://github.com/games-on-whales/smithay` (fork with specific patches)
   - Provides: `Dmabuf`, `EGLDisplay`, DRM types

3. **pipewire-rs** - Rust bindings for PipeWire
   - Version: 0.8
   - Provides: Stream, MainLoop, format negotiation

## Performance Comparison

| Metric | SHM Path | Zero-Copy Path |
|--------|----------|----------------|
| CPU copies | 2 | 0 |
| Memory bandwidth | 2x frame size/frame | 0 |
| Latency | +2-5ms (copy overhead) | Minimal |
| CPU usage | Higher | Lower |
| GPU utilization | Normal | Slightly higher (EGL interop) |

Expected improvement: 30-50% reduction in CPU usage for video streaming.

## Testing Plan

1. **Unit tests**: Run plugin's built-in tests
   ```bash
   cd desktop/gst-pipewire-zerocopy && cargo test
   ```

2. **Integration test**: Verify plugin loads in container
   ```bash
   docker compose exec desktop gst-inspect-1.0 pipewirezerocopysrc
   ```

3. **E2E test**: Compare SHM vs zero-copy FPS and CPU usage
   ```bash
   # With SHM
   /tmp/helix spectask benchmark ses_xxx --duration 60 --stress-gpu 50

   # With zero-copy
   HELIX_ZERO_COPY=true /tmp/helix spectask benchmark ses_xxx --duration 60 --stress-gpu 50
   ```

4. **Visual test**: Verify no color corruption (DRM fourcc mapping)
   - Check that BGRA/RGBA formats map correctly
   - Test with different NVIDIA modifiers (linear, tiled)

## Files to Create/Modify

### New Files
- `desktop/gst-pipewire-zerocopy/` - Plugin source (copied from Wolf)
- `desktop/gst-pipewire-zerocopy/src/lib.rs`
- `desktop/gst-pipewire-zerocopy/src/pipewiresrc/mod.rs`
- `desktop/gst-pipewire-zerocopy/src/pipewiresrc/imp.rs`
- `desktop/gst-pipewire-zerocopy/src/pipewire_stream.rs`
- `desktop/gst-pipewire-zerocopy/Cargo.toml`
- `desktop/gst-pipewire-zerocopy/build.rs`

### Modified Files
- `Dockerfile.sway-helix` - Build and install plugin
- `Dockerfile.ubuntu-helix` - Build and install plugin
- `api/pkg/desktop/ws_stream.go` - Use new pipeline
- `api/pkg/desktop/session.go` - Add zero-copy config

## Migration Path

1. **Phase 1**: Add plugin to desktop containers (this design)
2. **Phase 2**: A/B test with `HELIX_ZERO_COPY` env var
3. **Phase 3**: Make zero-copy default, SHM as fallback
4. **Phase 4**: Remove SHM codepath once stable

## Open Questions

1. **Modifier compatibility**: Will all NVIDIA GPUs support the same DRM modifiers?
   - Recommendation: Use `DRM_FORMAT_MOD_LINEAR` as preferred, with `DRM_FORMAT_MOD_INVALID` fallback

2. **AMD support**: Does waylanddisplaycore support AMD GPUs?
   - Current status: NVIDIA only via CUDA
   - Future: Could add VA-API path for AMD/Intel

3. **Rust build in container**: Should we pre-build the plugin or build in Dockerfile?
   - Recommendation: Build in Dockerfile for reproducibility
   - Alternative: Pre-build and cache the .so file

## Conclusion

True zero-copy video streaming eliminates CPU copies in the video pipeline. We have two options:

### Option A: Native GStreamer (Preferred if works)
- Test `pipewiresrc ! video/x-raw(memory:DMABuf) ! cudaupload` in Ubuntu 25.10
- Zero maintenance, upstream supported
- Risk: NVIDIA DMA-BUF support may be incomplete

### Option B: pipewirezerocopysrc Plugin (Fallback)
- Copy Wolf's Rust plugin to Helix
- Battle-tested, includes GNOME 49 keepalive
- Risk: Maintenance burden, build complexity

### Next Steps

1. **Test native GStreamer** in Ubuntu 25.10 container
2. **If works**: Use Option A (simpler)
3. **If fails**: Implement Option B (reliable)

Expected benefits: 30-50% CPU reduction, lower latency, cleaner architecture.

---

## Implementation Status (2026-01-10)

### Completed

1. **HELIX_VIDEO_MODE environment variable** (`api/pkg/desktop/ws_stream.go`)
   - Three modes: `shm` (default), `native`, `zerocopy`
   - Automatically selects appropriate GStreamer pipeline based on mode
   - Works with all encoders: NVIDIA NVENC, Intel QSV, AMD/Intel VA-API, x264 software

2. **pipewirezerocopysrc plugin copied to Helix** (`desktop/gst-pipewire-zerocopy/`)
   - Source copied from Wolf's gst-pipewire-zerocopy
   - True zero-copy via EGL → CUDA interop for NVIDIA
   - DMA-BUF output for AMD/Intel VA-API

3. **Dockerfile updates**
   - `Dockerfile.ubuntu-helix`: Added Rust build stage, installs plugin to `/usr/lib/x86_64-linux-gnu/gstreamer-1.0/`
   - `Dockerfile.sway-helix`: Same changes

### Video Mode Details

| Mode | Source Element | NVIDIA Path | AMD/Intel Path | CPU Copies |
|------|----------------|-------------|----------------|------------|
| `shm` | pipewiresrc | cudaupload → nvh264enc | videoconvert → vah264enc | 1-2 |
| `native` | pipewiresrc (DMABuf) | cudaupload → nvh264enc | vapostproc → vah264enc | 0-1 |
| `zerocopy` | pipewirezerocopysrc | CUDAMemory → nvh264enc | DMABuf → vapostproc → vah264enc | 0 |

### Usage

```bash
# Set in desktop container environment:
HELIX_VIDEO_MODE=zerocopy  # True zero-copy (requires plugin)
HELIX_VIDEO_MODE=native    # Native GStreamer DMA-BUF (GStreamer 1.24+)
HELIX_VIDEO_MODE=shm       # Default, most compatible
```

### Files Changed

- `api/pkg/desktop/ws_stream.go` - Added VideoMode type, getVideoMode(), updated buildPipelineArgs()
- `desktop/gst-pipewire-zerocopy/` - New plugin source (copied from Wolf)
- `Dockerfile.ubuntu-helix` - Added Rust build stage
- `Dockerfile.sway-helix` - Added Rust build stage
- `api/pkg/cli/spectask/spectask.go` - Added --video-mode flag to stream command

### Architecture Change: Same-Container Streaming (Hypothesis)

**Previous Architecture (Wolf mode)**:
```
Desktop Container             Sandbox Container (Wolf)
├── GNOME/Mutter              ├── Wolf streaming server
├── PipeWire                  └── GStreamer pipeline
│   └── ScreenCast DMA-BUF ──────> (some mechanism) ──> Wolf
```

**New Architecture (Wolf-free mode)**:
```
Desktop Container (all in one)
├── GNOME/Mutter
├── PipeWire
│   └── ScreenCast DMA-BUF
├── screenshot-server (Go)
│   └── GStreamer pipeline ──> pipewirezerocopysrc
│       └── DMA-BUF ──> EGL ──> CUDA ──> nvh264enc ──> WebSocket
```

**Hypothesis**: Running the GStreamer pipeline inside the same container as the
compositor might eliminate whatever was preventing zero-copy from working previously.

**However, this hypothesis may be wrong.** Key counter-evidence:
- Wolf successfully achieves zero-copy with NVIDIA GPUs across container boundaries
- Wolf uses lobbies/sessions that share FDs between desktop and streaming containers
- Wolf's gst-wayland-display plugin is battle-tested for exactly this use case

**What we don't fully understand:**
1. What specifically caused zero-copy to fail in our previous attempts?
2. Does Wolf do something special to make cross-container DMA-BUF work?
3. Was the issue actually DMA-BUF security, or something else entirely?

**Possible explanations for previous failures:**
- Misconfiguration of the GStreamer pipeline elements
- Missing GPU driver capabilities or permissions
- PipeWire version incompatibilities
- EGL/CUDA context initialization order issues
- Something specific to GNOME 49's ScreenCast implementation

The same-container architecture is simpler and eliminates cross-container complexity,
but we should not assume it was the cross-container aspect that caused the failure.

---

## Multiple Video Streams from Same Desktop

**Question**: Can multiple clients stream video from the same desktop container?

### How PipeWire ScreenCast Works

1. **Portal session**: Client calls `org.freedesktop.portal.ScreenCast.CreateSession()`
2. **Source selection**: Client calls `SelectSources()` to pick monitors/windows
3. **Start capture**: Client calls `Start()` which returns a PipeWire node ID
4. **Stream consumption**: Client connects to the PipeWire node via `pipewiresrc`

### Option A: Multiple Portal Sessions (Simple but Wasteful)

Each client creates its own ScreenCast session:
```
Client 1 → CreateSession → SelectSources → Start → Node 42 → pipewiresrc
Client 2 → CreateSession → SelectSources → Start → Node 43 → pipewiresrc
Client 3 → CreateSession → SelectSources → Start → Node 44 → pipewiresrc
```

**Pros:**
- Simple - no coordination needed
- Each client is independent
- Standard portal usage

**Cons:**
- Compositor renders frame N times for N clients
- GPU memory multiplied by N
- CPU overhead for multiple ScreenCast sessions
- User sees N "screen sharing" indicators in GNOME

### Option B: Shared PipeWire Node (Efficient)

One ScreenCast session, multiple consumers on the same node:
```
Session Owner → CreateSession → Start → Node 42
                                          ↓
Client 1 ─────────────────────────> pipewiresrc path=42
Client 2 ─────────────────────────> pipewiresrc path=42
Client 3 ─────────────────────────> pipewiresrc path=42
```

**PipeWire architecture supports this:**
- PipeWire nodes can have multiple output links
- Each consumer creates a stream connecting to the source
- PipeWire handles buffer sharing/copying as needed

**Key questions:**
1. **Does pipewiresrc support multiple instances on same node?**
   - Likely yes - PipeWire is designed for this (like JACK audio)
   - Need to test with `pw-cli` and multiple consumers

2. **DMA-BUF sharing with multiple consumers:**
   - If source provides DMA-BUF, can multiple consumers import it?
   - GPU memory is shared, but each consumer needs its own EGL/CUDA mapping
   - Performance depends on whether copy-on-read is needed

3. **GNOME ScreenCast specifics:**
   - Does Mutter's ScreenCast implementation support multiple links?
   - GNOME 49's damage-based ScreenCast sends frames on-damage only
   - Multiple consumers should receive the same damage notifications

### Option C: Application-Level Fan-Out

One GStreamer pipeline with `tee` element:
```
pipewiresrc path=42 → tee → queue → nvh264enc → Client 1 WebSocket
                        ├── queue → nvh264enc → Client 2 WebSocket
                        └── queue → nvh264enc → Client 3 WebSocket
```

**Pros:**
- Single PipeWire connection
- Single compositor capture
- Explicit control over fan-out

**Cons:**
- Multiple encode operations (CPU/GPU intensive)
- All clients get same resolution/bitrate

### Option D: Encode Once, Distribute Many (Ideal)

One encode, multiple WebSocket outputs:
```
pipewiresrc → nvh264enc → tee → queue → Client 1 WebSocket
                            ├── queue → Client 2 WebSocket
                            └── queue → Client 3 WebSocket
```

**Pros:**
- Single capture
- Single encode
- Minimal GPU usage
- All clients get identical stream

**Cons:**
- All clients locked to same resolution/bitrate/codec
- No per-client adaptation
- Joining mid-stream requires keyframe

### Recommendation

For Helix's use case (typically one user viewing their own desktop):
1. **Start with Option A** (multiple portal sessions) - simplest, works today
2. **If performance matters**, implement Option D (encode once, tee to WebSockets)
3. **For adaptive bitrate**, consider Option C with per-client encoders

Testing needed:
```bash
# Test multiple pipewiresrc on same node
pw-cli ls Node  # List nodes
gst-launch-1.0 pipewiresrc path=42 ! fakesink &
gst-launch-1.0 pipewiresrc path=42 ! fakesink &
# Do both receive frames?
```
