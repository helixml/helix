#!/bin/bash

# Start test in background
./test-websocket-message-flow.sh > /tmp/latest-test.log 2>&1 &
TEST_PID=$!

# Wait for container to start
sleep 35

# Find container
CONTAINER=$(docker ps --format '{{.Names}}' | grep "zed-external" | head -1)
echo "Container: $CONTAINER"

if [ -n "$CONTAINER" ]; then
    # Test if /tmp is writable
    docker exec "$CONTAINER" sh -c "echo 'manual test' > /tmp/manual-test.txt"
    docker exec "$CONTAINER" cat /tmp/manual-test.txt

    # Check for zed debug files specifically
    echo ""
    echo "Checking for Zed debug files:"
    docker exec "$CONTAINER" ls -la /tmp/zed*.txt 2>&1 || echo "No zed*.txt files found"

    # Check for init debug files
    echo ""
    echo "Checking for init debug files:"
    docker exec "$CONTAINER" ls -la /tmp/init*.txt 2>&1 || echo "No init*.txt files found"

    # Check for websocket debug files
    echo ""
    echo "Checking for websocket debug files:"
    docker exec "$CONTAINER" ls -la /tmp/websocket*.txt 2>&1 || echo "No websocket*.txt files found"

    # Check for tokio debug files
    echo ""
    echo "Checking for tokio debug files:"
    docker exec "$CONTAINER" ls -la /tmp/tokio*.txt 2>&1 || echo "No tokio*.txt files found"
    docker exec "$CONTAINER" ls -la /tmp/before*.txt 2>&1 || echo "No before*.txt files found"

    # List ALL txt files in /tmp
    echo ""
    echo "All .txt files in /tmp:"
    docker exec "$CONTAINER" find /tmp -name "*.txt" 2>&1 | head -30
fi

# Wait for test to complete
wait $TEST_PID
