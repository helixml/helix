#!/bin/bash
set -euo pipefail

# Start Windows 11 VM with QEMU/KVM
# Requires setup.sh to be run first

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
VM_DIR="${SCRIPT_DIR}/vm"

# VM Configuration
VM_CPUS="${VM_CPUS:-8}"
VM_RAM="${VM_RAM:-16G}"
VNC_DISPLAY="${VNC_DISPLAY:-0}"  # VNC port = 5900 + display number

# Files
QCOW2_IMAGE="${VM_DIR}/windows11.qcow2"
VIRTIO_ISO="${VM_DIR}/virtio-win.iso"
OVMF_VARS="${VM_DIR}/OVMF_VARS.fd"
TPM_DIR="${VM_DIR}/tpm"
MONITOR_SOCK="${VM_DIR}/qemu-monitor.sock"
PID_FILE="${VM_DIR}/qemu.pid"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log() { echo -e "${GREEN}[+]${NC} $1"; }
warn() { echo -e "${YELLOW}[!]${NC} $1"; }
error() { echo -e "${RED}[!]${NC} $1"; exit 1; }

# Find OVMF code file
find_ovmf_code() {
    for path in \
        /usr/share/OVMF/OVMF_CODE_4M.fd \
        /usr/share/OVMF/OVMF_CODE.fd \
        /usr/share/edk2/ovmf/OVMF_CODE.fd \
        /usr/share/qemu/OVMF_CODE.fd; do
        if [[ -f "$path" ]]; then
            echo "$path"
            return
        fi
    done
    error "OVMF_CODE not found. Install ovmf package."
}

# Check if VM is already running
check_running() {
    if [[ -f "$PID_FILE" ]]; then
        local pid
        pid=$(cat "$PID_FILE")
        if kill -0 "$pid" 2>/dev/null; then
            error "VM already running (PID $pid). Use ./stop.sh first."
        fi
        rm -f "$PID_FILE"
    fi
}

# Start TPM emulator
start_tpm() {
    log "Starting TPM 2.0 emulator..."

    # Kill any existing swtpm for this VM
    pkill -f "swtpm.*${TPM_DIR}" 2>/dev/null || true
    sleep 1

    # Create TPM directory
    mkdir -p "$TPM_DIR"

    # Start swtpm
    swtpm socket \
        --tpmstate dir="$TPM_DIR" \
        --ctrl type=unixio,path="${TPM_DIR}/swtpm-sock" \
        --tpm2 \
        --daemon

    # Wait for socket
    for _ in {1..10}; do
        if [[ -S "${TPM_DIR}/swtpm-sock" ]]; then
            log "TPM emulator started"
            return
        fi
        sleep 0.5
    done

    error "TPM emulator failed to start"
}

# Start QEMU
start_qemu() {
    local ovmf_code
    ovmf_code=$(find_ovmf_code)

    log "Starting Windows 11 VM..."
    log "  CPUs: ${VM_CPUS}"
    log "  RAM: ${VM_RAM}"
    log "  VNC: :${VNC_DISPLAY} (port $((5900 + VNC_DISPLAY)))"

    # Remove old monitor socket
    rm -f "$MONITOR_SOCK"

    qemu-system-x86_64 \
        -enable-kvm \
        -m "$VM_RAM" \
        -smp "$VM_CPUS" \
        -cpu host \
        -machine q35,accel=kvm \
        -drive file="$QCOW2_IMAGE",format=qcow2,if=none,id=hd0 \
        -device ahci,id=ahci \
        -device ide-hd,drive=hd0,bus=ahci.0 \
        -drive if=pflash,format=raw,readonly=on,file="$ovmf_code" \
        -drive if=pflash,format=raw,file="$OVMF_VARS" \
        -chardev socket,id=chrtpm,path="${TPM_DIR}/swtpm-sock" \
        -tpmdev emulator,id=tpm0,chardev=chrtpm \
        -device tpm-tis,tpmdev=tpm0 \
        -device virtio-net,netdev=net0 \
        -netdev user,id=net0,hostfwd=tcp::3389-:3389 \
        -device usb-ehci \
        -device usb-tablet \
        -cdrom "$VIRTIO_ISO" \
        -vnc ":${VNC_DISPLAY}" \
        -daemonize \
        -pidfile "$PID_FILE" \
        -monitor unix:"$MONITOR_SOCK",server,nowait

    # Wait for QEMU to start
    sleep 2

    if [[ -f "$PID_FILE" ]]; then
        local pid
        pid=$(cat "$PID_FILE")
        log "VM started (PID $pid)"
    else
        error "Failed to start VM"
    fi
}

main() {
    echo "========================================"
    echo "Starting Windows 11 VM"
    echo "========================================"
    echo

    # Check prerequisites
    [[ -f "$QCOW2_IMAGE" ]] || error "Windows image not found. Run ./setup.sh first."
    [[ -f "$VIRTIO_ISO" ]] || error "VirtIO ISO not found. Run ./setup.sh first."
    [[ -f "$OVMF_VARS" ]] || error "OVMF vars not found. Run ./setup.sh first."
    [[ -e /dev/kvm ]] || error "/dev/kvm not found. KVM required."

    check_running
    start_tpm
    start_qemu

    echo
    echo "========================================"
    log "VM is running!"
    echo "========================================"
    echo
    echo "Connect via VNC:"
    echo "  vncviewer localhost:$((5900 + VNC_DISPLAY))"
    echo
    echo "Connect via RDP (after Windows boots):"
    echo "  xfreerdp /v:localhost:3389"
    echo
    echo "Other commands:"
    echo "  ./stop.sh        - Shut down VM"
    echo "  ./screenshot.sh  - Take screenshot"
    echo "  ./send-keys.sh   - Send keystrokes"
    echo
}

main "$@"
