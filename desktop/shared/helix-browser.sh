#!/bin/bash
# helix-browser: the one owner of the sandbox Chrome and its persistent profile.
#
#   helix-browser status [--json]    report profile, mount, process, DevTools
#                                    and login separately; never starts Chrome
#   helix-browser ensure             start Chrome on the configured profile if
#                                    it is not running (a browser request)
#   helix-browser open [URL...]      ensure, then open a window in that Chrome
#   helix-browser connect            ensure, then relay stdio to DevTools (run
#                                    by the debugging-port listener)
#   helix-browser relay [MCP ARGS]   start the lazy debugging-port listener
#   helix-browser declare DIR [--login-url URL --logged-out-match TEXT...]
#                                    make an existing profile in the workspace
#                                    the configured one
#   helix-browser login-check        open the declared login URL and report
#                                    logged_in / logged_out (no cookie access)
#   helix-browser stop               close the browser (the profile is kept)
#
# Chrome starts only on a browser request: an MCP tool call through the
# listener, `open`, `ensure` or `login-check`. Starting a desktop, an MCP
# server or `status` never starts it. One Chrome owns the profile: a live owner
# is reused or reported, never joined by a second browser, and a profile lock
# is cleared only after proving its owner is gone.
#
# The configured profile lives in the durable workspace: .helix/browser.json
# names a directory under the workspace and the identity written into it at
# declaration. A declared profile that is missing, unreadable or another
# profile fails the request; Chrome is never pointed at an empty directory in
# its place. Without a declaration the profile is .chrome-state.
#
# Never reads cookie, login or history databases.

set -euo pipefail

WORK_DIR="${HELIX_BROWSER_WORK_DIR:-$HOME/work}"
DECLARATION="$WORK_DIR/.helix/browser.json"
DEFAULT_PROFILE="$WORK_DIR/.chrome-state"
IDENTITY_FILE=".helix-profile.json"
MOUNTINFO="${HELIX_BROWSER_MOUNTINFO:-/proc/self/mountinfo}"
CHROME_DEBUG_PORT="${HELIX_CHROME_DEBUG_PORT:-9222}"
CHROME_INTERNAL_PORT="${HELIX_CHROME_INTERNAL_PORT:-9223}"
CHROME_INTERNAL_URL="http://127.0.0.1:${CHROME_INTERNAL_PORT}"
CHROME_BIN="${CHROME_PATH:-/usr/bin/google-chrome-stable}"
RUN_DIR="${HELIX_BROWSER_RUN_DIR:-/tmp}"
LOCK_FILE="$RUN_DIR/helix-chrome.lock"
STATE_FILE="$RUN_DIR/helix-chrome.state"
CHROME_LOG="$RUN_DIR/helix-chrome.log"
RELAY_LOG="$RUN_DIR/helix-chrome-proxy.log"
START_TIMEOUT="${HELIX_BROWSER_START_TIMEOUT:-20}"
SELF="$(readlink -f "${BASH_SOURCE[0]}")"

die() {
    echo "helix-browser: $*" >&2
    exit 1
}

log() {
    echo "helix-browser: $*" >&2
}

# ---------------------------------------------------------------------------
# Profile resolution and integrity

