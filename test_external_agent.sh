#!/bin/bash
set -e

API_URL="http://localhost:8080"
SESSION_ID="test-$(date +%s)"

echo "🧪 External Agent Test Suite"
echo "============================"
echo ""

# Step 1: Create external agent session
echo "📝 Step 1: Creating external agent session..."
CREATE_RESPONSE=$(curl -s -X POST "$API_URL/api/v1/external-agents" \
  -H "Content-Type: application/json" \
  -d '{
    "session_id": "'$SESSION_ID'",
    "input": "Hello, can you help me test the integration?"
  }')

echo "Response: $CREATE_RESPONSE"
AGENT_SESSION_ID=$(echo "$CREATE_RESPONSE" | jq -r '.session_id')
if [ -z "$AGENT_SESSION_ID" ] || [ "$AGENT_SESSION_ID" == "null" ]; then
  echo "❌ Failed to create session"
  exit 1
fi

echo "✅ Session created: $AGENT_SESSION_ID"
echo ""

# Step 2: Wait for container to start
echo "⏳ Step 2: Waiting for container to start (30 seconds)..."
sleep 30
echo ""

# Step 3: Find container name
echo "🔍 Step 3: Finding container name..."
CONTAINER_NAME=$(docker ps --filter "name=zed-external" --format "{{.Names}}" | head -1)
if [ -z "$CONTAINER_NAME" ]; then
  echo "❌ Container not found"
  docker ps --filter "name=zed-external"
  exit 1
fi
echo "✅ Container found: $CONTAINER_NAME"
echo ""

# Step 4: Check settings.json
echo "📄 Step 4: Checking Zed settings.json..."
SETTINGS=$(docker exec "$CONTAINER_NAME" cat /root/.config/zed/settings.json 2>/dev/null || echo "{}")
echo "Settings content:"
echo "$SETTINGS" | jq '.'
echo ""

# Check for external_sync
if echo "$SETTINGS" | jq -e '.external_sync' > /dev/null; then
  echo "✅ external_sync config found"
  echo "$SETTINGS" | jq '.external_sync'
else
  echo "❌ external_sync config MISSING"
fi
echo ""

# Step 5: Check settings-sync-daemon logs
echo "📋 Step 5: Checking settings-sync-daemon logs..."
docker exec "$CONTAINER_NAME" bash -c "ps aux | grep settings-sync-daemon | grep -v grep" || echo "⚠️  Daemon not running"
echo ""

# Step 6: Test screenshot server in container
echo "📸 Step 6: Testing screenshot server in container..."
docker exec "$CONTAINER_NAME" bash -c 'curl -s -I http://localhost:9876/screenshot --max-time 5' || echo "Screenshot server not responding"
echo ""

# Step 7: Get Helix session ID from API logs
echo "🔗 Step 7: Finding Helix session ID..."
HELIX_SESSION_ID=$(docker compose -f docker-compose.dev.yaml logs --tail 100 api | grep "agent_session_id.*$AGENT_SESSION_ID" | grep "helix_session_id" | tail -1 | sed -n 's/.*helix_session_id=\([a-z0-9-]*\).*/\1/p')
echo "Helix Session ID: $HELIX_SESSION_ID"
echo ""

# Step 8: Test API screenshot endpoint (using Helix session ID)
if [ -n "$HELIX_SESSION_ID" ]; then
  echo "🖼️  Step 8: Testing API screenshot endpoint..."
  API_SCREENSHOT_RESPONSE=$(curl -s -w "\n%{http_code}" "$API_URL/api/v1/external-agents/$HELIX_SESSION_ID/screenshot" --max-time 10 2>&1)
  API_HTTP_CODE=$(echo "$API_SCREENSHOT_RESPONSE" | tail -1)

  if [ "$API_HTTP_CODE" == "200" ]; then
    echo "✅ API screenshot endpoint responding with 200"
    # Save to file for inspection
    echo "$API_SCREENSHOT_RESPONSE" | head -n -1 > /tmp/test-screenshot.png
    FILE_SIZE=$(stat -c%s /tmp/test-screenshot.png 2>/dev/null || stat -f%z /tmp/test-screenshot.png 2>/dev/null)
    echo "✅ Screenshot saved to /tmp/test-screenshot.png (${FILE_SIZE} bytes)"
  else
    echo "❌ API screenshot endpoint returned: $API_HTTP_CODE"
  fi
else
  echo "⚠️  Skipping screenshot test - no Helix session ID found"
fi
echo ""

# Step 9: Check Zed process
echo "🔧 Step 9: Checking Zed process..."
docker exec "$CONTAINER_NAME" bash -c "ps aux | grep zed | grep -v grep" || echo "⚠️  Zed not running"
echo ""

# Step 10: Check WebSocket connection
echo "🌐 Step 10: Checking WebSocket logs in API..."
docker compose -f docker-compose.dev.yaml logs --tail 20 api | grep -i "websocket\|external.*$AGENT_SESSION_ID" || echo "No WebSocket logs found"
echo ""

echo "📊 Test Summary"
echo "==============="
echo "Agent Session ID: $AGENT_SESSION_ID"
echo "Helix Session ID: $HELIX_SESSION_ID"
echo "Container: $CONTAINER_NAME"
echo ""
echo "Next steps:"
echo "1. Go to http://localhost:3000"
echo "2. Find session: $HELIX_SESSION_ID"
echo "3. Send a test message"
echo "4. Check for Zed responses in the frontend"
echo ""
echo "To cleanup: docker stop $CONTAINER_NAME && docker rm $CONTAINER_NAME"
