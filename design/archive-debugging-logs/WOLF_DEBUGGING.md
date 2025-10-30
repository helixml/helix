# Wolf Streaming Debugging Guide

## Intermittent Connection Failures

### Problem Description
Moonlight client connections to sessions intermittently fail with errors:
- "Something went wrong on your host PC"
- "Connection terminated"

The issue is **intermittent** - restarting Wolf container fixes it temporarily, suggesting state accumulation.

### Root Causes Identified

#### 1. GPU Memory Exhaustion
```
2025-10-02T06:40:11.752176Z ERROR smithay::backend::renderer::gles: [GL] GL_OUT_OF_MEMORY error generated. Failed to allocate memory for buffer object.
2025-10-02T06:40:11.752200Z ERROR smithay::backend::renderer::gles: [GL] GL_OUT_OF_MEMORY error generated. Failed to allocate memory for buffer object.
```

Wolf's Wayland compositor (Smithay) runs out of GPU memory after multiple session creations/deletions.

#### 2. CUDA Permission Errors
```
40:56:30.661481638     1 0x77fb7c0562e0 WARN             cudacontext gstcudacontext.cpp:402:gst_create_cucontext: CUDA call failed: CUDA_ERROR_NOT_PERMITTED, operation not permitted
40:56:30.661507548     1 0x77fb7c0562e0 WARN             cudacontext gstcudacontext.cpp:403:gst_create_cucontext: Failed to create CUDA context for cuda device 0
40:56:30.661515478     1 0x77fb7c0562e0 ERROR            GST_CONTEXT gstcudautils.cpp:230:gst_cuda_ensure_element_context:<nvh265enc10> Failed to create CUDA context with device-id 0
```

The NVIDIA H.265 encoder can't create CUDA context - likely GPU context leak from previous sessions.

#### 3. Container Sway Compositor Crashes
```
00:00:00.002 [ERROR] [sway/main.c:62] !!! Proprietary Nvidia drivers are in use !!!
00:00:00.200 [ERROR] [wlr] [backend/wayland/backend.c:60] Failed to read from remote Wayland display
(waybar:132): Gtk-WARNING **: 07:40:13.036: cannot open display: :0
```

Sway inside the container can't connect to Wolf's Wayland display, causing immediate container shutdown.

#### 4. EGL Image Creation Failures
```
2025-10-02T06:40:11.784585Z ERROR smithay::backend::egl::ffi: [EGL] 0x300c (BAD_PARAMETER) eglCreateImageKHR: EGL_BAD_PARAMETER error: In eglCreateImageKHR: could not bind to DMA buffer
2025-10-02T06:40:11.784595Z ERROR smithay::backend::egl::display: error=Failed to create `EGLImage` from the buffer
2025-10-02T06:40:11.784607Z ERROR waylanddisplaycore::comp: Rendering failed. err=BindBufferEGLError(EGLImageCreationFailed)
```

DMA buffer binding fails - suggests GPU resource exhaustion or corruption.

### What Restarting Wolf Fixes

When Wolf is restarted, it clears:
- ✅ GPU memory allocations
- ✅ CUDA contexts
- ✅ Broken Wayland compositor state
- ✅ EGL contexts and DMA buffers

This is why the **intermittent** issue goes away after restart but comes back later.

### Wolf Internal State at Failure

**Active Sessions:**
```json
{
  "success": true,
  "sessions": [{
    "app_id": "221569744",
    "client_id": "342532221405053742",
    "client_ip": "139.28.87.179",
    "video_width": 3024,
    "video_height": 1890,
    "video_refresh_rate": 60
  }]
}
```

**Active Apps:** (7 total)
- 1x Desktop (xfce) - static config
- 1x Test ball - static config
- 1x Test Integration - static config
- 1x Personal Dev AA - dynamic PDE
- 4x External Agent sessions - dynamic sessions

### Container Resource Usage (at failure time)
```
NAME           CPU %     MEM USAGE / LIMIT   MEM %
helix-wolf-1   3.67%     410MiB / 503.6GiB   0.08%
```

Wolf container itself has plenty of memory - the issue is GPU memory, not system RAM.

### Wolf Backtrace Files Found
```
-rw-r--r-- 1 root root 148328 Sep 30 12:40 backtrace.2025-09-30-11-40-36.326784363.dump
-rw-r--r-- 1 root root  78288 Sep 30 12:57 backtrace.2025-09-30-11-57-25.727348359.dump
-rw-r--r-- 1 root root  98888 Sep 30 13:11 backtrace.2025-09-30-12-11-38.857465743.dump
-rw-r--r-- 1 root root 148328 Sep 30 13:21 backtrace.2025-09-30-12-21-27.146723740.dump
-rw-r--r-- 1 root root  98888 Sep 30 14:12 backtrace.2025-09-30-13-12-49.922676237.dump
-rw-r--r-- 1 root root 103008 Sep 30 14:34 backtrace.2025-09-30-13-34-38.569728250.dump
-rw-r--r__ 1 root root  98888 Sep 30 14:43 backtrace.2025-09-30-13-43-42.817138220.dump
```

