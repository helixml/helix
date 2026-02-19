# Helix Frame Export for QEMU

Zero-copy video encoding modification for UTM's QEMU fork.

## Overview

This module enables zero-copy H.264 encoding by intercepting virtio-gpu resources and encoding them directly with VideoToolbox, keeping frame data on the GPU throughout the pipeline.

## Architecture

```
Guest (Linux VM):
  PipeWire ScreenCast → DMA-BUF → virtio-gpu resource ID
                                         ↓
                                    vsock message
                                         ↓
Host (QEMU + this module):
  resource ID → virgl_renderer_resource_get_info_ext()
                         ↓
               MTLTexture (native_handle)
                         ↓
               MTLTexture.iosurface → IOSurfaceRef
                         ↓
               CVPixelBufferCreateWithIOSurface() [zero-copy]
                         ↓
               VTCompressionSessionEncodeFrame()
                         ↓
               H.264 NAL units → vsock back to guest
```

## Integration with UTM's QEMU

1. Clone UTM's QEMU fork
2. Copy these files to `hw/display/helix/`
3. Add to `hw/display/meson.build`:
   ```meson
   subdir('helix')
   ```
4. Modify `hw/display/virtio-gpu-virgl.c`:
   ```c
   #include "helix/helix-frame-export.h"

   int virtio_gpu_virgl_init(VirtIOGPU *g) {
       // ... existing init ...

       if (g->conf.helix_frame_export) {
           helix_frame_export_init(g, HELIX_VSOCK_PORT);
       }
   }
   ```
5. Add QEMU option in `hw/virtio/virtio-gpu.c`:
   ```c
   DEFINE_PROP_BOOL("helix-frame-export", VirtIOGPU,
                    conf.helix_frame_export, false),
   ```

## Protocol

Messages are sent over vsock (port 5000 by default):

| Message | Direction | Purpose |
|---------|-----------|---------|
| FRAME_REQUEST | Guest→Host | Encode this virtio-gpu resource |
| FRAME_RESPONSE | Host→Guest | Encoded H.264 NAL units |
| CONFIG_REQ | Guest→Host | Configure encoder (resolution, bitrate) |
| KEYFRAME_REQ | Guest→Host | Force next frame to be keyframe |
| PING/PONG | Both | Keepalive |

## Guest-Side Integration

The guest uses a GStreamer element (`vsockenc`) that:
1. Receives DMA-BUF from PipeWire ScreenCast
2. Extracts virtio-gpu resource ID from DMA-BUF
3. Sends FRAME_REQUEST over vsock
4. Receives FRAME_RESPONSE with H.264 NALs
5. Outputs encoded data to GStreamer pipeline

See `desktop/gst-vsockenc/` for the guest element.

## Building

Requires macOS with:
- Xcode Command Line Tools
- VideoToolbox framework
- virglrenderer (from UTM)

Build as part of QEMU:
```bash
cd qemu-utm
meson setup build --native-file=macos-native.ini
ninja -C build
```

## Testing

1. Start UTM VM with virtio-gpu and vsock enabled
2. Enable helix-frame-export in QEMU args
3. Run guest-side test:
   ```bash
   # In VM
   gst-launch-1.0 pipewiresrc ! vsockenc ! fakesink
   ```
4. Check QEMU logs for encoding output
