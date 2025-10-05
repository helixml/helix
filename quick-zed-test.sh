#!/bin/bash
cd /home/luke/pm/helix
API_KEY="hl-_pwrvW_Foqw1mggPOs6lnnq0aS13ppQecIss-HG71WQ="

echo "🧪 Quick Zed WebSocket Test"
echo "============================"

# Skip pairing for now - assume already paired or will pair on first connect

# Create Zed session
echo "1. Creating Zed external agent session..."
SESSION_RESPONSE=$(curl -s -X POST \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "session_id": "",
    "input": "Hello Zed! What is 2+2? Please respond with just the number.",
    "user_id": "test-user",
    "project_path": "/tmp/test-ws"
  }' \
  "http://localhost:8080/api/v1/external-agents")

SESSION_ID=$(echo "$SESSION_RESPONSE" | jq -r '.session_id')
STREAM_URL=$(echo "$SESSION_RESPONSE" | jq -r '.stream_url')
WOLF_APP_ID=$(echo "$STREAM_URL" | sed 's|moonlight://localhost:47989/||')

echo "✅ Session: $SESSION_ID"
echo "✅ App ID: $WOLF_APP_ID"

# Launch Moonlight
echo ""
echo "2. Launching Moonlight client..."
timeout 25 moonlight stream localhost "$WOLF_APP_ID" --quit-after --1080 > /tmp/stream.log 2>&1 &
STREAM_PID=$!
echo "✅ Moonlight started (PID: $STREAM_PID), waiting 20 seconds for Zed to boot..."
sleep 20

# Find container
echo ""
echo "3. Finding Zed container..."
CONTAINER=$(docker ps --format '{{.Names}}' | grep "zed-external" | tail -1)

if [ -z "$CONTAINER" ]; then
    echo "❌ No Zed container found"
    docker ps | grep zed
    kill $STREAM_PID 2>/dev/null
    exit 1
fi

echo "✅ Container: $CONTAINER"

# Check logs
echo ""
echo "4. Zed Startup Logs:"
echo "===================="
docker logs "$CONTAINER" 2>&1 | grep -E "ZED.*WebSocket|WEBSOCKET|CALLBACK|THREAD_SERVICE" | head -50

echo ""
echo "5. Any Errors:"
echo "=============="
docker logs "$CONTAINER" 2>&1 | grep -iE "error|panic|failed" | grep -v "fuse\|dbus\|xdg-desktop" | head -20

echo ""
echo "6. Waiting 10 more seconds for message processing..."
sleep 10

echo ""
echo "7. Checking for AI response..."
SESSION_DATA=$(curl -s -H "Authorization: Bearer $API_KEY" "http://localhost:8080/api/v1/sessions/$SESSION_ID")
RESPONSE=$(echo "$SESSION_DATA" | jq -r '.interactions[0].response' 2>/dev/null)

if [ "$RESPONSE" != "null" ] && [ -n "$RESPONSE" ]; then
    echo "✅ GOT RESPONSE: $RESPONSE"
else
    echo "❌ No response yet"
    echo "Session data:"
    echo "$SESSION_DATA" | jq '{id, interactions}' 2>/dev/null
fi

# Cleanup
kill $STREAM_PID 2>/dev/null || true

echo ""
echo "✅ Test complete - check logs above for diagnostics"