Multiple crash dumps indicate this has been happening repeatedly. Would need to analyze with:
```bash
strings /home/luke/pm/helix/wolf/backtrace.*.dump | head -50
```

### Debugging Commands

#### Check Wolf logs
```bash
docker compose -f docker-compose.dev.yaml logs --tail 150 wolf
```

#### Query Wolf internal state
```bash
# From Wolf container (has curl built-in)
docker compose -f docker-compose.dev.yaml exec -T wolf curl -s --unix-socket /var/run/wolf/wolf.sock http://localhost/api/v1/sessions
docker compose -f docker-compose.dev.yaml exec -T wolf curl -s --unix-socket /var/run/wolf/wolf.sock http://localhost/api/v1/apps

# From API container (now has curl installed)
docker compose -f docker-compose.dev.yaml exec -T api curl -s --unix-socket /var/run/wolf/wolf.sock http://localhost/api/v1/sessions
docker compose -f docker-compose.dev.yaml exec -T api curl -s --unix-socket /var/run/wolf/wolf.sock http://localhost/api/v1/apps
```

#### Check container resource usage
```bash
docker stats helix-wolf-1 --no-stream --format "table {{.Name}}\t{{.CPUPerc}}\t{{.MemUsage}}\t{{.MemPerc}}"
```

#### Check for backtrace dumps
```bash
ls -la /home/luke/pm/helix/wolf/*.dump
strings /home/luke/pm/helix/wolf/backtrace.*.dump | head -50
```

#### Monitor GPU memory (if nvidia-smi available)
```bash
nvidia-smi --query-gpu=memory.used,memory.free,memory.total --format=csv
watch -n 1 nvidia-smi
```

### Temporary Workaround
```bash
docker compose -f docker-compose.dev.yaml restart wolf
```

Restarting Wolf clears GPU state and allows connections to work again temporarily.

### Potential Long-term Fixes

1. **GPU Resource Cleanup**
   - Ensure Wolf properly releases GPU memory when sessions end
   - Add periodic GPU context cleanup
   - Investigate if Smithay compositor has memory leak

2. **CUDA Context Management**
   - Check if CUDA contexts are properly destroyed on session teardown
   - May need to explicitly reset CUDA device between sessions

3. **Wayland Display Lifecycle**
   - Verify Wolf's Wayland display stays alive across sessions
   - Ensure display socket permissions are correct
   - Check if display cleanup happens on session end

4. **Container Configuration**
   - Review GPU device access permissions
   - Check if `DeviceCgroupRules` need adjustment
   - Verify NVIDIA driver volume mounts

5. **Monitoring & Auto-recovery**
   - Add GPU memory monitoring to Wolf
   - Implement automatic Wolf restart when GPU memory is low
   - Add healthcheck that detects broken GPU state

### Investigation Needed

- [ ] Profile GPU memory usage over multiple session create/delete cycles
- [ ] Check if Wolf has configurable GPU memory limits
- [ ] Analyze backtrace dumps for crash patterns
- [ ] Test if issue occurs with fewer concurrent apps
- [ ] Verify if issue happens with XFCE containers vs Sway containers
- [ ] Check Wolf source code for GPU cleanup on session teardown

### Related Files

- `/home/luke/pm/helix/docker-compose.dev.yaml` - Wolf service configuration
- `/home/luke/pm/helix/api/pkg/external-agent/wolf_executor.go` - Wolf app creation logic
- `/home/luke/pm/wolf/` - Wolf source code (for investigating GPU cleanup)
- `/home/luke/pm/helix/wolf/config.toml` - Wolf static configuration
- `/home/luke/pm/helix/wolf/*.dump` - Crash backtrace files

### Session Flow That Triggers Issue

1. Create external agent session (via API)
2. Wolf creates Wayland compositor + GPU context
3. Wolf creates Docker container with Sway
4. Container connects to Wolf's Wayland display
5. Client connects via Moonlight
6. **If GPU memory accumulation:** New sessions fail with GL_OUT_OF_MEMORY
7. **If CUDA context leak:** Encoder fails with CUDA_ERROR_NOT_PERMITTED
8. Wolf detects container crash and stops session
9. Client sees "Connection terminated" or "Something went wrong"

The more sessions created/destroyed, the more likely the failure - classic resource leak pattern.
