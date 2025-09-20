#!/bin/bash

echo "🔧 Simple Streaming Test"
echo "========================="

# Kill any hanging moonlight processes
killall moonlight 2>/dev/null || true
sleep 1

echo "1. Testing direct streaming (assumes pairing already done)..."

# Start streaming with aggressive timeout
timeout 20 moonlight stream localhost "Hyprland Desktop" --quit-after --1080 &
STREAM_PID=$!

echo "   Streaming PID: $STREAM_PID"
echo "   Waiting 5 seconds for streaming to start..."
sleep 5

# Check if still running
if ps -p $STREAM_PID > /dev/null 2>&1; then
    echo "   ✓ Client is streaming (receiving data)"

    # Check server logs for streaming activity
    echo "2. Checking server activity..."
    docker compose -f docker-compose.dev.yaml logs zed-runner --since="30s" | grep -E "\[LAUNCH DEBUG\]|\[RESUME DEBUG\]|\[RTSP DEBUG\]" | tail -10

    echo "3. Checking RTSP session matching..."
    docker compose -f docker-compose.dev.yaml logs zed-runner --since="30s" | grep -E "get_session.*Looking|found.*session.*by.*matching" | tail -5

else
    echo "   ⚠️ Client streaming ended early"
fi

# Clean up
kill $STREAM_PID 2>/dev/null || true
echo "🔧 Test completed"