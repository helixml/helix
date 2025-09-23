#!/bin/bash

echo "🔍 Testing if Zed actually tries to connect to WebSocket"
echo "======================================================"

# Clean up
pkill -f 'zed-build/zed' 2>/dev/null || true
sleep 2

# Start Zed with WebSocket sync enabled and capture logs
echo "🚀 Starting Zed with WebSocket sync enabled..."
echo "   Environment variables:"
echo "   ZED_EXTERNAL_SYNC_ENABLED=true"
echo "   ZED_WEBSOCKET_SYNC_ENABLED=true"
echo "   ZED_HELIX_URL=localhost:8080"
echo "   ZED_HELIX_TOKEN=oh-hallo-insecure-token"

ZED_EXTERNAL_SYNC_ENABLED=true \
ZED_WEBSOCKET_SYNC_ENABLED=true \
ZED_HELIX_URL=localhost:8080 \
ZED_HELIX_TOKEN=oh-hallo-insecure-token \
ZED_HELIX_TLS=false \
RUST_LOG=error,external_websocket_sync=error \
timeout 10 ./zed-build/zed 2>&1 | tee /tmp/zed-real-test.log &

ZED_PID=$!
echo "✅ Zed started with PID: $ZED_PID"

# Wait and check logs
sleep 8
echo ""
echo "📋 Checking Zed logs for WebSocket initialization:"
echo "================================================="

if [ -f /tmp/zed-real-test.log ]; then
    echo "🔍 Looking for initialization logs..."
    grep -E "(INIT|External|WebSocket|sync|enabled)" /tmp/zed-real-test.log || echo "❌ No initialization logs found"
    
    echo ""
    echo "🔍 Looking for connection attempts..."
    grep -E "(connect|handshake|established|failed)" /tmp/zed-real-test.log || echo "❌ No connection logs found"
    
    echo ""
    echo "📋 Full log content:"
    echo "==================="
    cat /tmp/zed-real-test.log
else
    echo "❌ No log file found"
fi

# Clean up
pkill -f 'zed-build/zed' 2>/dev/null || true
echo ""
echo "✅ Test complete!"
