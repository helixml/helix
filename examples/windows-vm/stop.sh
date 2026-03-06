#!/bin/bash
set -euo pipefail

# Stop Windows 11 VM gracefully

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
VM_DIR="${SCRIPT_DIR}/vm"
MONITOR_SOCK="${VM_DIR}/qemu-monitor.sock"
PID_FILE="${VM_DIR}/qemu.pid"
TPM_DIR="${VM_DIR}/tpm"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log() { echo -e "${GREEN}[+]${NC} $1"; }
warn() { echo -e "${YELLOW}[!]${NC} $1"; }
error() { echo -e "${RED}[!]${NC} $1"; exit 1; }

# Send command to QEMU monitor
qemu_cmd() {
    if [[ -S "$MONITOR_SOCK" ]]; then
        echo "$1" | socat - UNIX-CONNECT:"$MONITOR_SOCK" 2>/dev/null || true
    fi
}

# Graceful shutdown via ACPI
shutdown_acpi() {
    log "Sending ACPI shutdown signal..."
    qemu_cmd "system_powerdown"
}

# Force quit
force_quit() {
    log "Forcing VM quit..."
    qemu_cmd "quit"
}

# Kill process directly
kill_process() {
    if [[ -f "$PID_FILE" ]]; then
        local pid
        pid=$(cat "$PID_FILE")
        if kill -0 "$pid" 2>/dev/null; then
            log "Killing QEMU process (PID $pid)..."
            kill "$pid" 2>/dev/null || true
            sleep 2
            # Force kill if still running
            if kill -0 "$pid" 2>/dev/null; then
                kill -9 "$pid" 2>/dev/null || true
            fi
        fi
        rm -f "$PID_FILE"
    fi
}

# Stop TPM emulator
stop_tpm() {
    log "Stopping TPM emulator..."
    pkill -f "swtpm.*${TPM_DIR}" 2>/dev/null || true
}

# Check if VM is running
is_running() {
    if [[ -f "$PID_FILE" ]]; then
        local pid
        pid=$(cat "$PID_FILE")
        if kill -0 "$pid" 2>/dev/null; then
            return 0
        fi
    fi
    return 1
}

# Wait for VM to stop
wait_for_stop() {
    local timeout="${1:-30}"
    local count=0

    while is_running && [[ $count -lt $timeout ]]; do
        sleep 1
        ((count++))
        echo -n "."
    done
    echo

    if is_running; then
        return 1
    fi
    return 0
}

main() {
    local force=false

    # Parse arguments
    while [[ $# -gt 0 ]]; do
        case "$1" in
            -f|--force)
                force=true
                shift
                ;;
            -h|--help)
                echo "Usage: $0 [-f|--force]"
                echo "  -f, --force  Force immediate shutdown (no graceful ACPI)"
                exit 0
                ;;
            *)
                echo "Unknown option: $1"
                exit 1
                ;;
        esac
    done

    echo "========================================"
    echo "Stopping Windows 11 VM"
    echo "========================================"
    echo

    if ! is_running; then
        warn "VM is not running"
        # Clean up stale files anyway
        rm -f "$PID_FILE"
        stop_tpm
        exit 0
    fi

    if [[ "$force" == true ]]; then
        force_quit
        sleep 2
        if is_running; then
            kill_process
        fi
    else
        # Try graceful shutdown first
        shutdown_acpi

        log "Waiting for VM to shut down (30s timeout)..."
        if ! wait_for_stop 30; then
            warn "Graceful shutdown timed out"
            log "Forcing quit..."
            force_quit
            sleep 2
            if is_running; then
                kill_process
            fi
        fi
    fi

    # Clean up
    rm -f "$PID_FILE" "$MONITOR_SOCK"
    stop_tpm

    if is_running; then
        error "Failed to stop VM"
    fi

    log "VM stopped"
}

main "$@"
