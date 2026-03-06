# Requirements: Windows VM with GPU Acceleration in Helix Spectask Session

## User Stories

### Primary Story
As a developer in a Helix spectask session, I want to run a Windows VM with GPU-accelerated graphics so that I can test Windows-specific software with reasonable 3D performance.

## Context

The spectask session runs inside a Docker container (helix-ubuntu) on an AMD EPYC server with:
- `/dev/kvm` available (KVM virtualization support)
- NVIDIA RTX 2000 Ada 16GB GPU (currently bound to nvidia driver)
- `/dev/vfio/vfio` available (VFIO infrastructure present)
- 48 CPUs, ~257GB available RAM, 779GB free disk
- Ubuntu 25.10 x86_64 host
- Sudo access for installing packages

## Acceptance Criteria

1. **VM Boots Successfully**
   - Windows 11 VM starts and reaches desktop
   - KVM acceleration is used (not pure emulation)

2. **GPU Acceleration Working**
   - VirtIO-GPU with virglrenderer provides OpenGL acceleration
   - Windows Device Manager shows VirtIO GPU (not "Basic Display Adapter")
   - Basic 3D applications run with acceptable performance

3. **Display Access**
   - Can view and interact with VM via SDL, VNC, or RDP
   - Mouse and keyboard input works correctly

4. **Resource Constraints**
   - VM uses 8 vCPUs, 16GB RAM, 80GB disk
   - Does not crash the host container
   - Host retains GPU access (nvidia-smi still works)

## Out of Scope

- Full GPU passthrough (would break host GPU access)
- Windows activation/licensing (use evaluation/trial)
- Persistent VM state across spectask session restarts
- Integration with Helix video streaming pipeline
- DirectX 12 / Vulkan (virgl only supports OpenGL currently)