#!/bin/bash

echo "🔍 REAL WebSocket Flow Test - Testing actual Zed ↔ Helix integration"
echo "=================================================================="

# Clean up
pkill -f 'zed-build/zed' 2>/dev/null || true
sleep 2

# Clear the threads database
echo "🗄️ Clearing Zed threads database..."
rm -f test-zed-config/zed/threads/threads.db
mkdir -p test-zed-config/zed/threads

# Start Zed with WebSocket sync enabled in background
echo "🚀 Starting Zed with WebSocket sync enabled..."
ZED_EXTERNAL_SYNC_ENABLED=true \
ZED_WEBSOCKET_SYNC_ENABLED=true \
ZED_HELIX_URL=localhost:8080 \
ZED_HELIX_TOKEN=hl-A_7_46c0-jtUklU1RakIp1o8drgFLz5IZSlqowjrS-g= \
ZED_HELIX_TLS=false \
ZED_CONFIG_DIR=/home/luke/pm/helix/test-zed-config/config \
ZED_DATA_DIR=/home/luke/pm/helix/test-zed-config/data \
RUST_LOG=error,external_websocket_sync=error \
./zed-build/zed > /dev/null 2>&1 &

ZED_PID=$!
echo "✅ Zed started with PID: $ZED_PID"

# Wait for Zed to initialize
echo "⏳ Waiting for Zed to initialize..."
sleep 5

# Check if Zed is still running
if ! kill -0 $ZED_PID 2>/dev/null; then
    echo "❌ Zed crashed during startup!"
    exit 1
fi

echo "✅ Zed is running"

# Now create a REAL Helix session with external agent and send a message
echo "📝 Creating Helix session with external Zed agent..."

# Create session with external agent
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
          "parts": [
            {
              "text": "Hello Zed! This is a real test message from Helix. Please create a thread and respond."
            }
          ]
        },
        "role": "user"
      }
    ]
  }')

echo "📋 Session response: $SESSION_RESPONSE"

# Extract session ID
SESSION_ID=$(echo "$SESSION_RESPONSE" | jq -r '.id // empty')
if [ -z "$SESSION_ID" ]; then
    echo "❌ Failed to create session"
    echo "Response: $SESSION_RESPONSE"
    pkill -f 'zed-build/zed' 2>/dev/null || true
    exit 1
fi

echo "✅ Created session: $SESSION_ID"

# Wait for WebSocket processing
echo "⏳ Waiting for WebSocket sync to process..."
sleep 5

# Check if threads were created in Zed database
echo "🔍 Checking Zed threads database..."
if [ -f "test-zed-config/zed/threads/threads.db" ]; then
    THREAD_COUNT=$(sqlite3 test-zed-config/zed/threads/threads.db "SELECT COUNT(*) FROM threads;" 2>/dev/null || echo "0")
    echo "📊 Found $THREAD_COUNT thread(s) in Zed database"
    
    if [ "$THREAD_COUNT" -gt "0" ]; then
        echo "✅ SUCCESS: Zed created threads!"
        echo "📋 Thread details:"
        sqlite3 test-zed-config/zed/threads/threads.db "SELECT id, summary FROM threads LIMIT 5;" 2>/dev/null || echo "Could not query thread details"
    else
        echo "❌ FAILURE: No threads created in Zed"
    fi
else
    echo "❌ FAILURE: No threads database found"
fi

# Check Helix session for responses
echo "🔍 Checking Helix session for responses..."
HELIX_RESPONSE=$(curl -s -X GET "http://localhost:8080/api/v1/sessions/$SESSION_ID" \
  -H "Authorization: Bearer hl-A_7_46c0-jtUklU1RakIp1o8drgFLz5IZSlqowjrS-g=")

echo "📋 Helix session state:"
echo "$HELIX_RESPONSE" | jq '.interactions | length' 2>/dev/null || echo "Could not parse interactions"

# Clean up
echo "🧹 Cleaning up..."
pkill -f 'zed-build/zed' 2>/dev/null || true

echo "✅ Real WebSocket flow test complete!"
