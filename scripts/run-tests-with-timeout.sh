#!/bin/sh
set -eu

# Run go test with timeout and failure summary.
# Streams output in real-time — no buffering issues.

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

echo "Running tests with ${TIMEOUT}s timeout: go test $ARGS"
echo ""

# Just run go test directly — no -json, no pipes, no buffering.
# Output goes straight to stdout/stderr in real-time.
EXIT_CODE=0
timeout -s QUIT -k 10 "${TIMEOUT}" go test $ARGS || EXIT_CODE=$?

if [ "$EXIT_CODE" = "131" ] || [ "$EXIT_CODE" = "137" ] || [ "$EXIT_CODE" = "124" ]; then
    echo ""
    echo "ERROR: Test suite timed out after ${TIMEOUT} seconds (exit code: ${EXIT_CODE})"
fi

exit "$EXIT_CODE"
