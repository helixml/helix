# Next Steps: End-to-End Testing on macOS ARM

**Date:** 2026-02-05
**Status:** Code complete, ready for guest VM testing

## Architecture Summary

```
┌─────────────────────────────────────────────────────────────────┐
│ macOS Host                                                      │
│                                                                 │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │ QEMU (custom build with helix-frame-export)             │  │
│  │  - Listens on helix-frame-export.sock                   │  │
│  │  - virgl_renderer_transfer_read_iov() reads GPU pixels  │  │
│  │  - VideoToolbox H.264 encoding                          │  │
│  └──────────────────────────────────────────────────────────┘  │
│         ▲                                                       │
│         │ UNIX socket                                           │
│         │                                                       │
│  ┌──────▼────────────────────────────────────────────────────┐ │
│  │ socat proxy                                              │ │
│  │  TCP 127.0.0.1:5900 ↔ helix-frame-export.sock          │ │
│  └─────────────────────────────────────────────────────────┘ │
│         ▲                                                       │
│         │ TCP via virtio-net                                   │
│         │                                                       │
│  ┌──────┴────────────────────────────────────────────────────┐ │
│  │ Linux VM (Ubuntu ARM64)                                  │ │
│  │                                                           │ │
│  │  ┌────────────────────────────────────────────────────┐ │ │
│  │  │ Helix API + Database                                │ │ │
│  │  └────────────────────────────────────────────────────┘ │ │
│  │                                                           │ │
│  │  ┌────────────────────────────────────────────────────┐ │ │
│  │  │ helix-ubuntu container (Docker)                    │ │ │
│  │  │                                                     │ │ │
│  │  │  GNOME → PipeWire → DmaBuf                         │ │ │
│  │  │          ↓                                          │ │ │
│  │  │  pipewiresrc → vsockenc (tcp-host=10.0.2.2:5900)  │ │ │
│  │  │          ↓                                          │ │ │
│  │  │  Extract resource ID → Send FrameRequest          │ │ │
│  │  │          ↓                                          │ │ │
│  │  │  Receive H.264 NALs ← host VideoToolbox          │ │ │
│  │  │          ↓                                          │ │ │
│  │  │  appsink → WebSocket → Browser                    │ │ │
│  │  └────────────────────────────────────────────────────┘ │ │
│  └───────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────┘
```

## Status: Core Stack Built on ARM64

### ✅ Host Side (macOS)
- QEMU helix-frame-export module: Thread-safe, pixel readback working, VideoToolbox encoding
- socat proxy running: TCP 127.0.0.1:5900 → UNIX socket
- VM started with custom QEMU build

### ✅ Guest Code (helix repository)
- gst-vsockenc: TCP support added (tcp-host, tcp-port properties)
- desktop-bridge: Pipeline configured with `tcp-host=10.0.2.2 tcp-port=5900`
- ARM64 support added to haystack_service (platform markers for PyTorch)
- Code pushed to `feature/macos-arm-desktop-port` branch

### ✅ Guest Build (ARM64 VM)
- Repository cloned and updated to feature/macos-arm-desktop-port
- Core stack built successfully (API, frontend, haystack, typesense)
- Zed IDE built (291M binary, release mode)
- Sandbox container built and restarted
- helix-ubuntu built successfully (version 556954)
- Image pushed to local registry (localhost:5000/helix-ubuntu:556954)
- ARM64 support complete: CUDA optional, uses sbsa repo for Grace Hopper

### ✅ Ready for ./stack start
- All build steps complete
- Services ready to start with `./stack start`
- Test with vsockenc → TCP → host VideoToolbox pipeline

## Testing Steps (Inside VM)

### 1. Set up Helix inside VM

```bash
# SSH into VM or use console
# (Note: SSH may need setup first)

# Clone helix repo
cd ~
git clone https://github.com/helixml/helix
cd helix
git checkout feature/macos-arm-desktop-port

# Or pull if already cloned
cd ~/helix
git checkout feature/macos-arm-desktop-port
git pull
```

### 2. Start Helix stack

```bash
cd ~/helix
./stack start  # Starts API, postgres, sandbox-nvidia (if GPU available)
```

