#!/bin/bash
set -euo pipefail

# Coding harnesses intentionally pass a minimal environment to MCP servers.
# Restore the graphical-session variables Chrome needs, selecting the newest
# compositor socket when both the host compositor and desktop compositor exist.
export XDG_RUNTIME_DIR="${XDG_RUNTIME_DIR:-/run/user/$(id -u)}"
shopt -s nullglob
if [ -z "${WAYLAND_DISPLAY:-}" ]; then
    for socket in "$XDG_RUNTIME_DIR"/wayland-*; do
        [ -S "$socket" ] || continue
        WAYLAND_DISPLAY="${socket##*/}"
    done
fi

# Headless sandboxes run no compositor. Desktop sessions always have a socket
# by the time an MCP server starts (Zed is itself a Wayland client), so a
# missing socket means headless: drop the Wayland backend and run Chrome
# headless instead of failing every call with "Target closed".
if [ -z "${WAYLAND_DISPLAY:-}" ] || [ ! -S "$XDG_RUNTIME_DIR/$WAYLAND_DISPLAY" ]; then
    args=()
    for arg in "$@"; do
        [ "$arg" = "--chrome-arg=--ozone-platform=wayland" ] || args+=("$arg")
    done
    exec /usr/bin/chrome-devtools-mcp "${args[@]}" --headless
fi

export WAYLAND_DISPLAY
exec /usr/bin/chrome-devtools-mcp "$@"