# Only the workspace survives a container recreate, so a declared profile must
# be a directory inside it. Prints the absolute path; inside_workspace checks
# where it really is.
workspace_path() {
    local rel="$1"
    case "$rel" in
        ""|/*|..|../*|*/..|*/../*) return 1 ;;
    esac
    printf '%s/%s\n' "$WORK_DIR" "${rel%/}"
}

# Whether a path, with every symlink resolved, is inside the workspace: a
# profile reached through a symlink to the container overlay or another
# volume would keep its login on storage that does not survive.
inside_workspace() {
    local root real
    root=$(readlink -f "$WORK_DIR") || return 1
    real=$(readlink -m "$1") || return 1
    [[ "$real" == "$root"/* ]]
}

# Sets PROFILE, PROFILE_SOURCE (declared/default), PROFILE_ID (declared only)
# and the login check (LOGIN_URL, LOGGED_OUT_MATCH, LOGGED_IN_MATCH,
# LOGGED_IN_TITLE) from the declaration.
resolve_profile() {
    PROFILE_ID=""
    LOGIN_URL=""
    LOGGED_OUT_MATCH=()
    LOGGED_IN_MATCH=()
    LOGGED_IN_TITLE=""
    if [ -e "$DECLARATION" ]; then
        local rel
        jq -e 'type == "object"' "$DECLARATION" >/dev/null 2>&1 \
            || die "$DECLARATION is not valid JSON; fix or remove it (the profile is not touched)"
        rel=$(jq -r '.profile // ""' "$DECLARATION")
        PROFILE=$(workspace_path "$rel") \
            || die "$DECLARATION: profile \"$rel\" must be a directory inside $WORK_DIR"
        PROFILE_ID=$(jq -r '.profile_id // ""' "$DECLARATION")
        [ -n "$PROFILE_ID" ] || die "$DECLARATION has no profile_id; run: helix-browser declare $rel"
        LOGIN_URL=$(jq -r '.login_check.url // ""' "$DECLARATION")
        mapfile -t LOGGED_OUT_MATCH < <(jq -r '.login_check.logged_out_url_contains // [] | .[]' "$DECLARATION")
        mapfile -t LOGGED_IN_MATCH < <(jq -r '.login_check.logged_in_url_contains // [] | .[]' "$DECLARATION")
        LOGGED_IN_TITLE=$(jq -r '.login_check.logged_in_title_contains // ""' "$DECLARATION")
        PROFILE_SOURCE=declared
    else
        PROFILE="${1:-$DEFAULT_PROFILE}"
        PROFILE_SOURCE=default
    fi
}

json_file_valid() {
    jq -e . "$1" >/dev/null 2>&1
}

# Sets PROFILE_STATE (ok, new, missing, outside_workspace, inconsistent,
# wrong_identity) and PROFILE_DETAIL. Only "ok" and, for the default profile,
# "new" may launch.
check_profile() {
    PROFILE_DETAIL=""
    if ! inside_workspace "$PROFILE"; then
        PROFILE_STATE=outside_workspace
        PROFILE_DETAIL="$PROFILE resolves to $(readlink -m "$PROFILE"), outside the persistent workspace $WORK_DIR; its login would not survive a container recreate"
        return
    fi
    if [ ! -e "$PROFILE" ] && [ ! -L "$PROFILE" ]; then
        if [ "$PROFILE_SOURCE" = declared ]; then
            PROFILE_STATE=missing
            PROFILE_DETAIL="declared profile $PROFILE does not exist; refusing to start a logged-out replacement"
        else
            PROFILE_STATE=new
            PROFILE_DETAIL="no profile yet; Chrome creates $PROFILE on first use"
        fi
        return
    fi
    if [ ! -d "$PROFILE" ]; then
        PROFILE_STATE=inconsistent
        PROFILE_DETAIL="$PROFILE is not a directory"
        return
    fi
    if [ -e "$PROFILE/Local State" ] && ! json_file_valid "$PROFILE/Local State"; then
        PROFILE_STATE=inconsistent
        PROFILE_DETAIL="$PROFILE/Local State is not valid JSON; the profile may be damaged"
        return
    fi
    if [ "$PROFILE_SOURCE" = declared ]; then
        local id
        if [ ! -e "$PROFILE/Local State" ]; then
            PROFILE_STATE=inconsistent
            PROFILE_DETAIL="declared profile $PROFILE has no Local State; it is empty or was replaced"
            return
        fi
        if [ ! -f "$PROFILE/$IDENTITY_FILE" ]; then
            PROFILE_STATE=wrong_identity
            PROFILE_DETAIL="$PROFILE has no $IDENTITY_FILE; it is not the declared profile"
            return
        fi
        id=$(jq -r '.profile_id // ""' "$PROFILE/$IDENTITY_FILE" 2>/dev/null || true)
        if [ "$id" != "$PROFILE_ID" ]; then
            PROFILE_STATE=wrong_identity
            PROFILE_DETAIL="$PROFILE has identity \"$id\", the declaration expects \"$PROFILE_ID\""
            return
        fi
    fi
    PROFILE_STATE=ok
}

# Sets MOUNT_STATE (mounted / not_mounted): whether the workspace is its own
# mount, i.e. the persistent bind mount and not the container's overlay.
check_mount() {
    local root
    root=$(readlink -f "$WORK_DIR" 2>/dev/null || printf '%s' "$WORK_DIR")
    if awk -v d="$root" '$5 == d { found = 1 } END { exit !found }' "$MOUNTINFO" 2>/dev/null; then
        MOUNT_STATE=mounted
    else
        MOUNT_STATE=not_mounted
    fi
}

# ---------------------------------------------------------------------------
# Ownership

is_chrome_pid() {
    local pid="$1" stat comm
    [[ "$pid" =~ ^[0-9]+$ ]] || return 1
    stat=$(ps -o stat= -p "$pid" 2>/dev/null) || return 1
    # A killed Chrome stays a zombie until a long-lived parent (MCP server,
    # relay) reaps it, which may be never: a zombie is dead.
    [[ "$stat" != Z* ]] || return 1
    # PIDs restart from 1 in every container, and the container hostname is
    # stable for a session, so a recreated container can give the old owner's
    # PID to an unrelated process. Only a Chrome counts as the owner. Chrome
    # rewrites its command line, so its name comes from comm.
    comm=$(cat "/proc/$pid/comm" 2>/dev/null) || return 1
    case "$comm" in
        *chrome*|*chromium*) return 0 ;;
    esac
    return 1
}

# Sets OWNER_STATE (none, live, stale) and OWNER_PID / OWNER_DETAIL from the
# profile's SingletonLock ("<hostname>-<pid>").
check_owner() {
    local target host
    OWNER_PID=""
    OWNER_DETAIL=""
    if ! target=$(readlink "$PROFILE/SingletonLock" 2>/dev/null); then
        OWNER_STATE=none
        return
    fi
    host="${target%-*}"
    OWNER_PID="${target##*-}"
    if [ "$host" != "$(hostname)" ]; then
        # Written in another container. The workspace is mounted by one
        # container at a time (Hydra fences bot homes and session
        # workspaces), and that container has been replaced.
        OWNER_STATE=stale
        OWNER_DETAIL="lock written by container $host"
    elif is_chrome_pid "$OWNER_PID"; then
        OWNER_STATE=live
    else
        OWNER_STATE=stale
        OWNER_DETAIL="lock owner pid $OWNER_PID is not a running Chrome"
    fi
}

cdp_ready() {
    curl -fsS --max-time 2 "$CHROME_INTERNAL_URL/json/version" 2>/dev/null
}

# The browser Helix launched: "<pid> <profile>". Its profile is verified
# against the configured one before anyone is connected to it.
launched_pid() {
    local pid profile
    [ -f "$STATE_FILE" ] || return 1
    read -r pid profile < "$STATE_FILE" || return 1
    [ "$profile" = "$PROFILE" ] || return 1
    printf '%s\n' "$pid"
}

wait_for_cdp() {
    local i
    for i in $(seq 1 $((START_TIMEOUT * 5))); do
        cdp_ready >/dev/null && return 0
        sleep 0.2
    done
    return 1
}

# ---------------------------------------------------------------------------
# Launch

# Split the MCP server's arguments: browser options become Chrome flags.
parse_server_args() {
    REQUESTED_PROFILE=""
    VIEWPORT="1280x800"
    CHROME_FLAGS=()
    MCP_ARGS=()
    while [ $# -gt 0 ]; do
        case "$1" in
            --user-data-dir=*) REQUESTED_PROFILE="${1#*=}" ;;
            --user-data-dir) REQUESTED_PROFILE="$2"; shift ;;
            --viewport=*) VIEWPORT="${1#*=}" ;;
            --viewport) VIEWPORT="$2"; shift ;;
            --chrome-arg=*) CHROME_FLAGS+=("${1#*=}") ;;
            --headless) ;;
            *) MCP_ARGS+=("$1") ;;
        esac
        shift
    done
}

server_args_from_env() {
    local args=()
    [ -z "${HELIX_CHROME_SERVER_ARGS:-}" ] || mapfile -t args <<< "$HELIX_CHROME_SERVER_ARGS"
    parse_server_args "${args[@]}"
}

# Coding harnesses pass MCP servers a minimal environment. Restore the
# graphical-session variables, choosing the newest compositor socket.
detect_display() {
    export XDG_RUNTIME_DIR="${XDG_RUNTIME_DIR:-/run/user/$(id -u)}"
    local socket
    if [ -z "${WAYLAND_DISPLAY:-}" ]; then
        for socket in "$XDG_RUNTIME_DIR"/wayland-*; do
            [ -S "$socket" ] || continue
            WAYLAND_DISPLAY="${socket##*/}"
        done
    fi
    # Headless sandboxes run no compositor. Desktop sessions always have a
    # socket by the time anything asks for a browser.
    HEADLESS=true
    if [ -n "${WAYLAND_DISPLAY:-}" ] && [ -S "$XDG_RUNTIME_DIR/$WAYLAND_DISPLAY" ]; then
        HEADLESS=false
        export WAYLAND_DISPLAY
    fi
}

