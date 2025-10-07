#!/bin/bash

set -e

echo "🧪 Wolf Lobbies Auto-Start Test"
echo "================================"
echo ""

cd /home/luke/pm/helix
API_KEY="hl-_pwrvW_Foqw1mggPOs6lnnq0aS13ppQecIss-HG71WQ="

# Step 1: Check Wolf is running with wolf-ui image
echo "1. Checking Wolf-UI is running..."
WOLF_IMAGE=$(docker compose -f docker-compose.dev.yaml ps wolf --format json | jq -r '.Image')
if [[ "$WOLF_IMAGE" != *"wolf-ui"* ]]; then
    echo "❌ Wolf is not using wolf-ui image: $WOLF_IMAGE"
    exit 1
fi
echo "✅ Wolf-UI image confirmed: $WOLF_IMAGE"

# Step 2: Verify lobbies API is available
echo ""
echo "2. Testing Wolf lobbies API..."
LOBBIES=$(docker compose -f docker-compose.dev.yaml exec api curl -s --unix-socket /var/run/wolf/wolf.sock http://localhost/api/v1/lobbies)
if ! echo "$LOBBIES" | jq -e '.success == true' > /dev/null 2>&1; then
    echo "❌ Lobbies API not working"
    echo "$LOBBIES"
    exit 1
fi
echo "✅ Lobbies API responding: $(echo "$LOBBIES" | jq -r '.lobbies | length') active lobbies"

# Step 3: Create external agent session (should create lobby and start immediately)
echo ""
echo "3. Creating external agent session (lobby should auto-start)..."
TIMESTAMP=$(date +%s)

CREATE_RESPONSE=$(curl -s -X POST \
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
  "http://localhost:8080/api/v1/sessions/chat")

echo "   API Response (first 500 chars):"
echo "$CREATE_RESPONSE" | head -c 500
echo ""

# Extract session ID from response
SESSION_ID=$(echo "$CREATE_RESPONSE" | jq -r '.id // empty')
if [ -z "$SESSION_ID" ]; then
    echo "❌ Failed to create session or extract session ID"
    echo "$CREATE_RESPONSE" | jq '.'
    exit 1
fi

echo "✅ Session created: $SESSION_ID"

# Step 4: Check if lobby was created
echo ""
echo "4. Checking if Wolf lobby was created..."
sleep 3 # Give Wolf time to create lobby

LOBBIES_AFTER=$(docker compose -f docker-compose.dev.yaml exec api curl -s --unix-socket /var/run/wolf/wolf.sock http://localhost/api/v1/lobbies)
LOBBY_COUNT=$(echo "$LOBBIES_AFTER" | jq -r '.lobbies | length')

echo "   Active lobbies: $LOBBY_COUNT"
echo "$LOBBIES_AFTER" | jq -r '.lobbies[] | "   - " + .name + " (ID: " + .id + ")"'

if [ "$LOBBY_COUNT" -eq 0 ]; then
    echo "❌ No lobbies found! Auto-start failed?"
    exit 1
fi

echo "✅ Lobby created successfully"

# Step 5: Check if container is running (proves auto-start worked)
echo ""
echo "5. Checking if Zed container is running..."
sleep 5 # Give container time to start

CONTAINERS=$(docker ps --format '{{.Names}}' | grep -i "personal-dev\|zed-external\|lobby" || true)
if [ -z "$CONTAINERS" ]; then
    echo "❌ No Zed containers found running!"
    echo "   This suggests lobby didn't start container automatically"
    docker ps
    exit 1
fi

echo "✅ Container(s) running:"
echo "$CONTAINERS" | while read CONTAINER; do
    echo "   - $CONTAINER"
done

# Step 6: Check API logs for WebSocket connection (proves Zed connected to Helix)
echo ""
echo "6. Checking for Zed WebSocket connection..."
sleep 5 # Give Zed time to connect

WS_CONNECTION=$(docker compose -f docker-compose.dev.yaml logs api 2>&1 | grep "External agent WebSocket connected\|Registered external agent connection" | tail -1)
if [ -z "$WS_CONNECTION" ]; then
    echo "⚠️  No WebSocket connection found yet (may still be starting)"
    echo "   Last 20 API log lines:"
    docker compose -f docker-compose.dev.yaml logs --tail 20 api
else
    echo "✅ WebSocket connection established:"
    echo "   $WS_CONNECTION"
fi

# Step 7: Wait for response
echo ""
echo "7. Waiting for AI response (up to 60 seconds)..."
for i in {1..60}; do
    RESPONSE=$(echo "$CREATE_RESPONSE" | jq -r '.interactions[0].response_message // empty' 2>/dev/null || echo "")
    if [ -n "$RESPONSE" ] && [ "$RESPONSE" != "null" ]; then
        echo "✅ Got response: ${RESPONSE:0:100}..."
        break
    fi

    # Check session endpoint for updates
    SESSION_DATA=$(curl -s -H "Authorization: Bearer $API_KEY" "http://localhost:8080/api/v1/sessions/$SESSION_ID")
    RESPONSE=$(echo "$SESSION_DATA" | jq -r '.interactions[-1].response_message // empty' 2>/dev/null || echo "")
    if [ -n "$RESPONSE" ] && [ "$RESPONSE" != "null" ] && [ "$RESPONSE" != "" ]; then
        echo "✅ Got response from session endpoint: ${RESPONSE:0:100}..."
        break
    fi

    echo -n "."
    sleep 1
done

echo ""

# Step 8: Verify lobby PIN is stored
echo ""
echo "8. Checking if lobby PIN was stored in session..."
PIN=$(echo "$SESSION_DATA" | jq -r '.metadata.wolf_lobby_pin // empty')
if [ -n "$PIN" ] && [ "$PIN" != "null" ]; then
    echo "✅ Lobby PIN stored in session: $PIN"
else
    echo "⚠️  No lobby PIN found in session metadata"
fi

echo ""
echo "========================================="
echo "🎉 AUTO-START TEST COMPLETE!"
echo "========================================="
echo ""
echo "Summary:"
echo "  ✅ Wolf-UI image running"
echo "  ✅ Lobbies API working"
echo "  ✅ Session created via chat API"
echo "  ✅ Lobby auto-started (no Moonlight needed!)"
echo "  ✅ Container running immediately"
echo "  ${WS_CONNECTION:+✅}${WS_CONNECTION:-⚠️ } WebSocket connection"
echo "  ${RESPONSE:+✅}${RESPONSE:-⚠️ } AI response received"
echo "  ${PIN:+✅}${PIN:-⚠️ } Lobby PIN stored"
echo ""
echo "🎯 KEY ACHIEVEMENT: Container started WITHOUT Moonlight connection!"
echo "   This proves lobby auto-start is working."
echo ""
