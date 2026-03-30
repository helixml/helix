#!/bin/sh
set -eu

# Script to run tests with goroutine dump on timeout and failure summary
# Streams output in real-time (no buffering) so CI always shows what happened.

# Extract timeout from args if present (e.g. -timeout 8m)
TIMEOUT=480
ARGS=""
while [ $# -gt 0 ]; do
    case "$1" in
        -timeout)
            shift
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

JSON_OUTPUT_FILE=$(mktemp)
cleanup() { rm -f "$JSON_OUTPUT_FILE"; }
trap cleanup EXIT

echo "Running tests with ${TIMEOUT}s timeout: go test -json $ARGS"
echo ""

# Run go test -json, piping through sed to pretty-print in real-time.
# stderr is passed through untouched (compile errors, panics show immediately).
# tee captures JSON to a file for the failure summary at the end.
#
# The sed filter extracts the Output field from JSON action=output lines
# and passes non-JSON lines (errors, panics) through unchanged.
EXIT_CODE=0
timeout -s QUIT -k 10 "${TIMEOUT}" go test -json $ARGS 2>&1 \
    | tee "$JSON_OUTPUT_FILE" \
    | sed -u '
        /^{.*"Action":"output"/{
            s/.*"Output":"//
            s/"}$//
            s/\\n/\
/g
            s/\\t/	/g
            s/\\r//g
            s/\\"/"/g
            s/\\\\/\\/g
            p
            d
        }
        /^{.*"Action"/d
        p
    ' \
    || EXIT_CODE=$?

# Timeout detection
if [ "$EXIT_CODE" = "131" ] || [ "$EXIT_CODE" = "137" ] || [ "$EXIT_CODE" = "124" ]; then
    echo ""
    echo "ERROR: Test suite timed out after ${TIMEOUT} seconds (exit code: ${EXIT_CODE})"
fi

# Failure summary
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
    fi

    echo "==========================================="
fi

exit "$EXIT_CODE"
