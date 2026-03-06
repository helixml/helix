# Design: Windows VM with GPU Acceleration in Helix Spectask Session

## Overview

Run a Windows VM inside the spectask container using QEMU/KVM with GPU-accelerated graphics. The container has `/dev/kvm`, an NVIDIA RTX 2000 Ada (16GB), and VFIO infrastructure available.

## ✅ COMPLETED - Final Working Configuration

**Status:** Successfully running Windows 11 Enterprise Evaluation (Build 22621) with VirtIO drivers installed.

### Working QEMU Command

```bash
# Start TPM emulator first
mkdir -p /tmp/windows-vm/tpm
swtpm socket --tpmstate dir=/tmp/windows-vm/tpm \
  --ctrl type=unixio,path=/tmp/windows-vm/tpm/swtpm-sock \
  --tpm2 --daemon

# Create OVMF vars copy
cp /usr/share/OVMF/OVMF_VARS_4M.fd /tmp/windows-vm/OVMF_VARS.fd

# Start Windows 11 VM
qemu-system-x86_64 \
  -enable-kvm \
  -m 16G \
  -smp 8 \
  -cpu host \
  -machine q35,accel=kvm \
  -drive file=/tmp/windows-vm/windows11-prebuilt.qcow2,format=qcow2,if=none,id=hd0 \
  -device ahci,id=ahci \
  -device ide-hd,drive=hd0,bus=ahci.0 \
  -drive if=pflash,format=raw,readonly=on,file=/usr/share/OVMF/OVMF_CODE_4M.fd \
  -drive if=pflash,format=raw,file=/tmp/windows-vm/OVMF_VARS.fd \
  -chardev socket,id=chrtpm,path=/tmp/windows-vm/tpm/swtpm-sock \
  -tpmdev emulator,id=tpm0,chardev=chrtpm \
  -device tpm-tis,tpmdev=tpm0 \
  -device virtio-net,netdev=net0 \
  -netdev user,id=net0 \
  -device usb-ehci \
  -device usb-tablet \
  -cdrom /tmp/windows-vm/virtio-win.iso \
  -vnc :0 \
  -daemonize \
  -monitor unix:/tmp/windows-vm/qemu-monitor.sock,server,nowait
```

### Key Configuration Details

| Component | Configuration | Notes |
|-----------|---------------|-------|
| **Base Image** | Microsoft WinDev2407Eval.vhdx → qcow2 | Pre-built dev VM with VS2022 |
| **Firmware** | OVMF UEFI (non-SecureBoot) | OVMF_CODE_4M.fd |
| **TPM** | swtpm software emulator | TPM 2.0 required for Win11 |
| **Storage** | AHCI/IDE-HD | VirtIO caused boot issues with VHDX |
| **Network** | VirtIO-net | User-mode networking |
| **Display** | VNC on :5900 | No GL acceleration yet |
| **Drivers** | virtio-win-0.1.285 | All drivers installed |

### Files Created

| File | Size | Description |
|------|------|-------------|
| `/tmp/windows-vm/windows11-prebuilt.qcow2` | 43GB | Windows 11 disk (with drivers) |
| `/tmp/windows-vm/virtio-win.iso` | 753MB | VirtIO drivers |
| `/tmp/windows-vm/Win11_Eval.iso` | 5.8GB | Windows installer (unused) |
| `/tmp/windows-vm/WinDev2407Eval.HyperV.zip` | 22GB | Original Microsoft VM |

### Installed VirtIO Drivers

All drivers installed via `virtio-win-guest-tools.exe`:
- Balloon (memory management)
- Network (VirtIO-net)
- Pvpanic
- Fwcfg
- Qemupciserial
- Vioinput
- Viorng
- Vioscsi
- Vioserial
- SPICE Guest Agent

---

## Environment Discovery

| Resource | Available |
|----------|-----------|
| KVM | `/dev/kvm` ✅ |
| GPU | NVIDIA RTX 2000 Ada 16GB |
| VFIO | `/dev/vfio/vfio` ✅ |
| IOMMU Groups | 82 groups, GPU in group 74 |
| GPU binding | Currently `nvidia` driver (host using it) |
| CPU | AMD EPYC 7443P 24-Core (48 threads) |
| RAM | ~257GB available |
| Disk | 779GB free |

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

### Option 4: Software Rendering (Fallback) ← Currently Used

**How it works:** Standard VGA/VESA with CPU-based rendering via VNC.

**Pros:**
- Always works
- Simple setup
- Remote access via VNC

