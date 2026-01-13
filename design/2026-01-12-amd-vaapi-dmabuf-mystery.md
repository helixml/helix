# AMD VA-API DMABuf Zero-Copy Mystery

**Date:** 2026-01-12
**Status:** Investigation In Progress
**Author:** Luke (with Claude)

## Problem Statement

We want true zero-copy video encoding on AMD GPUs:
```
PipeWire ScreenCast → DMABuf → vapostproc → VAMemory → vah264enc
```

Wolf claims these pipelines work (from their documentation):

### AMD Pipeline (from Wolf gst-wayland-display README)
```
gst-launch-1.0 waylanddisplaysrc ! \
  'video/x-raw(memory:DMABuf),width=1920,height=1080,framerate=60/1' ! \
  vapostproc ! \
  'video/x-raw(memory:VAMemory),format=NV12' ! \
  vah265enc ! \
  vah265dec ! \
  autovideosink
```

### NVIDIA Pipeline (from Wolf gst-wayland-display README)
```
gst-launch-1.0 waylanddisplaysrc ! \
  'video/x-raw(memory:DMABuf),width=1920,height=1080,framerate=60/1' ! \
  glupload ! \
  glcolorconvert ! \
  'video/x-raw(memory:GLMemory),format=NV12' ! \
  cudadownload ! \
  'video/x-raw(memory:CUDAMemory),format=NV12' ! \
  nvh265enc ! \
  fakesink
```

**Key observation:** AMD pipeline goes directly DMABuf → vapostproc, while NVIDIA
needs glupload/glcolorconvert/cudadownload.

### Our Goal: Match Wolf's AMD Pipeline

```
Wolf:   waylanddisplaysrc   ! video/x-raw(memory:DMABuf) ! vapostproc ! vah264enc
Ours:   pipewirezerocopysrc ! video/x-raw(memory:DMABuf) ! vapostproc ! vah264enc
```

This is the exact pipeline we're trying to achieve. Wolf's works, so ours should too.

**Confirmed:** Wolf's AMD pipeline definitely works - verified with own eyes. It's
zero-copy, high performance, smooth. So the pipeline IS achievable.

**The mystery:** When we inspect vapostproc's pad templates, it does NOT advertise
DMABuf support - yet Wolf's pipeline definitely works.

## Observed Facts

### 1. vapostproc Pad Templates (AMD VM with GPU access)

```
gst-inspect-1.0 vapostproc

SINK template: 'sink'
  Capabilities:
    video/x-raw(memory:VAMemory)  <-- accepts VAMemory
    video/x-raw                    <-- accepts system memory

    NO DMABuf listed!
```

This is the same on both:
- helix-ubuntu (GStreamer 1.26.6)
- Wolf's gstreamer:1.26.7

### 2. vaapipostproc (Legacy Plugin) - Same Story

```
gst-inspect-1.0 vaapipostproc

SINK template: 'sink'
  Capabilities:
    video/x-raw(memory:VASurface)  <-- accepts VASurface
    video/x-raw                     <-- accepts system memory

    NO DMABuf listed!
```

### 3. glupload DOES Accept DMABuf

```
gst-inspect-1.0 glupload

SINK template: 'sink'
  Capabilities:
    video/x-raw(memory:DMABuf)
      format: DMA_DRM             <-- accepts DMABuf with DRM format!
    video/x-raw(memory:GLMemory)
    video/x-raw(memory:SystemMemory)
```

### 4. Our Current pipewirezerocopysrc Output

After recent changes, we output:
```
video/x-raw(memory:DMABuf),format=BGRx  (regular format, not DMA_DRM)
```

Originally we used:
```
video/x-raw(memory:DMABuf),format=DMA_DRM,drm-format=XR24:0x0  (DRM format)
```

## The Mystery

**Why does Wolf's pipeline work if vapostproc doesn't advertise DMABuf support?**

Possible explanations:

### Hypothesis A: GStreamer Auto-Conversion

GStreamer may automatically insert conversion elements when caps don't match.
When DMABuf flows into an element that only accepts system memory, GStreamer
might transparently mmap the DMABuf and pass system memory.

**Test:** Run Wolf's pipeline with GST_DEBUG=4 and look for automatic element insertion.

### Hypothesis B: VA-API Driver DMABuf Import

The VA-API driver on AMD may support DMABuf import even though the GStreamer
element doesn't advertise it in caps. The VASurface backing might be created
from the DMABuf fd internally.

**Test:** Check if vapostproc logs indicate DMABuf import at runtime.

### Hypothesis C: Wolf's waylanddisplaysrc Does Something Special

Wolf's waylanddisplaysrc might output caps that vapostproc accepts, not pure DMABuf.
Maybe it negotiates VAMemory directly with the compositor?

