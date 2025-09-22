#!/bin/bash

echo "🚀 Quick WebSocket Test (non-hanging)"
echo "====================================="

# Start Zed with timeout and capture output
echo "📱 Starting Zed with 60 second timeout - you can see the UI now..."
timeout 60 bash -c 'ZED_EXTERNAL_SYNC_ENABLED=true ZED_WEBSOCKET_SYNC_ENABLED=true ZED_HELIX_URL=localhost:8080 ZED_HELIX_TOKEN=hl-A_7_46c0-jtUklU1RakIp1o8drgFLz5IZSlqowjrS-g= ZED_HELIX_TLS=false ./zed-build/zed 2>&1' > /tmp/zed-quick.log &

ZED_PID=$!
sleep 3

# Create session quickly
echo "📝 Creating session..."
curl -s -X POST http://localhost:8080/api/v1/sessions/chat \
  -H "Authorization: Bearer hl-A_7_46c0-jtUklU1RakIp1o8drgFLz5IZSlqowjrS-g=" \
  -H "Content-Type: application/json" \
  -d '{
    "session_id": "",
    "agent_type": "zed_external", 
    "app_id": "app_01k5qka10zk6fp4daw3pjwv7xz",
    "stream": false,
    "messages": [{"content": {"content_type": "text", "parts": ["Test message"]}, "role": "user"}]
  }' > /tmp/session-response.json

echo "✅ Session created"

# Wait for processing and let you see the UI
echo "🎯 Session created! Check Zed UI - thread should appear in AI panel"
echo "⏰ Zed will stay open for 60 seconds so you can see the thread creation..."
echo "📋 You should see the thread with message: 'Test message'"

# Wait for the full 60 seconds, then clean up
sleep 55

echo "⏰ 5 seconds left..."
sleep 5

# Kill Zed 
echo "🛑 Stopping Zed..."
kill $ZED_PID 2>/dev/null || true
wait $ZED_PID 2>/dev/null || true

echo ""
echo "📋 Zed logs:"
echo "============"
cat /tmp/zed-quick.log

echo ""
echo "📋 Session response:"
echo "==================="
cat /tmp/session-response.json

echo ""
echo "✅ Test complete!"
