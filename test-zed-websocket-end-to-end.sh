#!/bin/bash

set -e

echo "🧪 Automated Zed WebSocket Integration Test"
echo "==========================================="
echo ""

cd /home/luke/pm/helix
API_KEY="hl-_pwrvW_Foqw1mggPOs6lnnq0aS13ppQecIss-HG71WQ="

# Step 0: Restart Wolf to clear old containers
echo "0. Restarting Wolf to clear old containers..."
docker compose -f docker-compose.dev.yaml down wolf
docker compose -f docker-compose.dev.yaml up -d wolf
echo "   Waiting 5 seconds for Wolf to start..."
sleep 5
echo "✅ Wolf restarted"

# Step 1: Build latest Zed
echo ""
echo "1. Building latest Zed..."
./stack build-zed > /tmp/zed_build.log 2>&1
if [ $? -ne 0 ]; then
    echo "❌ Build failed"
    tail -20 /tmp/zed_build.log
    exit 1
fi
echo "✅ Zed built successfully"

# Step 2: Skip pairing - use existing pairing
echo ""
echo "2. Using existing Moonlight pairing..."
echo "✅ Assuming system is already paired with Wolf"

# Step 3: Create session with external agent type (this creates both session and Wolf app)
# Run in background since it will block waiting for WebSocket
echo ""
echo "3. Creating external agent session via chat API (in background)..."
timeout 180 curl -s -X POST \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "messages": [
      {
        "role": "user",
        "content": {
          "contentType": "text",
          "parts": ["Hello Zed! What is 2+2? Please respond."]
        }
      }
    ],
    "agent_type": "zed_external",
    "stream": false
  }' \
  "http://localhost:8080/api/v1/sessions/chat" > /tmp/chat-response.log 2>&1 &
CHAT_PID=$!

echo "   Chat request sent (PID: $CHAT_PID), waiting for Wolf app creation..."

# Wait for Wolf app to be created (check API logs for session ID)
for i in {1..10}; do
    SESSION_ID=$(docker logs helix-api-1 2>&1 | grep "External Agent ses_" | tail -1 | grep -oP 'ses_[a-z0-9]+' | head -1)
    if [ -n "$SESSION_ID" ]; then
        echo "✅ Found session ID in logs: $SESSION_ID"
        break
    fi
    echo "   Waiting for session creation (attempt $i/10)..."
    sleep 1
done

if [ -z "$SESSION_ID" ]; then
    echo "❌ Failed to find session ID in API logs"
    kill $CHAT_PID 2>/dev/null
    exit 1
fi

# Step 4: Launch Moonlight client to start streaming
echo ""
echo "4. Launching Moonlight client to start streaming session..."

# The app name in Wolf is "External Agent <session_id>"
APP_NAME="External Agent $SESSION_ID"
echo "   Using app: $APP_NAME"

# Launch Moonlight stream in background (will open GUI)
timeout 30 moonlight stream localhost "$APP_NAME" --quit-after --1080 > /tmp/stream.log 2>&1 &
STREAM_PID=$!

echo "   Moonlight client launched (PID: $STREAM_PID)"
echo "   Waiting 20 seconds for container, Zed, and AI response..."
sleep 20

# Step 5: Find Zed container
echo ""
echo "5. Finding Zed container..."

CONTAINER=$(docker ps --format '{{.Names}}' | grep "zed-external" | head -1)

if [ -z "$CONTAINER" ]; then
    echo "❌ Zed container not found"
    echo "All containers:"
    docker ps --format '{{.Names}}' | head -20
    kill $STREAM_PID 2>/dev/null
    exit 1
fi

echo "✅ Found container: $CONTAINER"

# Step 6: Check Zed logs for WebSocket activity
echo ""
echo "6. Checking Zed logs for WebSocket initialization..."
echo "=================================================="
docker logs "$CONTAINER" 2>&1 | grep -E "ZED.*WebSocket|WEBSOCKET|CALLBACK|THREAD_SERVICE" | head -50

echo ""
echo "7. Checking for errors..."
echo "========================="
docker logs "$CONTAINER" 2>&1 | grep -iE "error|panic|crash|failed" | head -20

echo ""
echo "8. Checking Helix API logs..."
echo "=============================="
docker logs helix-api-1 2>&1 | grep -i "external.*agent.*${SESSION_ID:0:15}" | tail -10

# Step 9: Check session for response
echo ""
echo "9. Checking session for AI response..."
SESSION_DATA=$(curl -s -H "Authorization: Bearer $API_KEY" \
    "http://localhost:8080/api/v1/sessions/$SESSION_ID")

INTERACTIONS=$(echo "$SESSION_DATA" | jq -r '.interactions | length' 2>/dev/null || echo "0")
echo "   Session has $INTERACTIONS interactions"

if [ "$INTERACTIONS" -gt 0 ]; then
    RESPONSE=$(echo "$SESSION_DATA" | jq -r '.interactions[0].response' 2>/dev/null)
    if [ "$RESPONSE" != "null" ] && [ -n "$RESPONSE" ]; then
        echo "✅ Got response from Zed!"
        echo "   Response: ${RESPONSE:0:200}..."
    else
        echo "❌ No response in interaction"
    fi
else
    echo "❌ No interactions in session"
fi

# Cleanup
kill $STREAM_PID 2>/dev/null || true
kill $CHAT_PID 2>/dev/null || true

echo ""
echo "✅ Test complete!"
echo ""
echo "Summary:"
echo "- Session ID: $SESSION_ID"
echo "- Container: $CONTAINER"  
echo "- Check logs above for WebSocket activity"
echo ""
echo "Expected logs to see:"
echo "  🔧 [ZED] Setting up WebSocket integration"
echo "  ✅ [WEBSOCKET] WebSocket connected"
echo "  📥 [WEBSOCKET-IN] Received text: {\"type\":\"chat_message\"...}"
echo "  📨 [THREAD_SERVICE] Received thread creation request"
