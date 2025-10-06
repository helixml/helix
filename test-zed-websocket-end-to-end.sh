#!/bin/bash

set -e

echo "🧪 Automated Zed WebSocket Integration Test"
echo "==========================================="
echo ""

cd /home/luke/pm/helix
API_KEY="hl-_pwrvW_Foqw1mggPOs6lnnq0aS13ppQecIss-HG71WQ="

# Step 0: Restart Wolf to clear old containers and apps
echo "0. Restarting Wolf to clear old containers and apps..."
docker compose -f docker-compose.dev.yaml down wolf
docker compose -f docker-compose.dev.yaml up -d wolf
echo "   Waiting 5 seconds for Wolf to start..."
sleep 5

# Remove any existing external agent apps from Wolf
echo "   Clearing old external agent apps from Wolf..."
WOLF_APPS=$(docker compose -f docker-compose.dev.yaml exec api curl -s --unix-socket /var/run/wolf/wolf.sock http://localhost/api/v1/apps)
echo "$WOLF_APPS" | jq -r '.apps[] | select(.title | startswith("External Agent ses_")) | .id' | while read APP_ID; do
    echo "   Removing app $APP_ID..."
    docker compose -f docker-compose.dev.yaml exec api curl -s -X POST --unix-socket /var/run/wolf/wolf.sock "http://localhost/api/v1/apps/${APP_ID}/remove"
done

echo "✅ Wolf restarted and cleaned"

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

# Use timestamp to make message unique
TIMESTAMP=$(date +%s)
timeout 180 curl -s -X POST \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d "{
    \"messages\": [
      {
        \"role\": \"user\",
        \"content\": {
          \"contentType\": \"text\",
          \"parts\": [\"Test ${TIMESTAMP}: What is 2+2?\"]
        }
      }
    ],
    \"agent_type\": \"zed_external\",
    \"stream\": false
  }" \
  "http://localhost:8080/api/v1/sessions/chat" > /tmp/chat-response.log 2>&1 &
CHAT_PID=$!

echo "   Chat request sent (PID: $CHAT_PID), waiting for Wolf app creation..."

# Wait for Wolf app to be created (check API logs for NEW session ID since we started the chat request)
echo "   Waiting for new session creation in API logs..."
sleep 3

# Get the most recent session ID from Wolf apps list (more reliable than grepping logs)
SESSION_ID=$(docker compose -f docker-compose.dev.yaml exec api curl -s --unix-socket /var/run/wolf/wolf.sock http://localhost/api/v1/apps | jq -r '.apps[] | select(.title | startswith("External Agent ses_")) | .title' | sed 's/External Agent //' | head -1)

if [ -z "$SESSION_ID" ]; then
    echo "❌ Failed to find session ID in Wolf apps"
    kill $CHAT_PID 2>/dev/null
    exit 1
fi

echo "✅ Found session ID from Wolf apps: $SESSION_ID"

# Step 4: Launch Moonlight client to start streaming
echo ""
echo "4. Launching Moonlight client to start streaming session..."

# The app name in Wolf is "External Agent <session_id>"
APP_NAME="External Agent $SESSION_ID"
echo "   Using app: $APP_NAME"

# Launch Moonlight stream in background (will open GUI)
# Use longer timeout so container stays alive for follow-up message test
timeout 90 moonlight stream localhost "$APP_NAME" --quit-after --1080 > /tmp/stream.log 2>&1 &
STREAM_PID=$!

echo "   Moonlight client launched (PID: $STREAM_PID)"
echo "   Waiting 20 seconds for container, Zed, and first AI response..."
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
    RESPONSE=$(echo "$SESSION_DATA" | jq -r '.interactions[0].response_message' 2>/dev/null)
    if [ "$RESPONSE" != "null" ] && [ -n "$RESPONSE" ]; then
        echo "✅ Got response from Zed (interaction 1)!"
        echo "   Response: ${RESPONSE:0:200}..."
    else
        echo "❌ No response in interaction 1"
    fi
else
    echo "❌ No interactions in session"
fi

# Step 10: Send follow-up message
echo ""
echo "10. Testing follow-up message..."
echo "================================"
echo "   Sending second message to same session..."

timeout 180 curl -s -X POST \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "messages": [
      {
        "role": "user",
        "content": {
          "contentType": "text",
          "parts": ["And what is 3+3?"]
        }
      }
    ],
    "agent_type": "zed_external",
    "stream": false,
    "session_id": "'"$SESSION_ID"'"
  }' \
  "http://localhost:8080/api/v1/sessions/chat" > /tmp/chat-response-2.log 2>&1 &
CHAT_PID_2=$!

echo "   Follow-up message sent (PID: $CHAT_PID_2)"
echo "   Waiting 30 seconds for AI response to follow-up..."
sleep 30

# Step 11: Check API logs for follow-up message routing
echo ""
echo "11. Checking API logs for follow-up message routing..."
echo "======================================================="
docker logs helix-api-1 2>&1 | grep -E "Sending follow-up message to existing Zed thread|acp_thread_id" | tail -10

