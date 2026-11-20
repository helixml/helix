#!/bin/bash
set -euo pipefail

# One Chrome per sandbox, shared by every chrome-devtools-mcp server, started
# only when a browser tool first needs it.
#
# Some harnesses (DeepSeek Harness, Goose) start a new MCP server for every
# ACP session and never stop the old ones. When each server launched its own
# Chrome on the persistent profile, the first Chrome kept the profile lock and
# every later server failed with "The browser is already running for
# .../.chrome-state". So every server attaches with --browserUrl to one
# loopback debugging port. Logins also survive a new thread, because it is the
# same browser.
#
# Zed and the agent start their MCP servers with every session, but most
# sessions never use the browser, and Chrome costs over a gigabyte. The
# debugging port is therefore a socat listener: the first connection to it
# (chrome-devtools-mcp connects on its first tool call) runs
# `helix-browser connect`, which starts Chrome on the configured profile and
# relays to it. helix-browser owns the profile, its single owner and resume.

CHROME_DEBUG_PORT="${HELIX_CHROME_DEBUG_PORT:-9222}"
# Another chrome-devtools-mcp build (e.g. a version under test) can be used
# with the shared browser by pointing this at its binary.
MCP_BIN="${HELIX_CHROME_DEVTOOLS_MCP:-/usr/bin/chrome-devtools-mcp}"
HELIX_BROWSER="${HELIX_BROWSER:-/usr/local/bin/helix-browser}"

# --isolated asks for a throwaway profile of the server's own, which never
# touches the shared one, so it is passed through unchanged.
for arg in "$@"; do
    if [ "$arg" = "--isolated" ]; then
        exec "$MCP_BIN" "$@"
    fi
done

# Browser-launch options are for the shared browser (the relay keeps them for
# the browser it launches); the rest are for the MCP server.
"$HELIX_BROWSER" relay "$@"
mcp_args=()
while [ $# -gt 0 ]; do
    case "$1" in
        --user-data-dir|--viewport) shift ;;
        --user-data-dir=*|--viewport=*|--chrome-arg=*|--headless) ;;
        *) mcp_args+=("$1") ;;
    esac
    shift
done

exec "$MCP_BIN" --browserUrl "http://127.0.0.1:${CHROME_DEBUG_PORT}" "${mcp_args[@]}"
