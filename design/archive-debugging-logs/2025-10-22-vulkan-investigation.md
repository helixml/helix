# Vulkan Instance Limit Investigation

## Reality Check

After investigation, I need to be honest: **I don't have concrete evidence of a hard NVIDIA Vulkan instance limit**.

### What I Know
1. **Error**: `VK_ERROR_INITIALIZATION_FAILED` in `BladeContext::new()`
2. **Symptom**: Crashes with 13 containers, works fine with 1-2
3. **GPU VRAM**: Only 13% used - NOT the bottleneck
4. **System resources**: All healthy

### What I DON'T Know
- ❓ Whether NVIDIA drivers have a hard Vulkan instance limit
- ❓ Exact cause of VK_ERROR_INITIALIZATION_FAILED
- ❓ Whether it's Vulkan, Wayland, or something else
- ❓ If there's a configuration to increase limits

### Alternative Explanations

#### 1. Wayland Display/Surface Exhaustion
Each container runs its own Sway compositor. With parent window support:
- More XDG surfaces created per Zed instance
- More wl_display connections
- Compositor might have resource limits

#### 2. DRM Render Node Context Limit
Each BladeContext creates GPU contexts via /dev/dri/renderD128:
- Currently 16 processes using it (not particularly high)
- But kernel drivers may have per-application limits

#### 3. File Descriptor Leaks
The parent window code might be leaking:
- Wayland protocol file descriptors
- DRM device handles
- VkInstance internal resources

#### 4. Blade Graphics Library Issue
BladeContext is a third-party graphics abstraction:
- May have bugs with concurrent initialization
- Parent window support might trigger different code paths
- Could be a blade-graphics issue, not Vulkan/NVIDIA

### Test to Determine Root Cause

Run this to check actual VkInstance creation:
```bash
# In container with crash
VK_INSTANCE_LAYERS=VK_LAYER_LUNARG_api_dump /zed-build/zed 2>&1 | grep vkCreateInstance
```

This would show if VkInstance creation is actually failing vs. some downstream initialization.

### Practical Recommendation

**Without definitive proof of instance limits**, the safest approach is:

1. **Operational workaround**: Restart Wolf periodically (already works)
2. **Investigate blade-graphics**: May be the actual culprit
3. **Monitor in production**: See if same issue occurs with real usage patterns
4. **Report upstream**: Let Zed team know about reduced resilience after parent window merge

### Bottom Line

I **hypothesized** a Vulkan instance limit but cannot confirm it exists. The crash is real, but the root cause needs deeper investigation with proper Vulkan debugging tools (validation layers, API dumps, etc.).

The WebSocket sync fix is solid and tested. The GPU crash is a **separate upstream regression** that needs investigation beyond my current evidence.
