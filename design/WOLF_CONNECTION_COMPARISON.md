# Wolf Connection: Success vs Failure Comparison

## Successful Connection (After Wolf Restart)

### Timeline
```
06:45:31 - API request to delete/add new external agent app
06:46:02 - Client calls /launch endpoint
06:46:02 - Wolf creates Wayland compositor (wayland-1)
06:46:02 - Wolf creates virtual audio sink (success: ID 3)
06:46:02 - Wayland display ready, listening on: wayland-1
06:46:02 - Container starts successfully
06:46:03 - RTSP negotiation (OPTIONS, DESCRIBE, SETUP x3, ANNOUNCE, PLAY)
06:46:03 - Audio pipeline starts successfully
06:46:03 - Video pipeline starts successfully
06:46:03 - Client connected via ENET
06:46:03+ - Streaming active (IDR frames being forced)
```

### Key Success Indicators

**GPU/CUDA Working:**
```
06:46:03 - nvh265enc starts successfully (no CUDA errors)
06:46:03 - Video pipeline running
06:46:03+ - Forcing IDR frames continuously (encoder working)
```

**Wayland Compositor Healthy:**
```
06:46:02.412 INFO | Wayland display ready, listening on: wayland-1
2025-10-02T06:46:02.416511Z WARN waylanddisplaycore::comp: Error during GetRenderDevice: No GPU device for given DRM node
```
**Note:** The "No GPU device" warning appears in BOTH successful and failed cases - it's not the root cause.

**Container Startup Clean:**
No Sway errors visible in Wolf logs (container starts and stays running).

**Pipeline Creation:**
- Audio pipeline: ✅ Created successfully
- Video pipeline: ✅ Created successfully  
- Both pipelines: ✅ Connected to client

---

## Failed Connection (Before Wolf Restart)

### Timeline
```
06:40:11 - Client calls /launch endpoint
06:40:11 - Wolf attempts to create Wayland compositor (wayland-2)
06:40:11 - **GL_OUT_OF_MEMORY errors** (multiple)
06:40:11 - **EGL image creation failed**
06:40:12 - Container starts
06:40:12 - RTSP negotiation begins
06:40:13 - Audio pipeline starts
06:40:13 - **CUDA_ERROR_NOT_PERMITTED** (multiple)
06:40:13 - **GStreamer pipeline error: Could not initialize supporting library**
06:40:13 - Sway crashes inside container
06:40:13 - Wolf detects crash and stops container
06:40:13 - Client connection terminated
```

### Key Failure Indicators

**GPU Memory Exhaustion:**
```
2025-10-02T06:40:11.752176Z ERROR smithay::backend::renderer::gles: [GL] GL_OUT_OF_MEMORY 
2025-10-02T06:40:11.752200Z ERROR smithay::backend::renderer::gles: [GL] GL_OUT_OF_MEMORY
```

**EGL/DMA Buffer Failure:**
```
2025-10-02T06:40:11.784585Z ERROR smithay::backend::egl::ffi: [EGL] 0x300c (BAD_PARAMETER) 
  eglCreateImageKHR: could not bind to DMA buffer
2025-10-02T06:40:11.784595Z ERROR smithay::backend::egl::display: 
  error=Failed to create `EGLImage` from the buffer
2025-10-02T06:40:11.784607Z ERROR waylanddisplaycore::comp: 
  Rendering failed. err=BindBufferEGLError(EGLImageCreationFailed)
```

**CUDA Context Corruption:**
```
40:56:30.661481638 WARN cudacontext: CUDA call failed: CUDA_ERROR_NOT_PERMITTED
40:56:30.661507548 WARN cudacontext: Failed to create CUDA context for cuda device 0
40:56:30.661515478 ERROR GST_CONTEXT: Failed to create CUDA context with device-id 0
40:56:30.661522498 ERROR nvencoder: failed to create CUDA context
```

**Pipeline Creation Fails:**
```
06:40:13.479059649 ERROR | [GSTREAMER] Pipeline error: Could not initialize supporting library.
```

**Container Crash:**
```
00:00:00.200 [ERROR] [wlr] Failed to read from remote Wayland display
(waybar:132): Gtk-WARNING **: cannot open display: :0
06:40:13.334148116 DEBUG | [DOCKER] Stopping container: /zed-external-01k6hs0zybt5bcbj2c0axg5rkd_16531021386011371178
06:40:13.360004699 INFO | Stopped container
```