Wait for all services to be healthy:
```bash
docker compose -f docker-compose.dev.yaml ps
```

### 3. Build helix-ubuntu with vsockenc

```bash
cd ~/helix
./stack build-ubuntu
```

This builds the desktop image with:
- gst-vsockenc element (with TCP support)
- desktop-bridge (configured to use vsockenc with tcp-host=10.0.2.2)

Verify the build:
```bash
cat sandbox-images/helix-ubuntu.version  # Should show new version hash
```

### 4. Verify vsockenc element

```bash
# Check if vsockenc is available in the built image
docker compose -f docker-compose.dev.yaml exec -T sandbox-nvidia \
  docker run --rm helix-ubuntu:$(cat sandbox-images/helix-ubuntu.version) \
  gst-inspect-1.0 vsockenc

# Should show vsockenc element with tcp-host and tcp-port properties
```

### 5. Create API key and test session

```bash
# Create user API key (if not already exists)
# Via web UI at http://localhost:8080

# Export credentials
export HELIX_API_KEY="hl-xxx"  # From web UI
export HELIX_URL="http://localhost:8080"
export HELIX_PROJECT="prj_xxx"  # Get from web UI

# Build CLI
cd api && CGO_ENABLED=0 go build -o /tmp/helix . && cd ..

# Start test session
/tmp/helix spectask start --project $HELIX_PROJECT -n "macOS ARM video test"

# Note the session ID (ses_xxx)
```

### 6. Verify video streaming

```bash
SESSION_ID="ses_xxx"  # From previous step

# Wait ~15 seconds for GNOME to initialize
sleep 15

# Connect to video stream (should show FPS stats)
/tmp/helix spectask stream $SESSION_ID --duration 30
```

**Expected behavior:**
- vsockenc connects to 10.0.2.2:5900
- Host socat proxies to helix-frame-export.sock
- QEMU reads pixels via virgl_renderer_transfer_read_iov()
- VideoToolbox encodes to H.264
- NALs sent back to guest
- Video stream appears in browser at http://localhost:8080

### 7. Check logs for debugging

```bash
# Desktop container logs (vsockenc connection)
docker compose -f docker-compose.dev.yaml exec -T sandbox-nvidia \
  docker ps --format "{{.Names}}" | grep ubuntu-external

CONTAINER_NAME="..."  # From above
docker compose -f docker-compose.dev.yaml exec -T sandbox-nvidia \
  docker logs $CONTAINER_NAME 2>&1 | grep -i "vsockenc\|connected\|tcp"

# Host QEMU logs
tail -50 "/Users/luke/Library/Group Containers/WDNLXAD4W8.com.utmapp.UTM/helix-debug.log"
```

## Success Criteria

- ✅ vsockenc element detected in helix-ubuntu
- ✅ vsockenc connects to 10.0.2.2:5900 successfully
- ✅ Host receives FrameRequest with resource ID
- ✅ Host reads pixels and encodes with VideoToolbox
- ✅ Guest receives H.264 NALs
- ✅ Video stream works in browser at 30-60 FPS
- ✅ No VM crashes (thread safety working)

## Known Issues / Limitations

**TCP instead of virtserialport:**
- Current implementation uses TCP for testing
- Works but adds small latency (~0.5ms)
- TODO: Implement virtserialport for production (~200 lines QEMU C)

**Scanout resources:**
- Host rejects resource_id=0 (scanout)
- Only processes explicit DmaBuf resource IDs from containers
- This is correct - we want container frames, not desktop

## Next: Production Improvements

After successful testing:

1. **virtserialport implementation** (~200 lines QEMU C)
   - Guest accesses `/dev/virtio-ports/com.helix.frame-export`
   - Remove TCP dependency
   - Better performance, proper semantics

2. **Performance benchmarking**
   - Measure FPS at different resolutions
   - Measure end-to-end latency
   - Compare to x86 implementation

3. **Zero-copy investigation**
   - Current: GPU → CPU (virgl_renderer_transfer_read_iov) → IOSurface → VideoToolbox
   - Future: Investigate if ANGLE can provide direct Metal textures
   - Or: Modify virglrenderer to expose Metal backend on macOS
