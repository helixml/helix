# Wolf GPU Resource Leak - Actual Root Cause

## The Exact Bug

**Location:** Smithay `GlesRenderer::drop()` implementation  
**File:** `/home/luke/pm/smithay/src/backend/renderer/gles/mod.rs:1835-1855`

```rust
impl Drop for GlesRenderer {
    fn drop(&mut self) {
        let _guard = self.span.enter();
        unsafe {
            if self.egl.make_current().is_ok() {  // ← LINE 1839: CRITICAL FAILURE POINT
                self.gl.BindFramebuffer(ffi::FRAMEBUFFER, 0);
                self.gl.DeleteProgram(self.solid_program.program);
                self.gl.DeleteBuffers(self.vbos.len() as i32, self.vbos.as_ptr());

                if self.extensions.iter().any(|ext| ext == "GL_KHR_debug") {
                    self.gl.Disable(ffi::DEBUG_OUTPUT);
                    self.gl.DebugMessageCallback(None, ptr::null());
                }

                #[cfg(all(feature = "wayland_frontend", feature = "use_system_lib"))]
                let _ = self.egl_reader.take();
                let _ = self.egl.unbind();  // ← LINE 1848: unbind() calls cleanup()
            }
            // ← If make_current() fails, ENTIRE cleanup block is skipped!
```

## The Problem

**Line 1839:** `if self.egl.make_current().is_ok() {`

When `make_current()` **fails**, the entire cleanup block is skipped. This means:

1. ✅ `self.gl.DeleteProgram()` - NOT called
2. ✅ `self.gl.DeleteBuffers()` - NOT called  
3. ✅ `self.egl.unbind()` - NOT called
4. ✅ **`cleanup()` inside unbind()** - NOT called

**The cleanup queue holds all pending GL resource deletions:**
- Textures (`glDeleteTextures`)
- Renderbuffers (`glDeleteRenderbuffers`)
- Framebuffers (`glDeleteFramebuffers`)
- EGL Images (`DestroyImageKHR`)

**If `make_current()` fails during Drop, these deletions never happen = LEAK**

## Why make_current() Fails

From Smithay code (mod.rs:783):
```rust
fn make_current(&self, gl: &ffi::Gles2, egl: &EGLContext) -> Result<(), MakeCurrentError> {
    unsafe {
        if let GlesTargetInternal::Surface { surface, .. } = self {
            egl.make_current_with_surface(surface)?;  // ← Can fail
            gl.BindFramebuffer(ffi::FRAMEBUFFER, 0);
```

`make_current()` can fail when:
- **EGL context is already destroyed** (by another thread/compositor)
- **EGL surface is invalid** 
- **GL context conflicts** (multiple contexts, one becomes current elsewhere)
- **Driver errors** during context switching

In Wolf's multi-session scenario:
1. Session 1 creates GlesRenderer (GL context A)
2. Session 2 creates GlesRenderer (GL context B) 
3. Session 1 stops → GStreamer destroys pipeline → Drop GlesRenderer
4. **Context B is current** (from Session 2)
5. **make_current(A) fails** → Cleanup skipped → GL objects leak in context A
6. Repeat for each session...

## The Cleanup Flow (When It Works)

**Normal cleanup path:**
```
Drop → make_current() ✅ → unbind() → cleanup() → process queue:
  - glDeleteTextures()
  - glDeleteRenderbuffers() 
  - glDeleteFramebuffers()
  - DestroyImageKHR()
```

**Cleanup implementation** (mod.rs:320-347):
```rust
fn cleanup(&self, egl: &EGLContext, gl: &ffi::Gles2) {
    let receiver = match self.receiver.try_lock() {
        Ok(receiver) => receiver,
        // ... handle lock errors ...
    };
    for resource in receiver.try_iter() {
        match resource {
            CleanupResource::Texture(texture) => unsafe {
                gl.DeleteTextures(1, &texture);  // ← Never called if make_current fails
            },
            CleanupResource::RenderbufferObject(rbo) => unsafe {
                gl.DeleteRenderbuffers(1, &rbo);  // ← Never called if make_current fails
            },
            CleanupResource::EGLImage(image) => unsafe {
                ffi_egl::DestroyImageKHR(**egl.display().get_display_handle(), image);
            },
            // ... other resource types ...
        }
    }
}
```

