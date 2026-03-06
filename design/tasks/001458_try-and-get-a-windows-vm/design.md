# Design: Windows VM with GPU Acceleration in Helix Spectask Session

## Overview

Run a Windows VM inside the spectask container using QEMU/KVM with GPU-accelerated graphics. The container has `/dev/kvm`, an NVIDIA RTX 2000 Ada (16GB), and VFIO infrastructure available.

## Environment Discovery

| Resource | Available |
|----------|-----------|
| KVM | `/dev/kvm` ✅ |
| GPU | NVIDIA RTX 2000 Ada 16GB |
| VFIO | `/dev/vfio/vfio` ✅ |
| IOMMU Groups | 82 groups, GPU in group 74 |
| GPU binding | Currently `nvidia` driver (host using it) |

## GPU Acceleration Options

### Option 1: VirtIO-GPU with VirGL (Recommended for this session)

**How it works:** QEMU's virtio-gpu device with virglrenderer translates OpenGL calls from guest to host GPU.

**Pros:**
- No GPU passthrough required - host keeps GPU access
- Works with nvidia driver loaded
- Decent 3D acceleration for basic tasks

**Cons:**
- OpenGL only (no DirectX, but DXVK/WineD3D can translate)
- Performance ~30-50% of native
- Windows support is experimental

```bash
qemu-system-x86_64 \
  -enable-kvm \
  -device virtio-vga-gl \
  -display sdl,gl=on
```

### Option 2: GPU Passthrough (VFIO)

**How it works:** Unbind GPU from nvidia driver, bind to vfio-pci, pass entire GPU to VM.

**Pros:**
- Full native GPU performance
- Full DirectX/Vulkan support
- Best gaming/graphics experience

**Cons:**
- **Breaks host GPU access** - no more nvidia-smi, no host CUDA
- Requires unbinding nvidia driver (risky in container)
- GPU exclusive to VM while running

**Not recommended for this session** - would break the spectask's own GPU access.

### Option 3: Looking Glass + IVSHMEM

**How it works:** GPU passthrough but capture framebuffer via shared memory back to host.

**Pros:**
- Near-native GPU performance
- Host can still see VM display

**Cons:**
- Still requires GPU passthrough (same problem as Option 2)
- Complex setup

### Option 4: Software Rendering (Fallback)

**How it works:** QXL/VGA with CPU-based rendering.

**Pros:**
- Always works
- Simple setup

**Cons:**
- No GPU acceleration
- Slow for graphics-heavy tasks

## Architecture (VirtIO-GPU Approach)

```
┌─────────────────────────────────────────────────────────┐
│  Helix Spectask Container (Ubuntu 25.10)                │
│                                                         │
│  ┌─────────────────────────────────────────────────┐    │
│  │  QEMU/KVM                                       │    │
│  │  ┌─────────────────────────────────────────┐    │    │
│  │  │  Windows 11 VM                          │    │    │
│  │  │  - 8 vCPUs, 16GB RAM                    │    │    │
│  │  │  - 80GB qcow2 disk                      │    │    │
│  │  │  - VirtIO-GPU (virgl OpenGL)            │    │    │
│  │  │  - VirtIO disk/net                      │    │    │
│  │  └─────────────────────────────────────────┘    │    │
│  │                                                 │    │
│  │  virtio-vga-gl ──► virglrenderer ──► Host GPU  │    │
│  │  VNC :5900 for remote access                    │    │
│  └─────────────────────────────────────────────────┘    │
│                                                         │
│  NVIDIA RTX 2000 Ada (stays with host nvidia driver)    │
└─────────────────────────────────────────────────────────┘
```

## Implementation Approach

### Phase 1: Install Dependencies
```bash
sudo apt update
sudo apt install -y \
  qemu-system-x86 \
  qemu-utils \
  ovmf \
  libvirglrenderer1 \
  virtinst
```

### Phase 2: Download Images
```bash
# VirtIO drivers for Windows
wget -O virtio-win.iso \
  https://fedorapeople.org/groups/virt/virtio-win/direct-downloads/stable-virtio/virtio-win.iso

# Windows 11 evaluation ISO - manual download from Microsoft
# https://www.microsoft.com/en-us/evalcenter/evaluate-windows-11-enterprise
```

### Phase 3: Create VM Disk
```bash
qemu-img create -f qcow2 /tmp/windows11.qcow2 80G
```

### Phase 4: Install Windows (with VirtIO-GPU)
```bash
qemu-system-x86_64 \
  -enable-kvm \
  -m 16G \
  -smp 8 \
  -cpu host \
  -machine q35 \
  -device virtio-vga-gl \
  -display sdl,gl=on \
  -drive file=/tmp/windows11.qcow2,if=virtio,format=qcow2 \
  -cdrom Win11_Eval.iso \
  -drive file=virtio-win.iso,media=cdrom,index=1 \
  -boot d \
  -bios /usr/share/OVMF/OVMF_CODE.fd \
  -device virtio-net,netdev=net0 \
  -netdev user,id=net0 \
  -usb \
  -device usb-tablet
```

**Note:** During Windows install, load VirtIO storage driver from the virtio-win.iso to detect the disk.

### Phase 5: Run VM (Post-Install)
```bash
qemu-system-x86_64 \
  -enable-kvm \
  -m 16G \
  -smp 8 \
  -cpu host \
  -machine q35 \
  -device virtio-vga-gl \
  -display sdl,gl=on \
  -drive file=/tmp/windows11.qcow2,if=virtio,format=qcow2 \
  -device virtio-net,netdev=net0 \
  -netdev user,id=net0,hostfwd=tcp::3389-:3389 \
  -bios /usr/share/OVMF/OVMF_CODE.fd \
  -usb \
  -device usb-tablet
```

### Phase 6: Install VirtIO GPU Driver in Windows

After Windows boots:
1. Open Device Manager
2. Find "Microsoft Basic Display Adapter"
3. Update driver → Browse → virtio-win.iso → `viogpudo` folder
4. Install the VirtIO GPU DOD driver

## Display Access Options

| Method | Command | Use Case |
|--------|---------|----------|
| SDL (local) | `-display sdl,gl=on` | Best for virgl, needs X11 |
| VNC | `-vnc :0` | Remote access, no GL |
| SPICE | `-spice port=5930,disable-ticketing=on` | Remote + some GL |
| RDP | Port forward 3389 | Native Windows remote |

## Resource Allocation

| Resource | Value | Notes |
|----------|-------|-------|
| vCPUs | 8 | 48 available, leave headroom |
| RAM | 16GB | 257GB available |
| Disk | 80GB qcow2 | Windows needs ~50GB |
| GPU | VirtIO-GPU (virgl) | Shared with host |

## Risks & Mitigations

| Risk | Mitigation |
|------|------------|
| VirGL Windows driver immature | Fall back to QXL if broken |
| Large ISO download | Use wget with resume |
| Windows install slow | Run in tmux/background |
| SDL display needs X11 | Use VNC as fallback |

## Future: Full GPU Passthrough

If exclusive GPU access is acceptable later:
```bash
# Unbind from nvidia
echo 0000:01:00.0 | sudo tee /sys/bus/pci/devices/0000:01:00.0/driver/unbind
# Bind to vfio-pci
echo vfio-pci | sudo tee /sys/bus/pci/devices/0000:01:00.0/driver_override
echo 0000:01:00.0 | sudo tee /sys/bus/pci/drivers/vfio-pci/bind
# Pass to QEMU
qemu-system-x86_64 ... -device vfio-pci,host=01:00.0
```

**Warning:** This would break `nvidia-smi` and any host GPU workloads.