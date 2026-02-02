#!/bin/sh
set -eu

# Script to run tests with goroutine dump on timeout and failure summary
# Usage: ./run-tests-with-timeout.sh [timeout_seconds] [test_args...]

# Parse arguments - if first arg is a number, use it as timeout, otherwise use default
if [ -n "${1:-}" ] && echo "$1" | grep -q '^[0-9][0-9]*$'; then
    TIMEOUT=$1
    shift
else
    TIMEOUT=300  # Default 5 minutes (shorter than Go's 10min timeout)
fi

TEST_ARGS="$@"
JSON_OUTPUT_FILE=$(mktemp)
EXIT_CODE_FILE=$(mktemp)

# Cleanup on exit
cleanup() {
    rm -f "$JSON_OUTPUT_FILE" "$EXIT_CODE_FILE"
}
trap cleanup EXIT

echo "Running tests with ${TIMEOUT}s timeout: go test -json ${TEST_ARGS}"
echo ""

# Function to dump goroutines
dump_goroutines() {
    echo ""
    echo "=== TIMEOUT DETECTED - DUMPING GOROUTINES ==="
    echo "Timestamp: $(date)"

    # Find all go test processes and dump their goroutines
    echo "Looking for go test processes..."
    FOUND_PROCESSES=0

    # Use ps to find go test processes more reliably
    ps aux | grep "[g]o test" | while read -r user pid cpu mem vsz rss tty stat start time command; do
        echo "Found go test process: PID=$pid, Command: $command"
        echo "Sending SIGQUIT to PID $pid to dump goroutines..."
        kill -QUIT "$pid" 2>/dev/null || echo "Failed to send SIGQUIT to PID $pid"
        FOUND_PROCESSES=1
    done

    if [ $FOUND_PROCESSES -eq 0 ]; then
        echo "No go test processes found, trying pgrep..."
        pgrep -f "go test" | while read -r pid; do
            if [ "$pid" != "$$" ]; then
                echo "Sending SIGQUIT to go test process (PID: $pid)..."
                kill -QUIT "$pid" 2>/dev/null || echo "Failed to send SIGQUIT to PID $pid"
                FOUND_PROCESSES=1
            fi
        done
    fi

    echo "Waiting 5 seconds for goroutine dumps to appear..."
    sleep 5

    echo "Force killing any remaining go test processes..."
    ps aux | grep "[g]o test" | while read -r user pid cpu mem vsz rss tty stat start time command; do
        echo "Force killing PID $pid..."
        kill -KILL "$pid" 2>/dev/null || echo "Failed to kill PID $pid"
    done

    echo "=== END GOROUTINE DUMP ATTEMPT ==="
    echo ""
}

# Function to print failure summary
print_failure_summary() {
    echo ""
    echo "==========================================="
    echo "            FAILURE SUMMARY"
    echo "==========================================="

    # Extract failed tests from JSON output
    # Look for lines with "Action":"fail" and extract Package/Test
    FAILURES=$(grep '"Action":"fail"' "$JSON_OUTPUT_FILE" 2>/dev/null | while read -r line; do
        # Extract Package (required)
        pkg=$(echo "$line" | sed 's/.*"Package":"\([^"]*\)".*/\1/')
        # Extract Test (optional) - check if it exists first
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

# Run tests with JSON output in background, parse to human-readable format
# We use a subshell to capture the exit code since PIPESTATUS isn't portable.
(
    go test -json ${TEST_ARGS} 2>&1
    echo $? > "$EXIT_CODE_FILE"
) | tee "$JSON_OUTPUT_FILE" | sed -n '
    # Only process lines with "Action":"output"
    /"Action":"output"/!d

    # Extract the Output field value:
    # 1. Remove everything up to and including "Output":"
    s/.*"Output":"//
    # 2. Remove the trailing "}
    s/"}$//
    # 3. Handle JSON escape sequences
    s/\\n/\
/g
    s/\\t/	/g
    s/\\r//g
    s/\\"/"/g
    s/\\\\/\\/g

    # Print the result
    p
' &
TEST_PID=$!

echo "Started test pipeline with PID: $TEST_PID"

# Set up a timeout using a more reliable approach
(
    sleep "$TIMEOUT"
    echo ""
    echo "TIMEOUT: Tests have been running for ${TIMEOUT} seconds"
    dump_goroutines

    # Kill the test process
    if kill -0 "$TEST_PID" 2>/dev/null; then
        echo "Killing main test process PID: $TEST_PID"
        kill -KILL "$TEST_PID" 2>/dev/null || true
    fi

    # Also kill any go test processes
    pkill -KILL -f "go test" 2>/dev/null || true

    exit 124  # Standard timeout exit code
) &
TIMEOUT_PID=$!

# Wait for either the test to complete or timeout
wait $TEST_PID 2>/dev/null || true

# Read the exit code from the file (may not exist if timeout killed it)
TEST_EXIT_CODE=$(cat "$EXIT_CODE_FILE" 2>/dev/null || echo "1")

# If we get here, the test completed before timeout
# Kill the timeout process
if kill -0 "$TIMEOUT_PID" 2>/dev/null; then
    kill $TIMEOUT_PID 2>/dev/null || true
    wait $TIMEOUT_PID 2>/dev/null || true
fi

# Print failure summary if there were failures
if [ "$TEST_EXIT_CODE" != "0" ]; then
    print_failure_summary
fi

# Exit with the test's exit code
exit $TEST_EXIT_CODE