# The user agent a regular Chrome of this build sends (major version only).
headless_user_agent() {
    local major
    major=$("$CHROME_BIN" --version 2>/dev/null | grep -oE '[0-9]+\.[0-9.]+' | head -1 | cut -d. -f1)
    [ -n "$major" ] || return 1
    echo "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/${major}.0.0.0 Safari/537.36"
}

launch_chrome() {
    local width="${VIEWPORT%x*}" height="${VIEWPORT#*x}" flag ua ua_set=false
    local flags=(
        "--user-data-dir=$PROFILE"
        "--remote-debugging-port=$CHROME_INTERNAL_PORT"
        "--remote-debugging-address=127.0.0.1"
        "--window-size=${width},$((height + 80))"
        "--no-default-browser-check"
        # The first-run dialog blocks the DevTools endpoint until dismissed.
        "--no-first-run"
        # Whoever starts the browser (MCP, desktop, login check), it is the
        # same browser: less detectable as automation, no extension probing.
        "--disable-blink-features=AutomationControlled"
        "--disable-infobars"
        "--disable-extensions"
    )
    detect_display
    if $HEADLESS; then
        flags+=("--headless=new")
        # Headless Chrome announces "HeadlessChrome", which bot detection
        # may treat differently from the desktop browser.
        for flag in "${CHROME_FLAGS[@]}"; do
            [[ "$flag" == --user-agent=* ]] && ua_set=true
        done
        if ! $ua_set && ua=$(headless_user_agent); then
            flags+=("--user-agent=$ua")
        fi
    else
        flags+=("--ozone-platform=wayland")
    fi
    # The MCP server's own browser options, minus those already set here.
    for flag in "${CHROME_FLAGS[@]}"; do
        [ "$flag" != "--ozone-platform=wayland" ] || continue
        [[ " ${flags[*]} " == *" $flag "* ]] || flags+=("$flag")
    done
    # Detached: the browser outlives whatever asked for it, and must not
    # inherit the lock descriptor.
    setsid "$CHROME_BIN" "${flags[@]}" about:blank >"$CHROME_LOG" 2>&1 < /dev/null 9>&- &
    printf '%s %s\n' "$!" "$PROFILE" > "$STATE_FILE"
}