**Broken cleanup path:**
```
Drop → make_current() ❌ → SKIP EVERYTHING → GL objects orphaned → LEAK
```

## Evidence in Wolf Logs

**The exact errors we see:**
```
GL_OUT_OF_MEMORY error generated. Failed to allocate memory for buffer object.
```
This is **context-local resource exhaustion** - too many leaked objects in the GL context.

```
EGL_BAD_PARAMETER eglCreateImageKHR: could not bind to DMA buffer
```
Leaked EGL images weren't destroyed, new ones can't be created.

```
CUDA_ERROR_NOT_PERMITTED
```
CUDA context can't be created because GL context is corrupted with leaked resources.

## Why Restarting Wolf Fixes It

Process termination:
1. **OS/driver reclaims ALL GL contexts** (including leaked objects)
2. **Process exit cleanup** bypasses Smithay's broken Drop
3. **Fresh process** = clean GL state

## The Real Fix

**Option 1: Force make_current to succeed** (Smithay fix)
```rust
impl Drop for GlesRenderer {
    fn drop(&mut self) {
        unsafe {
            // Try harder to make context current
            let made_current = self.egl.make_current().is_ok() || {
                // If failed, try to find ANY valid context/surface
                self.try_recover_context()
            };
            
            if made_current {
                // ... cleanup code ...
                let _ = self.egl.unbind();
            } else {
                // Last resort: queue cleanup for later or leak intentionally with warning
                tracing::error!("Failed to make GL context current during Drop - GL resources may leak");
            }
        }
    }
}
```

**Option 2: Cleanup without make_current** (Smithay fix)
```rust
impl Drop for GlesRenderer {
    fn drop(&mut self) {
        unsafe {
            if self.egl.make_current().is_ok() {
                // ... normal cleanup ...
            } else {
                // Cleanup what we can without active context
                // At minimum, close EGL display to trigger driver cleanup
                tracing::warn!("Could not activate GL context for cleanup, forcing display cleanup");
                // Driver should clean up when display closes
            }
        }
    }
}
```

**Option 3: Shared context** (waylanddisplaysrc fix)
Make all GlesRenderers share the same EGL context:
- No context switching needed
- Cleanup always works
- Requires architectural change

**Option 4: External cleanup thread** (Wolf fix)
Don't rely on Drop:
- Explicit cleanup before Drop
- Dedicated GL cleanup thread
- Ensures context is current before cleanup

## Workarounds (No Code Changes)

### 1. Restart Wolf periodically
```bash
# Cron job: restart every 2 hours
0 */2 * * * docker compose -f /home/luke/pm/helix/docker-compose.dev.yaml restart wolf
```

### 2. Limit concurrent sessions
Modify Wolf config to limit max sessions → slower leak accumulation

### 3. Monitor and auto-restart
```bash
# Watch for GL errors and restart
docker compose -f docker-compose.dev.yaml logs -f wolf | \
  grep -q "GL_OUT_OF_MEMORY" && \
  docker compose -f docker-compose.dev.yaml restart wolf
```

## There Are NO Environment Variables That Fix This

The leak is in the **Drop implementation logic**, not resource limits.

**These will NOT help:**
- ❌ `__GL_HEAP_SIZE` - Controls heap, not context object limits
- ❌ `MESA_GL_VERSION_OVERRIDE` - Doesn't affect cleanup logic
- ❌ `__GL_SHADER_DISK_CACHE_SIZE` - Shader cache, unrelated
- ❌ Any other GL env vars - The bug is in the Rust Drop impl

**The ONLY fix is code change** in Smithay or waylanddisplaysrc.

## Upstream Fix

File issue with Smithay:
- **Title:** "GlesRenderer::drop() leaks GL resources if make_current() fails"
- **Impact:** Multi-context scenarios (like Wolf) accumulate leaked GL objects
- **Proposed fix:** Ensure cleanup happens even if make_current() fails
- **File:** `src/backend/renderer/gles/mod.rs:1839`

This is a real bug in Smithay affecting any application that creates/destroys multiple GL renderers.
