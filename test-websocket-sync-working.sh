#!/bin/bash
# WebSocket Sync E2E Test - WORKING VERSION
# Tests bidirectional sync between Helix API and Zed agent

API_URL="http://localhost:8080"
API_KEY="hl-CMxG1hM0UuedKIgrJwzGNQE9pEi3UPTlEezkSUuCJbI="
APP_ID="app_01k63mw4p0ezkgpt1hsp3reag4"

echo "============================================="
echo "WebSocket Sync E2E Test"
echo "============================================="
echo "Branch: $(cd ~/pm/zed && git branch --show-current)"
echo "Commit: $(cd ~/pm/zed && git log --oneline -1)"
echo ""

# 1. Create session (non-streaming to get session ID quickly)
echo "📝 Creating external agent session..."
SESSION_RESPONSE=$(curl -s -X POST "$API_URL/api/v1/sessions/chat" \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d "{\"session_id\": \"\", \"type\": \"text\", \"app_id\": \"$APP_ID\", \"messages\": [{\"role\": \"user\", \"content\": {\"content_type\": \"text\", \"parts\": [\"Write hello world in Python\"]}}]}")

# Extract session ID from response
SESSION_ID=$(echo "$SESSION_RESPONSE" | grep -o '"id":"ses_[^"]*"' | head -1 | cut -d'"' -f4)

if [ -z "$SESSION_ID" ]; then
  echo "❌ Failed to extract session ID"
  echo "Response:"
  echo "$SESSION_RESPONSE"
  exit 1
fi

echo "✅ Session created: $SESSION_ID"

# 2. Wait for container
echo "⏳ Waiting for Zed container..."
sleep 5

# 3. Check container
CONTAINER_ID=$(docker ps --filter "name=zed-external-${SESSION_ID/ses_/}" --format "{{.ID}}" | head -1)
if [ -n "$CONTAINER_ID" ]; then
  echo "✅ Zed container running: $CONTAINER_ID"

  # Check binary
  CONTAINER_MD5=$(docker exec "$CONTAINER_ID" md5sum /zed-build/zed 2>/dev/null | awk '{print $1}')
  HOST_MD5=$(md5sum ~/pm/helix/zed-build/zed | awk '{print $1}')

  if [ "$CONTAINER_MD5" = "$HOST_MD5" ]; then
    echo "✅ Binary matches: $HOST_MD5"
  else
    echo "⚠️  Binary mismatch! Container: $CONTAINER_MD5, Host: $HOST_MD5"
  fi
else
  echo "❌ No Zed container found!"
fi

echo ""

# 4. Poll for response using CORRECT field path
echo "⏳ Polling for AI response (max 30 seconds)..."
for i in {1..30}; do
  SESSION_DATA=$(curl -s "$API_URL/api/v1/sessions/$SESSION_ID" \
    -H "Authorization: Bearer $API_KEY")

  # Check for response - CORRECT field path: interactions[0].response_message
  RESPONSE=$(echo "$SESSION_DATA" | python3 -c "
import sys, json
try:
    data = json.load(sys.stdin)
    interactions = data.get('interactions', [])
    if interactions:
        msg = interactions[0].get('response_message', '')
        if msg:
            print(msg)
except:
    pass
" 2>/dev/null)

  if [ -n "$RESPONSE" ] && [ "$RESPONSE" != "null" ]; then
    echo ""
    echo "✅ Got AI response after $i seconds!"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "$RESPONSE" | head -30
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""

    # Show WebSocket logs
    if [ -n "$CONTAINER_ID" ]; then
      echo "📋 Zed WebSocket logs:"
      docker logs "$CONTAINER_ID" 2>&1 | grep -E "WEBSOCKET|thread_created|message_completed" | tail -10
    fi

    echo ""
    echo "✅ TEST PASSED - WebSocket sync working!"
    exit 0
  fi

  echo -ne "   Polling attempt $i/30...\r"
  sleep 1
done

echo ""
echo "❌ TEST FAILED - No response after 30 seconds"
echo ""

# Dump debugging info
echo "📋 Session data:"
echo "$SESSION_DATA" | python3 -m json.tool 2>&1 | head -50

if [ -n "$CONTAINER_ID" ]; then
  echo ""
  echo "📋 Zed container logs (last 50 lines):"
  docker logs "$CONTAINER_ID" 2>&1 | tail -50
fi

echo ""
echo "📋 API logs for session:"
docker compose -f ~/pm/helix/docker-compose.dev.yaml logs --tail 50 api | grep "$SESSION_ID"

exit 1
