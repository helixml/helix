#!/bin/bash

echo "🔍 Testing if Helix sends WebSocket messages to Zed"
echo "================================================="

# Clean up
pkill -f 'zed-build/zed' 2>/dev/null || true
sleep 2

# Start Zed with WebSocket sync enabled in background and capture all logs
echo "🚀 Starting Zed with WebSocket sync..."
ZED_EXTERNAL_SYNC_ENABLED=true \
ZED_WEBSOCKET_SYNC_ENABLED=true \
ZED_HELIX_URL=localhost:8080 \
ZED_HELIX_TOKEN=hl-A_7_46c0-jtUklU1RakIp1o8drgFLz5IZSlqowjrS-g= \
ZED_HELIX_TLS=false \
ZED_CONFIG_DIR=/home/luke/pm/helix/test-zed-config/config \
ZED_DATA_DIR=/home/luke/pm/helix/test-zed-config/data \
./zed-build/zed > /tmp/zed-websocket-messages.log 2>&1 &

ZED_PID=$!
echo "✅ Zed started with PID: $ZED_PID"

# Wait for Zed to connect
echo "⏳ Waiting for Zed to connect to WebSocket..."
sleep 5

# Check if Zed is still running
if ! kill -0 $ZED_PID 2>/dev/null; then
    echo "❌ Zed crashed during startup!"
    cat /tmp/zed-websocket-messages.log
    exit 1
fi

echo "✅ Zed should be connected to WebSocket"

# Now create a Helix session with external agent
echo "📝 Creating Helix session with zed_external agent..."
SESSION_RESPONSE=$(curl -s -X POST http://localhost:8080/api/v1/sessions/chat \
  -H "Authorization: Bearer hl-A_7_46c0-jtUklU1RakIp1o8drgFLz5IZSlqowjrS-g=" \
  -H "Content-Type: application/json" \
  -d '{
    "session_id": "",
    "agent_type": "zed_external",
    "app_id": "app_01k5qka10zk6fp4daw3pjwv7xz",
    "stream": false,
    "messages": [
      {
        "content": {
          "content_type": "text",
          "parts": ["Hello Zed! This message should be sent via WebSocket to trigger thread creation."]
        },
        "role": "user"
      }
    ]
  }')

SESSION_ID=$(echo "$SESSION_RESPONSE" | jq -r '.id // empty')
echo "✅ Created Helix session: $SESSION_ID"

# Wait for WebSocket message processing
echo "⏳ Waiting for WebSocket messages to be processed..."
sleep 8

# Check Zed logs for WebSocket messages
echo ""
echo "📋 Checking Zed logs for WebSocket messages:"
echo "============================================"

if [ -f /tmp/zed-websocket-messages.log ]; then
    echo "🔍 Looking for WebSocket messages received by Zed..."
    grep -E "(WebSocket|message|chat_message|CreateThread)" /tmp/zed-websocket-messages.log || echo "❌ No WebSocket messages found in Zed logs"
    
    echo ""
    echo "🔍 Looking for thread creation attempts..."
    grep -E "(thread.*creation|pending.*request|AGENT_PANEL)" /tmp/zed-websocket-messages.log || echo "❌ No thread creation logs found"
    
    echo ""
    echo "📋 Full Zed log (last 20 lines):"
    echo "================================="
    tail -20 /tmp/zed-websocket-messages.log
else
    echo "❌ No Zed log file found"
fi

# Check if threads were created in Zed database
echo ""
echo "🔍 Checking Zed threads database..."
if [ -f "test-zed-config/zed/threads/threads.db" ]; then
    THREAD_COUNT=$(sqlite3 test-zed-config/zed/threads/threads.db "SELECT COUNT(*) FROM threads;" 2>/dev/null || echo "0")
    echo "📊 Found $THREAD_COUNT thread(s) in Zed database"
else
    echo "❌ No threads database found"
fi

# Clean up
echo ""
echo "🧹 Cleaning up..."
kill $ZED_PID 2>/dev/null || true
echo "✅ Test complete!"