# Brings up Chrome on the configured profile, or fails saying why. Caller
# holds the lock.
ensure_locked() {
    local pid
    check_profile
    case "$PROFILE_STATE" in
        ok|new) ;;
        *) die "$PROFILE_DETAIL" ;;
    esac
    check_mount
    if [ "$MOUNT_STATE" != mounted ]; then
        # A declared profile exists to keep a login; on the container overlay
        # it would be lost on the next recreate.
        [ "$PROFILE_SOURCE" != declared ] \
            || die "$WORK_DIR is not the persistent workspace mount; refusing to use the declared profile"
        log "warning: $WORK_DIR is not a mount; $PROFILE will not survive a container recreate"
    fi
    check_owner

    if cdp_ready >/dev/null; then
        pid=$(launched_pid) || pid=""
        if [ "$OWNER_STATE" = live ] && [ "$OWNER_PID" = "$pid" ]; then
            return 0
        fi
        die "a browser already answers on $CHROME_INTERNAL_URL but does not own $PROFILE; run: helix-browser stop"
    fi

    case "$OWNER_STATE" in
        live)
            # Starting up or stalled: give it time. Never a second browser.
            wait_for_cdp && [ "$(launched_pid || true)" = "$OWNER_PID" ] && return 0
            die "Chrome (pid $OWNER_PID) owns $PROFILE without a Helix DevTools endpoint; close that window and retry (a second browser would corrupt the profile)"
            ;;
        stale)
            # Only Chrome's lock files: the profile itself is never touched.
            log "clearing stale profile lock on $PROFILE ($OWNER_DETAIL)"
            rm -f "$PROFILE"/Singleton{Lock,Cookie,Socket}
            ;;
    esac

    if [ "$PROFILE_STATE" = new ]; then
        mkdir -p "$PROFILE"
    fi
    launch_chrome
    wait_for_cdp && return 0
    die "Chrome did not open $CHROME_INTERNAL_URL; see $CHROME_LOG"
}