**Test:** Run waylanddisplaysrc in Wolf and inspect its actual output caps at runtime.

### Hypothesis D: Runtime Caps Negotiation vs Static Templates

Static pad templates show what an element *could* accept. At runtime, the actual
caps negotiated might be different. vapostproc might dynamically add DMABuf
support when VA-API driver supports it.

**Test:** Use `gst-launch-1.0 -v` to see actual negotiated caps at runtime.

### Hypothesis E: DMABuf Caps Format Matters

There are two ways to specify DMABuf caps:

1. **DMA_DRM format** (GStreamer 1.24+):
   ```
   video/x-raw(memory:DMABuf),format=DMA_DRM,drm-format=XR24:0x0
   ```
   This is what glupload accepts. Includes DRM fourcc and modifier.

2. **Regular format with DMABuf feature**:
   ```
   video/x-raw(memory:DMABuf),format=BGRx,width=1920,height=1080
   ```
   This is what Wolf's pipeline example uses.

Wolf's example shows regular format (`width=1920,height=1080`) not DMA_DRM.
Maybe vapostproc accepts regular format DMABuf but not DMA_DRM format?

**Test:** Try both caps formats with pipewirezerocopysrc and see which one vapostproc accepts.

## What We've Tried

### Approach 1: DMA_DRM Format with glupload/gldownload

```
pipewirezerocopysrc ! glupload ! gldownload ! vapostproc ! vah264enc
```

**Problem:** gldownload outputs system memory, defeating zero-copy.

### Approach 2: Regular Format with DMABuf Feature

```
pipewirezerocopysrc ! vapostproc ! vah264enc
```

Output: `video/x-raw(memory:DMABuf),format=BGRx`

**Status:** Untested on AMD. vapostproc pad templates don't show DMABuf,
but this might work at runtime (see Hypothesis D).

## Path Forward

### Immediate Test Plan

1. **On AMD VM**, run pipewirezerocopysrc and see what actually happens:
   ```bash
   # Start a session with our plugin
   # Check if vapostproc accepts DMABuf at runtime
   GST_DEBUG=3 gst-launch-1.0 -v \
     pipewirezerocopysrc pipewire-node-id=X output-mode=dmabuf ! \
     vapostproc ! fakesink
   ```

2. **Compare with Wolf** - run their pipeline and capture debug:
   ```bash
   GST_DEBUG=3 gst-launch-1.0 -v \
     waylanddisplaysrc ! vapostproc ! fakesink
   ```

3. **Check VA-API DMABuf support:**
   ```bash
   vainfo --display drm
   # Look for DMABuf import/export capabilities
   ```

### Code State

Current implementation (2026-01-12):

- `pipewiresrc/imp.rs`: Outputs `video/x-raw(memory:DMABuf),format=BGRx`
  (regular format, not DMA_DRM)
- `ws_stream.go`: Direct pipeline `pipewirezerocopysrc ! vapostproc ! vah264enc`
  (no glupload/gldownload)

This might work if Hypothesis A or D is correct. Needs testing.

### Hypothesis F: Wolf's waylanddisplaysrc Outputs VAMemory, Not DMABuf

**DISPROVEN:** Code inspection of Wolf's gst-wayland-display shows waylanddisplaysrc
outputs these formats (from `imp.rs:352-394`):

```rust
// pad_templates():
// 1. DMABuf with DMA_DRM format
let dmabuf_caps = VideoCapsBuilder::new()
    .features([CAPS_FEATURE_MEMORY_DMABUF])
    .format(VideoFormat::DmaDrm)
    .field("drm-format", ...)  // includes fourcc:modifier
    .build();

// 2. CUDA memory (when cuda feature enabled)
let cuda_caps = VideoCapsBuilder::new()
    .features([CAPS_FEATURE_MEMORY_CUDA_MEMORY])
    .format_list([VideoFormat::Bgra, VideoFormat::Rgba])
    .build();

// 3. System memory
let caps = VideoCapsBuilder::new()
    .format(VideoFormat::Rgbx)
    .build();
```

**Wolf does NOT output VAMemory.** It outputs DMABuf with DMA_DRM format.

### Hypothesis G: GStreamer Runtime DMABuf → VA-API Bridge (NEW)

Wolf's pipeline uses `format=DMA_DRM` with drm-format field:
```
waylanddisplaysrc outputs: video/x-raw(memory:DMABuf),format=DMA_DRM,drm-format=AB24:0x...
```

The caps filter in their example forces DMABuf output:
```
video/x-raw(memory:DMABuf),width=1920,height=1080,framerate=60/1
```

But vapostproc's pad template doesn't show DMABuf support. So either:

