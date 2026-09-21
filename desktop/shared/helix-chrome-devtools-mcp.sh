#!/bin/bash
set -euo pipefail

# Coding harnesses intentionally pass a minimal environment to MCP servers.
# Restore the graphical-session variables Chrome needs, selecting the newest
# compositor socket when both the host compositor and desktop compositor exist.
export XDG_RUNTIME_DIR="${XDG_RUNTIME_DIR:-/run/user/$(id -u)}"
if [ -z "${WAYLAND_DISPLAY:-}" ]; then
    shopt -s nullglob
    for socket in "$XDG_RUNTIME_DIR"/wayland-*; do
        [ -S "$socket" ] || continue
        WAYLAND_DISPLAY="${socket##*/}"
    done
    export WAYLAND_DISPLAY="${WAYLAND_DISPLAY:-wayland-0}"
fi

exec /usr/bin/chrome-devtools-mcp "$@"
