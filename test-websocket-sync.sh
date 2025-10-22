#!/bin/bash
# WebSocket Sync E2E Test Script
# Tests bidirectional sync between Helix API and Zed agent

set -e

API_URL="http://localhost:8080"
API_KEY="hl-CMxG1hM0UuedKIgrJwzGNQE9pEi3UPTlEezkSUuCJbI="
APP_ID="app_01k63mw4p0ezkgpt1hsp3reag4"

echo "============================================="
echo "WebSocket Sync E2E Test"
echo "============================================="
echo ""

# 1. Create session with test message
echo "📝 Creating new external agent session with test message..."
SESSION_RESPONSE=$(curl -s -X POST "$API_URL/api/v1/sessions/chat" \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d "{\"session_id\": \"\", \"type\": \"text\", \"app_id\": \"$APP_ID\", \"messages\": [{\"role\": \"user\", \"content\": {\"content_type\": \"text\", \"parts\": [\"Write a hello world function in Python\"]}}]}")

echo "Response: $SESSION_RESPONSE"

# Check if response is empty
if [ -z "$SESSION_RESPONSE" ]; then
  echo "❌ Empty response from session creation"
  exit 1
fi

# Parse session ID - handle both direct ID or JSON response
SESSION_ID=$(echo "$SESSION_RESPONSE" | python3 -c "
import sys, json
try:
    data = json.load(sys.stdin)
    print(data.get('id', ''))
except:
    sys.exit(1)
" 2>/dev/null || echo "")

if [ -z "$SESSION_ID" ]; then
  echo "❌ Failed to extract session ID from response"
  exit 1
fi

echo "✅ Session created: $SESSION_ID"
echo ""

# 2. Wait for container to start
echo "⏳ Waiting for Zed container to start (10 seconds)..."
sleep 10

# 3. Check if container is running
CONTAINER_ID=$(docker ps --filter "name=zed-external" --format "{{.ID}}" | head -1)
if [ -z "$CONTAINER_ID" ]; then
  echo "❌ No Zed container found!"
  exit 1
fi

echo "✅ Zed container running: $CONTAINER_ID"

# Check binary md5sum
CONTAINER_MD5=$(docker exec "$CONTAINER_ID" md5sum /zed-build/zed 2>/dev/null | awk '{print $1}')
HOST_MD5=$(md5sum ./zed-build/zed | awk '{print $1}')
echo "   Container binary MD5: $CONTAINER_MD5"
echo "   Host binary MD5: $HOST_MD5"

if [ "$CONTAINER_MD5" != "$HOST_MD5" ]; then
  echo "⚠️  WARNING: Binary mismatch! Container may be using old image."
fi
echo ""

# 4. Message already sent during session creation
echo "✅ Initial message sent during session creation"
echo ""

# 5. Poll for response
echo "⏳ Polling for AI response (max 30 seconds)..."
for i in {1..30}; do
  FULL_SESSION=$(curl -s "$API_URL/api/v1/sessions/$SESSION_ID" \
    -H "Authorization: Bearer $API_KEY")

  # Extract response message
  RESPONSE=$(echo "$FULL_SESSION" | python3 -c "
import sys, json
try:
    data = json.load(sys.stdin)
    interactions = data.get('interactions', [])
    if interactions and len(interactions) > 0:
        responses = interactions[0].get('responses', [])
        if responses and len(responses) > 0:
            msg = responses[0].get('message', '')
            if msg:
                print(msg)
except:
    pass
" 2>/dev/null)

  if [ -n "$RESPONSE" ] && [ "$RESPONSE" != "null" ]; then
    echo ""
    echo "✅ Got response after $i seconds!"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "$RESPONSE" | head -20
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""
    echo "✅ TEST PASSED - WebSocket sync is working!"
    exit 0
  fi

  echo -ne "   Attempt $i/30...\r"
  sleep 1
done

echo ""
echo "❌ TEST FAILED - No response after 30 seconds"
echo ""

# Dump logs for debugging
echo "📋 Recent Zed container logs:"
docker logs "$CONTAINER_ID" 2>&1 | grep -E "WEBSOCKET|THREAD|ERROR|FATAL" | tail -20

echo ""
echo "📋 Recent API logs:"
docker compose -f docker-compose.dev.yaml logs --tail 20 api | grep -i "websocket\|$SESSION_ID"

exit 1