# Several MCP servers, relays and desktop launches can ask at once; only one
# may decide whether to launch.
with_lock() {
    exec 9>"$LOCK_FILE"
    flock 9
    "$@"
    flock -u 9
    exec 9>&-
}

cmd_ensure() {
    server_args_from_env
    resolve_profile "$REQUESTED_PROFILE"
    if [ "$PROFILE_SOURCE" = declared ] && [ -n "$REQUESTED_PROFILE" ] \
        && [ "$(readlink -m "$REQUESTED_PROFILE")" != "$(readlink -m "$PROFILE")" ]; then
        log "using declared profile $PROFILE instead of requested $REQUESTED_PROFILE"
    fi
    with_lock ensure_locked
}

# ---------------------------------------------------------------------------
# Debugging-port listener

listener_pid() {
    ss -Hltnp "sport = :$CHROME_DEBUG_PORT" 2>/dev/null | grep -oE 'pid=[0-9]+' | head -1 | cut -d= -f2
}

listener_running() {
    ss -Hltn "sport = :$CHROME_DEBUG_PORT" 2>/dev/null | grep -q .
}

# Only Helix's relay may hold the shared port: anything else there (a script
# that launched its own Chrome) can be a different profile.
listener_is_relay() {
    local pid
    pid=$(listener_pid)
    [ -n "$pid" ] || return 1
    tr '\0' ' ' < "/proc/$pid/cmdline" 2>/dev/null | grep -q -- "EXEC:$SELF connect"
}

relay_locked() {
    if listener_running; then
        listener_is_relay && return 0
        die "127.0.0.1:$CHROME_DEBUG_PORT is held by something other than the Helix browser relay (pid $(listener_pid)); it may be a browser on another profile. Stop it so browser tools use the configured profile."
    fi
    # Detached, and without the lock descriptor. The listener keeps the
    # arguments of the server that started it for the browser it launches.
    HELIX_CHROME_SERVER_ARGS="$(printf '%s\n' "$@")" setsid socat \
        "TCP-LISTEN:$CHROME_DEBUG_PORT,bind=127.0.0.1,reuseaddr,fork" \
        "EXEC:$SELF connect" >"$RELAY_LOG" 2>&1 < /dev/null 9>&- &
    local i
    for i in $(seq 1 50); do
        listener_running && return 0
        sleep 0.1
    done
    die "nothing listens on 127.0.0.1:$CHROME_DEBUG_PORT; see $RELAY_LOG"
}

cmd_relay() {
    with_lock relay_locked "$@"
}

cmd_connect() {
    cmd_ensure
    exec socat STDIO "TCP:127.0.0.1:$CHROME_INTERNAL_PORT"
}

# ---------------------------------------------------------------------------
# Desktop and operator commands

cmd_open() {
    # Through the relay when it exists, so the browser gets the same flags
    # the MCP servers configured; otherwise directly.
    local err
    if listener_running && listener_is_relay \
        && curl -fsS --max-time "$((START_TIMEOUT + 5))" "http://127.0.0.1:$CHROME_DEBUG_PORT/json/version" >/dev/null 2>&1; then
        :
    elif ! err=$( (cmd_ensure) 2>&1 >/dev/null); then
        # Launched from the dock or a link: stderr goes nowhere visible.
        command -v notify-send >/dev/null && notify-send -u critical "Browser not started" "${err#helix-browser: }"
        die "${err#helix-browser: }"
    fi
    resolve_profile
    detect_display
    # Chrome hands the request to the running browser on this profile and
    # exits, so this never becomes a second browser.
    exec "$CHROME_BIN" "--user-data-dir=$PROFILE" "$@"
}

# Closes the browser Helix launched, whatever its profile, or a Chrome that
# owns the configured profile.
cmd_stop() {
    local pid="" profile i
    if [ -f "$STATE_FILE" ]; then
        read -r pid profile < "$STATE_FILE" || pid=""
        is_chrome_pid "$pid" || pid=""
    fi
    if [ -z "$pid" ]; then
        resolve_profile
        check_owner
        [ "$OWNER_STATE" = live ] || { log "no browser is running"; rm -f "$STATE_FILE"; return 0; }
        pid="$OWNER_PID"
    fi
    # TERM lets Chrome flush its databases and release the profile lock.
    kill -TERM "$pid"
    for i in $(seq 1 100); do
        is_chrome_pid "$pid" || { rm -f "$STATE_FILE"; return 0; }
        sleep 0.2
    done
    die "Chrome (pid $pid) did not exit; its profile lock stays until it does"
}

