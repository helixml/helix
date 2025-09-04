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

# Run the actual test in background
go test ${TEST_ARGS} &
TEST_PID=$!

echo "Started go test with PID: $TEST_PID"

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
    exit 124  # Standard timeout exit code
) &
TIMEOUT_PID=$!

# Wait for either the test to complete or timeout
wait $TEST_PID 2>/dev/null
TEST_EXIT_CODE=$?

# If we get here, the test completed before timeout
# Kill the timeout process
if kill -0 "$TIMEOUT_PID" 2>/dev/null; then
    kill $TIMEOUT_PID 2>/dev/null || true
    wait $TIMEOUT_PID 2>/dev/null || true
fi

# Exit with the test's exit code
exit $TEST_EXIT_CODE
