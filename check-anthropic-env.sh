#!/bin/bash

# Start test in background
./test-websocket-message-flow.sh > /tmp/env-check-test.log 2>&1 &
TEST_PID=$!

# Wait for container to start
sleep 35

# Find container
CONTAINER=$(docker ps --format '{{.Names}}' | grep "zed-external" | head -1)
echo "Container: $CONTAINER"

if [ -n "$CONTAINER" ]; then
    echo ""
    echo "Environment variables with ANTHROPIC:"
    docker exec "$CONTAINER" printenv | grep ANTHROPIC || echo "No ANTHROPIC env vars found"

    echo ""
    echo "All Zed-related environment variables:"
    docker exec "$CONTAINER" printenv | grep -E "ZED_|HELIX_|ANTHROPIC" | sort

    echo ""
    echo "=== Checking for AI/Agent errors in logs ==="
    docker logs "$CONTAINER" 2>&1 | grep -i -E "error|failed|anthropic|model" | grep -v "outofdate_header" | tail -20

    echo ""
    echo "=== Checking for thread send/agent activity ==="
    docker logs "$CONTAINER" 2>&1 | grep -E "Sent initial message|agent|ACP" | tail -20
fi

# Wait for test to complete
wait $TEST_PID
