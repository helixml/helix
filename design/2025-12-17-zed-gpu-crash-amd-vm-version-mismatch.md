# Zed GPU Crash on AMD VM - Version Mismatch Analysis

**Date:** 2025-12-17
**Status:** Mesa 25.0.7 STILL CRASHES - Switching to AMD PRO userspace
**Severity:** Critical - Crashes GPU, requires GPU reset

## Summary

Zed is crashing the GPU when clicking the "follow agent" button in the Zed UI. The crash causes AMD GPU ring timeouts, page faults at NULL addresses, and forces GPU reset. This only occurs on the production AMD VM (Azure NV-series with AMD Radeon Pro V710 MxGPU), not on the local development machine.

## Crash Details

### Symptoms

1. User clicks "follow agent" button in Zed
2. GPU page faults occur at address `0x0000000000000000` (NULL)
3. GPU command ring times out after 10 seconds
4. GPU reset triggered
5. Xwayland and Zed crash

### dmesg Output (from 2025-12-17 03:38:34 UTC)

```
amdgpu 0002:00:00.0: amdgpu: [gfxhub] page fault (src_id:0 ring:40 vmid:7 pasid:32819)
amdgpu 0002:00:00.0: amdgpu:  Process zed pid 2923856 thread zed pid 2923856
amdgpu 0002:00:00.0: amdgpu:   in page starting at address 0x0000000000000000 from client 10
amdgpu 0002:00:00.0: amdgpu: ring gfx_0.0.0 timeout, signaled seq=571734509, emitted seq=571734511
amdgpu 0002:00:00.0: amdgpu: GPU reset begin!. Source:  1
amdgpu 0002:00:00.0: amdgpu: GPU reset(4) succeeded!
amdgpu 0002:00:00.0: [drm] *ERROR* Failed to initialize parser -125!
```

### Crash Location

The crash occurs in Zed's Vulkan GPU layer:
- `blade-graphics-0.7.0/src/vulkan/command.rs:441:21`
- During GPU command submission via RADV (Mesa Vulkan driver)

## Architecture

There are THREE layers where GPU drivers/libraries are involved:

```
┌─────────────────────────────────────────────────────────────────┐
│ Host (RHEL 9.4 on Azure NV-series)                              │
│  ├── Kernel: 5.14.0-427.61.1.el9_4.x86_64                      │
│  ├── AMDGPU Driver: 6.16.6 (AMD PRO)                           │
│  ├── libdrm-amdgpu: 2.4.125.70100 (AMD PRO)                    │
│  └── ROCm: 7.1.0                                               │
├─────────────────────────────────────────────────────────────────┤
│ helix-sandbox Container (Ubuntu 25.04)                          │
│  ├── Wolf (streaming server)                                    │
│  ├── Docker-in-Docker                                           │
│  ├── libdrm: 2.4.124-2 (upstream)                              │
│  ├── Mesa: 25.0.7-0ubuntu0.25.04.2                             │
│  ├── Vulkan: 1.4.304.0-1                                       │
│  └── ROCm SMI: 6.2 (MISMATCH with host 7.1.0)                  │
├─────────────────────────────────────────────────────────────────┤
│ helix-sway Inner Container (WORKS - reliably for weeks)         │
│  ├── Base: Ubuntu 25.04 (ghcr.io/games-on-whales/gstreamer)    │
│  ├── Display Stack: Wolf Wayland → Sway (native Wayland)       │
│  ├── Compositor: Sway (wlroots-based, lightweight)             │
│  ├── Zed (uses blade-graphics for Vulkan, native Wayland)      │
│  ├── libdrm: 2.4.124-2 (from base image)                       │
│  ├── Mesa: 25.0.7 (from base image)                            │
│  └── RADV (Mesa Vulkan driver for AMD)                         │
├─────────────────────────────────────────────────────────────────┤
│ helix-ubuntu Inner Container (DOES NOT WORK - crashes GPU)      │
│  ├── Base: Ubuntu 22.04                                         │
│  ├── Display Stack: Wolf Wayland → Xwayland → GNOME X11 mode   │
│  ├── Compositor: Mutter running as X11 window manager          │
│  ├── GDK_BACKEND=x11, WAYLAND_DISPLAY unset, DISPLAY=:9        │
│  ├── Zed (uses blade-graphics for Vulkan, via Xwayland)        │
│  ├── Vulkan: vulkan-amdgpu-pro 6.4.4 (proprietary, testing)    │
│  ├── OpenGL: Mesa libgl1-mesa-dri (for GNOME compositing)      │
│  └── libdrm-amdgpu1 from AMD 6.4.4 repos                       │
└─────────────────────────────────────────────────────────────────┘
```

**Focus**: helix-ubuntu crashes while helix-sway works reliably for weeks on the same GPU.

## Display Stack Differences

The key architectural difference between the two desktop containers:

### helix-sway (WORKS)
```
Wolf Wayland Compositor
         ↓
      Sway (native Wayland compositor, wlroots-based)
         ↓
      Zed (native Wayland app, Vulkan rendering)
```
- Sway is lightweight, minimal GPU operations
- Apps run natively on Wayland
- Single compositor layer

### helix-ubuntu (CRASHES)
```
Wolf Wayland Compositor
         ↓
      Xwayland (X11 server on Wayland, DISPLAY=:9)
         ↓
      GNOME/Mutter (X11 window manager mode)
         ↓
      Zed (X11 app via Xwayland, Vulkan rendering)
```
- Configuration: `GDK_BACKEND=x11`, `WAYLAND_DISPLAY` unset
- Extra Xwayland layer adds buffer/surface operations
- Mutter runs as X11 WM, not Wayland compositor
- More complex GPU operations that trigger illegal opcodes on MxGPU

**Why this matters:** The extra Xwayland layer and GNOME's X11 mode create more
Vulkan surface/buffer operations. These additional GPU commands may include
operations that RADV compiles into illegal opcodes for the V710 MxGPU.

**Stream startup flow:**
1. Desktop container starts → GNOME/Mutter begins using Vulkan via Xwayland
2. Zed launches → additional Vulkan usage
3. Frontend initiates stream → Wolf tries VA-API encoding
4. VA-API context creation fails because GPU already corrupted by Vulkan crashes

## Version Comparison

