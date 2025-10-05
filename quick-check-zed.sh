#!/bin/bash

# Start test in background
./test-zed-websocket-end-to-end.sh > /tmp/test-output.log 2>&1 &

echo "Test started, waiting 25 seconds..."
sleep 25

CONTAINER=$(docker ps --format '{{.Names}}' | grep "zed-external" | head -1)
if [ -z "$CONTAINER" ]; then
    echo "❌ No container found"
    docker ps --format '{{.Names}}' | head -10
    exit 1
fi

echo "✅ Found: $CONTAINER"
echo ""
echo "=== Zed process check ==="
docker exec "$CONTAINER" ps aux | grep -E "PID|zed" | grep -v grep
echo ""
echo "=== Environment variables ==="
docker exec "$CONTAINER" env | grep -E "ZED_|HELIX_"
echo ""
echo "=== Full container logs (last 100 lines) ==="
docker logs "$CONTAINER" 2>&1 | tail -100