# Step 12: Check Zed logs for follow-up message (show ALL thread service activity)
echo ""
echo "12. Checking Zed logs for follow-up message..."
echo "==============================================="
echo "Last 30 WebSocket and Thread Service logs:"
docker logs "$CONTAINER" 2>&1 | grep -E "WEBSOCKET-IN|THREAD_SERVICE" | tail -30

# Step 13: Check session for second response
echo ""
echo "13. Checking session for follow-up response..."
SESSION_DATA_2=$(curl -s -H "Authorization: Bearer $API_KEY" \
    "http://localhost:8080/api/v1/sessions/$SESSION_ID")

INTERACTIONS_2=$(echo "$SESSION_DATA_2" | jq -r '.interactions | length' 2>/dev/null || echo "0")
echo "   Session now has $INTERACTIONS_2 total interactions"

if [ "$INTERACTIONS_2" -ge 2 ]; then
    RESPONSE_2=$(echo "$SESSION_DATA_2" | jq -r '.interactions[1].response_message' 2>/dev/null)
    if [ "$RESPONSE_2" != "null" ] && [ -n "$RESPONSE_2" ]; then
        echo "✅ Got response from Zed (interaction 2)!"
        echo "   Response: ${RESPONSE_2:0:200}..."
    else
        echo "❌ No response in interaction 2"
        echo "   Interaction 2 state: $(echo "$SESSION_DATA_2" | jq -r '.interactions[1].state' 2>/dev/null)"
    fi
else
    echo "❌ Second interaction not found (total: $INTERACTIONS_2)"
fi

# Check database directly
echo ""
echo "14. Checking database for interactions..."
docker compose -f docker-compose.dev.yaml exec -T postgres psql -U postgres -d postgres -c \
  "SELECT id, LEFT(prompt_message, 30) as prompt, LENGTH(response_message) as response_len, state FROM interactions WHERE session_id = '$SESSION_ID' ORDER BY created;"

kill $CHAT_PID_2 2>/dev/null || true

# Step 15: Test new session creation (verify sessions are isolated)
echo ""
echo "15. Testing new session creation (session isolation)..."
echo "========================================================"
echo "   Creating a brand new session to verify isolation..."

TIMESTAMP_2=$(date +%s)
timeout 180 curl -s -X POST \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d "{
    \"messages\": [
      {
        \"role\": \"user\",
        \"content\": {
          \"contentType\": \"text\",
          \"parts\": [\"New session ${TIMESTAMP_2}: What is 5+5?\"]
        }
      }
    ],
    \"agent_type\": \"zed_external\",
    \"stream\": false
  }" \
  "http://localhost:8080/api/v1/sessions/chat" > /tmp/chat-response-new-session.log 2>&1 &
CHAT_PID_NEW=$!

echo "   New session request sent (PID: $CHAT_PID_NEW)"
echo "   Waiting 10 seconds for new session to be created..."
sleep 10

# Get the new session ID (should be different from first session)
SESSION_ID_2=$(docker compose -f docker-compose.dev.yaml exec api curl -s --unix-socket /var/run/wolf/wolf.sock http://localhost/api/v1/apps | jq -r '.apps[] | select(.title | startswith("External Agent ses_")) | .title' | sed 's/External Agent //' | grep -v "$SESSION_ID" | head -1)

if [ -z "$SESSION_ID_2" ]; then
    echo "❌ Failed to create new session (no new Wolf app found)"
else
    echo "✅ New session created: $SESSION_ID_2"
    echo "   Waiting 25 seconds for AI response..."
    sleep 25

    # Check new session for response
    SESSION_DATA_NEW=$(curl -s -H "Authorization: Bearer $API_KEY" \
        "http://localhost:8080/api/v1/sessions/$SESSION_ID_2")

    INTERACTIONS_NEW=$(echo "$SESSION_DATA_NEW" | jq -r '.interactions | length' 2>/dev/null || echo "0")
    echo "   New session has $INTERACTIONS_NEW interactions"

    if [ "$INTERACTIONS_NEW" -gt 0 ]; then
        RESPONSE_NEW=$(echo "$SESSION_DATA_NEW" | jq -r '.interactions[0].response_message' 2>/dev/null)
        if [ "$RESPONSE_NEW" != "null" ] && [ -n "$RESPONSE_NEW" ]; then
            echo "✅ Got response from Zed in new session!"
            echo "   Response: ${RESPONSE_NEW:0:100}..."
        else
            echo "❌ No response in new session"
        fi
    else
        echo "❌ No interactions in new session"
    fi
fi

kill $CHAT_PID_NEW 2>/dev/null || true

# Cleanup
kill $STREAM_PID 2>/dev/null || true
kill $CHAT_PID 2>/dev/null || true

echo ""
echo "✅ Test complete!"
echo ""
echo "Summary:"
echo "- First Session ID: $SESSION_ID"
echo "- Second Session ID: ${SESSION_ID_2:-none}"
echo "- Container: $CONTAINER"
echo "- Check logs above for WebSocket activity"
echo ""
echo "Expected logs to see:"
echo "  🔧 [ZED] Setting up WebSocket integration"
echo "  ✅ [WEBSOCKET] WebSocket connected"
echo "  📥 [WEBSOCKET-IN] Received text: {\"type\":\"chat_message\"...}"
echo "  📨 [THREAD_SERVICE] Received thread creation request"
