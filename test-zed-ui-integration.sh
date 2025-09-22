#!/bin/bash

echo "🚀 Zed UI Integration Test"
echo "========================="

# Kill any existing Zed processes
pkill -f 'zed-build/zed' 2>/dev/null || true

# Start Zed in background and let it fully initialize
echo "📱 Starting Zed with full UI initialization..."
export ZED_EXTERNAL_SYNC_ENABLED=true
export ZED_WEBSOCKET_SYNC_ENABLED=true
export ZED_HELIX_URL=localhost:8080
export ZED_HELIX_TOKEN=hl-A_7_46c0-jtUklU1RakIp1o8drgFLz5IZSlqowjrS-g=
export ZED_HELIX_TLS=false

# Start Zed and capture logs
./zed-build/zed 2>/tmp/zed-ui-integration.log &
ZED_PID=$!

echo "🔄 Waiting for Zed to fully initialize (10 seconds)..."
sleep 10

echo "📝 Creating external session..."
SESSION_RESPONSE=$(curl -s -X POST http://localhost:8080/api/v1/sessions/chat \
  -H "Authorization: Bearer hl-A_7_46c0-jtUklU1RakIp1o8drgFLz5IZSlqowjrS-g=" \
  -H "Content-Type: application/json" \
  -d '{
    "session_id": "",
    "agent_type": "zed_external", 
    "app_id": "app_01k5qka10zk6fp4daw3pjwv7xz",
    "stream": false,
    "messages": [{"content": {"content_type": "text", "parts": ["Hello from integration test! This should create a thread in Zed."]}, "role": "user"}]
  }')

echo "✅ Session created"
echo "📋 Session response: $SESSION_RESPONSE"

echo ""
echo "🔄 Waiting for thread creation processing (5 seconds)..."
sleep 5

echo ""
echo "📋 Zed logs:"
echo "============"
tail -50 /tmp/zed-ui-integration.log

echo ""
echo "🔍 Checking for threads in database..."
if [ -f "test-zed-config/zed/threads/threads.db" ]; then
    THREAD_COUNT=$(sqlite3 test-zed-config/zed/threads/threads.db "SELECT COUNT(*) FROM threads;" 2>/dev/null || echo "0")
    echo "📊 Thread count in database: $THREAD_COUNT"
    
    if [ "$THREAD_COUNT" -gt 0 ]; then
        echo "✅ SUCCESS: Thread(s) created in Zed database!"
        sqlite3 test-zed-config/zed/threads/threads.db "SELECT id, title FROM threads LIMIT 5;" 2>/dev/null || true
    else
        echo "❌ No threads found in Zed database"
    fi
else
    echo "❌ Zed threads database not found"
fi

# Clean up
echo ""
echo "🧹 Cleaning up..."
kill $ZED_PID 2>/dev/null || true
wait $ZED_PID 2>/dev/null || true

echo "✅ Test complete!"
