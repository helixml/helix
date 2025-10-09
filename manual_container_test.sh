#!/bin/bash
set -e

echo "🧪 MANUAL CONTAINER TEST - Creating Zed External Agent Container Directly"
echo "========================================================================="
echo ""

SESSION_ID="manual-test-$(date +%s)"
HELIX_SESSION_ID="session-$(uuidgen | tr '[:upper:]' '[:lower:]')"

echo "📝 Session IDs:"
echo "  Agent Session ID: $SESSION_ID"
echo "  Helix Session ID: $HELIX_SESSION_ID"
echo ""

# Create container directly using Wolf's image settings
echo "🐋 Step 1: Creating container with Wolf configuration..."
docker run -d \
  --name "zed-external-$SESSION_ID" \
  --privileged \
  --cap-add SYS_ADMIN \
  --cap-add SYS_NICE \
  --cap-add SYS_PTRACE \
  --cap-add NET_RAW \
  --cap-add MKNOD \
  --cap-add NET_ADMIN \
  --security-opt seccomp=unconfined \
  --security-opt apparmor=unconfined \
  --device-cgroup-rule="c 13:* rmw" \
  --device-cgroup-rule="c 244:* rmw" \
  --ipc=host \
  -e HELIX_SESSION_ID="$HELIX_SESSION_ID" \
  -e SESSION_ID="$SESSION_ID" \
  -e DISPLAY=:1 \
  -v /home/luke/pm/helix/zed-build:/zed-build:ro \
  helix-sway:latest

CONTAINER_ID=$(docker ps --filter "name=zed-external-$SESSION_ID" -q)
if [ -z "$CONTAINER_ID" ]; then
  echo "❌ Failed to create container"
  exit 1
fi

echo "✅ Container created: $CONTAINER_ID"
echo ""

# Wait for Sway to start
echo "⏳ Step 2: Waiting for Sway to initialize (15 seconds)..."
sleep 15
echo ""

# Create Zed config directory and settings.json
echo "📄 Step 3: Creating Zed settings.json with full config..."
docker exec "$CONTAINER_ID" bash -c 'mkdir -p /root/.config/zed'

docker exec "$CONTAINER_ID" bash -c 'cat > /root/.config/zed/settings.json << "SETTINGS"
{
  "context_servers": {},
  "language_models": {
    "anthropic": {
      "version": "1",
      "api_url": "http://api:8080/api/v1/worker/openai/chat/completions"
    }
  },
  "assistant": {
    "version": "2",
    "default_model": {
      "provider": "anthropic",
      "model": "claude-sonnet-4-5"
    }
  },
  "external_sync": {
    "websocket_sync": {
      "enabled": true,
      "external_url": "ws://api:8080/api/v1/external-agents/sync?session_id='$SESSION_ID'",
      "auto_create_thread": true
    }
  },
  "agent": {
    "enabled": true,
    "auto_create_thread": true
  },
  "theme": "One Dark"
}
SETTINGS'

echo "✅ settings.json created"
echo ""

# Verify settings
echo "📋 Step 4: Verifying settings.json..."
docker exec "$CONTAINER_ID" cat /root/.config/zed/settings.json | jq '.'
echo ""

# Start Zed
echo "🚀 Step 5: Starting Zed editor..."
docker exec -d "$CONTAINER_ID" bash /usr/local/bin/start-zed-helix.sh
sleep 5
echo ""

# Check Zed process
echo "🔧 Step 6: Checking Zed process..."
docker exec "$CONTAINER_ID" ps aux | grep zed | grep -v grep || echo "⚠️  Zed not running"
echo ""

# Test screenshot server
echo "📸 Step 7: Testing screenshot server..."
docker exec "$CONTAINER_ID" curl -s -I http://localhost:9876/screenshot --max-time 5 || echo "Screenshot server not responding"
echo ""

# Check settings-sync-daemon
echo "🔄 Step 8: Checking for settings-sync-daemon..."
docker exec "$CONTAINER_ID" ps aux | grep settings-sync-daemon | grep -v grep || echo "⚠️  Daemon not running (expected - we created settings manually)"
echo ""

echo "📊 TEST SUMMARY"
echo "==============="
echo "Container ID: $CONTAINER_ID"
echo "Container Name: zed-external-$SESSION_ID"
echo "Agent Session ID: $SESSION_ID"
echo "Helix Session ID: $HELIX_SESSION_ID"
echo ""
echo "Next steps:"
echo "1. Container is running with Zed and full settings.json"
echo "2. WebSocket URL configured: ws://api:8080/api/v1/external-agents/sync?session_id=$SESSION_ID"
echo "3. Screenshot server should be available on port 9876"
echo ""
echo "To cleanup: docker stop $CONTAINER_ID && docker rm $CONTAINER_ID"
