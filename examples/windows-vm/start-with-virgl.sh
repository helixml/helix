#!/bin/bash
set -euo pipefail

# Start Windows 11 VM with VirGL GPU acceleration
# EXPERIMENTAL: VirGL support on Windows is limited
#
# This script attempts to use virtio-gpu with virglrenderer for
# OpenGL acceleration. Note that:
# - Windows VirtIO-GPU drivers (viogpudo) have limited virgl support
# - Works best with SDL display (requires X11/Wayland)
# - Falls back to software rendering if virgl unavailable

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
VM_DIR="${SCRIPT_DIR}/vm"

# VM Configuration
VM_CPUS="${VM_CPUS:-8}"
VM_RAM="${VM_RAM:-16G}"
VNC_DISPLAY="${VNC_DISPLAY:-0}"

# Display mode: sdl, gtk, spice, or vnc
# SDL and GTK support OpenGL acceleration
DISPLAY_MODE="${DISPLAY_MODE:-sdl}"

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

# Check if we can use GL acceleration
check_gl_support() {
    # Check for display
    if [[ -z "${DISPLAY:-}" ]] && [[ -z "${WAYLAND_DISPLAY:-}" ]]; then
        warn "No DISPLAY or WAYLAND_DISPLAY set"
        warn "GL acceleration requires X11 or Wayland"
        return 1
    fi

    # Check for virglrenderer
    if ! ldconfig -p 2>/dev/null | grep -q virglrenderer; then
        warn "libvirglrenderer not found"
        return 1
    fi

    # Check QEMU has virgl support
    if ! qemu-system-x86_64 -device help 2>&1 | grep -q virtio-vga-gl; then
        warn "QEMU does not have virtio-vga-gl support"
        return 1
    fi

    return 0
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
    pkill -f "swtpm.*${TPM_DIR}" 2>/dev/null || true
    sleep 1
    mkdir -p "$TPM_DIR"

    swtpm socket \
        --tpmstate dir="$TPM_DIR" \
        --ctrl type=unixio,path="${TPM_DIR}/swtpm-sock" \
        --tpm2 \
        --daemon

    for _ in {1..10}; do
        if [[ -S "${TPM_DIR}/swtpm-sock" ]]; then
            log "TPM emulator started"
            return
        fi
        sleep 0.5
    done
    error "TPM emulator failed to start"
}

# Build display arguments based on mode
get_display_args() {
    local mode="$1"
    local gl_available="$2"

    case "$mode" in
        sdl)
            if [[ "$gl_available" == "true" ]]; then
                echo "-device virtio-vga-gl -display sdl,gl=on"
            else
                echo "-device virtio-vga -display sdl"
            fi
            ;;
        gtk)
            if [[ "$gl_available" == "true" ]]; then
                echo "-device virtio-vga-gl -display gtk,gl=on"
            else
                echo "-device virtio-vga -display gtk"
            fi
            ;;
        spice)
            # SPICE with GL requires spice-app display
            if [[ "$gl_available" == "true" ]]; then
                echo "-device virtio-vga-gl -display spice-app,gl=on"
            else
                echo "-device qxl-vga -spice port=5930,disable-ticketing=on"
            fi
            ;;
        vnc)
            # VNC doesn't support GL passthrough
            echo "-device virtio-vga -vnc :${VNC_DISPLAY}"
            ;;
        *)
            error "Unknown display mode: $mode"
            ;;
    esac
}

