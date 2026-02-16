#!/bin/sh
set -eu

# Script to run tests with goroutine dump on timeout and failure summary
# Usage: ./run-tests-with-timeout.sh [timeout_seconds] [test_args...]
#
# Uses BusyBox/coreutils `timeout` to handle the timeout reliably:
#   1. SIGQUIT after ${TIMEOUT}s → Go prints goroutine dumps
#   2. SIGKILL 10s later if still alive
#   3. Exit code 143 on timeout (SIGQUIT), propagated correctly

# Parse arguments - if first arg is a number, use it as timeout, otherwise use default
if [ -n "${1:-}" ] && echo "$1" | grep -q '^[0-9][0-9]*$'; then
    TIMEOUT=$1
    shift
else
    TIMEOUT=300  # Default 5 minutes (shorter than Go's 10min timeout)
fi

JSON_OUTPUT_FILE=$(mktemp)

# Cleanup on exit
cleanup() {
    rm -f "$JSON_OUTPUT_FILE" "${JSON_OUTPUT_FILE}.exit"
}
trap cleanup EXIT

echo "Running tests with ${TIMEOUT}s timeout: go test -json $@"
echo ""

# Function to print failure summary from JSON output
print_failure_summary() {
    echo ""
    echo "==========================================="
    echo "            FAILURE SUMMARY"
    echo "==========================================="

    FAILURES=$(grep '"Action":"fail"' "$JSON_OUTPUT_FILE" 2>/dev/null | while read -r line; do
        pkg=$(echo "$line" | sed 's/.*"Package":"\([^"]*\)".*/\1/')
        if echo "$line" | grep -q '"Test":"'; then
            test=$(echo "$line" | sed 's/.*"Test":"\([^"]*\)".*/\1/')
            echo "FAIL: $pkg / $test"
        else
            echo "FAIL: $pkg (package)"
        fi
    done | sort -u)

    if [ -n "$FAILURES" ]; then
        echo "$FAILURES"
    else
        echo "Test run failed but no specific test failures found in JSON output."
        echo "This may indicate a build error, panic, or timeout."
    fi

    echo "==========================================="
    echo ""
}

# Run tests under `timeout`:
#   --signal=QUIT  → sends SIGQUIT first (Go dumps goroutines)
#   --kill-after=10 → sends SIGKILL 10s later if still alive
#
# Pipeline: go test -json | tee (capture JSON) | sed (human-readable output)
# We need the exit code from `timeout go test`, not from sed/tee.
# So we write the exit code to a file from inside the subshell.
EXIT_CODE=0
(
    timeout -s QUIT -k 10 "${TIMEOUT}" go test -json "$@" 2>&1 || echo $? > "${JSON_OUTPUT_FILE}.exit"
) | tee "$JSON_OUTPUT_FILE" | sed -n '
    /"Action":"output"/!d
    s/.*"Output":"//
    s/"}$//
    s/\\n/\
/g
    s/\\t/	/g
    s/\\r//g
    s/\\"/"/g
    s/\\\\/\\/g
    p
' || true

# Read exit code: timeout writes it, or 0 if tests passed (file won't exist)
if [ -f "${JSON_OUTPUT_FILE}.exit" ]; then
    EXIT_CODE=$(cat "${JSON_OUTPUT_FILE}.exit")
    rm -f "${JSON_OUTPUT_FILE}.exit"
fi

# timeout returns 124+signal when it kills the process
# SIGQUIT=3, so timeout returns 131 (128+3) when it sends SIGQUIT
# After --kill-after SIGKILL, it returns 137 (128+9)
if [ "$EXIT_CODE" = "131" ] || [ "$EXIT_CODE" = "137" ] || [ "$EXIT_CODE" = "124" ]; then
    echo ""
    echo "ERROR: Test suite timed out after ${TIMEOUT} seconds (exit code: ${EXIT_CODE})"
fi

if [ "$EXIT_CODE" != "0" ]; then
    print_failure_summary
fi

exit "$EXIT_CODE"