new_id() {
    cat /proc/sys/kernel/random/uuid
}

write_json_atomic() {
    local dest="$1" tmp
    tmp=$(mktemp "$dest.XXXXXX")
    cat > "$tmp"
    chmod 644 "$tmp"
    mv -f "$tmp" "$dest"
}

cmd_declare() {
    local rel="" replace=false path current_rel
    DECLARE_LOGIN_URL="" DECLARE_LOGGED_IN_TITLE=""
    DECLARE_LOGGED_OUT=() DECLARE_LOGGED_IN=()
    while [ $# -gt 0 ]; do
        case "$1" in
            --login-url) DECLARE_LOGIN_URL="$2"; shift ;;
            --logged-out-match) DECLARE_LOGGED_OUT+=("$2"); shift ;;
            --logged-in-match) DECLARE_LOGGED_IN+=("$2"); shift ;;
            --logged-in-title) DECLARE_LOGGED_IN_TITLE="$2"; shift ;;
            --replace) replace=true ;;
            -*) die "unknown option $1" ;;
            *) [ -z "$rel" ] || die "one profile directory"; rel="$1" ;;
        esac
        shift
    done
    [ -n "$rel" ] || die "usage: helix-browser declare DIR [--login-url URL --logged-out-match TEXT... --logged-in-title TEXT [--logged-in-match TEXT...]] [--replace]"
    rel="${rel#"$WORK_DIR"/}"
    path=$(workspace_path "$rel") || die "$rel must be a directory inside $WORK_DIR"
    [ -d "$path" ] || die "$path does not exist; declare an existing profile (Helix never creates one here)"
    inside_workspace "$path" \
        || die "$path resolves to $(readlink -m "$path"), outside the persistent workspace $WORK_DIR"
    json_file_valid "$path/Local State" || die "$path is not a Chrome profile (no valid Local State)"
    if [ -n "$DECLARE_LOGIN_URL" ]; then
        [[ "$DECLARE_LOGIN_URL" =~ ^https?:// ]] || die "--login-url must be an http(s) URL"
        [ ${#DECLARE_LOGGED_OUT[@]} -gt 0 ] \
            || die "--login-url needs at least one --logged-out-match (text in the URL a logged-out visit lands on)"
        # The URL alone is not evidence: Chrome's error pages keep the
        # requested URL. The title comes from the page the site served.
        [ -n "$DECLARE_LOGGED_IN_TITLE" ] \
            || die "--login-url needs --logged-in-title (text in the title of the page a logged-in visit shows)"
    elif [ ${#DECLARE_LOGGED_OUT[@]} -gt 0 ] || [ ${#DECLARE_LOGGED_IN[@]} -gt 0 ] || [ -n "$DECLARE_LOGGED_IN_TITLE" ]; then
        die "login matches need --login-url"
    fi
    if [ -e "$DECLARATION" ]; then
        current_rel=$(jq -r '.profile // ""' "$DECLARATION" 2>/dev/null || true)
        if [ "$current_rel" != "$rel" ] && ! $replace; then
            die "the configured profile is \"$current_rel\"; pass --replace to switch to \"$rel\""
        fi
    fi

    with_lock declare_locked "$rel" "$path"
}

declare_locked() {
    local rel="$1" path="$2" id
    # Keep an existing identity: re-declaring the same profile is a no-op for
    # anything that already recorded it.
    if [ -f "$path/$IDENTITY_FILE" ]; then
        id=$(jq -r '.profile_id // ""' "$path/$IDENTITY_FILE")
        [ -n "$id" ] || die "$path/$IDENTITY_FILE has no profile_id"
    else
        id=$(new_id)
        jq -n --arg id "$id" --arg at "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
            '{profile_id: $id, declared_at: $at}' | write_json_atomic "$path/$IDENTITY_FILE"
    fi
    mkdir -p "$(dirname "$DECLARATION")"
    jq -n --arg profile "$rel" --arg id "$id" --arg url "$DECLARE_LOGIN_URL" \
        --arg title "$DECLARE_LOGGED_IN_TITLE" \
        --argjson out "$(json_array "${DECLARE_LOGGED_OUT[@]}")" \
        --argjson in "$(json_array "${DECLARE_LOGGED_IN[@]}")" \
        '{profile: $profile, profile_id: $id}
            + (if $url == "" then {} else {login_check: (
                {url: $url, logged_out_url_contains: $out}
                + (if ($in | length) > 0 then {logged_in_url_contains: $in} else {} end)
                + (if $title != "" then {logged_in_title_contains: $title} else {} end))} end)' \
        | write_json_atomic "$DECLARATION"
    echo "configured browser profile: $path (id $id)"
    echo "a running browser on another profile keeps running until: helix-browser stop"
}

json_array() {
    jq -n '$ARGS.positional' --args "$@"
}

# Origin and path only: query strings and fragments can carry tokens.
redact_url() {
    sed -E 's/[?#].*$//' <<< "$1"
}

# Opens the declared login URL in a new tab, waits for redirects to settle and
# classifies where it landed. Reads URL and title from DevTools' HTTP target
# list only. logged_in needs positive evidence: an http(s) page at the
# declared URL (or a declared logged-in URL) whose title contains the
# declared logged-in title. Chrome's error pages keep the requested URL and
# are titled with the bare host, so they, and anywhere unexpected, are
# unknown.
cmd_login_check() {
    local target id url="" title="" prev="" stable=0 state reason i match landed expected entry
    resolve_profile
    [ -n "$LOGIN_URL" ] || die "no login check declared; run: helix-browser declare <profile> --login-url URL --logged-out-match TEXT --logged-in-title TEXT"
    [ -n "$LOGGED_IN_TITLE" ] || die "the login check has no logged-in title, so it cannot prove a login; re-declare with --logged-in-title TEXT"
    cmd_ensure
    # Chrome percent-decodes the whole query; a raw & or % would cut the URL.
    target=$(curl -fsS --max-time 10 -X PUT "$CHROME_INTERNAL_URL/json/new?$(jq -rn --arg u "$LOGIN_URL" '$u|@uri')") \
        || die "could not open a tab for the login check"
    id=$(jq -r '.id' <<< "$target")
    for i in $(seq 1 30); do
        sleep 1
        entry=$(curl -fsS --max-time 2 "$CHROME_INTERNAL_URL/json/list" | jq -c --arg id "$id" '.[] | select(.id == $id)')
        [ -n "$entry" ] || entry='{}'
        url=$(jq -r '.url // ""' <<< "$entry")
        title=$(jq -r '.title // ""' <<< "$entry")
        if [ -n "$url" ] && [ "$url" = "$prev" ] && [ "$url" != "about:blank" ]; then
            stable=$((stable + 1))
            [ $stable -ge 2 ] && break
        else
            stable=0
        fi
        prev="$url"
    done
    curl -fsS --max-time 5 "$CHROME_INTERNAL_URL/json/close/$id" >/dev/null 2>&1 || true

    landed=$(redact_url "$url")
    expected=$(redact_url "$LOGIN_URL")
    state=unknown
    if [ -z "$url" ] || [ $stable -lt 2 ]; then
        reason="the page did not settle"
    elif ! [[ "$url" =~ ^https?:// ]] || [ "$title" = "$(sed -E 's#^https?://([^/:]+).*#\1#' <<< "$url")" ]; then
        reason="landed on a browser or error page, not the site"
    else
        reason="landed somewhere the declaration does not expect"
        for match in "${LOGGED_OUT_MATCH[@]}"; do
            if [[ "$url" == *"$match"* ]]; then
                state=logged_out
                reason="landed on the declared logged-out page"
            fi
        done
        if [ "$state" = unknown ]; then
            local at_expected=false
            [ "${landed%/}" = "${expected%/}" ] && at_expected=true
            for match in "${LOGGED_IN_MATCH[@]}"; do
                [[ "$url" == *"$match"* ]] && at_expected=true
            done
            if ! $at_expected; then
                :
            elif [[ "$title" != *"$LOGGED_IN_TITLE"* ]]; then
                reason="at the expected page without the declared logged-in title"
            else
                state=logged_in
                reason="at the expected page with the declared logged-in title"
            fi
        fi
    fi
    jq -n --arg state "$state" --arg landed "$landed" --arg reason "$reason" \
        '{login: {state: $state, landed: $landed, reason: $reason}}'
    [ "$state" = logged_in ]
}

# ---------------------------------------------------------------------------
# Status: separate results, no side effects

cmd_status() {
    local as_json=false process cdp_state browser="" listener=absent pid
    [ "${1:-}" = "--json" ] && as_json=true
    resolve_profile
    check_profile
    check_mount
    check_owner
    case "$OWNER_STATE" in
        live) process=running ;;
        stale) process=stopped_with_stale_lock ;;
        *) process=stopped ;;
    esac
    if version=$(cdp_ready); then
        cdp_state=ready
        browser=$(jq -r '.Browser // ""' <<< "$version")
        pid=$(launched_pid || true)
        if [ "$OWNER_STATE" != live ] || [ "$OWNER_PID" != "$pid" ]; then
            cdp_state=other_profile
        fi
    elif [ "$OWNER_STATE" = live ]; then
        cdp_state=not_answering
    else
        cdp_state=not_running
    fi
    if listener_running; then
        if listener_is_relay; then listener=relay; else listener=foreign; fi
    fi
    local login=not_configured
    [ -z "$LOGIN_URL" ] || login=not_checked
    local out
    out=$(jq -n \
        --arg path "$PROFILE" --arg source "$PROFILE_SOURCE" --arg pstate "$PROFILE_STATE" \
        --arg pdetail "$PROFILE_DETAIL" --arg pid_decl "$PROFILE_ID" \
        --argjson local_state "$( [ -f "$PROFILE/Local State" ] && echo true || echo false )" \
        --argjson cookies_db "$( { [ -f "$PROFILE/Default/Cookies" ] || [ -f "$PROFILE/Default/Network/Cookies" ]; } && echo true || echo false )" \
        --arg work "$WORK_DIR" --arg mount "$MOUNT_STATE" \
        --arg process "$process" --arg owner_pid "$OWNER_PID" --arg owner_detail "$OWNER_DETAIL" \
        --arg cdp "$cdp_state" --arg browser "$browser" --arg listener "$listener" \
        --arg login "$login" \
        '{
          profile: {path: $path, source: $source, state: $pstate, detail: $pdetail,
                    profile_id: $pid_decl, local_state_present: $local_state, cookies_db_present: $cookies_db},
          mount: {workspace: $work, state: $mount},
          process: {state: $process, pid: $owner_pid, detail: $owner_detail},
          cdp: {state: $cdp, browser: $browser, debug_port_listener: $listener},
          login: {state: $login}
        }')
    if $as_json; then
        printf '%s\n' "$out"
    else
        jq -r '"profile: \(.profile.state) (\(.profile.source)) \(.profile.path)\(if .profile.detail != "" then " — " + .profile.detail else "" end)",
               "mount:   \(.mount.state) \(.mount.workspace)",
               "process: \(.process.state)\(if .process.pid != "" then " pid " + .process.pid else "" end)\(if .process.detail != "" then " — " + .process.detail else "" end)",
               "cdp:     \(.cdp.state)\(if .cdp.browser != "" then " " + .cdp.browser else "" end) (debug port: \(.cdp.debug_port_listener))",
               "login:   \(.login.state)\(if .login.state == "not_checked" then " (run: helix-browser login-check)" else "" end)"' <<< "$out"
    fi
}

# Prints the profile the desktop's default Chrome paths should point at, or
# fails when the configured profile cannot be used.
cmd_profile_path() {
    resolve_profile
    check_profile
    case "$PROFILE_STATE" in
        ok|new) printf '%s\n' "$PROFILE" ;;
        *) die "$PROFILE_DETAIL" ;;
    esac
}

cmd="${1:-}"
[ $# -eq 0 ] || shift
case "$cmd" in
    status) cmd_status "$@" ;;
    ensure)
        [ $# -eq 0 ] || export HELIX_CHROME_SERVER_ARGS="$(printf '%s\n' "$@")"
        cmd_ensure ;;
    open) cmd_open "$@" ;;
    connect) cmd_connect ;;
    relay) cmd_relay "$@" ;;
    declare) cmd_declare "$@" ;;
    login-check) cmd_login_check ;;
    stop) cmd_stop ;;
    profile-path) cmd_profile_path ;;
    *) sed -n '2,20p' "$SELF" | sed 's/^# \{0,1\}//' >&2; exit 2 ;;
esac
