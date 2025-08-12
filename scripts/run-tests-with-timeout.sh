#!/bin/sh
set -eu

# Script to run tests with goroutine dump on timeout
# Usage: ./run-tests-with-timeout.sh [timeout_seconds] [test_args...]

# Parse arguments - if first arg is a number, use it as timeout, otherwise use default
if [ -n "${1:-}" ] && echo "$1" | grep -q '^[0-9][0-9]*$'; then
    TIMEOUT=$1
    shift
else
    TIMEOUT=300  # Default 5 minutes (shorter than Go's 10min timeout)
fi

TEST_ARGS="$@"

echo "Running tests with ${TIMEOUT}s timeout: go test ${TEST_ARGS}"

# Create a temporary file to store the test PID
PIDFILE=$(mktemp)
trap "rm -f $PIDFILE" EXIT

# Function to dump goroutines
dump_goroutines() {
    echo "=== TIMEOUT DETECTED - DUMPING GOROUTINES ==="
    echo "Timestamp: $(date)"
    
    if [ -f "$PIDFILE" ]; then
        TEST_PID=$(cat "$PIDFILE")
        if kill -0 "$TEST_PID" 2>/dev/null; then
            echo "Sending SIGQUIT to test process (PID: $TEST_PID) to dump goroutines..."
            kill -QUIT "$TEST_PID" 2>/dev/null || true
            sleep 3
            
            # If process is still running, force kill it
            if kill -0 "$TEST_PID" 2>/dev/null; then
                echo "Force killing test process..."
                kill -KILL "$TEST_PID" 2>/dev/null || true
            fi
        else
            echo "Test process (PID: $TEST_PID) not found or already dead"
        fi
    fi
    
    # Also try to find any other go test processes and dump their goroutines
    echo "Looking for other go test processes..."
    pgrep -f "go test" | while read -r pid; do
        if [ "$pid" != "$$" ]; then
            echo "Sending SIGQUIT to go test process (PID: $pid)..."
            kill -QUIT "$pid" 2>/dev/null || true
        fi
    done
    
    sleep 2
    echo "=== END GOROUTINE DUMP ==="
}

# Set up timeout handler
(
    sleep "$TIMEOUT"
    echo "TIMEOUT: Tests have been running for ${TIMEOUT} seconds"
    dump_goroutines
    exit 124  # Standard timeout exit code
) &
TIMEOUT_PID=$!

# Run the actual test
go test ${TEST_ARGS} &
TEST_PID=$!
echo "$TEST_PID" > "$PIDFILE"

# Wait for either the test to complete or timeout
wait $TEST_PID 2>/dev/null
TEST_EXIT_CODE=$?

# Kill the timeout process if test completed
kill $TIMEOUT_PID 2>/dev/null || true
wait $TIMEOUT_PID 2>/dev/null || true

# Exit with the test's exit code
exit $TEST_EXIT_CODE
