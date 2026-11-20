#!/bin/bash
#
# Tests helix-browser (profile resolution, single owner, lazy start, resume)
# against a fake Chrome that keeps Chrome's profile lock and DevTools HTTP API.
#
# Usage: ./test-helix-browser.sh   (needs bash, jq, socat, curl, flock, ss, ps)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TMP=$(mktemp -d)
BROWSER="$SCRIPT_DIR/helix-browser.sh"

export HELIX_BROWSER_WORK_DIR="$TMP/work"
export HELIX_BROWSER_RUN_DIR="$TMP/run"
export HELIX_BROWSER_MOUNTINFO="$TMP/mountinfo"
export HELIX_BROWSER_START_TIMEOUT=5
export CHROME_PATH="$TMP/bin/fake-chrome"
export FAKE_DIR="$TMP/fake"
export XDG_RUNTIME_DIR="$TMP/xdg"
unset WAYLAND_DISPLAY
port_base=$((20000 + RANDOM % 20000))
export HELIX_CHROME_DEBUG_PORT=$port_base HELIX_CHROME_INTERNAL_PORT=$((port_base + 1))
INTERNAL="http://127.0.0.1:$HELIX_CHROME_INTERNAL_PORT"

cleanup() {
    pkill -f "$TMP/" 2>/dev/null || true
    jobs -p | xargs -r kill 2>/dev/null || true
    rm -rf "$TMP"
}
trap cleanup EXIT

mkdir -p "$HELIX_BROWSER_WORK_DIR" "$HELIX_BROWSER_RUN_DIR" "$TMP/bin" "$FAKE_DIR" "$XDG_RUNTIME_DIR"
mounted() { printf '1 0 0:1 / %s rw - ext4 /dev/x rw\n' "$(readlink -f "$HELIX_BROWSER_WORK_DIR")" > "$HELIX_BROWSER_MOUNTINFO"; }
unmounted() { printf '1 0 0:1 / / rw - overlay overlay rw\n' > "$HELIX_BROWSER_MOUNTINFO"; }
mounted

