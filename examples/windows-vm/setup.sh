#!/bin/bash
set -euo pipefail

# Windows 11 VM Setup Script for Helix Spectask Container
# Downloads and prepares a Windows 11 VM using Microsoft's pre-built developer image

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
VM_DIR="${SCRIPT_DIR}/vm"
VIRTIO_URL="https://fedorapeople.org/groups/virt/virtio-win/direct-downloads/stable-virtio/virtio-win.iso"
WINDEV_URL="https://aka.ms/windev_VM_hyperv"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log() { echo -e "${GREEN}[+]${NC} $1"; }
warn() { echo -e "${YELLOW}[!]${NC} $1"; }
error() { echo -e "${RED}[!]${NC} $1"; exit 1; }

# Check prerequisites
check_prerequisites() {
    log "Checking prerequisites..."

    # Check KVM
    if [[ ! -e /dev/kvm ]]; then
        error "/dev/kvm not found. KVM support required."
    fi

    if [[ ! -r /dev/kvm ]] || [[ ! -w /dev/kvm ]]; then
        warn "/dev/kvm not accessible. Trying to fix permissions..."
        sudo chmod 666 /dev/kvm || error "Cannot fix /dev/kvm permissions"
    fi

    # Check required tools
    for cmd in qemu-system-x86_64 qemu-img swtpm wget unzip socat; do
        if ! command -v "$cmd" &> /dev/null; then
            warn "$cmd not found. Installing dependencies..."
            install_dependencies
            break
        fi
    done

    # Check disk space (need ~80GB for download + conversion)
    available_gb=$(df -BG "${SCRIPT_DIR}" | awk 'NR==2 {print $4}' | tr -d 'G')
    if [[ "$available_gb" -lt 80 ]]; then
        error "Not enough disk space. Need 80GB, have ${available_gb}GB"
    fi

    log "Prerequisites OK"
}

install_dependencies() {
    log "Installing dependencies..."
    sudo apt-get update
    sudo apt-get install -y \
        qemu-system-x86 \
        qemu-utils \
        ovmf \
        swtpm \
        swtpm-tools \
        socat \
        wget \
        unzip \
        imagemagick
}

download_virtio() {
    local virtio_iso="${VM_DIR}/virtio-win.iso"

    if [[ -f "$virtio_iso" ]]; then
        log "VirtIO drivers already downloaded"
        return
    fi

    log "Downloading VirtIO drivers (~750MB)..."
    wget -O "$virtio_iso" "$VIRTIO_URL"
    log "VirtIO drivers downloaded"
}

download_windows() {
    local zip_file="${VM_DIR}/WinDev.HyperV.zip"
    local vhdx_file="${VM_DIR}/WinDev.vhdx"
    local qcow2_file="${VM_DIR}/windows11.qcow2"

    if [[ -f "$qcow2_file" ]]; then
        log "Windows 11 image already exists"
        return
    fi

    # Download if zip doesn't exist
    if [[ ! -f "$zip_file" ]]; then
        log "Downloading Microsoft Windows 11 Developer VM (~22GB)..."
        log "This may take a while depending on your connection..."
        wget -O "$zip_file" "$WINDEV_URL"
    fi

    # Extract VHDX
    if [[ ! -f "$vhdx_file" ]]; then
        log "Extracting VHDX from zip..."
        # Find the VHDX file in the zip and extract it
        vhdx_name=$(unzip -l "$zip_file" | grep -o '[^ ]*\.vhdx' | head -1)
        unzip -o "$zip_file" -d "${VM_DIR}"
        # Rename to standard name
        mv "${VM_DIR}/${vhdx_name}" "$vhdx_file" 2>/dev/null || true
    fi

    # Convert to qcow2
    log "Converting VHDX to qcow2 (this takes several minutes)..."
    qemu-img convert -p -f vhdx -O qcow2 "$vhdx_file" "$qcow2_file"

    # Clean up large intermediate files
    log "Cleaning up intermediate files..."
    rm -f "$zip_file" "$vhdx_file"

    log "Windows 11 image ready"
}

setup_uefi() {
    local ovmf_vars="${VM_DIR}/OVMF_VARS.fd"

    if [[ -f "$ovmf_vars" ]]; then
        log "UEFI vars already configured"
        return
    fi

    log "Setting up UEFI firmware..."

    # Find OVMF vars file
    local ovmf_source=""
    for path in /usr/share/OVMF/OVMF_VARS_4M.fd /usr/share/OVMF/OVMF_VARS.fd /usr/share/edk2/ovmf/OVMF_VARS.fd; do
        if [[ -f "$path" ]]; then
            ovmf_source="$path"
            break
        fi
    done

    if [[ -z "$ovmf_source" ]]; then
        error "OVMF_VARS not found. Install ovmf package."
    fi

    cp "$ovmf_source" "$ovmf_vars"
    log "UEFI configured"
}

setup_tpm() {
    local tpm_dir="${VM_DIR}/tpm"

    log "Setting up TPM 2.0 emulation..."
    mkdir -p "$tpm_dir"

    # Kill any existing swtpm
    pkill -f "swtpm.*${tpm_dir}" 2>/dev/null || true
    sleep 1

    log "TPM directory ready at ${tpm_dir}"
}

main() {
    echo "========================================"
    echo "Windows 11 VM Setup for Helix Spectask"
    echo "========================================"
    echo

    # Create VM directory
    mkdir -p "$VM_DIR"

    check_prerequisites
    download_virtio
    download_windows
    setup_uefi
    setup_tpm

    echo
    echo "========================================"
    log "Setup complete!"
    echo "========================================"
    echo
    echo "To start the VM:"
    echo "  ./start.sh"
    echo
    echo "To connect:"
    echo "  Use a VNC client to connect to localhost:5900"
    echo
    echo "First boot tasks:"
    echo "  1. Wait for Windows to finish OOBE setup"
    echo "  2. Install VirtIO drivers from D:\\ drive"
    echo "     Run: D:\\virtio-win-guest-tools.exe"
    echo
}

main "$@"