1. **Runtime caps addition**: vapostproc adds DMABuf to its caps at runtime when VA-API
   driver supports DRM PRIME import (not visible in static templates)

2. **Auto converter insertion**: GStreamer automatically inserts a converter element
   (like vaaimport or glupload) when linking DMABuf → vapostproc

3. **Different GStreamer build**: Wolf's GStreamer might have patches we don't have

**Test:** Run Wolf's pipeline with `GST_DEBUG=*:4` and check:
- Actual negotiated caps at runtime
- Whether any elements are auto-inserted
- What vapostproc's actual sink caps are at runtime

## Test Result: Linking Failure (2026-01-12)

**Error:** `could not link queue to vapostproc`

Our current pipeline with DMABuf output FAILS at link time:
```
pipewirezerocopysrc pipewire-node-id=44 output-mode=dmabuf keepalive-time=100 !
  queue max-size-buffers=3 leaky=downstream !
  vapostproc !  <-- LINKING FAILS HERE
  video/x-raw(memory:VAMemory),format=NV12 !
  vah264enc ...
```

This confirms vapostproc rejects DMABuf caps at link time, not just at runtime.
**Hypothesis F is now the most likely explanation for why Wolf works.**

## Critical Difference Found

**Wolf uses `format=DMA_DRM` caps, we were using regular format caps!**

Wolf's waylanddisplaysrc outputs (from code inspection):
```
video/x-raw(memory:DMABuf),format=DMA_DRM,drm-format=AB24:0x0300000000000013
```

Our pipewirezerocopysrc was outputting (after a mistaken change):
```
video/x-raw(memory:DMABuf),format=BGRx,width=1920,height=1080
```

This is a significant difference. `format=DMA_DRM` includes the modifier in the drm-format
field, which might trigger different caps negotiation behavior in vapostproc.

## Fix Applied (2026-01-12 13:30)

Updated pipewirezerocopysrc to match Wolf's output format:

1. **pad_templates()**: Changed DMABuf caps to use `format=DMA_DRM`
2. **caps()**: Changed DmaBuf mode to use `format=DMA_DRM`
3. **create()**: Build caps with `format=DMA_DRM` and `drm-format` field

Now outputs:
```
video/x-raw(memory:DMABuf),format=DMA_DRM,drm-format=XR24:0x0000000000000000
```

Helper function `drm_format_to_gst_string()` added to format the drm-format field
matching Wolf's `drm_to_gst_format()` pattern.

## Next Steps

### Step 1: Rebuild and Deploy (DONE - 2026-01-12 13:50)

Fix has been applied to pipewirezerocopysrc. Built and deployed to AMD VM:

```bash
# Build completed: helix-ubuntu:b9a370
./stack build-ubuntu

# Deployed to AMD VM (172.201.248.88)
~/deploy-helix-ubuntu-to-amd.sh

# Image verified in sandbox:
# helix-ubuntu:b9a370   b9a370e060d3   5.42GB
```

### Step 2: Wolf Inspection Confirmed "Legacy Pipeline" (2026-01-12 14:15)

Ran Wolf on AMD VM and inspected its behavior:

**Wolf log output:**
```
WARN  | Unable to find any compatible DMA formats for vapostproc, disabling zero copy pipeline.
INFO  | Using legacy pipeline on AMD (/dev/dri/renderD128)
INFO  | Using h264 encoder: va
```

**vapostproc pad templates (GStreamer 1.26.7 on AMD):**
```
SINK template: 'sink'
  Capabilities:
    video/x-raw(memory:VAMemory)    <-- GPU surface memory
    video/x-raw                      <-- system memory

    NO DMABuf support!
```

**waylanddisplaysrc output caps:**
```
SRC template: 'src'
  Capabilities:
    video/x-raw(memory:DMABuf),format=DMA_DRM
    video/x-raw,format=RGBx           <-- Wolf uses this on AMD
    video/x-raw(memory:CUDAMemory)    <-- NVIDIA only
```

**Conclusion:** vapostproc in GStreamer 1.26.x does NOT support DMABuf input.
Wolf's "legacy pipeline" uses system memory → vapostproc (GPU upload) → vah264enc (GPU encode).
This is still fast because encoding happens on GPU; only the initial frame upload goes through CPU.

### Step 3: Fix Applied (2026-01-12 14:13)

Updated `ws_stream.go` to use `output-mode=system` for VA-API encoders:
```go
switch encoder {
case "vaapi", "vaapi-legacy", "vaapi-lp", "qsv":
    outputMode = "system"  // Match Wolf's "legacy pipeline"
...
}
```

Built helix-ubuntu:d6bf49 and deployed to AMD VM.