# DevTools HTTP endpoints. /json/new remembers the URL; /json/list reports it,
# or a login page when the fake site has no session, or the landing page and
# title a test sets.
cat > "$TMP/bin/fake-devtools" <<'EOF'
#!/bin/bash
read -r _ path _
reply() { printf 'HTTP/1.0 200 OK\r\nContent-Type: application/json\r\n\r\n%s' "$1"; }
case "$path" in
    /json/version) reply '{"Browser":"FakeChrome/1.0"}' ;;
    /json/new\?*)
        # Chrome percent-decodes the query.
        q=${path#/json/new?}; printf '%b' "${q//%/\\x}" > "$FAKE_DIR/tab"; cp "$FAKE_DIR/tab" "$FAKE_DIR/opened"
        reply '{"id":"T1"}' ;;
    /json/list)
        url=$(cat "$FAKE_DIR/tab" 2>/dev/null || echo about:blank)
        [ -f "$FAKE_DIR/session" ] || url="https://site.test/login?next=secret-token"
        [ ! -f "$FAKE_DIR/landing" ] || url=$(cat "$FAKE_DIR/landing")
        title=$(cat "$FAKE_DIR/title" 2>/dev/null || echo Home)
        reply "[{\"id\":\"T1\",\"url\":\"$url\",\"title\":\"$title\"}]" ;;
    /json/close/*) rm -f "$FAKE_DIR/tab"; reply '{}' ;;
    *) reply '{}' ;;
esac
EOF
# Chrome's singleton: a second process on an owned profile hands its
# arguments to the owner and exits.
cat > "$TMP/bin/fake-chrome" <<'EOF'
#!/bin/bash
[ "${1:-}" != --version ] || { echo "Fake Chrome 152.0.1.2"; exit 0; }
profile="" port=""
for a in "$@"; do
    case "$a" in
        --user-data-dir=*) profile="${a#*=}" ;;
        --remote-debugging-port=*) port="${a#*=}" ;;
    esac
done
target=$(readlink "$profile/SingletonLock" 2>/dev/null || true)
if [ -n "$target" ] && [ "${target%-*}" = "$(hostname)" ] && kill -0 "${target##*-}" 2>/dev/null; then
    printf '%s\n' "$*" >> "$FAKE_DIR/handoffs"
    exit 0
fi
# Like Chrome: a dead owner on this host is taken over, another host's is not.
if [ -n "$target" ] && [ "${target%-*}" = "$(hostname)" ]; then
    rm -f "$profile/SingletonLock"
elif [ -n "$target" ]; then
    echo "profile in use by ${target%-*}" >&2; exit 21
fi
mkdir -p "$profile"
[ -f "$profile/Local State" ] || echo '{"profile":{}}' > "$profile/Local State"
ln -s "$(hostname)-$$" "$profile/SingletonLock"
printf '%s\n' "$profile" >> "$FAKE_DIR/launches"
printf '%s\n' "$*" > "$FAKE_DIR/last-args"
socat_pid=""
trap 'kill $socat_pid 2>/dev/null; rm -f "$profile/SingletonLock"; exit 0' TERM
if [ -n "$port" ] && [ -z "${FAKE_NO_CDP:-}" ]; then
    socat "TCP-LISTEN:$port,bind=127.0.0.1,reuseaddr,fork" "EXEC:$(dirname "$0")/fake-devtools" &
    socat_pid=$!
fi
while :; do sleep 1 & wait $!; done
EOF
chmod +x "$TMP/bin/fake-chrome" "$TMP/bin/fake-devtools"

pass=0
check() {
    local name="$1" expected="$2" actual="$3"
    if [ "$actual" != "$expected" ]; then
        echo "FAIL: $name: expected '$expected', got '$actual'"
        exit 1
    fi
    pass=$((pass + 1))
    echo "ok: $name"
}
contains() {
    local name="$1" needle="$2" haystack="$3"
    if [[ "$haystack" != *"$needle"* ]]; then
        echo "FAIL: $name: '$needle' not in: $haystack"
        exit 1
    fi
    pass=$((pass + 1))
    echo "ok: $name"
}
launches() { [ -f "$FAKE_DIR/launches" ] && wc -l < "$FAKE_DIR/launches" | tr -d ' ' || echo 0; }
status() { "$BROWSER" status --json | jq -r "$1"; }
chrome_pid() { readlink "$1/SingletonLock" | sed 's/.*-//'; }
wait_gone() { for _ in $(seq 1 50); do kill -0 "$1" 2>/dev/null || return 0; sleep 0.1; done; return 1; }
kill_chrome() {
    pkill -f "fake-devtools" 2>/dev/null || true
    pkill -KILL -f "$TMP/bin/fake-chrome" 2>/dev/null || true
    pkill -f "TCP-LISTEN:$HELIX_CHROME_INTERNAL_PORT" 2>/dev/null || true
    for _ in $(seq 1 50); do curl -fs --max-time 1 "$INTERNAL/json/version" >/dev/null 2>&1 || return 0; sleep 0.1; done
}
DEFAULT="$HELIX_BROWSER_WORK_DIR/.chrome-state"
MCP_ARGS=(--user-data-dir=$DEFAULT --viewport 1280x800 --chrome-arg=--no-first-run --chrome-arg=--lang=en-US)

# --- lazy start -------------------------------------------------------------
check "status reports no browser" "stopped not_running new" "$(status '"\(.process.state) \(.cdp.state) \(.profile.state)"')"
"$BROWSER" relay "${MCP_ARGS[@]}"
check "relay listens without starting Chrome" "0 relay" "$(launches) $(status .cdp.debug_port_listener)"
"$BROWSER" relay "${MCP_ARGS[@]}"
check "second MCP server reuses the relay" "1" "$(pgrep -fc "TCP-LISTEN:$HELIX_CHROME_DEBUG_PORT")"
check "status and relay leave the profile alone" "no" "$([ -e "$DEFAULT" ] && echo yes || echo no)"

# --- one owner under concurrency ---------------------------------------------
for _ in $(seq 1 8); do
    curl -fsS --max-time 15 "http://127.0.0.1:$HELIX_CHROME_DEBUG_PORT/json/version" > "$TMP/c.$RANDOM.$BASHPID" &
done
wait
check "8 concurrent browser requests launch one Chrome" "1" "$(launches)"
contains "the MCP server's browser options reach Chrome" " --lang=en-US " " $(cat "$FAKE_DIR/last-args") "
check "a flag set by both is passed once" "1" "$(tr ' ' '\n' < "$FAKE_DIR/last-args" | grep -cx -- --no-first-run)"
check "every request reached it" "8" "$(cat "$TMP"/c.* | grep -o FakeChrome | wc -l | tr -d " ")"
for _ in $(seq 1 4); do "$BROWSER" ensure "${MCP_ARGS[@]}" & done
wait
check "concurrent ensure keeps one owner" "1" "$(launches)"
check "status: separate results" "running ready ok mounted not_configured" \
    "$(status '"\(.process.state) \(.cdp.state) \(.profile.state) \(.mount.state) \(.login.state)"')"
echo canary-1 > "$DEFAULT/canary.txt"

# --- resume after death -------------------------------------------------------
kill_chrome
check "dead Chrome: stale lock reported, nothing started" "stopped_with_stale_lock not_running 1" \
    "$(status '"\(.process.state) \(.cdp.state)"') $(launches)"
"$BROWSER" ensure "${MCP_ARGS[@]}" 2>"$TMP/err"
contains "stale lock is cleared with a reason" "is not a running Chrome" "$(cat "$TMP/err")"
check "resumed on the same profile" "2 $DEFAULT canary-1" "$(launches) $(tail -1 "$FAKE_DIR/launches") $(cat "$DEFAULT/canary.txt")"

kill_chrome
ln -sfn "previous-container-4242" "$DEFAULT/SingletonLock"
"$BROWSER" ensure 2>"$TMP/err"
contains "lock from a replaced container is stale" "lock written by container previous-container" "$(cat "$TMP/err")"

kill_chrome
sleep 300 & reused=$!
ln -sfn "$(hostname)-$reused" "$DEFAULT/SingletonLock"
"$BROWSER" ensure 2>"$TMP/err"
contains "a reused PID is not an owner" "pid $reused is not a running Chrome" "$(cat "$TMP/err")"
kill $reused
check "canary survives three resumes" "canary-1 4" "$(cat "$DEFAULT/canary.txt") $(launches)"

# A Chrome on the profile without Helix's DevTools endpoint (started by hand):
# reported, never joined by a second browser.
kill_chrome
before=$(launches)
FAKE_NO_CDP=1 setsid "$CHROME_PATH" "--user-data-dir=$DEFAULT" >/dev/null 2>&1 &
for _ in $(seq 1 50); do [ "$(launches)" -gt "$before" ] && break; sleep 0.1; done
before=$(launches)
manual=$(chrome_pid "$DEFAULT")
if "$BROWSER" ensure 2>"$TMP/err"; then echo "FAIL: ensure joined a foreign owner"; exit 1; fi
contains "foreign owner is reported" "owns $DEFAULT without a Helix DevTools endpoint" "$(cat "$TMP/err")"
check "no second browser on an owned profile" "$before $manual" "$(launches) $(chrome_pid "$DEFAULT")"
check "status: running but not answering" "running not_answering" "$(status '"\(.process.state) \(.cdp.state)"')"
kill -TERM "$manual"; wait_gone "$manual"

# --- declared custom profile --------------------------------------------------
CUSTOM="$HELIX_BROWSER_WORK_DIR/chrome-profile"
if "$BROWSER" declare chrome-profile 2>"$TMP/err"; then echo "FAIL: declared a missing profile"; exit 1; fi
contains "declare refuses a missing profile" "does not exist" "$(cat "$TMP/err")"
check "declare created nothing" "no" "$([ -e "$CUSTOM" ] && echo yes || echo no)"
mkdir -p "$CUSTOM/Default"
if "$BROWSER" declare chrome-profile 2>/dev/null; then echo "FAIL: declared a non-profile"; exit 1; fi
echo '{"profile":{"info_cache":{}}}' > "$CUSTOM/Local State"
echo profile-canary > "$CUSTOM/canary.txt"
if "$BROWSER" declare ../outside 2>/dev/null; then echo "FAIL: declared outside the workspace"; exit 1; fi
# A symlink in the workspace that leads off it (container overlay, another
# volume) is outside, whatever its name says.
OUTSIDE="$TMP/overlay/profile"
mkdir -p "$OUTSIDE"; cp "$CUSTOM/Local State" "$OUTSIDE/"
ln -s "$OUTSIDE" "$HELIX_BROWSER_WORK_DIR/linked-profile"
if "$BROWSER" declare linked-profile 2>"$TMP/err"; then echo "FAIL: declared a profile linked off the workspace"; exit 1; fi
contains "declare refuses a symlink out of the workspace" "outside the persistent workspace" "$(cat "$TMP/err")"
check "nothing written for it" "no no" \
    "$([ -e "$HELIX_BROWSER_WORK_DIR/.helix/browser.json" ] && echo yes || echo no) $([ -e "$OUTSIDE/.helix-profile.json" ] && echo yes || echo no)"
rm "$HELIX_BROWSER_WORK_DIR/linked-profile"
"$BROWSER" declare chrome-profile --login-url "https://site.test/home?x=1&y=a%2Fb" --logged-out-match /login --logged-in-title Home >/dev/null
id=$(jq -r .profile_id "$HELIX_BROWSER_WORK_DIR/.helix/browser.json")
check "identity written into the profile" "$id" "$(jq -r .profile_id "$CUSTOM/.helix-profile.json")"
"$BROWSER" declare chrome-profile --login-url "https://site.test/home?x=1&y=a%2Fb" --logged-out-match /login --logged-in-title Home >/dev/null
check "re-declaring keeps the identity" "$id" "$(jq -r .profile_id "$CUSTOM/.helix-profile.json")"
if "$BROWSER" declare .chrome-state 2>"$TMP/err"; then echo "FAIL: switched profile without --replace"; exit 1; fi
contains "switching needs --replace" "pass --replace" "$(cat "$TMP/err")"

before=$(launches)
"$BROWSER" ensure "${MCP_ARGS[@]}" 2>"$TMP/err"
check "MCP request for .chrome-state gets the declared profile" "$CUSTOM" "$(tail -1 "$FAKE_DIR/launches")"
contains "the override is logged" "using declared profile" "$(cat "$TMP/err")"
check "status: declared profile ok" "declared ok mounted running ready not_checked" \
    "$(status '"\(.profile.source) \(.profile.state) \(.mount.state) \(.process.state) \(.cdp.state) \(.login.state)"')"
check "profile-path for the desktop" "$CUSTOM" "$("$BROWSER" profile-path)"

# Login: classified by where the login URL lands; query strings never printed.
out=$("$BROWSER" login-check || true)
check "logged out: landed on the login page" "logged_out https://site.test/login" "$(jq -r '"\(.login.state) \(.login.landed)"' <<< "$out")"
check "no token in the output" "no" "$([[ "$out" == *secret-token* ]] && echo yes || echo no)"
touch "$FAKE_DIR/session"
check "logged in" "logged_in https://site.test/home" "$("$BROWSER" login-check | jq -r '"\(.login.state) \(.login.landed)"')"
check "the tab opened the declared URL intact (& and %)" "https://site.test/home?x=1&y=a%2Fb" "$(cat "$FAKE_DIR/opened")"

# logged_in needs positive evidence; anything else is unknown and fails.
login_state() {
    local out code=0
    out=$("$BROWSER" login-check 2>/dev/null) || code=$?
    echo "$(jq -r .login.state <<< "$out") $code"
}
echo "chrome-error://chromewebdata/" > "$FAKE_DIR/landing"
check "browser error page is unknown, not logged in" "unknown 1" "$(login_state)"
# Real Chrome keeps the requested URL on a network error page and titles it
# with the host.
rm "$FAKE_DIR/landing"; echo "site.test" > "$FAKE_DIR/title"
check "error page at the requested URL is unknown" "unknown 1" "$(login_state)"
rm "$FAKE_DIR/title"
if "$BROWSER" declare chrome-profile --login-url "https://site.test/home" --logged-out-match /login 2>"$TMP/err"; then
    echo "FAIL: declared a login check without a logged-in title"; exit 1
fi
contains "a login check needs a logged-in title" "needs --logged-in-title" "$(cat "$TMP/err")"
echo "https://other.test/home" > "$FAKE_DIR/landing"
check "another origin is unknown" "unknown 1" "$(login_state)"
echo "https://site.test/somewhere-else" > "$FAKE_DIR/landing"
check "an unexpected page on the site is unknown" "unknown 1" "$(login_state)"
"$BROWSER" declare chrome-profile --login-url "https://site.test/home?x=1&y=a%2Fb" --logged-out-match /login \
    --logged-in-title Home --logged-in-match /somewhere-else >/dev/null
check "a declared logged-in page counts" "logged_in 0" "$(login_state)"
rm "$FAKE_DIR/landing"
"$BROWSER" declare chrome-profile --login-url "https://site.test/home?x=1&y=a%2Fb" --logged-out-match /login \
    --logged-in-title "Inbox" >/dev/null
check "expected page without the declared title is unknown" "unknown 1" "$(login_state)"
echo "Inbox (3)" > "$FAKE_DIR/title"
check "expected page with the declared title is logged in" "logged_in 0" "$(login_state)"
rm "$FAKE_DIR/title"
if "$BROWSER" declare chrome-profile --login-url "file:///etc/passwd" --logged-out-match x 2>/dev/null; then
    echo "FAIL: accepted a non-http login URL"; exit 1
fi
"$BROWSER" declare chrome-profile --login-url "https://site.test/home?x=1&y=a%2Fb" --logged-out-match /login --logged-in-title Home >/dev/null
check "re-declaring the login check keeps the identity" "$id" "$(jq -r .profile_id "$HELIX_BROWSER_WORK_DIR/.helix/browser.json")"

# Open from the desktop: hand-off to the owner, never a second browser.
before=$(launches)
"$BROWSER" open https://example.test/
check "desktop open hands off to the running browser" "$before" "$(launches)"
contains "hand-off carries the declared profile and URL" "--user-data-dir=$CUSTOM https://example.test/" "$(tail -1 "$FAKE_DIR/handoffs")"

# Session replacement / rebuild: a new container has a new run dir, the old
# Chrome is gone and its lock names another host.
kill_chrome
ln -sfn "ubuntu-external-old-77" "$CUSTOM/SingletonLock"
export HELIX_BROWSER_RUN_DIR="$TMP/run2"; mkdir -p "$HELIX_BROWSER_RUN_DIR"
check "new container: nothing runs until asked" "stopped_with_stale_lock not_running ok" \
    "$(status '"\(.process.state) \(.cdp.state) \(.profile.state)"')"
"$BROWSER" ensure 2>/dev/null
check "new container resumes the declared profile and identity" "$CUSTOM profile-canary $id" \
    "$(tail -1 "$FAKE_DIR/launches") $(cat "$CUSTOM/canary.txt") $(jq -r .profile_id "$CUSTOM/.helix-profile.json")"
check "and the login" "logged_in" "$("$BROWSER" login-check | jq -r .login.state)"

# --- missing, replaced and damaged profiles fail without a replacement ---------
"$BROWSER" stop
check "stop closes the browser, keeps the profile" "stopped profile-canary" "$(status .process.state) $(cat "$CUSTOM/canary.txt")"
mv "$CUSTOM" "$TMP/moved-aside"
before=$(launches)
if "$BROWSER" ensure 2>"$TMP/err"; then echo "FAIL: started without the declared profile"; exit 1; fi
contains "missing profile fails clearly" "refusing to start a logged-out replacement" "$(cat "$TMP/err")"
check "nothing created, nothing launched" "no $before" "$([ -e "$CUSTOM" ] && echo yes || echo no) $(launches)"
check "status: missing" "missing" "$(status .profile.state)"
if "$BROWSER" profile-path 2>/dev/null; then echo "FAIL: profile-path for a missing profile"; exit 1; fi
if curl -fsS --max-time 10 "http://127.0.0.1:$HELIX_CHROME_DEBUG_PORT/json/version" >/dev/null 2>&1; then
    echo "FAIL: relay connected to a browser without the declared profile"; exit 1
fi
check "relay request fails instead of launching" "$before no" "$(launches) $([ -e "$CUSTOM" ] && echo yes || echo no)"

mkdir -p "$CUSTOM"; echo '{"profile":{}}' > "$CUSTOM/Local State"
if "$BROWSER" ensure 2>"$TMP/err"; then echo "FAIL: started an empty replacement"; exit 1; fi
contains "replacement without identity fails" "it is not the declared profile" "$(cat "$TMP/err")"
echo '{"profile_id":"someone-else"}' > "$CUSTOM/.helix-profile.json"
if "$BROWSER" ensure 2>"$TMP/err"; then echo "FAIL: started another profile"; exit 1; fi
contains "another profile's identity fails" "the declaration expects \"$id\"" "$(cat "$TMP/err")"
check "status: wrong identity" "wrong_identity" "$(status .profile.state)"
rm -rf "$CUSTOM"; mv "$TMP/moved-aside" "$CUSTOM"

cp "$CUSTOM/Local State" "$TMP/ls"; printf '{"profile":' > "$CUSTOM/Local State"
if "$BROWSER" ensure 2>"$TMP/err"; then echo "FAIL: started a damaged profile"; exit 1; fi
contains "damaged Local State fails" "not valid JSON" "$(cat "$TMP/err")"
cp "$TMP/ls" "$CUSTOM/Local State"

# The declared profile swapped for a symlink to a full copy (identity
# included) on other storage: refused at every launch, not only at declare.
mv "$CUSTOM" "$TMP/real-profile"; cp -a "$TMP/real-profile" "$TMP/overlay/copy"
ln -s "$TMP/overlay/copy" "$CUSTOM"
if "$BROWSER" ensure 2>"$TMP/err"; then echo "FAIL: launched a profile linked off the workspace"; exit 1; fi
contains "symlinked-out profile fails at launch" "outside the persistent workspace" "$(cat "$TMP/err")"
check "status: outside_workspace, still mounted" "outside_workspace mounted" "$(status '"\(.profile.state) \(.mount.state)"')"
if "$BROWSER" profile-path 2>/dev/null; then echo "FAIL: profile-path for a profile off the workspace"; exit 1; fi
rm "$CUSTOM"; mv "$TMP/real-profile" "$CUSTOM"

unmounted
if "$BROWSER" ensure 2>"$TMP/err"; then echo "FAIL: declared profile used off the workspace mount"; exit 1; fi
contains "unmounted workspace fails for a declared profile" "not the persistent workspace mount" "$(cat "$TMP/err")"
check "status: not mounted" "not_mounted" "$(status .mount.state)"
mounted
check "launches only when allowed" "$before" "$(launches)"

# --- a browser on another profile blocks, it is not reused -----------------------
"$BROWSER" declare .chrome-state --replace >/dev/null
"$BROWSER" ensure 2>/dev/null
check "switched declaration starts that profile" "$DEFAULT" "$(tail -1 "$FAKE_DIR/launches")"
"$BROWSER" declare chrome-profile --replace >/dev/null
before=$(launches)
if "$BROWSER" ensure 2>"$TMP/err"; then echo "FAIL: used a browser on the previous profile"; exit 1; fi
contains "browser on the previous profile is reported" "does not own $CUSTOM" "$(cat "$TMP/err")"
check "status: other profile" "other_profile" "$(status .cdp.state)"
# open must fail closed too, with the relay up and with it down: the running
# browser is not on the declared profile, and a second one must not start.
if timeout 20 "$BROWSER" open https://example.test/ 2>/dev/null; then echo "FAIL: open (relay up) ran with a switched declaration"; exit 1; fi
check "open fails closed on a switched declaration (relay up)" "$before" "$(launches)"
pkill -f "TCP-LISTEN:$HELIX_CHROME_DEBUG_PORT" || true
for _ in $(seq 1 50); do ss -Hltn "sport = :$HELIX_CHROME_DEBUG_PORT" | grep -q . || break; sleep 0.1; done
if timeout 20 "$BROWSER" open https://example.test/ 2>/dev/null; then echo "FAIL: open (relay down) ran with a switched declaration"; exit 1; fi
check "open fails closed on a switched declaration (relay down)" "$before" "$(launches)"
"$BROWSER" relay "${MCP_ARGS[@]}"
"$BROWSER" stop
"$BROWSER" ensure 2>/dev/null
check "after stop the declared profile starts" "$((before + 1)) $CUSTOM" "$(launches) $(tail -1 "$FAKE_DIR/launches")"
# Started without an MCP server (desktop open, login check), the browser
# still gets the automation and extension flags.
for flag in --disable-blink-features=AutomationControlled --disable-infobars --disable-extensions --no-first-run; do
    contains "launch without MCP flags has $flag" " $flag " " $(cat "$FAKE_DIR/last-args") "
done

# --- the default profile is held to the same rule --------------------------------
"$BROWSER" stop
mv "$HELIX_BROWSER_WORK_DIR/.helix/browser.json" "$TMP/browser.json"
mv "$DEFAULT" "$TMP/default-real"; ln -s "$TMP/overlay/default" "$DEFAULT"
before=$(launches)
if "$BROWSER" ensure 2>"$TMP/err"; then echo "FAIL: launched .chrome-state linked off the workspace"; exit 1; fi
contains "default profile linked off the workspace fails" "outside the persistent workspace" "$(cat "$TMP/err")"
check "nothing launched or created" "$before no" "$(launches) $([ -e "$TMP/overlay/default" ] && echo yes || echo no)"
rm "$DEFAULT"; mv "$TMP/default-real" "$DEFAULT"; mv "$TMP/browser.json" "$HELIX_BROWSER_WORK_DIR/.helix/browser.json"

# --- only Helix's relay may hold the debugging port --------------------------------
pkill -f "TCP-LISTEN:$HELIX_CHROME_DEBUG_PORT" || true
for _ in $(seq 1 50); do ss -Hltn "sport = :$HELIX_CHROME_DEBUG_PORT" | grep -q . || break; sleep 0.1; done
socat "TCP-LISTEN:$HELIX_CHROME_DEBUG_PORT,bind=127.0.0.1,reuseaddr,fork" SYSTEM:true &
for _ in $(seq 1 50); do ss -Hltn "sport = :$HELIX_CHROME_DEBUG_PORT" | grep -q . && break; sleep 0.1; done
if "$BROWSER" relay "${MCP_ARGS[@]}" 2>"$TMP/err"; then echo "FAIL: accepted a foreign debugging port"; exit 1; fi
contains "foreign listener on the debugging port fails" "something other than the Helix browser relay" "$(cat "$TMP/err")"

echo "PASS: $pass checks"
