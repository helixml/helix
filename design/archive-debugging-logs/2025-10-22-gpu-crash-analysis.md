# GPU Crash Analysis - Upstream Merge Impact

## The Symptom
With 13 concurrent Zed external agent containers running:
```
🔥 ZED CRASH DETECTED 🔥
Panic: Unable to init GPU context: Platform(Init(ERROR_INITIALIZATION_FAILED))
Location: crates/gpui/src/platform/linux/wayland/client.rs:501:47
```

## Resource Analysis

### GPU Memory - NOT THE ISSUE
```
GPU Memory: 2130MB / 16380MB = 13% usage
Plenty of VRAM available ✅
```

### System Resources - NOT THE ISSUE
```
File descriptors: 34112 / 9223372036854775807 ✅
Open files limit: 1048576 ✅
Max processes: 2062218 ✅
DRM file descriptors: 20 (very low) ✅
```

## What Changed in Upstream Merge

### Parent Window Support (Commit a70aa4b40a)
The merge added floating/child window support to Wayland:

**Before (working):**
```rust
let (window, surface_id) = WaylandWindow::new(
    handle, globals, &state.gpu_context,
    client_state, params, appearance
)?;
```

**After (merged):**
```rust
let parent = state.keyboard_focused_window.as_ref().map(|w| w.toplevel());
let (window, surface_id) = WaylandWindow::new(
    handle, globals, &state.gpu_context,
    client_state, params, appearance,
    parent  // NEW: Parent window support
)?;
```

## Hypothesis: Vulkan Instance/Context Limits

The error `VK_ERROR_INITIALIZATION_FAILED` typically indicates:

1. **Vulkan Instance Limit**: Driver may limit concurrent VkInstance objects
   - NVIDIA drivers have undocumented limits on concurrent Vulkan apps
   - Typically 64-256 instances depending on driver version

2. **Wayland XDG Surface Limit**: Compositor resource exhaustion
   - Parent-child window relationships create additional protocol objects
   - Multiple Wayland surfaces per application

3. **DRM Render Node Contexts**: Kernel-level GPU context limit
   - Each BladeContext creates VkDevice + VkInstance
   - May hit per-application or system-wide limits

## Why It's Worrying

### Old Code Behavior
- Simple window creation, no parent relationships
- Fewer Wayland protocol objects per instance
- Apparently more resilient to running many instances

### New Code Behavior
- Parent window tracking adds complexity
- May create additional GPU contexts or surfaces
- **Less resilient under high container count**

## Potential Root Causes

### Theory 1: Parent Window State Corruption
With many containers, `keyboard_focused_window` state might get confused, passing invalid parent references that cause GPU init to fail.

### Theory 2: Vulkan Instance Exhaustion
Each Zed creates a VkInstance via BladeContext::new(). With parent windows, there may be more instances created or they're not being properly cleaned up.

### Theory 3: Wayland Protocol Resource Limits
The Wayland compositor (Sway) running in each container has finite resources. Parent-child windows consume more compositor state than flat windows.

## Evidence Against "Just Resource Exhaustion"

1. **GPU VRAM**: 87% free - not the bottleneck
2. **File descriptors**: Barely used
3. **System resources**: All healthy
4. **Only 13 containers**: Not an extreme number
5. **Works fine after cleanup**: Suggests recoverable state, not hard limits

## Recommended Actions

### Immediate Workaround
✅ **Restart Wolf periodically** to clean up old containers
```bash
docker compose -f docker-compose.dev.yaml down wolf && \
docker compose -f docker-compose.dev.yaml up -d wolf
```

### Investigation Needed
1. Compare Vulkan instance count between old/new Zed builds
2. Check if parent window feature can be disabled for headless agents
3. Monitor Wayland protocol traffic to identify resource leaks
4. Test with `WAYLAND_DEBUG=1` to see protocol errors

### Potential Fix
If parent window support is causing issues, we could:
1. Disable parent window passing for headless external agents
2. Add fallback logic in BladeContext::new() to retry without parent
3. Report upstream to Zed about GPU init being less resilient after parent window merge

## Current Status

- ✅ **WebSocket sync**: FIXED and working
- ⚠️  **GPU crashes**: Only occur with 10+ concurrent containers
- ✅ **Workaround**: Restart Wolf to clear old containers
- ❓ **Root cause**: Likely parent window feature interacting with Vulkan limits, needs deeper investigation

The crashes are NOT a WebSocket issue - they're a GPU initialization regression from the upstream merge that manifests under high container load.
