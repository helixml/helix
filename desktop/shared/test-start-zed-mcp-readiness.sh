#!/bin/bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TEST_TMP=$(mktemp -d)
trap 'find "$TEST_TMP" -depth -delete' EXIT

printf '0\n' > "$TEST_TMP/gets"
printf '0\n' > "$TEST_TMP/time"

curl() {
    local arg body count
    for arg in "$@"; do
        if [[ "$arg" == *'/mcp-readiness/gave-up?'* ]]; then
            body=$(cat)
            printf '%s\n' "$body" >> "$TEST_TMP/reports"
            return 0
        fi
    done

    count=$(<"$TEST_TMP/gets")
    printf '%s\n' "$((count + 1))" > "$TEST_TMP/gets"
    case "$count" in
        0) printf 'first unauthorized\n503' ;;
        1) printf 'still unauthorized\n503' ;;
        *) printf 'ready\n200' ;;
    esac
}

date() {
    local now
    now=$(<"$TEST_TMP/time")
    now=$((now + 200))
    printf '%s\n' "$now" > "$TEST_TMP/time"
    printf '%s\n' "$now"
}

sleep() {
    :
}

source "$SCRIPT_DIR/start-zed-core.sh"
wait_for_mcp_endpoints > "$TEST_TMP/output"

report_count=$(wc -l < "$TEST_TMP/reports" | tr -d ' ')
[ "$report_count" -eq 2 ] || { echo "FAIL: expected 2 failure reports, got $report_count"; exit 1; }
[ "$(sed -n '1p' "$TEST_TMP/reports")" = "first unauthorized" ] || { echo "FAIL: first failure was not reported"; exit 1; }
[ "$(sed -n '2p' "$TEST_TMP/reports")" = "still unauthorized" ] || { echo "FAIL: persistent failure was not re-reported"; exit 1; }
[ "$(<"$TEST_TMP/gets")" -eq 3 ] || { echo "FAIL: readiness was not checked after re-reporting"; exit 1; }
grep -q 'Helix MCP context servers reachable' "$TEST_TMP/output" || { echo "FAIL: recovery did not release the gate"; exit 1; }

echo "ok: persistent MCP failure is re-reported and recovery releases the gate"
