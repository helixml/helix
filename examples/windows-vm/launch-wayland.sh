#!/bin/bash
set -euo pipefail

# Launch Windows 11 VM with GPU-accelerated Wayland display
# This runs on the headless GNOME Shell's Wayland compositor

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
VM_DIR="${SCRIPT_DIR}/vm"

# Ensure we're using Wayland
export GDK_BACKEND=wayland
export QT_QPA_PLATFORM=wayland

# VM Configuration
VM_CPUS="${VM_CPUS:-8}"
VM_RAM="${VM_RAM:-16G}"

# Files
QCOW2_IMAGE="${VM_DIR}/windows11-prebuilt.qcow2"
VIRTIO_ISO="${VM_DIR}/virtio-win.iso"
OVMF_CODE="/usr/share/OVMF/OVMF_CODE_4M.fd"
OVMF_VARS="${VM_DIR}/OVMF_VARS_prebuilt2.fd"
TPM_DIR="${VM_DIR}/tpm"
PID_FILE="${VM_DIR}/qemu-wayland.pid"

log() { echo "[+] $1"; }
error() { echo "[!] $1"; exit 1; }

# Check prerequisites
[[ -f "$QCOW2_IMAGE" ]] || error "Windows image not found: $QCOW2_IMAGE"
[[ -f "$VIRTIO_ISO" ]] || error "VirtIO ISO not found: $VIRTIO_ISO"
[[ -f "$OVMF_VARS" ]] || error "OVMF vars not found: $OVMF_VARS"
[[ -e /dev/kvm ]] || error "/dev/kvm not found"
[[ -n "${WAYLAND_DISPLAY:-}" ]] || error "WAYLAND_DISPLAY not set"

# Kill any existing instance
if [[ -f "$PID_FILE" ]]; then
    pid=$(cat "$PID_FILE")
    kill "$pid" 2>/dev/null || true
    rm -f "$PID_FILE"
fi

# Start TPM emulator
log "Starting TPM emulator..."
mkdir -p "$TPM_DIR"
pkill -f "swtpm.*${TPM_DIR}" 2>/dev/null || true
sleep 1

swtpm socket \
    --tpmstate dir="$TPM_DIR" \
    --ctrl type=unixio,path="${TPM_DIR}/swtpm-sock" \
    --tpm2 \
    --daemon

# Wait for TPM socket
for i in {1..10}; do
    [[ -S "${TPM_DIR}/swtpm-sock" ]] && break
    sleep 0.5
done
[[ -S "${TPM_DIR}/swtpm-sock" ]] || error "TPM socket not created"

log "Starting Windows 11 VM with Wayland display..."
log "  CPUs: $VM_CPUS"
log "  RAM: $VM_RAM"
log "  Display: GTK on Wayland with VirGL"

# Launch QEMU with GTK/Wayland and VirGL
exec qemu-system-x86_64 \
    -enable-kvm \
    -m "$VM_RAM" \
    -smp "$VM_CPUS" \
    -cpu host \
    -machine q35,accel=kvm \
    -drive file="$QCOW2_IMAGE",format=qcow2,if=none,id=hd0 \
    -device ahci,id=ahci \
    -device ide-hd,drive=hd0,bus=ahci.0 \
    -drive if=pflash,format=raw,readonly=on,file="$OVMF_CODE" \
    -drive if=pflash,format=raw,file="$OVMF_VARS" \
    -chardev socket,id=chrtpm,path="${TPM_DIR}/swtpm-sock" \
    -tpmdev emulator,id=tpm0,chardev=chrtpm \
    -device tpm-tis,tpmdev=tpm0 \
    -device virtio-vga-gl \
    -display gtk,gl=on \
    -device virtio-net,netdev=net0 \
    -netdev user,id=net0,hostfwd=tcp::3389-:3389 \
    -device usb-ehci \
    -device usb-tablet \
    -cdrom "$VIRTIO_ISO" \
    -name "Windows 11"