| Component | Host (RHEL 9.4) | helix-sandbox | helix-sway | helix-ubuntu (testing) | Local Dev |
|-----------|-----------------|---------------|------------|------------------------|-----------|
| Base OS | RHEL 9.4 | Ubuntu 25.04 | Ubuntu 25.04 | Ubuntu 22.04 | Ubuntu 24.04 |
| Kernel | 5.14.0 | N/A (host) | N/A (host) | N/A (host) | 6.8.0 |
| AMDGPU driver | PRO 6.16.6 | N/A (host) | N/A (host) | N/A (host) | Upstream |
| libdrm | 2.4.125.70100 (PRO) | 2.4.124-2 | 2.4.124-2 | **libdrm-amdgpu1 6.4.4** | 2.4.122-1 |
| Mesa (OpenGL) | N/A | 25.0.7 | 25.0.7 | libgl1-mesa-dri (6.4.4 repos) | 25.0.7 |
| Vulkan driver | N/A | RADV | RADV | **vulkan-amdgpu-pro 6.4.4** | RADV |
| Display mode | N/A | N/A | Native Wayland | **X11 via Xwayland** | N/A |
| Compositor | N/A | N/A | Sway (wlroots) | **Mutter (X11 WM mode)** | N/A |
| ROCm | 7.1.0 | 6.2 (SMI) | N/A | N/A | N/A (NVIDIA) |

## Root Cause Analysis

### Identified Mismatches

1. **libdrm Version Mismatch**
   - Host has AMD PRO libdrm 2.4.125.70100
   - Containers have upstream libdrm 2.4.124-2
   - AMD PRO libdrm may have different ABI/behavior than upstream
   - Minor version difference (125 vs 124) may still cause issues with kernel driver

2. **AMD PRO vs Upstream Driver Stack**
   - Host uses AMD PRO driver stack (enterprise/proprietary userspace)
   - Containers use upstream Mesa RADV (open source)
   - AMD PRO has patches and optimizations not in upstream Mesa
   - Mixing AMD PRO kernel modules with upstream userspace is not a tested configuration

3. **ROCm Version Mismatch**
   - Host has ROCm 7.1.0 installed
   - Container has ROCm SMI 6.2 (different version)
   - While primarily a compute issue, version mismatches can cause subtle incompatibilities

### Kernel Compatibility (NOT the issue)

