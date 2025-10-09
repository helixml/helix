#!/bin/bash
set -e

echo "🧪 FULL AUTONOMOUS EXTERNAL AGENT TEST"
echo "======================================"
echo ""

# Get Keycloak token for admin user
echo "🔐 Step 1: Getting Keycloak token..."
TOKEN_RESPONSE=$(curl -s -X POST "http://localhost:8080/auth/realms/helix/protocol/openid-connect/token" \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "username=admin" \
  -d "password=oh-hallo-insecure-password" \
  -d "grant_type=password" \
  -d "client_id=helix-client")

ACCESS_TOKEN=$(echo "$TOKEN_RESPONSE" | jq -r '.access_token')
if [ -z "$ACCESS_TOKEN" ] || [ "$ACCESS_TOKEN" == "null" ]; then
  echo "❌ Failed to get access token"
  echo "Response: $TOKEN_RESPONSE"
  exit 1
fi
echo "✅ Got access token (${#ACCESS_TOKEN} chars)"
echo ""

# Create external agent session
echo "📝 Step 2: Creating external agent session..."
SESSION_ID="test-$(date +%s)"
CREATE_RESPONSE=$(curl -s -X POST "http://localhost:8080/api/v1/external-agents" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -d '{
    "session_id": "'$SESSION_ID'",
    "input": "Hello! Can you help me test the bidirectional WebSocket sync between Zed and Helix? Please respond with a simple greeting."
  }')

echo "Create response: $CREATE_RESPONSE" | jq '.'
AGENT_SESSION_ID=$(echo "$CREATE_RESPONSE" | jq -r '.session_id')
HELIX_SESSION_ID=$(echo "$CREATE_RESPONSE" | jq -r '.helix_session_id // empty')

if [ -z "$AGENT_SESSION_ID" ] || [ "$AGENT_SESSION_ID" == "null" ]; then
  echo "❌ Failed to create session"
  exit 1
fi
echo "✅ Agent Session ID: $AGENT_SESSION_ID"
echo "✅ Helix Session ID: $HELIX_SESSION_ID"
echo ""

# Wait for container to start
echo "⏳ Step 3: Waiting for container to start (45 seconds)..."
sleep 45
echo ""

# Find container
echo "🔍 Step 4: Finding container..."
CONTAINER_NAME=$(docker ps --filter "name=zed-external" --format "{{.Names}}" | head -1)
if [ -z "$CONTAINER_NAME" ]; then
  echo "❌ Container not found"
  docker ps --filter "name=zed"
  exit 1
fi
echo "✅ Container: $CONTAINER_NAME"
echo ""

# Check settings.json
echo "📄 Step 5: Checking Zed settings.json..."
SETTINGS=$(docker exec "$CONTAINER_NAME" cat /root/.config/zed/settings.json 2>/dev/null || echo "{}")
echo "$SETTINGS" | jq '.'
echo ""

# Verify critical fields
HAS_EXTERNAL_SYNC=$(echo "$SETTINGS" | jq -e '.external_sync' >/dev/null 2>&1 && echo "YES" || echo "NO")
HAS_LANGUAGE_MODELS=$(echo "$SETTINGS" | jq -e '.language_models' >/dev/null 2>&1 && echo "YES" || echo "NO")
HAS_WEBSOCKET_URL=$(echo "$SETTINGS" | jq -r '.external_sync.websocket_sync.external_url // empty')

echo "Configuration Check:"
echo "  external_sync: $HAS_EXTERNAL_SYNC"
echo "  language_models: $HAS_LANGUAGE_MODELS"  
echo "  WebSocket URL: $HAS_WEBSOCKET_URL"
echo ""

if [ "$HAS_EXTERNAL_SYNC" != "YES" ]; then
  echo "❌ FAILED: external_sync config missing!"
  echo ""
  echo "Checking settings-sync-daemon:"
  docker exec "$CONTAINER_NAME" ps aux | grep settings-sync
  echo ""
  echo "Checking API endpoint response:"
  curl -s "http://localhost:8080/api/v1/sessions/$HELIX_SESSION_ID/zed-config" \
    -H "Authorization: Bearer $ACCESS_TOKEN" | jq '.'
  exit 1
fi
echo "✅ Settings configuration looks good!"
echo ""

# Check Zed process
echo "🔧 Step 6: Checking Zed process..."
ZED_RUNNING=$(docker exec "$CONTAINER_NAME" ps aux | grep -v grep | grep zed || echo "NOT RUNNING")
echo "$ZED_RUNNING"
echo ""

# Wait a bit more for WebSocket connection
echo "⏳ Step 7: Waiting for WebSocket connection (15 seconds)..."
sleep 15
echo ""

# Check WebSocket connection status
echo "🌐 Step 8: Checking WebSocket connection..."
WS_LOGS=$(docker compose -f docker-compose.dev.yaml logs --tail 50 api | grep -i "websocket.*$AGENT_SESSION_ID\|external.*$AGENT_SESSION_ID" || echo "No logs found")
echo "$WS_LOGS"
echo ""

# Test screenshot endpoint
echo "📸 Step 9: Testing screenshot endpoint..."
if [ -n "$HELIX_SESSION_ID" ]; then
  curl -s "http://localhost:8080/api/v1/external-agents/$HELIX_SESSION_ID/screenshot" \
    -H "Authorization: Bearer $ACCESS_TOKEN" \
    -o /tmp/test-screenshot.png \
    -w "HTTP Status: %{http_code}\n"
  
  if [ -f /tmp/test-screenshot.png ]; then
    FILE_SIZE=$(stat -c%s /tmp/test-screenshot.png 2>/dev/null || stat -f%z /tmp/test-screenshot.png 2>/dev/null || echo "0")
    if [ "$FILE_SIZE" -gt 100 ]; then
      echo "✅ Screenshot saved: /tmp/test-screenshot.png (${FILE_SIZE} bytes)"
      file /tmp/test-screenshot.png
    else
      echo "⚠️  Screenshot file too small: ${FILE_SIZE} bytes"
      cat /tmp/test-screenshot.png
    fi
  fi
else
  echo "⚠️  Skipping - no Helix session ID"
fi
echo ""

# Check for Zed responses in the session
echo "💬 Step 10: Checking for Zed responses..."
INTERACTIONS=$(curl -s "http://localhost:8080/api/v1/sessions/$HELIX_SESSION_ID" \
  -H "Authorization: Bearer $ACCESS_TOKEN" | jq -r '.interactions // []')
echo "Interactions in session:"
echo "$INTERACTIONS" | jq -r '.[] | "[\(.created)] \(.creator): \(.message)"'
echo ""

# Final summary
echo "📊 FINAL TEST SUMMARY"
echo "===================="
echo "Agent Session ID: $AGENT_SESSION_ID"
echo "Helix Session ID: $HELIX_SESSION_ID"
echo "Container: $CONTAINER_NAME"
echo ""
echo "✅ = Working | ❌ = Failed | ⚠️ = Warning"
echo ""
echo "Configuration:"
echo "  external_sync config: $HAS_EXTERNAL_SYNC"
echo "  language_models config: $HAS_LANGUAGE_MODELS"
echo "  WebSocket URL configured: $([ -n "$HAS_WEBSOCKET_URL" ] && echo 'YES' || echo 'NO')"
echo ""
echo "Container name: $CONTAINER_NAME"
echo "Cleanup: docker stop $CONTAINER_NAME && docker rm $CONTAINER_NAME"
