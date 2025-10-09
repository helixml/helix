#!/bin/bash
set -e

API_KEY="hl-80B8fQwxPScxobApjVvA-ag8N7_m6X48ss0qPu3Dvig="
ZED_APP_ID="app_01k63mw4p0ezkgpt1hsp3reag4"

echo "🧪 CORRECT BIDIRECTIONAL SYNC TEST"
echo "==================================="
echo "Using: /api/v1/sessions/chat with app_id=$ZED_APP_ID"
echo ""

echo "Creating new Zed agent session..."
RESPONSE=$(curl -s -X POST "http://localhost:8080/api/v1/sessions/chat" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $API_KEY" \
  -d '{
    "session_id": "",
    "app_id": "'$ZED_APP_ID'",
    "messages": [{
      "role": "user",
      "content": {"content_type": "text", "parts": ["Please respond with: BIDIRECTIONAL_SYNC_SUCCESS"]}
    }]
  }')

# Extract session ID from response
SESSION_ID=$(echo "$RESPONSE" | grep -o 'ses_[a-z0-9]*' | head -1)

if [ -z "$SESSION_ID" ]; then
  echo "❌ Failed to create session"
  echo "Response: $RESPONSE" | head -c 500
  exit 1
fi

echo "✅ Session created: $SESSION_ID"

echo ""
echo "Waiting 60 seconds for:"
echo "  - Container to start"
echo "  - Settings daemon to sync (agent.default_model)"
echo "  - Zed to start with correct config"
echo "  - WebSocket to connect"
echo "  - Zed to process message and respond"
sleep 60

echo ""
echo "Checking for Zed response..."
INTERACTIONS=$(curl -s "http://localhost:8080/api/v1/sessions/$SESSION_ID" \
  -H "Authorization: Bearer $API_KEY" | jq -r '.interactions[]')

echo "Interaction count: $(echo "$INTERACTIONS" | jq -s 'length')"
echo ""

RESPONSE_TEXT=$(echo "$INTERACTIONS" | jq -r 'select(.response_message != null and .response_message != "") | .response_message' | head -1)

if [ -n "$RESPONSE_TEXT" ]; then
  echo "✅✅✅ SUCCESS! ZED RESPONDED! ✅✅✅"
  echo ""
  echo "Response: $RESPONSE_TEXT"
  echo ""
else
  echo "❌ No response yet"
  echo ""
  echo "Interaction states:"
  echo "$INTERACTIONS" | jq -r '.state'
  
  echo ""
  echo "Finding container for debugging..."
  CONTAINER=$(docker ps --filter "name=zed-external" --format "{{.Names}}" --latest)
  if [ -n "$CONTAINER" ]; then
    echo "Container: $CONTAINER"
    echo ""
    echo "Settings.json:"
    docker exec "$CONTAINER" cat /home/retro/.config/zed/settings.json | jq '.agent.default_model'
    echo ""
    echo "Last 20 lines of Zed logs:"
    docker logs "$CONTAINER" 2>&1 | tail -20
  fi
fi

echo ""
echo "Session: $SESSION_ID"
echo "App ID: $ZED_APP_ID"
