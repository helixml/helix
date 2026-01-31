# VA-API VAMemory Optimization Opportunity

**Date:** 2026-01-13
**Status:** Deferred (tracking for later)
**Priority:** Medium (affects AMD/Intel encoding performance)

## Problem

On Sway + AMD, video streaming maxes out at ~16 FPS at 1080p. Investigation on Azure VM with AMD Radeon Pro V710 MxGPU revealed the bottleneck.

## Current Pipeline

```
pipewirezerocopysrc (SHM) → queue → vapostproc → video/x-raw,format=NV12 → vah264enc
```

The `video/x-raw,format=NV12` caps (system memory) between vapostproc and vah264enc forces a GPU→CPU→GPU roundtrip:

1. vapostproc outputs to system memory
2. CPU accesses the memory
3. vah264enc re-uploads to GPU for encoding

## Proposed Optimization

Use VAMemory caps to keep data on GPU:

```go
// Current (system memory - forces roundtrip)
"vapostproc",
"video/x-raw,format=NV12",

// Proposed (GPU memory - zero-copy)
"vapostproc",
"video/x-raw(memory:VAMemory),format=NV12",
```

## Evidence

gst-inspect shows both elements support VAMemory:

```
# vapostproc SRC caps
video/x-raw(memory:VAMemory)
    format: { NV12, YV12, I420, ... }
video/x-raw
    format: { NV12, P010_10LE, ... }

# vah264enc SINK caps
video/x-raw(memory:VAMemory)
    format: { NV12, ... }
video/x-raw
    format: { NV12, ... }
```

## Why Deferred

1. Current pipeline matches Wolf's working AMD config exactly
2. Need to verify VAMemory works on all AMD/Intel hardware variants
3. Ubuntu + AMD path needs to work first (more critical)

## Testing Plan

When ready to test:

1. Apply the VAMemory change to `api/pkg/desktop/ws_stream.go`
2. Rebuild desktop images: `./stack build-sway && ./stack build-ubuntu`
3. Rebuild sandbox: `./stack build-sandbox`
4. Start new session and run benchmark:
   ```bash
   /tmp/helix spectask benchmark ses_xxx --duration 30
   ```
5. Compare FPS before/after

## Expected Impact

If VAMemory works, expect FPS to increase from ~16 to ~60 (matching NVIDIA path performance).

## Related Files

- `api/pkg/desktop/ws_stream.go:471-489` - VA-API encoder pipeline
- `design/2026-01-13-pixel-format-flow-analysis.md` - Full format flow documentation
