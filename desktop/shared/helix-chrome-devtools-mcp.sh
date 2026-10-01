#!/bin/bash
set -euo pipefail

# One Chrome per sandbox, shared by every chrome-devtools-mcp server.
#
# Some harnesses (DeepSeek Harness, Goose) start a new MCP server for every
# ACP session and never stop the old ones. When each server launched its own
# Chrome on the persistent profile, the first Chrome kept the profile lock and
# every later server failed with "The browser is already running for
# .../.chrome-state" — the agent then spent minutes killing Chrome by hand.
# Instead, the first server starts Chrome with a loopback debugging port and
# every server attaches to it with --browserUrl. Logins also survive a new
# thread, because it is the same browser.

CHROME_DEBUG_PORT="${HELIX_CHROME_DEBUG_PORT:-9222}"
CHROME_URL="http://127.0.0.1:${CHROME_DEBUG_PORT}"
CHROME_BIN="${CHROME_PATH:-/usr/bin/google-chrome-stable}"
# Another chrome-devtools-mcp build (e.g. a version under test) can be used
# with the shared browser by pointing this at its binary.
MCP_BIN="${HELIX_CHROME_DEVTOOLS_MCP:-/usr/bin/chrome-devtools-mcp}"

# --isolated asks for a throwaway profile of the server's own, which never
# touches the shared one, so it is passed through unchanged.
for arg in "$@"; do
    if [ "$arg" = "--isolated" ]; then
        exec "$MCP_BIN" "$@"
    fi
done

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
# missing socket means headless.
headless=true
if [ -n "${WAYLAND_DISPLAY:-}" ] && [ -S "$XDG_RUNTIME_DIR/$WAYLAND_DISPLAY" ]; then
    headless=false
    export WAYLAND_DISPLAY
fi

# Split the arguments: browser-launch options become Chrome flags, everything
# else is for the MCP server.
user_data_dir="$HOME/.cache/chrome-devtools-mcp/chrome-profile"
viewport="1280x800"
chrome_flags=()
mcp_args=()
while [ $# -gt 0 ]; do
    case "$1" in
        --user-data-dir=*) user_data_dir="${1#*=}" ;;
        --user-data-dir) user_data_dir="$2"; shift ;;
        --viewport=*) viewport="${1#*=}" ;;
        --viewport) viewport="$2"; shift ;;
        --chrome-arg=*) chrome_flags+=("${1#*=}") ;;
        --headless) ;;
        *) mcp_args+=("$1") ;;
    esac
    shift
done

# The user agent a regular (non-headless) Chrome of this build sends: Chrome reports
# only the major version ("Chrome/152.0.0.0").
headless_user_agent() {
    local major
    major=$("$CHROME_BIN" --version 2>/dev/null | grep -oE '[0-9]+\.[0-9.]+' | head -1 | cut -d. -f1)
    [ -n "$major" ] || return 1
    echo "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/${major}.0.0.0 Safari/537.36"
}

chrome_running() {
    curl -fsS --max-time 2 "$CHROME_URL/json/version" >/dev/null 2>&1
}

wait_for_chrome() {
    for _ in $(seq 1 100); do
        chrome_running && return 0
        sleep 0.2
    done
    return 1
}

# Chrome's profile lock is a symlink to "<hostname>-<pid>". A live owner on
# this host is a Chrome that is still starting or has stalled, not a crash.
profile_lock_owner_alive() {
    local target
    target=$(readlink "$user_data_dir/SingletonLock" 2>/dev/null) || return 1
    [ "${target%-*}" = "$(hostname)" ] && kill -0 "${target##*-}" 2>/dev/null
}

start_chrome() {
    local width="${viewport%x*}" height="${viewport#*x}"
    local flags=(
        "--user-data-dir=$user_data_dir"
        "--remote-debugging-port=$CHROME_DEBUG_PORT"
        "--remote-debugging-address=127.0.0.1"
        "--window-size=${width},$((height + 80))"
        "--no-default-browser-check"
    )
    if $headless; then
        flags+=("--headless=new")
        local ua_set=false
        for flag in "${chrome_flags[@]}"; do
            [[ "$flag" == --user-agent=* ]] && ua_set=true
            [ "$flag" = "--ozone-platform=wayland" ] || flags+=("$flag")
        done
        # Headless Chrome announces itself as "HeadlessChrome/<version>", which sites
        # with bot detection may treat differently from the desktop browser. Send the
        # user agent a normal Chrome of the same major version sends.
        local ua
        if ! $ua_set && ua=$(headless_user_agent); then
            flags+=("--user-agent=$ua")
        fi
    else
        flags+=("${chrome_flags[@]}")
    fi
    mkdir -p "$user_data_dir"
    # A Chrome that holds the profile but is not answering yet gets time to
    # come up; a second Chrome on the same profile would corrupt it.
    if profile_lock_owner_alive; then
        wait_for_chrome && return 0
        echo "helix-chrome-devtools-mcp: a Chrome holds $user_data_dir but does not answer on $CHROME_URL" >&2
        return 1
    fi
    # The lock's owner is gone (a crash, or a previous container: Chrome
    # refuses a lock written under another hostname), so it is stale.
    rm -f "$user_data_dir"/Singleton{Lock,Cookie,Socket}
    # Detached: the browser must outlive the MCP server that started it. It
    # must not inherit the lock descriptor either, or it would hold the lock
    # for as long as it runs.
    setsid "$CHROME_BIN" "${flags[@]}" about:blank >/tmp/helix-chrome.log 2>&1 < /dev/null 9>&- &
    wait_for_chrome && return 0
    echo "helix-chrome-devtools-mcp: Chrome did not open $CHROME_URL; see /tmp/helix-chrome.log" >&2
    return 1
}

# Several MCP servers can start at once; only one may launch the browser.
exec 9>"/tmp/helix-chrome.lock"
flock 9
chrome_running || start_chrome
flock -u 9
exec 9>&-

exec "$MCP_BIN" --browserUrl "$CHROME_URL" "${mcp_args[@]}"
