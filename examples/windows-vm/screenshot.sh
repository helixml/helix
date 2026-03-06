#!/bin/bash
set -euo pipefail

# Take a screenshot of the Windows VM display

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
VM_DIR="${SCRIPT_DIR}/vm"
MONITOR_SOCK="${VM_DIR}/qemu-monitor.sock"

# Default output file
OUTPUT="${1:-${VM_DIR}/screenshot.png}"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log() { echo -e "${GREEN}[+]${NC} $1"; }
warn() { echo -e "${YELLOW}[!]${NC} $1"; }
error() { echo -e "${RED}[!]${NC} $1"; exit 1; }

# Check prerequisites
[[ -S "$MONITOR_SOCK" ]] || error "VM not running (monitor socket not found)"
command -v socat &>/dev/null || error "socat not installed"
command -v convert &>/dev/null || error "imagemagick not installed (needed for convert)"

# Create temp file for PPM
TMP_PPM=$(mktemp --suffix=.ppm)
trap "rm -f '$TMP_PPM'" EXIT

# Take screenshot via QEMU monitor
log "Taking screenshot..."
echo "screendump ${TMP_PPM}" | socat - UNIX-CONNECT:"$MONITOR_SOCK" > /dev/null 2>&1

# Wait for file to be written
sleep 1

if [[ ! -f "$TMP_PPM" ]] || [[ ! -s "$TMP_PPM" ]]; then
    error "Failed to capture screenshot"
fi

# Convert to PNG
log "Converting to PNG..."
convert "$TMP_PPM" "$OUTPUT"

log "Screenshot saved: ${OUTPUT}"

# Show file info
ls -lh "$OUTPUT"
