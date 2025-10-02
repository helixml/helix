# Wolf GPU Resource Leak - Root Cause Analysis

## Summary
Wolf experiences intermittent connection failures due to **OpenGL context resource exhaustion**, NOT global GPU memory exhaustion. The issue is in the Smithay-based Wayland compositor (waylanddisplaysrc GStreamer plugin).

## Evidence

### GPU Memory is NOT Exhausted
```bash
$ nvidia-smi --query-gpu=memory.used,memory.free,memory.total --format=csv
memory.used [MiB], memory.free [MiB], memory.total [MiB]
2915 MiB, 13041 MiB, 16380 MiB
```
Only 2.9GB / 16GB used - plenty of global memory available.

### Exact Error Messages

**OpenGL Context Resource Limit Hit:**
```
2025-10-02T06:40:11.752176Z ERROR smithay::backend::renderer::gles: 
  [GL] GL_OUT_OF_MEMORY error generated. Failed to allocate memory for buffer object.
```

**EGL Image Creation Failure:**
```
2025-10-02T06:40:11.784585Z ERROR smithay::backend::egl::ffi: 
  [EGL] 0x300c (BAD_PARAMETER) eglCreateImageKHR: EGL_BAD_PARAMETER error: 
  In eglCreateImageKHR: could not bind to DMA buffer

2025-10-02T06:40:11.784595Z ERROR smithay::backend::egl::display: 
  error=Failed to create `EGLImage` from the buffer

2025-10-02T06:40:11.784607Z ERROR waylanddisplaycore::comp: 
  Rendering failed. err=BindBufferEGLError(EGLImageCreationFailed)
```

**CUDA Context Exhaustion:**
```
40:56:30.661481638 WARN cudacontext gstcudacontext.cpp:402:gst_create_cucontext: 
  CUDA call failed: CUDA_ERROR_NOT_PERMITTED, operation not permitted
```
This is misleading - "NOT_PERMITTED" here actually means resource exhaustion, not permissions.

## Root Cause: OpenGL Context Resource Leak

### The Architecture
1. **Wolf** (C++) creates GStreamer pipelines for each session
2. Each pipeline includes **waylanddisplaysrc** (Rust/Smithay compositor plugin)
3. Each compositor creates its own **GlesRenderer** with GL context
4. Each renderer calls `renderer.create_buffer()` to allocate GPU textures/renderbuffers

### The Leak Pattern

**Buffer Creation** (`gst-wayland-display/wayland-display-core/src/utils/allocator/mod.rs:25-37`):
```rust
impl GsGlesbuffer {
    pub fn new(renderer: &mut GlesRenderer, video_info: VideoInfo) -> Option<Self> {
        let format = Fourcc::try_from(video_info.format().to_fourcc()).unwrap_or(Fourcc::Abgr8888);

        let result = renderer.create_buffer(
            format,
            (video_info.width() as i32, video_info.height() as i32).into(),
        );
        match result {
            Ok(buffer) => Some(GsGlesbuffer {
                buffer,  // GlesRenderbuffer from Smithay
                format,
                video_info,
            }),
            Err(_) => None,
        }
    }
}
```

**Buffer Storage** (`gst-wayland-display/wayland-display-core/src/comp/mod.rs:346-360`):
```rust
let allocator = GsGlesbuffer::new(&mut state.renderer, base_info)
    .expect("Failed to create GsGlesbuffer");
state.output_buffer = Some(GsBufferType::RAW(allocator));
```

**The Problem:**
- `state.output_buffer` is `Option<GsBufferType>`
- When replaced with `Some(new_buffer)`, Rust **drops** the old buffer automatically
- `GsGlesbuffer` contains `GlesRenderbuffer` from Smithay
- **No explicit Drop implementation** for `GsGlesbuffer`
- Relies on Smithay's `GlesRenderbuffer` Drop to clean up GL resources
- **If Smithay doesn't properly delete GL textures/renderbuffers**, they leak in the GL context

### OpenGL Context vs Global GPU Memory

**Key Distinction:**
- **Global GPU Memory:** 16GB total, shared across all processes/contexts
- **GL Context Resources:** Each GL context has internal limits on:
  - Number of textures
  - Number of renderbuffers  
  - Number of framebuffer objects
  - Internal driver bookkeeping structures

**GL_OUT_OF_MEMORY means:**
- Hit a **per-context resource limit** (e.g., max textures)
- NOT that the GPU physically ran out of VRAM
- Even with 13GB free, a single context can't allocate more objects

### GStreamer Pipeline Cleanup

**Wolf's Cleanup** (`wolf/src/moonlight-server/streaming/streaming.hpp:105-110`):
```cpp
/* Out of the main loop, clean up nicely */
gst_element_set_state(pipeline.get(), GST_STATE_PAUSED);
gst_element_set_state(pipeline.get(), GST_STATE_READY);
gst_element_set_state(pipeline.get(), GST_STATE_NULL);
```