**Cons:**
- No GPU acceleration
- Adequate for desktop use, slow for 3D

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│  Helix Spectask Container (Ubuntu 25.10)                │
│                                                         │
│  ┌─────────────────────────────────────────────────┐    │
│  │  QEMU/KVM                                       │    │
│  │  ┌─────────────────────────────────────────┐    │    │
│  │  │  Windows 11 Enterprise Eval             │    │    │
│  │  │  - 8 vCPUs, 16GB RAM                    │    │    │
│  │  │  - 43GB qcow2 disk                      │    │    │
│  │  │  - AHCI storage (SATA emulation)        │    │    │
│  │  │  - VirtIO network                       │    │    │
│  │  │  - TPM 2.0 (swtpm)                      │    │    │
│  │  │  - UEFI firmware (OVMF)                 │    │    │
│  │  └─────────────────────────────────────────┘    │    │
│  │                                                 │    │
│  │  VNC :5900 for remote access                    │    │
│  │  QEMU Monitor socket for control                │    │
│  └─────────────────────────────────────────────────┘    │
│                                                         │
│  NVIDIA RTX 2000 Ada (stays with host nvidia driver)    │
└─────────────────────────────────────────────────────────┘
```

## Implementation Steps Completed

### ✅ Phase 1: Install Dependencies
```bash
sudo apt update
sudo apt install -y \
  qemu-system-x86 \
  qemu-utils \
  ovmf \
  swtpm \
  socat \
  imagemagick
```

### ✅ Phase 2: Download Images
```bash
# VirtIO drivers
wget -O virtio-win.iso \
  https://fedorapeople.org/groups/virt/virtio-win/direct-downloads/stable-virtio/virtio-win.iso

# Microsoft pre-built Windows 11 dev VM
wget "https://aka.ms/windev_VM_hyperv" -O WinDev2407Eval.HyperV.zip
unzip WinDev2407Eval.HyperV.zip
```

### ✅ Phase 3: Convert VHDX to qcow2
```bash
qemu-img convert -f vhdx -O qcow2 WinDev2407Eval.vhdx windows11-prebuilt.qcow2
```

### ✅ Phase 4: Boot Windows with TPM
Key learnings:
- Pre-built Hyper-V images use UEFI, not BIOS
- Need AHCI storage controller (not VirtIO) for pre-built images
- TPM 2.0 emulation via swtpm required for Windows 11
- User networking works without bridge configuration

### ✅ Phase 5: Install VirtIO Drivers
1. Boot Windows
2. Run `D:\virtio-win-guest-tools.exe` from CD-ROM
3. Accept license, install all drivers
4. Reboot (optional)

## Display Access

| Method | Command | Status |
|--------|---------|--------|
| VNC | `-vnc :0` | ✅ Working (port 5900) |
| QEMU Monitor | `socat - UNIX-CONNECT:/tmp/windows-vm/qemu-monitor.sock` | ✅ Working |
| SDL (local) | `-display sdl` | Requires X11 |
| RDP | Port forward 3389 | Available after config |

### Taking Screenshots via Monitor
```bash
echo "screendump /tmp/screenshot.ppm" | socat - UNIX-CONNECT:/tmp/windows-vm/qemu-monitor.sock
convert /tmp/screenshot.ppm /tmp/screenshot.png
```

### Sending Keys via Monitor
```bash
echo "sendkey meta_l-e" | socat - UNIX-CONNECT:/tmp/windows-vm/qemu-monitor.sock  # Win+E
echo "sendkey ret" | socat - UNIX-CONNECT:/tmp/windows-vm/qemu-monitor.sock       # Enter
echo "sendkey ctrl-alt-del" | socat - UNIX-CONNECT:/tmp/windows-vm/qemu-monitor.sock
```

## Resource Allocation

| Resource | Allocated | Available | Notes |
|----------|-----------|-----------|-------|
| vCPUs | 8 | 48 | Leave headroom for host |
| RAM | 16GB | 257GB | Windows 11 minimum 4GB |
| Disk | 43GB actual | 779GB free | Dynamic qcow2 |
| GPU | None (VNC) | RTX 2000 Ada | Future: VirtIO-GPU |

## Known Issues & Solutions

| Issue | Solution |
|-------|----------|
| "Press any key to boot from CD" times out | Spam keys immediately after boot |
| Windows 11 TPM requirement | Use swtpm emulator |
| Pre-built VM won't boot with VirtIO disk | Use AHCI controller |
| OVMF_VARS.fd missing | Use OVMF_VARS_4M.fd |
| sendkey space invalid | Use `sendkey spc` |

## Future Improvements

1. **Enable VirtIO-GPU with VirGL** for 3D acceleration
2. **Add SPICE** for better remote experience
3. **Port forward RDP** (3389) for native Windows remote desktop
4. **Shared folders** via 9p or virtio-fs
5. **Snapshot support** for quick restore

## Risks & Mitigations

| Risk | Mitigation |
|------|------------|
| VirGL Windows driver immature | Fall back to QXL/VNC |
| Large image files | Use qcow2 compression |
| Windows license expires | Microsoft eval VMs have 90-day limit |
| VM state lost on container restart | Save qcow2 to persistent storage |