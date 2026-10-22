#!/bin/bash
#
# Test helix-golden-startup.sh with fast idle settings.
#
# Usage: ./test-golden-startup.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT
export HELIX_GOLDEN_IDLE_TIMEOUT=4 HELIX_GOLDEN_IDLE_POLL=2
# A fake agent CPU tier: tests simulate build activity by bumping usage_usec.
export HELIX_GOLDEN_ACTIVITY_CGROUP="$TMP/cgroup"
mkdir "$HELIX_GOLDEN_ACTIVITY_CGROUP"
echo "259:0 rbytes=0 wbytes=0" > "$HELIX_GOLDEN_ACTIVITY_CGROUP/io.stat"

check() {
    local name="$1" expected="$2" actual="$3"
    if [ "${actual}" != "${expected}" ]; then
        echo "FAIL: ${name}: expected '${expected}', got '${actual}'"
        exit 1
    fi
    echo "ok: ${name}"
}

run() {
    echo "usage_usec 0" > "$HELIX_GOLDEN_ACTIVITY_CGROUP/cpu.stat"
    printf '%s\n' "$1" > "$TMP/startup.sh"
    local start=$SECONDS code=0
    timeout 60 "$SCRIPT_DIR/helix-golden-startup.sh" "$TMP/startup.sh" >"$TMP/out" 2>&1 || code=$?
    echo "$code $((SECONDS - start))"
}

read -r code _ <<<"$(run 'echo built; exit 0')"
check "exiting script succeeds" 0 "$code"

read -r code _ <<<"$(run 'echo broken; exit 3')"
check "failing script keeps its exit code" 3 "$code"

read -r code took <<<"$(run 'echo built; sleep 4242 & exec sleep 4242')"
check "foreground server is stopped once idle" 0 "$code"
[ "$took" -lt 30 ] || { echo "FAIL: idle stop took ${took}s"; exit 1; }
pgrep -f "^sleep 4242" >/dev/null && { echo "FAIL: server processes left running"; exit 1; }
echo "ok: server processes killed"

read -r code _ <<<"$(run 'read -r line; echo "got: $line"; exit 0')"
check "prompt does not block" 0 "$code"

# Builds by the inner engine show up only in the tier's counters.
read -r code took <<<"$(run 'for i in $(seq 1 10); do echo "usage_usec ${i}000000" > '"$HELIX_GOLDEN_ACTIVITY_CGROUP"'/cpu.stat; sleep 1; done; exit 0')"
check "busy script is not stopped" 0 "$code"
[ "$took" -ge 10 ] || { echo "FAIL: busy script stopped after ${took}s"; exit 1; }
echo "ok: busy script ran to completion"