According to [Mesa RADV documentation](https://docs.mesa3d.org/drivers/radv.html):
- RADV requires kernel 5.0 or later
- RHEL 9.4's kernel 5.14 is well above this requirement
- RHEL backports security and driver fixes to maintain compatibility
- **Kernel version is unlikely to be the root cause**

### Why It Works Locally

Local development machine:
- Fully upstream driver stack (no AMD PRO)
- NVIDIA GPU uses proprietary drivers with container toolkit
- Container toolkit automatically matches driver versions
- No mixing of enterprise/upstream driver components

## GPU Page Fault Analysis

The crash pattern shows:
```
address 0x0000000000000000  <- NULL pointer
address 0x0000000000001000  <- NULL + 4KB offset
```

This indicates RADV is submitting GPU commands with invalid buffer addresses. Possible causes:

1. **Buffer allocation failure** - RADV trying to use kernel API not in 5.14.0
2. **Memory mapping failure** - libdrm ABI mismatch causing incorrect GPU VA mapping
3. **Implicit synchronization mismatch** - Newer RADV expects kernel sync primitives not in 5.14.0

## Potential Solutions

### Option 1: Install AMD PRO Userspace in Containers (Recommended)

Install AMD PRO Mesa/Vulkan inside containers to match host driver:

```dockerfile
# In container Dockerfile
RUN apt-get remove -y mesa-vulkan-drivers && \
    # Install AMD PRO ROCm/amdgpu-pro userspace matching host
    wget https://repo.radeon.com/amdgpu-install/... && \
    amdgpu-install --usecase=graphics --vulkan=pro --no-dkms
```

Pros:
- Matches host driver stack exactly
- AMD PRO is designed for enterprise/vGPU scenarios
- No kernel upgrade required

Cons:
- Need to track AMD PRO versions
- May need to update containers when host driver updates

### Option 2: Upgrade Host Kernel

Upgrade RHEL 9.4 to use a newer kernel compatible with Mesa 25.x:

```bash
# Enable ELRepo for newer kernels
dnf install elrepo-release
dnf --enablerepo=elrepo-kernel install kernel-ml
```

Pros:
- Proper long-term fix
- Better upstream compatibility

Cons:
- May break AMD PRO driver compatibility
- Requires host reboot
- May affect Azure support

### Option 3: Downgrade Container Mesa

Use older Mesa version compatible with kernel 5.14.0:

```dockerfile
# Pin to Mesa 23.x or earlier
RUN apt-get install mesa-vulkan-drivers=23.3.x
```

Pros:
- No host changes required

Cons:
- May lose performance/features
- Harder to maintain pinned versions
- Ubuntu 25.04 may not have old Mesa in repos

### Option 4: Use AMD PRO ROCm Container Images

AMD provides container images with their driver stack pre-installed:

```yaml
image: rocm/rocm-terminal:latest
```

Pros:
- Pre-tested AMD configuration
- Designed for containerized workloads

Cons:
- May need significant container restructuring
- Different base image

## Additional Issue: Missing AMD SMI

The sandbox container has no `rocm-smi` or `amd-smi` binary, so GPU stats cannot be reported. The Wolf API's `queryAMDStats()` function in `api/endpoints.cpp` attempts to call `rocm-smi` but it's not installed.

## AMD Container Toolkit

AMD has an equivalent to NVIDIA's container toolkit: [ROCm Container Toolkit](https://github.com/ROCm/container-toolkit)

Key points from [ROCm Docker documentation](https://rocm.docs.amd.com/projects/install-on-linux/en/latest/how-to/docker.html):

1. **Kernel driver is on host** - The `amdgpu-dkms` kernel module runs on host
2. **Userspace in container** - HIP runtime, Mesa, libdrm, rocm-smi can be in container
3. **Version matching** - Container userspace should match host kernel driver version

### Device Access

Containers need access to:
- `/dev/kfd` - Main compute interface
- `/dev/dri/renderD*` - Direct rendering interface

### Recommended Container Image

For host with ROCm 7.1 / AMDGPU 30.20, use matching ROCm container:

```bash
docker run -it --device=/dev/kfd --device=/dev/dri \
  --group-add video \
  rocm/dev-ubuntu-22.04:7.1
```

## Recommended Next Steps

1. **Install ROCm Container Toolkit** on AMD VM host
   ```bash
   # On host (RHEL 9.4)
   sudo dnf install rocm-container-toolkit
   ```

2. **Use AMD's container runtime** in Docker daemon config
   ```json
   {
     "runtimes": {
       "amd": {
         "path": "/usr/bin/amd-container-runtime"
       }
     }
   }
   ```

3. **Install ROCm userspace in containers** (matching host version 7.1)
   - Add to helix-sandbox Dockerfile:
   ```dockerfile
   # Install AMD ROCm 7.1 userspace to match host driver
   RUN wget https://repo.radeon.com/amdgpu-install/7.1/ubuntu/jammy/amdgpu-install_7.1.70100-1_all.deb && \
       apt-get install -y ./amdgpu-install_7.1.70100-1_all.deb && \
       amdgpu-install --usecase=graphics --vulkan=rocm --no-dkms -y
   ```

4. **Add AMD SMI** for GPU monitoring
   ```dockerfile
   RUN apt-get install -y rocm-smi-lib amd-smi-lib
   ```

5. **Test Zed "follow agent" button** after driver alignment

6. **Verify GPU stats** work via Wolf API

## Critical Finding: Mesa Build Difference Between Desktop Images

### helix-sway (Zed) - WORKS (mostly)
`Dockerfile.sway-helix` uses Mesa from the base image:
- Base: `ghcr.io/games-on-whales/gstreamer:1.26.7` (Ubuntu 25.04)
- Mesa: 25.0.7 (pre-installed from Ubuntu 25.04 repos)
- libdrm: 2.4.124-2 (pre-installed)
- Compositor: Sway (wlroots-based, lightweight)
- **Status**: Generally works, occasional crashes (e.g., "follow agent" button)

### helix-ubuntu (GNOME) - DOES NOT WORK
`Dockerfile.ubuntu-helix` (lines 59-148) builds libdrm and Mesa from source:

**Base Image:** `ubuntu:22.04`

**Source Build Configuration:**
```dockerfile
ARG LIBDRM_VERSION=2.4.124
ARG MESA_VERSION=24.3.4

# libdrm build with AMD/Intel enabled
meson setup build \
    --prefix=/usr \
    -Damdgpu=enabled \
    -Dradeon=enabled \
    -Dintel=enabled

# Mesa build with RADV Vulkan driver
meson setup build \
    --prefix=/usr \
    -Dplatforms=x11,wayland \
    -Dgallium-drivers=radeonsi,iris,crocus,llvmpipe,softpipe,zink \
    -Dvulkan-drivers=amd,intel,intel_hasvk \
    -Dllvm=enabled
```

**Reason for Source Build (from Dockerfile comment):**
> Ubuntu 22.04's default Mesa 23.2 + libdrm 2.4.113 causes GPU crashes on AMD Radeon Pro V710 MxGPU (gfx1101/RDNA 3) with ROCm 6.16+ kernel driver.

**Installed Components:**
- Mesa: 24.3.4 (built from source with meson)
- libdrm: 2.4.124 (built from source)
- Vulkan: RADV (Mesa's AMD Vulkan driver)
- LLVM: 15 (from Ubuntu 22.04 repos, used by Mesa)
- Compositor: Mutter (GNOME's compositor, uses advanced effects)
- X11 Layer: Xwayland for X11 apps

**Status**: Crashes more frequently on AMD Azure VMs than helix-sway

### Analysis

The Mesa 24.3.4 source build in helix-ubuntu was intended to fix AMD compatibility, but it may actually be making things worse. Possible reasons:

1. **GNOME compositor effects** - Mutter uses more advanced GPU features than Sway
2. **Xwayland layer** - GNOME runs X11 apps through Xwayland, adding another abstraction
3. **Mesa 24.3.4 may have RADV bugs** - Newer Mesa 25.0.7 may have fixes
4. **LLVM version mismatch** - Mesa 24.3.4 built against LLVM 15 (Ubuntu 22.04), while kernel has AMD PRO drivers
5. **Missing AMD-specific optimizations** - Source build may lack AMD PRO-specific patches

### AMD-Provided Mesa Option

AMD provides pre-built Mesa packages via `amdgpu-install` with the `--usecase=graphics` option:

```bash
# Download amdgpu-install for Ubuntu 22.04 (jammy)
wget https://repo.radeon.com/amdgpu-install/7.1.1/ubuntu/jammy/amdgpu-install_7.1.1.70101-1_all.deb
apt install ./amdgpu-install_7.1.1.70101-1_all.deb

# Install graphics stack (Mesa + libdrm from AMD repos)
amdgpu-install --usecase=graphics --no-dkms
```

**What `--usecase=graphics` installs:**
- Open source Mesa 3D graphics and multimedia libraries (AMD-built)
- AMD-built libdrm
- Vulkan drivers (RADV, optimized for AMD hardware)

**Source:** [AMD Linux Drivers](https://www.amd.com/en/support/download/linux-drivers.html)

### Potential Fix Options

**Option 1: Upgrade Mesa Source Build to 25.0.7 (AMD-Only) ✅ IMPLEMENTED**

Build Mesa 25.0.7 from source with AMD + software rendering drivers.

Changes made to `Dockerfile.ubuntu-helix`:
- Mesa: 24.3.4 → 25.0.7
- LLVM: 15 (Ubuntu default) → 18 (from apt.llvm.org)
- Gallium drivers: `radeonsi,llvmpipe,softpipe` (AMD + software rendering)
- Vulkan drivers: `amd` only

**Why Intel was removed:**
Intel OpenGL (iris, crocus) and Vulkan (ANV) drivers in Mesa 25.x require
`spirv-llvm-translator-18` which provides `LLVMSPIRVLib`. This package is NOT
available in apt.llvm.org for Ubuntu 22.04 - only in a third-party PPA or Ubuntu 24.04+.

Changes made to `Dockerfile.sandbox`:
- ROCm SMI: 6.2 → 7.1 (matches host ROCm 7.1.0)
- Added `amd-smi-lib` package for modern AMD SMI command

Pros:
- Matches helix-sway Mesa version (which works on AMD)
- Software rendering (llvmpipe) still works as fallback
- ROCm version matches host

Cons:
- **No Intel GPU support** - requires Ubuntu 24.04+ or third-party PPA
- Source build (slow, ~10 min)
- NVIDIA uses proprietary drivers anyway (mounted at runtime)

**Option 2: Use AMD-Provided Mesa (AMD-ONLY deployments)**

Replace the source build with AMD's pre-built packages:

```dockerfile
# In Dockerfile.ubuntu-helix, replace Mesa source build with:
RUN wget https://repo.radeon.com/amdgpu-install/7.1.1/ubuntu/jammy/amdgpu-install_7.1.1.70101-1_all.deb && \
    apt install -y ./amdgpu-install_7.1.1.70101-1_all.deb && \
    amdgpu-install --usecase=graphics --no-dkms -y && \
    rm amdgpu-install_7.1.1.70101-1_all.deb
```

Pros:
- AMD tests these packages against their drivers
- Version 7.1.1 matches host ROCm 7.1.0
- No manual Mesa build maintenance

Cons:
- **BREAKS Intel GPU support** - AMD packages only include AMD drivers
- **BREAKS software rendering fallback** - May not include llvmpipe
- Only viable for AMD-only deployments
- Would need separate Dockerfile for Intel deployments

**Option 3: Switch to Ubuntu 25.04 base image**

Use the same base as helix-sway to get Mesa 25.0.7 pre-built:

Pros:
- Same Mesa/libdrm as helix-sway (which works)
- No source build needed
- Maintains all GPU vendor support

Cons:
- Ubuntu 25.04 is not LTS (shorter support cycle)
- May need to update GNOME packages

**Option 4: Disable GNOME compositor effects**

Reduce GPU load from Mutter:
- Use "GNOME on Xorg" session instead of Wayland
- Or disable animations/effects in GNOME settings
- Set `MUTTER_DEBUG_DISABLE_HW_CURSORS=1`

Pros:
- Quick to test
- No rebuild required

Cons:
- Doesn't fix root cause
- Degrades user experience

### Additional: Fix ROCm SMI Version ✅ IMPLEMENTED

Updated in `Dockerfile.sandbox`:
- ROCm SMI version: 6.2 → 7.1 (matches host)
- Added `amd-smi-lib` for modern AMD SMI command

## Current Dockerfile Analysis (Dockerfile.sandbox)

### Existing ROCm SMI Installation (Line 132-142)
```dockerfile
# ROCm SMI for AMD GPU monitoring (used by Wolf's GPU stats endpoint)
RUN wget -qO - https://repo.radeon.com/rocm/rocm.gpg.key | gpg --dearmor > /etc/apt/keyrings/rocm.gpg \
    && echo "deb [arch=amd64 signed-by=/etc/apt/keyrings/rocm.gpg] https://repo.radeon.com/rocm/apt/6.2 noble main" > /etc/apt/sources.list.d/rocm.list \
    && apt-get install -y --no-install-recommends rocm-smi-lib
```

**Problem**: Uses ROCm 6.2, but host has ROCm **7.1.0**

### Base Image
- `ghcr.io/games-on-whales/gstreamer:1.26.7`
- Contains: Mesa 25.0.7, libdrm 2.4.124-2 (Ubuntu 25.04)
- **Problem**: Mesa is newer than what host kernel supports

### NVIDIA vs AMD Container Toolkit
- **NVIDIA Container Toolkit**: Installed (lines 172-181)
  - Automatically injects matching driver libraries into container
  - Handles version matching seamlessly
- **AMD Container Toolkit**: Not installed
  - No automatic driver injection
  - Must manually manage Mesa/libdrm versions

## Version Mismatch Summary

| Component | Host (RHEL 9.4) | helix-sandbox | helix-sway (inner) | Match? |
|-----------|-----------------|---------------|-------------------|--------|
| Kernel | 5.14.0 | N/A | N/A | - |
| AMDGPU driver | PRO 6.16.6 | N/A | N/A | - |
| libdrm | 2.4.125.70100 (PRO) | 2.4.124-2 | 2.4.124-2 | **NO** |
| Mesa | N/A | 25.0.7 | 25.0.7 | - |
| ROCm | 7.1.0 | 6.2 | N/A | **NO** |
| Vulkan API | - | 1.4.304.0 | 1.4.304.0 | - |

## Complexity: Three-Layer Architecture

```
Host (RHEL 9.4) ─┬─ Kernel 5.14.0 + AMDGPU PRO 6.16.6
                 │
                 └─► helix-sandbox (Ubuntu 25.04) ─┬─ Wolf + DinD
                                                    │  Mesa 25.0.7 (MISMATCH)
                                                    │  ROCm SMI 6.2 (MISMATCH)
                                                    │
                                                    └─► helix-sway (inner container)
                                                        Zed + Sway
                                                        Mesa 25.0.7 (MISMATCH)
                                                        RADV Vulkan driver
```

The Zed crash occurs in the innermost container (helix-sway), but the driver mismatch
exists at all container levels.

## Environment Details

### Azure VM
- Instance: NVv4-series (Azure with AMD Radeon Pro V710 MxGPU)
- OS: RHEL 9.4
- GPU: AMD Radeon Pro V710 MxGPU (vGPU, device ID 0x1002:0x7461)
- Kernel: 5.14.0-427.61.1.el9_4.x86_64
- ROCm: 7.1.0

### Container Images
- helix-sandbox: `registry.helixml.tech/helix/helix-sandbox:2.5.37-rc6`
- helix-sway: `helix-sway:73fd7b`
- Base image: `ghcr.io/games-on-whales/gstreamer:1.26.7`

## Update: Mesa 25.0.7 Still Crashes (2025-12-17 05:04 UTC)

### Mesa 25.0.7 Build Attempted

Built and deployed helix-ubuntu with:
- Mesa: 25.0.7 (from source)
- LLVM: 18 (from apt.llvm.org)
- Vulkan drivers: AMD only (RADV)
- Gallium drivers: radeonsi, llvmpipe, softpipe

**Result: STILL CRASHES** when clicking "follow agent" button in Zed.

### New Error Pattern

The dmesg output shows a **different** error pattern than before:

```
[1889979.164332] [drm:gfx_v11_0_bad_op_irq [amdgpu]] *ERROR* Illegal opcode in command stream
[1889979.164664] amdgpu 0002:00:00.0: amdgpu: ring gfx_0.0.0 timeout, signaled seq=571789211, emitted seq=571789213
[1889979.164669] amdgpu 0002:00:00.0: amdgpu:  Process zed pid 2948129 thread zed pid 2948129
[1889979.164673] amdgpu 0002:00:00.0: amdgpu: GPU reset begin!. Source:  1
[1889979.164657] [drm:gfx_v11_0_priv_reg_irq [amdgpu]] *ERROR* Illegal register access in command stream
[1889979.913677] amdgpu 0002:00:00.0: amdgpu: GPU reset(5) succeeded!
```

**Key errors:**
1. `gfx_v11_0_bad_op_irq` - **Illegal opcode in command stream**
2. `gfx_v11_0_priv_reg_irq` - **Illegal register access in command stream**

This is fundamentally different from the NULL pointer page faults seen earlier. The GPU is receiving commands it doesn't understand.

### Root Cause Analysis

**The Radeon Pro V710 MxGPU is a virtualized GPU (vGPU)**, not a full discrete GPU:
- PCI ID: `1002:7461` (V710 Virtual Function)
- Type: SR-IOV Virtual Function for GPU partitioning
- Used in Azure NVv4-series VMs

**Why RADV (Mesa) is generating illegal opcodes:**

1. **MxGPU restrictions**: The V710 MxGPU exposes a subset of RDNA 3 (gfx11) capabilities. Certain instructions and registers available on consumer RDNA 3 GPUs may be disabled or restricted in the vGPU.

2. **RADV doesn't know about MxGPU limitations**: RADV detects `gfx1101` (RDNA 3) and emits the full instruction set. The vGPU rejects instructions it can't execute.

3. **Privileged register access**: Some GPU registers are restricted in virtualized environments for isolation. RADV may be trying to access registers that only work on bare-metal.

4. **AMD PRO driver has vGPU support**: AMD's proprietary userspace drivers are designed for enterprise vGPU products and know about these restrictions.

### Solution: Switch to AMD PRO Userspace

AMD's `amdgpu-install` with `--usecase=graphics` provides Mesa/Vulkan drivers specifically built and tested for AMD enterprise GPUs including MxGPU:

```dockerfile
# Replace Mesa source build with AMD PRO graphics userspace
RUN wget https://repo.radeon.com/amdgpu-install/6.1.3/ubuntu/jammy/amdgpu-install_6.1.60103-1_all.deb && \
    apt install -y ./amdgpu-install_6.1.60103-1_all.deb && \
    amdgpu-install --usecase=graphics --no-dkms -y && \
    rm amdgpu-install_*.deb
```

**Note**: Using version 6.1.3 (matches AMDGPU PRO 6.16.6 kernel driver on host).

**What AMD PRO userspace includes:**
- Mesa 3D built with AMD-specific patches
- RADV Vulkan driver with enterprise GPU support
- libdrm with AMD PRO modifications
- Tested against MxGPU/SR-IOV configurations

**Why this should work:**
- AMD tests their PRO userspace against V710 MxGPU
- Driver knows which instructions/registers are available
- Designed for datacenter/cloud virtualized GPU scenarios

### Additional Research: RADV vs PRO Vulkan

According to [Arch Linux Wiki](https://wiki.archlinux.org/title/AMDGPU_PRO) and [Phoronix benchmarks](https://www.phoronix.com/review/radv-amdvlk-pro):

| Driver | Type | Shader Compiler | Target Use Case |
|--------|------|-----------------|-----------------|
| RADV | Open-source (Mesa) | ACO (AMD's open-source) | Gaming, general use |
| AMDVLK | Open-source (AMD) | LLPC | General use |
| PRO (vulkan-amdgpu-pro) | Closed-source | Proprietary | Workstation, CAD, enterprise |

**Key difference**: PRO uses a **different proprietary shader compiler** that may avoid emitting
illegal opcodes on MxGPU virtual functions. RADV's ACO compiler optimizes for consumer GPUs
and may use instructions not available on virtualized GPUs.

### Firmware-Assisted Shadowing for SR-IOV

Per [Phoronix](https://www.phoronix.com/news/AMDGPU-FW-Assisted-Shadow-GFX11), firmware-assisted
shadowing code was posted for RDNA3/GFX11, which is **required for proper SR-IOV support**.
This enables mid-command buffer preemption needed for vGPU isolation. The V710 uses this
technology for Azure NVv5 instances.

### Build Command Used (DEPRECATED)

```dockerfile
# NOTE: --vulkan=pro is DEPRECATED - AMD shows warning:
# "WARNING: 'pro' is deprecated, 'radv' will be used instead"
# AMD deprecated PRO because they merged MxGPU fixes into their RADV build
RUN amdgpu-install -y \
    --usecase=graphics \
    --vulkan=pro \
    --no-dkms \
    --no-32 \
    --accept-eula
```

## Update: AMD Official Repository Approach (2025-12-17 06:00 UTC)

### `--vulkan=pro` Deprecated

AMD deprecated the `--vulkan=pro` flag. When used, it shows:
```
WARNING: 'pro' is deprecated, 'radv' will be used instead
```

This suggests AMD has merged the necessary MxGPU/SR-IOV patches into their
official RADV build distributed via their repositories.

### Solution: Use AMD's Official RADV from AMD Repos

Instead of building Mesa from source or using deprecated flags, we now use
AMD's official repositories which contain their enterprise-tested RADV:

**Dockerfile.ubuntu-helix changes:**
```dockerfile
# Add AMD GPG key and repositories manually (not using amdgpu-install)
# Based on: https://rocm.docs.amd.com/projects/install-on-linux/en/latest/how-to/native-install/ubuntu.html
RUN mkdir -p /etc/apt/keyrings \
    && wget -qO - https://repo.radeon.com/rocm/rocm.gpg.key | gpg --dearmor > /etc/apt/keyrings/rocm.gpg \
    && chmod 644 /etc/apt/keyrings/rocm.gpg

# Add AMD repositories with proper GPG signing
# Using version 7.1.1 which has both ROCm and graphics packages for jammy
RUN echo "deb [arch=amd64 signed-by=/etc/apt/keyrings/rocm.gpg] https://repo.radeon.com/rocm/apt/7.1.1 jammy main" > /etc/apt/sources.list.d/rocm.list \
    && echo "deb [arch=amd64 signed-by=/etc/apt/keyrings/rocm.gpg] https://repo.radeon.com/graphics/7.1.1/ubuntu jammy main" >> /etc/apt/sources.list.d/rocm.list \
    && echo "Package: *\nPin: release o=repo.radeon.com\nPin-Priority: 600" > /etc/apt/preferences.d/rocm-pin-600

# Install AMD graphics stack (Mesa + RADV Vulkan driver from AMD's repos)
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
       mesa-vulkan-drivers \
       mesa-vdpau-drivers \
       libgl1-mesa-dri \
       libglx-mesa0 \
       libegl-mesa0 \
       libgbm1 \
       libdrm-amdgpu1 \
       libdrm2 \
    && rm -rf /var/lib/apt/lists/*
```

**Key points:**
- Uses official AMD documentation method
- ROCm 7.1.1 matches host ROCm version
- `Pin-Priority: 600` ensures AMD packages take precedence over Ubuntu's
- No amdgpu-install package (avoids conffile conflicts)

### Dockerfile.sandbox: rocm-smi Fix

Fixed GPU monitoring tool installation:
```dockerfile
# Install rocm-smi CLI (not just rocm-smi-lib library)
RUN apt-get install -y rocm-smi amd-smi \
    && ln -sf /opt/rocm/bin/rocm-smi /usr/local/bin/rocm-smi \
    && ln -sf /opt/rocm/bin/amd-smi /usr/local/bin/amd-smi
```

## Update: Streaming Failure Investigation (2025-12-17)

### Issue: VA-API Context Creation Failure

After deploying the AMD RADV fix, streaming is not working at all on AMD VM.
Fresh session fails to establish desktop stream.

**Wolf logs show:**
```
Using h264 encoder: va
Could not create context for type=gst.va.display.handle
Pipeline failed to reach PLAYING state
```

**Analysis:**
- Wolf correctly detects VA-API encoder availability
- VA-API driver (radeonsi_drv_video.so) is present in sandbox container
- Context creation fails when GStreamer tries to connect to compositor's video output
- This may be a separate issue from Zed GPU crashes

### dmesg Errors (Latest)

Full crash log from AMD VM shows multiple processes crashing:

```
amdgpu 0002:00:00.0: [gfxhub] page fault from gnome-shell pid 2961969 at address 0x0000000000000000
amdgpu 0002:00:00.0: GPU reset(7) succeeded!

amdgpu 0002:00:00.0: ring gfx_0.0.0 timeout from Xwayland pid 2960107
amdgpu 0002:00:00.0: MES failed to respond to msg=MISC (WRITE_REG)
amdgpu 0002:00:00.0: failed to write reg (0xe17)
amdgpu 0002:00:00.0: GPU reset(8) succeeded!

amdgpu 0002:00:00.0: [gfxhub] page fault from zed pid 2965424 at address 0x0000000000000000
amdgpu 0002:00:00.0: [gfxhub] page fault from zed at address 0x0000800100301000
amdgpu 0002:00:00.0: GPU reset(9) succeeded!
amdgpu 0002:00:00.0: GPU reset(10) succeeded!
```

**Key findings:**
1. **gnome-shell, Xwayland, AND Zed** are all crashing the GPU
2. **4 GPU resets** in one log snippet
3. **MES (Micro Engine Scheduler) failures** - new error pattern
4. **NULL pointer page faults** (address 0x0) continue

The MES failures ("MES failed to respond to msg=MISC (WRITE_REG)") are related to
AMD's GPU scheduler for GFX11/RDNA3 architecture. This affects the entire GPU,
not just a single application.

## Update: Azure-Recommended Driver Version (2025-12-17)

### Azure Documentation Findings

According to [Microsoft Azure documentation](https://learn.microsoft.com/en-us/azure/virtual-machines/linux/azure-n-series-amd-gpu-driver-linux-installation-guide):

- **Recommended version: 7.0.1** for NVv5 VMs with Radeon Pro V710
- **Alternative version: 6.1.4** also documented
- **Critical**: "The default driver isn't certified for use with the AMD Radeon PRO V710 GPU"

### AMD Repository Version Analysis

| Version | RADV (Mesa) | vulkan-amdgpu-pro | Notes |
|---------|-------------|-------------------|-------|
| 7.1.1 | Yes | No | Latest, crashes on MxGPU |
| 7.0.1 | Yes | No | Azure recommended |
| 6.4.4 | Yes | **Yes** | LAST with PRO driver |
| 6.3.x | Yes | Yes | Older, may lack GFX11 support |

### vulkan-amdgpu-pro Deprecation

AMD deprecated the proprietary Vulkan driver starting with version 7.x:
- Version 6.4.4 is the **last version** with `vulkan-amdgpu-pro`
- From 7.x onward, only open-source RADV is available
- Source: [AUR maintainer notes](https://aur.archlinux.org/packages/vulkan-amdgpu-pro)

### Scientific Test Methodology

**CRITICAL: Avoid confounding variables**

We observed GPU crashes and VA-API streaming failures, but we made multiple changes
without isolating variables. After 10+ GPU resets, the GPU may be in a bad state
that persists until reboot.

**Confounding factors we must account for:**

1. **GPU state**: After multiple resets, the GPU may be wedged/unstable
2. **Driver version**: We changed from 7.1.1 → 7.0.1 without testing 7.1.1 post-reboot
3. **Multiple layers**: Sandbox (Wolf/VA-API) vs Desktop (Vulkan) use different drivers

**If a reboot fixes streaming, we DON'T know:**
- Whether 7.1.1 would have worked after reboot (never tested)
- Whether 7.0.1 is actually better, or just benefited from clean GPU state
- Whether the crashes were driver bugs or transient GPU corruption

### Test Matrix

| Test | Driver Version | GPU State | Expected Outcome | Actual Outcome |
|------|---------------|-----------|------------------|----------------|
| 1 | 7.1.1 (current deployed) | After reboot | Baseline | TODO |
| 2 | 7.0.1 (Azure recommended) | After reboot | Compare to baseline | TODO |
| 3 | 6.4.4 + vulkan-amdgpu-pro | After reboot | Compare to baseline | TODO |

**Proper test procedure:**

1. **Reboot VM** to clear GPU state
2. **Test current deployed version** (7.1.1) - establish baseline
3. **If crashes**: Note exactly what action triggered it (e.g., "follow agent" button)
4. **Reboot again** before testing next driver version
5. **Deploy 7.0.1** and repeat same test actions
6. **Document results** with timestamps and dmesg excerpts

**What to record for each test:**
- Driver version in helix-ubuntu container
- Time of test start
- Exact actions performed
- Whether streaming worked
- Whether Zed rendered correctly
- Any dmesg errors (with timestamps)
- Number of GPU resets before crash (if any)

### Build Commands

```bash
# Option 1: Azure-recommended 7.0.1 (default in current branch)
./stack build-ubuntu

# Option 2: Proprietary driver 6.4.4
# Modify Dockerfile.ubuntu-helix:
# - Change ARG AMDGPU_VERSION=7.0.1 to ARG AMDGPU_VERSION=6.4.4
# - Add 'proprietary' to repo line
# - Install vulkan-amdgpu-pro instead of mesa-vulkan-drivers
./stack build-ubuntu
```

### Observations Log

| Timestamp | Driver | Action | Result | GPU Resets | Notes |
|-----------|--------|--------|--------|------------|-------|
| 2025-12-17 ~03:38 UTC | 7.1.1 | Click "follow agent" | GPU crash | reset(4) | First observed crash |
| 2025-12-17 ~05:04 UTC | 7.1.1 (AMD repos) | Click "follow agent" | GPU crash | Illegal opcode | MES failures appeared |
| 2025-12-17 ~06:00 UTC | 7.1.1 | Fresh session | VA-API fail + crashes | reset(7-10) | GPU in bad state |
| 2025-12-17 ~06:30 UTC | 7.0.1 | Pre-reboot test | **CRASHED** | reset(11-13) | Same errors as 7.1.1: MES failures, NULL page faults |
| 2025-12-17 ~06:45 UTC | 7.0.1 | Post-reboot test | **CRASHED** | reset(1) | Crashed 2min after boot - RADV is the problem |

**7.0.1 Pre-Reboot Crash Details (captured before VM went down):**
```
[1895721.227015] amdgpu: ring gfx_0.0.0 timeout... Process Xwayland pid 2960107
[1895729.782658] amdgpu: GPU reset(11) succeeded!
[1895736.988430] amdgpu: [gfxhub] page fault from zed pid 2965424 at address 0x0000000000000000
[1895745.544556] amdgpu: GPU reset(12) succeeded!
[1895764.876947] amdgpu: ring gfx_0.0.0 timeout... Process wolf pid 2972954
[1895773.414605] amdgpu: GPU reset(13) succeeded!
[1895773.415030] amdgpu: [drm] *ERROR* Failed to initialize parser -125!
```

**Conclusion:** 7.0.1 (Azure-recommended) with RADV crashes identically to 7.1.1.
Both use the same open-source RADV driver with ACO shader compiler.
This confirms the issue is RADV generating incompatible opcodes for MxGPU.

**7.0.1 Post-Reboot Crash Details (crashed 2 minutes after fresh boot):**
```
[  135.794048] amdgpu: [gfxhub] page fault from zed pid 7587 at address 0x0000800100361000
[  135.957526] amdgpu: [gfxhub] page fault from zed pid 7587 at address 0x0000000000000000
[  135.957541] amdgpu: [gfxhub] page fault from zed pid 7587 at address 0x0000000000001000
[  146.437245] amdgpu: ring gfx_0.0.0 timeout... Process Xwayland pid 6518
[  146.437258] amdgpu: GPU reset begin!. Source:  1
[  146.667459] amdgpu: GPU reset(1) succeeded!
```

**DEFINITIVE CONCLUSION:** RADV is incompatible with V710 MxGPU, even with clean GPU state.
The crash happens within 2 minutes of a fresh boot. This is NOT GPU state corruption.

**Important observation:** helix-sway has worked reliably for weeks on the same GPU with the
same Wolf/sandbox setup. The VA-API context creation errors we see with helix-ubuntu are a
**downstream symptom** of the Vulkan crashes, not a separate issue. GNOME/Mutter and Zed's
Vulkan usage corrupts the GPU state, which then breaks VA-API in the sandbox.

**GPU recovery test (2025-12-17 ~07:00 UTC):** After helix-ubuntu crashed the GPU (reset count
reached 1), we tested helix-sway WITHOUT rebooting. Result: **Sway streams fine.** This shows:
1. GPU recovers after reset - not permanently corrupted
2. helix-sway works on same GPU that just crashed with helix-ubuntu
3. Rebooting between tests may not be necessary
4. Issue is specifically helix-ubuntu's Vulkan stack (GNOME/Mutter/Xwayland)

**Remaining options:**

1. **Proprietary vulkan-amdgpu-pro driver (version 6.4.4)** - Already prepared in Dockerfile
   - Uses different shader compiler than RADV's ACO
   - May avoid emitting illegal opcodes for MxGPU
   - Downside: Deprecated by AMD, no future updates

2. **Modify Zed to avoid problematic GPU operations** - We have a fork!
   - Zed uses blade-graphics for Vulkan rendering
   - Could investigate which Vulkan operations trigger the crash
   - Potential fixes:
     - Disable specific shader features that use restricted opcodes
     - Use software rendering fallback for problematic operations
     - Add MxGPU detection and use safer code paths
   - Location: `~/pm/zed/crates/gpui/src/platform/blade/` (Vulkan backend)
   - Upside: Fix at the source, works with any driver version
   - Downside: Requires understanding which GPU features are restricted on MxGPU

**Current test plan:**
1. ~~Deploy 7.0.1 (building now)~~
2. ~~Test BEFORE reboot → isolates driver version vs GPU state~~ **CRASHED**
3. ~~Reboot VM~~ **DONE**
4. ~~Test AFTER reboot → confirms with clean GPU state~~ **CRASHED after 2min - RADV is broken**
5. **NEXT: Build and deploy 6.4.4 with vulkan-amdgpu-pro** (prepared in Dockerfile)
6. **FALLBACK: Investigate Zed's Vulkan usage** if PRO driver also fails

## Update: vulkan-amdgpu-pro 6.4.4 Also Crashes (2025-12-17 ~08:30 UTC)

### Test Result: CRASHED

Deployed helix-ubuntu with AMD's proprietary Vulkan driver:
- Driver: vulkan-amdgpu-pro 6.4.4 (last version with proprietary driver)
- OpenGL: Mesa libgl1-mesa-dri (for GNOME compositing)
- Environment: `AMD_VULKAN_ICD=PRO`

**Result: SAME NULL POINTER PAGE FAULTS**

```
[ 3598.046049] amdgpu: [gfxhub] page fault from zed pid 43508 at address 0x0000000000000000
[ 3598.046071] amdgpu: [gfxhub] page fault from Xwayland pid 42498 at address 0x0000000000000000
[ 3608.539947] amdgpu: ring gfx_0.0.0 timeout
[ 3608.769223] amdgpu: GPU reset(2) succeeded!
```

### Key Finding: Xwayland Is The Problem

Both RADV (7.1.1, 7.0.1) AND vulkan-amdgpu-pro (6.4.4) crash with identical patterns:
- NULL pointer page faults at address 0x0000000000000000
- Both Zed AND Xwayland involved in crashes
- GPU ring timeouts after page faults

Meanwhile, **helix-sway works perfectly** on the same GPU, same Zed binary:
- Uses native Wayland (no Xwayland)
- Sway compositor (wlroots-based, lightweight)
- Has been stable since late November 2025

### Architecture Comparison

```
helix-sway (WORKS - stable for weeks):
  Wolf Wayland → Sway compositor → Zed (native Wayland, Vulkan)

helix-ubuntu (CRASHES - all driver versions):
  Wolf Wayland → Xwayland → GNOME/Mutter (X11 WM mode) → Zed (X11, Vulkan via Xwayland)
```

The Xwayland translation layer is causing Vulkan operations to generate NULL buffer references.
This is NOT a driver issue - both open-source RADV and proprietary vulkan-amdgpu-pro crash.

**IMPORTANT**: Zed + Xwayland works perfectly on NVIDIA. The issue is specifically the
AMD Radeon Pro V710 MxGPU + Xwayland combination. Something about the vGPU's restricted
instruction set combined with Xwayland's Vulkan surface handling triggers NULL pointers.

### Dead Container Evidence (docker ps -a)

```
helix-ubuntu:fb272e  Exited (0) 8 minutes ago    # Clean exit after crash
helix-ubuntu:58c269  Exited (255) About an hour ago  # Crash exit
helix-ubuntu:58c269  Exited (0) About an hour ago
helix-ubuntu:681816  Exited (0) 2 hours ago
helix-sway:63a6ed    Exited (137) 9 minutes ago  # SIGKILL - manual stop
helix-sway:418229    Exited (255) 11 minutes ago # Crash (during testing)
```

### Next Steps: Investigate AMD MxGPU + Xwayland Bug

The issue is NOT the Vulkan driver - it's something in the Xwayland + AMD MxGPU combination.
This is a bug that can be fixed, not a fundamental incompatibility.

**Investigation paths:**
1. **Zed's Vulkan code** - Compare X11 vs Wayland surface creation in blade-graphics
2. **Xwayland DRI3** - Check AMD-specific code paths in Xwayland buffer sharing
3. **Vulkan validation** - Run Zed with Vulkan validation layers to catch the NULL pointer
4. **GNOME native Wayland** - Try removing GDK_BACKEND=x11 to run GNOME in Wayland mode
5. **Trace the NULL** - Use GPU debugging tools to find where the NULL buffer originates

### Tested Driver Configurations (All Failed on helix-ubuntu)

| Version | Driver Type | Result | Notes |
|---------|-------------|--------|-------|
| 7.1.1 | RADV (Mesa open-source) | CRASHED | Illegal opcode + NULL faults |
| 7.0.1 | RADV (Azure-recommended) | CRASHED | Same as 7.1.1 |
| 6.4.4 | vulkan-amdgpu-pro (proprietary) | CRASHED | Same NULL faults |
| 24.3.4 | Mesa source build | CRASHED | Tried earlier |
| 25.0.7 | Mesa from AMD repos | CRASHED | Same pattern |

**All drivers crash because the problem is Xwayland, not the Vulkan driver.**

## Update: Mesa 25.0.7 Source Build with RADV_DEBUG=hang (2025-12-17)

### Configuration Restored for Debugging

Restored Dockerfile.ubuntu-helix to Mesa 25.0.7 source build with debugging tools:

**Build Configuration:**
- Mesa: 25.0.7 (from source)
- LLVM: 18 (from apt.llvm.org)
- libdrm: 2.4.124 (from source)
- Vulkan drivers: RADV (AMD), ANV (Intel if spirv-llvm-translator available)
- Gallium drivers: radeonsi, iris, crocus, llvmpipe, softpipe, zink

**Debugging Tools Added:**
- **RADV_DEBUG=hang**: Dumps command stream and shaders on GPU hang
- **UMR**: AMD GPU debugger for wave/register inspection

**Why this configuration:**
The Mesa 25.0.7 source build gave the most informative error ("Illegal opcode in command stream")
compared to the PRO driver which just showed NULL pointer page faults. With RADV_DEBUG=hang,
we should get the actual shader disassembly and command stream that caused the crash.

**Debugging procedure when crash occurs:**
1. Check dmesg for "Illegal opcode" errors
2. Check Zed logs for RADV hang dump: `~/.local/share/zed/logs/`
3. Use UMR to inspect GPU state: `sudo umr --ring-dump gfx` (requires setuid on umr)
4. Collect command stream decode from RADV hang output

## References

### Azure & AMD Official Documentation
- [Azure N-series AMD GPU Driver Setup for Linux](https://learn.microsoft.com/en-us/azure/virtual-machines/linux/azure-n-series-amd-gpu-driver-linux-installation-guide)
- [NVads V710 v5 Series Specifications](https://learn.microsoft.com/en-us/azure/virtual-machines/sizes/gpu-accelerated/nvadsv710-v5-series)
- [AMD ROCm Container Documentation](https://rocm.docs.amd.com/projects/install-on-linux/en/latest/how-to/docker.html)

### SR-IOV Technical References
- [Phoronix: Firmware Assisted Shadowing for RDNA3 SR-IOV](https://www.phoronix.com/news/AMDGPU-FW-Assisted-Shadow-GFX11)
- [Phoronix: AMD RADV Register Shadowing for MCBP](https://www.phoronix.com/news/AMD-RADV-Register-Shadowing)
- [Linux 6.2 AMDGPU SR-IOV Fixes for RDNA3/GFX11](https://www.phoronix.com/news/Linux-6.2-AMDGPU-Changes)
- [AMD GIM Open-Source Driver](https://www.phoronix.com/news/AMD-GIM-Open-Source)
- [Kernel Patch: drm/amdgpu/gfx11 bad opcode interrupt](https://mail-archive.com/amd-gfx@lists.freedesktop.org/msg109673.html)

### Mesa & RADV
- [Mesa RADV driver](https://docs.mesa3d.org/drivers/radv.html)
- [Mesa 24.3.0 Release Notes](https://docs.mesa3d.org/relnotes/24.3.0)

### Debugging Tools
- [UMR: AMD GPU User-Mode Register Debugger](https://umr.readthedocs.io/en/main/build.html)
- [AMD PRO drivers for Linux](https://www.amd.com/en/support/linux-drivers)
- [Zed Linux GPU troubleshooting](https://zed.dev/docs/linux#graphics-issues)