**The Issue:**
- GStreamer transitions waylanddisplaysrc to NULL state
- This should trigger Rust Drop for the compositor State
- State contains `renderer: GlesRenderer`
- **If GlesRenderer doesn't properly destroy its GL context**, resources leak
- **If individual GlesRenderbuffers aren't deleted before context destruction**, they leak

## Why Restarting Wolf Fixes It

Restarting Wolf container:
1. **Kills all GStreamer pipelines** (process termination)
2. **Destroys all GL contexts** (driver reclaims resources at process exit)
3. **Clears all EGL contexts**
4. **Resets CUDA contexts**
5. **Fresh start** with clean resource slate

This proves the leak is **per-process resource accumulation**, not global GPU state.

## Resource Leak Accumulation Pattern

```
Session 1: Create compositor → GL context → buffers (some resources leaked)
Session 1: Stop → Pipeline NULL → Drop incomplete → GL resources remain

Session 2: Create compositor → GL context → buffers (more leaks)
Session 2: Stop → Pipeline NULL → Drop incomplete → more GL resources remain

Session 3: Create compositor → GL context → buffers
Session 3: **GL_OUT_OF_MEMORY** → Can't allocate more textures in context
Session 3: Container crashes → Client sees "Connection terminated"
```

Each session cycle leaks a small amount of GL context resources until the context limit is hit.

## Potential Fixes

### 1. Fix Smithay GlesRenderer Cleanup
Ensure Smithay properly deletes GL objects before destroying context:
- `glDeleteTextures()`
- `glDeleteRenderbuffers()`
- `glDeleteFramebuffers()`

### 2. Explicit Drop for GsGlesbuffer
Add explicit cleanup in waylanddisplaysrc:
```rust
impl Drop for GsGlesbuffer {
    fn drop(&mut self) {
        // Ensure GL resources are freed
        // This requires access to the GL context
    }
}
```

**Challenge:** GL cleanup requires active GL context, which might not be available during Drop.

### 3. Context Recreation Between Sessions
Force GL context recreation in waylanddisplaysrc:
- Destroy GL context completely when pipeline stops
- Create fresh context for new sessions
- Ensures driver reclaims resources

### 4. Limit Concurrent Sessions
Temporary mitigation:
- Limit max sessions to prevent accumulation
- Monitor GL resource usage
- Auto-restart Wolf when approaching limits

### 5. Use Different GL Contexts Per Session
Instead of reusing contexts, create isolated contexts:
- Each session gets its own GL context
- Destroy context completely when session ends
- Driver handles per-context cleanup

## Investigation Steps

1. **Profile GL resource usage:**
   ```bash
   # Check GL objects in process
   glxinfo -B
   # Monitor driver state
   nvidia-smi dmon
   ```

2. **Add logging to Smithay GlesRenderer Drop:**
   - Verify GL objects are deleted
   - Check glGetError() during cleanup

3. **Test with single session cycles:**
   - Create session → Stop → Create → Stop (repeat 10x)
   - Monitor when GL_OUT_OF_MEMORY appears
   - Determines leak rate

4. **Check Smithay source:**
   - Look for GlesRenderer Drop implementation
   - Verify GL resource cleanup
   - Check if context must be active during Drop

5. **Alternative: Use software rendering:**
   - Test if leak only affects GL path
   - Software path might have better cleanup

## Immediate Workarounds

### Auto-restart Wolf on GPU errors:
```bash
# Monitor Wolf logs for GL_OUT_OF_MEMORY
# Automatically restart when detected
docker compose -f docker-compose.dev.yaml logs -f wolf | \
  grep -q "GL_OUT_OF_MEMORY" && \
  docker compose -f docker-compose.dev.yaml restart wolf
```

### Periodic Wolf restarts:
```bash
# Restart Wolf every hour to prevent accumulation
*/60 * * * * cd /home/luke/pm/helix && docker compose -f docker-compose.dev.yaml restart wolf
```

### Session limit:
- Modify Wolf to limit concurrent compositors
- Queue sessions instead of creating unlimited contexts

## Files to Investigate

**Smithay (external dependency):**
- `GlesRenderer` Drop implementation
- GL resource cleanup code
- Context lifecycle management

**waylanddisplaysrc (games-on-whales):**
- `/home/luke/pm/gst-wayland-display/wayland-display-core/src/utils/allocator/mod.rs` - Buffer allocation
- `/home/luke/pm/gst-wayland-display/wayland-display-core/src/comp/mod.rs` - State/renderer lifecycle
- GStreamer state transition handlers

**Wolf (C++):**
- `/home/luke/pm/wolf/src/moonlight-server/streaming/streaming.hpp` - Pipeline cleanup
- Verify NULL state transition completes before thread exits

## Related Issues

This is likely a known issue in Smithay or the waylanddisplaysrc plugin. Check:
- https://github.com/Smithay/smithay/issues (GL resource leaks)
- https://github.com/games-on-whales/gst-wayland-display/issues
- GStreamer GL plugin cleanup issues

The fix likely belongs upstream in Smithay or waylanddisplaysrc, not in Wolf itself.