---

## Critical Differences

### 1. GPU State
| Aspect | Success | Failure |
|--------|---------|---------|
| GL Memory | ✅ Available | ❌ GL_OUT_OF_MEMORY |
| EGL Images | ✅ Created | ❌ BAD_PARAMETER |
| DMA Buffers | ✅ Bound | ❌ Bind failed |
| CUDA Context | ✅ Created | ❌ NOT_PERMITTED |

### 2. Wayland Display
| Aspect | Success | Failure |
|--------|---------|---------|
| Display Socket | wayland-1 | wayland-2 |
| Compositor State | ✅ Healthy | ❌ Out of memory |
| Container Connection | ✅ Connected | ❌ Failed to read |

### 3. Pipeline Creation
| Aspect | Success | Failure |
|--------|---------|---------|
| Audio Pipeline | ✅ Started | ✅ Started |
| Video Pipeline | ✅ Started | ❌ CUDA error |
| Encoder (nvh265enc) | ✅ Working | ❌ Init failed |

### 4. Container Lifecycle
| Aspect | Success | Failure |
|--------|---------|---------|
| Sway Startup | ✅ Running | ❌ Crashes |
| Container State | ✅ Running | ❌ Stopped |
| Client Connection | ✅ Streaming | ❌ Terminated |

---

## Root Cause Analysis

### GPU Resource Leak Pattern

The failures show a **progressive resource exhaustion pattern**:

1. **First sign:** GL_OUT_OF_MEMORY when creating Wayland compositor
2. **Second sign:** EGL image creation fails (DMA buffer binding)
3. **Third sign:** CUDA context creation denied (permission error is misleading - actually resource exhaustion)
4. **Fourth sign:** Video encoder can't initialize
5. **Final result:** Container crashes, session terminated

### Why Restart Fixes It

Restarting Wolf container:
- ✅ Releases all GPU memory allocations
- ✅ Destroys all EGL contexts
- ✅ Clears CUDA contexts
- ✅ Resets Wayland compositor state
- ✅ Removes orphaned DMA buffers

### Resource Accumulation Timeline

Based on session increments visible in logs:
- **Successful:** wayland-1 (fresh after restart)
- **Failed:** wayland-2 (after multiple session cycles)

This suggests resources leak with each session creation/deletion cycle.

---

## Diagnostic Commands for Detecting Bad State

### Check GPU Memory Usage
```bash
# From host
nvidia-smi --query-gpu=memory.used,memory.free --format=csv

# Look for high memory usage in Wolf process
docker stats helix-wolf-1 --no-stream
```

### Monitor Wayland Display Number
```bash
# Higher numbers = more session cycles = higher leak risk
docker compose -f docker-compose.dev.yaml logs wolf | grep "Wayland display ready"
```

### Check for GL/EGL Errors
```bash
# Presence of these = GPU exhaustion
docker compose -f docker-compose.dev.yaml logs wolf | grep -E "(GL_OUT_OF_MEMORY|EGLImage|BindBufferEGL)"
```

### Check CUDA Context Errors
```bash
# "NOT_PERMITTED" usually means resources exhausted, not actual permission issue
docker compose -f docker-compose.dev.yaml logs wolf | grep "CUDA_ERROR"
```

### Detect Container Crashes
```bash
# Rapid start->stop = Sway crashing due to display issues
docker compose -f docker-compose.dev.yaml logs wolf | grep -E "(Starting container|Stopping container)"
```

---

## Prevention Strategies

### Short-term
1. **Monitor Wolf uptime** - restart periodically
2. **Limit concurrent sessions** - reduce resource pressure
3. **Add healthcheck** - detect bad GPU state automatically

### Long-term
1. **Fix GPU memory leak in Wolf**
   - Audit Smithay compositor cleanup
   - Ensure EGL contexts are destroyed
   - Verify DMA buffers are freed

2. **Fix CUDA context management**
   - Ensure nvh265enc properly releases CUDA on session end
   - Add explicit CUDA device reset between sessions

3. **Add resource monitoring**
   - Track GPU memory per session
   - Auto-restart Wolf when memory high
   - Alert when approaching limits

4. **Improve error handling**
   - Better detection of GPU exhaustion
   - Graceful degradation instead of crash
   - Clear error messages to client
