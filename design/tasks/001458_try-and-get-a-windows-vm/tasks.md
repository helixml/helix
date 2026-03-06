# Implementation Tasks

## ✅ All Tasks Completed

### Phase 1: Setup
- [x] Install QEMU and dependencies (`qemu-system-x86`, `qemu-utils`, `ovmf`, `swtpm`)
- [x] Fix `/dev/kvm` permissions (`chmod 666 /dev/kvm`)
- [x] Download VirtIO drivers ISO from Fedora (753MB)
- [x] Download Windows 11 evaluation ISO from Microsoft (5.8GB)
- [x] Install swtpm for TPM 2.0 emulation

### Phase 2: Image Preparation
- [x] Download Microsoft Hyper-V pre-built VM (22GB zip → 42GB VHDX)
- [x] Extract VHDX from zip archive
- [x] Convert VHDX to qcow2 (`qemu-img convert -f vhdx -O qcow2`)

### Phase 3: VM Configuration
- [x] Configure UEFI firmware (OVMF_CODE_4M.fd + OVMF_VARS_4M.fd)
- [x] Configure TPM 2.0 emulation with swtpm
- [x] Configure AHCI storage controller (required for pre-built images)
- [x] Configure VirtIO networking
- [x] Configure VNC display on port 5900
- [x] Configure QEMU monitor socket

### Phase 4: Boot & Driver Installation
- [x] Boot Windows 11 successfully
- [x] Windows completes OOBE setup automatically
- [x] Install VirtIO guest tools (`virtio-win-guest-tools.exe`)
- [x] Install all VirtIO drivers (balloon, network, scsi, serial, etc.)
- [x] Install SPICE guest agent
- [x] Verify Device Manager shows no driver warnings

### Phase 5: Documentation
- [x] Document final working QEMU command line
- [x] Capture screenshots of boot process and working desktop
- [x] Update design.md with complete configuration
- [x] Verify host GPU still accessible (`nvidia-smi` unaffected)

## Key Learnings

1. **Pre-built VMs save hours** - Microsoft's WinDev2407Eval Hyper-V VM can be converted to qcow2, skipping the entire Windows installation process

2. **AHCI not VirtIO for pre-built images** - Pre-built Hyper-V images expect SATA storage; VirtIO disk won't boot without driver baked in

3. **TPM 2.0 via swtpm** - Windows 11 requires TPM; swtpm emulator provides this without hardware

4. **UEFI is required** - Modern Windows needs OVMF firmware, not SeaBIOS

5. **Monitor socket is powerful** - `screendump` and `sendkey` via monitor socket enables headless automation

6. **VNC works reliably** - No X11/SDL needed; VNC provides reliable remote display

7. **VirtIO drivers via guest tools** - Single `virtio-win-guest-tools.exe` installer handles all drivers + SPICE agent

## Final Working Configuration

```bash
# Start TPM emulator
mkdir -p /tmp/windows-vm/tpm
swtpm socket --tpmstate dir=/tmp/windows-vm/tpm \
  --ctrl type=unixio,path=/tmp/windows-vm/tpm/swtpm-sock \
  --tpm2 --daemon

# Copy OVMF vars (must be writable)
cp /usr/share/OVMF/OVMF_VARS_4M.fd /tmp/windows-vm/OVMF_VARS.fd

# Start Windows VM
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

## Files Created

| File | Size | Description |
|------|------|-------------|
| `/tmp/windows-vm/windows11-prebuilt.qcow2` | 43GB | Working Windows 11 with drivers |
| `/tmp/windows-vm/virtio-win.iso` | 753MB | VirtIO driver CD |
| `/tmp/windows-vm/OVMF_VARS.fd` | 540KB | UEFI variable store |
| `/tmp/windows-vm/tpm/` | - | TPM state directory |

## Result

**Windows 11 Enterprise Evaluation (Build 22621)** running successfully with:
- Full desktop functionality
- Visual Studio 2022 pre-installed
- VSCode pre-installed
- VirtIO drivers for optimal performance
- VNC remote access on port 5900
- QEMU monitor for automation