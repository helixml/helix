#!/bin/bash

set -e

echo "🧪 Testing WebSocket Message Flow"
echo "=================================="
echo ""

cd /home/luke/pm/helix
API_KEY="hl-_pwrvW_Foqw1mggPOs6lnnq0aS13ppQecIss-HG71WQ="

# Step 1: Restart Wolf
echo "1. Restarting Wolf..."
docker compose -f docker-compose.dev.yaml down wolf
docker compose -f docker-compose.dev.yaml up -d wolf
sleep 5
echo "✅ Wolf restarted"

# Step 2: Build Zed
echo ""
echo "2. Building Zed..."
./stack build-zed > /tmp/zed_build.log 2>&1
echo "✅ Zed built"

# Step 3: Create external agent via direct endpoint
echo ""
echo "3. Creating external agent..."
AGENT_RESPONSE=$(curl -s -X POST \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "session_id": "",
    "input": "test message",
    "user_id": "test-user",
    "project_path": "/tmp/test"
  }' \
  "http://localhost:8080/api/v1/external-agents")

SESSION_ID=$(echo "$AGENT_RESPONSE" | jq -r '.session_id')
echo "✅ External agent created: $SESSION_ID"

# Verify user mapping was stored in API logs
echo "   Verifying user mapping..."
sleep 2
USER_MAPPING_LOG=$(docker compose -f docker-compose.dev.yaml logs api 2>&1 | grep "Stored user mapping for external agent session" | grep "$SESSION_ID" | tail -1)
if [ -n "$USER_MAPPING_LOG" ]; then
    echo "   ✅ User mapping stored in API"
else
    echo "   ⚠️  User mapping not found in API logs"
fi

sleep 3

# Step 4: Launch Moonlight to start container
echo ""
echo "4. Launching Moonlight..."
timeout 60 moonlight stream localhost "External Agent $SESSION_ID" --quit-after --1080 > /tmp/stream.log 2>&1 &
STREAM_PID=$!
echo "   Moonlight PID: $STREAM_PID"
sleep 15

# Step 5: Find container
echo ""
echo "5. Finding container..."
CONTAINER=$(docker ps --format '{{.Names}}' | grep "zed-external" | head -1)
if [ -z "$CONTAINER" ]; then
    echo "❌ Container not found"
    kill $STREAM_PID 2>/dev/null
    exit 1
fi
echo "✅ Container: $CONTAINER"

# Step 6: Wait for WebSocket connection (poll until connected)
echo ""
echo "6. Waiting for WebSocket connection..."
MAX_WAIT=30
WAIT_COUNT=0
while [ "$WAIT_COUNT" -lt "$MAX_WAIT" ]; do
    WS_CONNECTED=$(docker logs helix-api-1 2>&1 | grep -E "Registered external agent.*$SESSION_ID" | wc -l)
    if [ "$WS_CONNECTED" -gt "0" ]; then
        echo "✅ WebSocket connected (took ${WAIT_COUNT}s)"
        docker logs helix-api-1 2>&1 | grep -E "WebSocket.*$SESSION_ID" | tail -5
        break
    fi
    echo "   Waiting for connection... (${WAIT_COUNT}s)"
    sleep 1
    WAIT_COUNT=$((WAIT_COUNT + 1))
done

if [ "$WS_CONNECTED" -eq "0" ]; then
    echo "❌ WebSocket connection timeout after ${MAX_WAIT}s"
    echo "Container logs:"
    docker logs "$CONTAINER" 2>&1 | tail -20
    kill $STREAM_PID 2>/dev/null
    exit 1
fi

# Additional wait to ensure receiver task is fully ready
echo "   Waiting additional 3s for receiver task to stabilize..."
sleep 3

# Step 7: Send command to external agent
echo ""
echo "7. Sending test command via WebSocket..."
SEND_RESPONSE=$(curl -s -X POST \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "type": "chat_message",
    "data": {
      "message": "What is 2+2?",
      "request_id": "test-req-001",
      "acp_thread_id": "test-thread-001"
    }
  }' \
  "http://localhost:8080/api/v1/external-agents/$SESSION_ID/command")

echo "$SEND_RESPONSE"

# Check if send was successful
if echo "$SEND_RESPONSE" | grep -q "failed to send"; then
    echo "❌ Command send failed"
    exit 1
fi

echo "✅ Command sent successfully"

# Step 8: Check Zed logs for message receipt and AI response
echo ""
echo "8. Checking Zed logs for message processing (waiting 3 seconds)..."
sleep 3
echo ""
echo "Zed WebSocket logs:"
docker logs "$CONTAINER" 2>&1 | grep -E "WEBSOCKET-IN|chat_message|THREAD_SERVICE" | tail -20

echo ""
echo "9. Waiting for AI response (additional 15 seconds)..."
sleep 15
echo ""
echo "Zed AI response logs:"
docker logs "$CONTAINER" 2>&1 | grep -E "EntryUpdated|Stopped|message_added|message_completed" | tail -10

echo ""
echo "API WebSocket receive logs:"
docker logs helix-api-1 2>&1 | grep -E "thread_created|message_added|message_completed.*$SESSION_ID" | tail -10

# Step 10: Check debug files
echo ""
echo "10. Checking debug files in container..."
docker exec "$CONTAINER" ls -la /tmp/websocket*.txt 2>/dev/null || echo "No debug files found"
echo ""
echo "Debug file contents:"
docker exec "$CONTAINER" cat /tmp/websocket*.txt 2>/dev/null || echo "Could not read debug files"

# Step 11: Verification
echo ""
echo "11. Verification"
echo "================"
RECEIVED_MESSAGE=$(docker logs "$CONTAINER" 2>&1 | grep -E "WEBSOCKET-IN.*Received text.*chat_message" | wc -l)
THREAD_REQUEST=$(docker logs "$CONTAINER" 2>&1 | grep -E "THREAD_SERVICE.*Received thread creation" | wc -l)
AI_RESPONSE=$(docker logs "$CONTAINER" 2>&1 | grep -E "EntryUpdated|message_added" | wc -l)
RESPONSE_COMPLETE=$(docker logs "$CONTAINER" 2>&1 | grep -E "Stopped.*message_completed" | wc -l)

echo "Session ID: $SESSION_ID"
echo "Container: $CONTAINER"
echo ""
if [ "$RECEIVED_MESSAGE" -gt "0" ]; then
    echo "✅ Message received by Zed"
else
    echo "❌ Message NOT received by Zed"
fi

if [ "$THREAD_REQUEST" -gt "0" ]; then
    echo "✅ Thread creation request processed"
else
    echo "⚠️  Thread creation request not seen"
fi

if [ "$AI_RESPONSE" -gt "0" ]; then
    echo "✅ AI response generated ($AI_RESPONSE updates)"
else
    echo "❌ AI response NOT generated (no EntryUpdated events)"
fi

if [ "$RESPONSE_COMPLETE" -gt "0" ]; then
    echo "✅ Response completed"
else
    echo "⚠️  Response completion not detected"
fi
echo ""

# Cleanup
kill $STREAM_PID 2>/dev/null || true

echo "✅ Test complete"
