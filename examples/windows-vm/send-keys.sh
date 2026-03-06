#!/bin/bash
set -euo pipefail

# Send keystrokes to the Windows VM via QEMU monitor

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
VM_DIR="${SCRIPT_DIR}/vm"
MONITOR_SOCK="${VM_DIR}/qemu-monitor.sock"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log() { echo -e "${GREEN}[+]${NC} $1"; }
warn() { echo -e "${YELLOW}[!]${NC} $1"; }
error() { echo -e "${RED}[!]${NC} $1"; exit 1; }

usage() {
    cat <<EOF
Usage: $0 <key> [key...]

Send keystrokes to the Windows VM via QEMU monitor.

Examples:
  $0 ret                    # Press Enter
  $0 meta_l-e               # Win+E (open File Explorer)
  $0 ctrl-alt-del           # Ctrl+Alt+Delete
  $0 alt-f4                 # Alt+F4 (close window)
  $0 tab                    # Tab key
  $0 spc                    # Space key
  $0 h e l l o              # Type "hello"

Common key names:
  ret, esc, tab, spc, backspace, delete
  up, down, left, right, home, end, pgup, pgdn
  f1-f12
  ctrl, alt, shift, meta_l (Windows key)

Modifiers (use hyphen to combine):
  ctrl-c, alt-tab, shift-tab, meta_l-r (Win+R)

Note: Use 'spc' for space, not 'space'
EOF
    exit 1
}

send_key() {
    local key="$1"
    echo "sendkey ${key}" | socat - UNIX-CONNECT:"$MONITOR_SOCK" > /dev/null 2>&1
}

# Check prerequisites
[[ -S "$MONITOR_SOCK" ]] || error "VM not running (monitor socket not found)"
command -v socat &>/dev/null || error "socat not installed"

# Check arguments
if [[ $# -eq 0 ]]; then
    usage
fi

# Send each key
for key in "$@"; do
    log "Sending: ${key}"
    send_key "$key"
    sleep 0.1  # Small delay between keys
done

log "Done"
