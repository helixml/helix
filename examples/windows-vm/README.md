# Windows 11 VM in Helix Spectask Container

Run a Windows 11 VM inside a Helix spectask container using QEMU/KVM.

## Prerequisites

The spectask container needs:
- `/dev/kvm` access (hardware virtualization)
- ~50GB free disk space
- ~16GB RAM available

## Quick Start

```bash
# Run the setup script
./setup.sh

# Start the VM
./start.sh

# Connect via VNC
# Use any VNC client to connect to localhost:5900
```

## What This Does

1. **Downloads** Microsoft's pre-built Windows 11 developer VM (~22GB)
2. **Converts** the Hyper-V VHDX image to QEMU qcow2 format
3. **Configures** TPM 2.0 emulation (required for Windows 11)
4. **Boots** the VM with UEFI firmware and VirtIO drivers

## Scripts

| Script | Description |
|--------|-------------|
| `setup.sh` | Downloads images, installs dependencies, prepares VM |
| `start.sh` | Starts the Windows VM with VNC on port 5900 |
| `stop.sh` | Gracefully shuts down the VM |
| `screenshot.sh` | Takes a screenshot of the VM display |
| `send-keys.sh` | Sends keystrokes to the VM |

## Configuration

Default VM specs (edit `start.sh` to change):

| Resource | Default |
|----------|---------|
| vCPUs | 8 |
| RAM | 16GB |
| VNC Port | 5900 |
| Disk | ~43GB (dynamic) |

## Installing VirtIO Drivers

After first boot, install VirtIO drivers for better performance:

1. Open File Explorer
2. Navigate to `D:\` (VirtIO drivers CD)
3. Run `virtio-win-guest-tools.exe`
4. Accept license and install all drivers

## Files Created

| File | Size | Description |
|------|------|-------------|
| `vm/windows11.qcow2` | ~43GB | Windows 11 disk image |
| `vm/virtio-win.iso` | 753MB | VirtIO drivers |
| `vm/OVMF_VARS.fd` | 540KB | UEFI variable store |
| `vm/tpm/` | - | TPM state directory |

## Troubleshooting

### VM won't start
```bash
# Check KVM access
ls -la /dev/kvm
# Fix permissions if needed
sudo chmod 666 /dev/kvm
```

### No display in VNC
```bash
# Check if VM is running
./screenshot.sh
# View the screenshot
ls -la vm/*.png
```

### Windows 11 TPM error
The setup script configures TPM 2.0 emulation via `swtpm`. If you see TPM errors:
```bash
# Restart TPM emulator
pkill swtpm
./start.sh
```

## GPU Acceleration (Future)

Currently uses software rendering via VNC. For GPU acceleration options:

1. **VirtIO-GPU with VirGL** - OpenGL translation (experimental on Windows)
2. **GPU Passthrough (VFIO)** - Full performance but requires exclusive GPU access

See `start-with-virgl.sh` for experimental VirGL support.

## License

The Windows 11 image is Microsoft's evaluation VM with a 90-day trial license.
Download from: https://developer.microsoft.com/en-us/windows/downloads/virtual-machines/