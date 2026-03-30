#!/bin/sh
set -eu

# Script to run tests with goroutine dump on timeout and failure summary
# Usage: ./run-tests-with-timeout.sh [test_args...]
#
# Uses BusyBox/coreutils `timeout` to handle the timeout reliably:
#   1. SIGQUIT after timeout → Go prints goroutine dumps
#   2. SIGKILL 10s later if still alive

# go test -json sends JSON to stdout and errors (compile failures, panics) to stderr.
# We capture JSON to a file for the failure summary, and let stderr flow to the console.
# Then we pretty-print the JSON output.

JSON_OUTPUT_FILE=$(mktemp)
STDERR_FILE=$(mktemp)

cleanup() {
    rm -f "$JSON_OUTPUT_FILE" "$STDERR_FILE"
}
trap cleanup EXIT

# Extract timeout from args if present (e.g. -timeout 8m)
TIMEOUT=480  # default 8 minutes in seconds
ARGS=""
while [ $# -gt 0 ]; do
    case "$1" in
        -timeout)
            shift
            # Convert Go duration to seconds
            RAW="$1"
            case "$RAW" in
                *m) TIMEOUT=$(echo "${RAW%m} * 60" | bc) ;;
                *s) TIMEOUT="${RAW%s}" ;;
                *h) TIMEOUT=$(echo "${RAW%h} * 3600" | bc) ;;
                *)  TIMEOUT="$RAW" ;;
            esac
            ARGS="$ARGS -timeout $RAW"
            shift
            ;;
        *)
            ARGS="$ARGS $1"
            shift
            ;;
    esac
done

echo "Running tests with ${TIMEOUT}s timeout: go test -json $ARGS"
echo ""

# Run tests:
#   - stdout (JSON) → captured to file
#   - stderr (compile errors, panics) → displayed immediately
#   - timeout sends SIGQUIT (goroutine dump) then SIGKILL
EXIT_CODE=0
timeout -s QUIT -k 10 "${TIMEOUT}" go test -json $ARGS \
    > "$JSON_OUTPUT_FILE" \
    2> "$STDERR_FILE" \
    || EXIT_CODE=$?

# Show stderr immediately if there was any (compile errors, panics)
if [ -s "$STDERR_FILE" ]; then
    cat "$STDERR_FILE" >&2
fi

# Pretty-print the JSON test output (extract Output field from action=output lines)
if [ -s "$JSON_OUTPUT_FILE" ]; then
    sed -n '
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
    ' "$JSON_OUTPUT_FILE"
fi

# Timeout detection
if [ "$EXIT_CODE" = "131" ] || [ "$EXIT_CODE" = "137" ] || [ "$EXIT_CODE" = "124" ]; then
    echo ""
    echo "ERROR: Test suite timed out after ${TIMEOUT} seconds (exit code: ${EXIT_CODE})"
fi

# Failure summary from JSON
if [ "$EXIT_CODE" != "0" ]; then
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
        echo "Check STDERR OUTPUT above for details."
    fi

    echo "==========================================="
fi

exit "$EXIT_CODE"