### Step 4: Verify Fix Works (PENDING)

Need to start helix sandbox on AMD VM (currently Wolf is running) and test:

```bash
# SSH to AMD VM
ssh -i ~/axa-private_key.pem azureuser@172.201.248.88

# Stop Wolf
docker stop wolf WolfPulseAudio Wolf-UI_342532221405053742

# Start helix sandbox (TODO: document actual command)

# Create a new session and test
/tmp/helix spectask benchmark <session-id> --video-mode zerocopy --duration 15
```

Expected behavior:
- Pipeline links successfully (no "could not link queue to vapostproc" error)
- pipewirezerocopysrc outputs system memory
- vapostproc uploads to GPU
- vah264enc encodes on GPU
- Video frames flowing

## Final Architecture: Video Pipeline Matrix

```
╔══════════════════╦═══════════════════════════════╦═══════════════════════════════╗
║                  ║          NVIDIA               ║         AMD/Intel             ║
╠══════════════════╬═══════════════════════════════╬═══════════════════════════════╣
║                  ║                               ║                               ║
║                  ║  ✨ TRUE ZERO-COPY            ║  ⚡ WOLF LEGACY PATH          ║
║                  ║                               ║                               ║
║                  ║  pipewirezerocopysrc          ║  pipewirezerocopysrc          ║
║                  ║    (CUDAMemory output)        ║    (system memory output)     ║
║      GNOME       ║           ↓                   ║           ↓                   ║
║                  ║  cudaupload (no-op)           ║  vapostproc (GPU upload)      ║
║                  ║           ↓                   ║           ↓                   ║
║                  ║  nvh264enc                    ║  vah264enc                    ║
║                  ║                               ║                               ║
║                  ║  No CPU involvement.          ║  One CPU→GPU copy,            ║
║                  ║  max_framerate 0/0 — Mutter   ║  GPU does all processing.     ║
║                  ║  limits to monitor refresh.   ║                               ║
║                  ║                               ║                               ║
╠══════════════════╬═══════════════════════════════╬═══════════════════════════════╣
║                  ║                               ║                               ║
║                  ║  📦 SYSTEM MEMORY PATH        ║  📦 SYSTEM MEMORY PATH        ║
║                  ║                               ║                               ║
║                  ║  pipewirezerocopysrc          ║  pipewirezerocopysrc          ║
║                  ║    (system memory output)     ║    (system memory output)     ║
║      Sway        ║           ↓                   ║           ↓                   ║
║                  ║  cudaupload (GPU upload)      ║  vapostproc (GPU upload)      ║
║                  ║           ↓                   ║           ↓                   ║
║                  ║  nvh264enc                    ║  vah264enc                    ║
║                  ║                               ║                               ║
║                  ║  One CPU→GPU copy,            ║  One CPU→GPU copy,            ║
║                  ║  GPU encoding.                ║  GPU encoding.                ║
║                  ║                               ║                               ║
╚══════════════════╩═══════════════════════════════╩═══════════════════════════════╝
```

**Legend:**
- **pipewirezerocopysrc** — Custom Rust GStreamer plugin for PipeWire ScreenCast
- **max_framerate 0/0** — Workaround for Mutter bug that dropped every other frame
- **Sway paths** — Use system memory; wlr modifiers aren't CUDA-compatible

## References

- Wolf gst-wayland-display: https://github.com/games-on-whales/gst-wayland-display
- Wolf AMD/VAAPI issues: https://github.com/games-on-whales/wolf/issues/103
- GStreamer DMABuf design: https://gstreamer.freedesktop.org/documentation/additional/design/dmabuf.html
- DMABuf modifier negotiation: https://blogs.igalia.com/vjaquez/dmabuf-modifier-negotiation-in-gstreamer/
- libva DRM PRIME2: https://github.com/intel/libva/pull/125
- gstreamer-vaapi EGLImage DMABuf: https://github.com/GStreamer/gstreamer-vaapi/commit/7a3b258

## Test Commands for AMD VM

```bash
# SSH to AMD VM
ssh -i axa-private_key.pem azureuser@172.201.248.88

# Run inside helix-ubuntu container with GPU
sudo docker run --rm -it --device=/dev/dri \
  -v /run/user/1000:/run/user/1000 \
  -e XDG_RUNTIME_DIR=/run/user/1000 \
  --entrypoint bash helix-ubuntu:latest

# Inside container:
export GST_DEBUG=3
export GST_DEBUG_FILE=/tmp/gst.log

# Test vapostproc with DMABuf input (needs actual PipeWire stream)
# This requires a running compositor - not possible headless

# Alternative: Check VA-API capabilities
vainfo --display drm 2>&1 | grep -i dmabuf
```