# Start QEMU with VirGL
start_qemu() {
    local ovmf_code
    ovmf_code=$(find_ovmf_code)

    local gl_available="false"
    if check_gl_support; then
        gl_available="true"
        log "GL acceleration available"
    else
        warn "GL acceleration not available, using software rendering"
        if [[ "$DISPLAY_MODE" == "sdl" ]] || [[ "$DISPLAY_MODE" == "gtk" ]]; then
            if [[ -z "${DISPLAY:-}" ]]; then
                warn "No display available, falling back to VNC"
                DISPLAY_MODE="vnc"
            fi
        fi
    fi

    local display_args
    display_args=$(get_display_args "$DISPLAY_MODE" "$gl_available")

    log "Starting Windows 11 VM with VirGL..."
    log "  CPUs: ${VM_CPUS}"
    log "  RAM: ${VM_RAM}"
    log "  Display: ${DISPLAY_MODE} (GL: ${gl_available})"

    rm -f "$MONITOR_SOCK"

    # Build QEMU command
    local qemu_cmd=(
        qemu-system-x86_64
        -enable-kvm
        -m "$VM_RAM"
        -smp "$VM_CPUS"
        -cpu host
        -machine q35,accel=kvm
        # Storage
        -drive file="$QCOW2_IMAGE",format=qcow2,if=none,id=hd0
        -device ahci,id=ahci
        -device ide-hd,drive=hd0,bus=ahci.0
        # UEFI firmware
        -drive if=pflash,format=raw,readonly=on,file="$ovmf_code"
        -drive if=pflash,format=raw,file="$OVMF_VARS"
        # TPM
        -chardev socket,id=chrtpm,path="${TPM_DIR}/swtpm-sock"
        -tpmdev emulator,id=tpm0,chardev=chrtpm
        -device tpm-tis,tpmdev=tpm0
        # Network with RDP forwarding
        -device virtio-net,netdev=net0
        -netdev user,id=net0,hostfwd=tcp::3389-:3389
        # USB
        -device usb-ehci
        -device usb-tablet
        # VirtIO drivers CD
        -cdrom "$VIRTIO_ISO"
        # Monitor
        -monitor unix:"$MONITOR_SOCK",server,nowait
    )

    # Add display arguments (split on spaces)
    # shellcheck disable=SC2206
    qemu_cmd+=($display_args)

    # For SDL/GTK, run in foreground so user can see the window
    # For VNC/SPICE, daemonize
    if [[ "$DISPLAY_MODE" == "vnc" ]] || [[ "$DISPLAY_MODE" == "spice" ]]; then
        qemu_cmd+=(-daemonize -pidfile "$PID_FILE")
        "${qemu_cmd[@]}"
        sleep 2
        if [[ -f "$PID_FILE" ]]; then
            log "VM started (PID $(cat "$PID_FILE"))"
        fi
    else
        # Run in foreground with nohup for SDL/GTK
        log "Starting VM in foreground (close window to stop)..."
        echo $$ > "$PID_FILE"
        exec "${qemu_cmd[@]}"
    fi
}

usage() {
    cat <<EOF
Usage: $0 [OPTIONS]

Start Windows 11 VM with VirGL GPU acceleration.

Options:
  -d, --display MODE    Display mode: sdl, gtk, spice, vnc (default: sdl)
  -h, --help            Show this help

Environment Variables:
  VM_CPUS       Number of CPUs (default: 8)
  VM_RAM        Amount of RAM (default: 16G)
  VNC_DISPLAY   VNC display number for vnc mode (default: 0)
  DISPLAY_MODE  Same as -d option

Examples:
  # Start with SDL display (OpenGL window)
  $0

  # Start with GTK display
  $0 -d gtk

  # Start with VNC (no GL, for remote access)
  $0 -d vnc

  # Start with SPICE (supports GL with spice-app)
  $0 -d spice

Notes:
  - SDL and GTK modes require X11 or Wayland
  - VNC mode does not support GL acceleration
  - VirGL on Windows is experimental; performance varies
  - Install viogpudo driver in Windows for best results
EOF
    exit 0
}

main() {
    # Parse arguments
    while [[ $# -gt 0 ]]; do
        case "$1" in
            -d|--display)
                DISPLAY_MODE="$2"
                shift 2
                ;;
            -h|--help)
                usage
                ;;
            *)
                error "Unknown option: $1"
                ;;
        esac
    done

    echo "========================================"
    echo "Starting Windows 11 VM with VirGL"
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
    if [[ "$DISPLAY_MODE" == "vnc" ]]; then
        echo "Connect via VNC: vncviewer localhost:$((5900 + VNC_DISPLAY))"
    elif [[ "$DISPLAY_MODE" == "spice" ]]; then
        echo "Connect via SPICE: spicy -h localhost -p 5930"
    fi
    echo
}

main "$@"
