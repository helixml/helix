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
# A fake agent CPU tier: tests simulate build activity by bumping its counters.
export HELIX_GOLDEN_AGENT_CGROUP="$TMP/cgroup"
PROCS="$HELIX_GOLDEN_AGENT_CGROUP/procs"
BUILDKIT="$HELIX_GOLDEN_AGENT_CGROUP/docker/buildkit"
mkdir -p "$PROCS" "$BUILDKIT"

check() {
    local name="$1" expected="$2" actual="$3"
    if [ "${actual}" != "${expected}" ]; then
        echo "FAIL: ${name}: expected '${expected}', got '${actual}'"
        exit 1
    fi
    echo "ok: ${name}"
}

run() {
    for cgroup in "$PROCS" "$BUILDKIT"; do
        echo "usage_usec 0" > "$cgroup/cpu.stat"
        echo "259:0 rbytes=0 wbytes=0" > "$cgroup/io.stat"
    done
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

# BuildKit steps run outside the script's processes; they show up only in the
# tier's counters.
read -r code took <<<"$(run 'for i in $(seq 1 10); do echo "usage_usec ${i}000000" > '"$BUILDKIT"'/cpu.stat; sleep 1; done; exit 0')"
check "script waiting on a BuildKit build is not stopped" 0 "$code"
[ "$took" -ge 10 ] || { echo "FAIL: script stopped after ${took}s"; exit 1; }
echo "ok: script with a BuildKit build ran to completion"

read -r code took <<<"$(run 'for i in $(seq 1 10); do echo "259:0 rbytes=0 wbytes=${i}00000000" > '"$PROCS"'/io.stat; sleep 1; done; exit 0')"
check "script pulling images is not stopped" 0 "$code"
[ "$took" -ge 10 ] || { echo "FAIL: script stopped after ${took}s"; exit 1; }
echo "ok: script pulling images ran to completion"
