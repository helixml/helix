#!/bin/bash

echo "🔍 Debug WebSocket Sync - Tracing the full flow"
echo "=============================================="

# Clean up any existing processes
echo "🧹 Cleaning up existing test processes..."
pkill -f 'zed-build/zed' 2>/dev/null || true
sleep 2

# Clear the threads database to start fresh
echo "🗄️ Clearing Zed threads database..."
rm -f test-zed-config/zed/threads/threads.db
mkdir -p test-zed-config/zed/threads

# Start Zed with maximum logging
echo "🚀 Starting Zed with debug logging..."
ZED_EXTERNAL_SYNC_ENABLED=true \
ZED_WEBSOCKET_SYNC_ENABLED=true \
ZED_HELIX_URL=localhost:8080 \
ZED_HELIX_TOKEN=oh-hallo-insecure-token \
ZED_HELIX_TLS=false \
ZED_AUTO_OPEN_AI_PANEL=true \
ZED_CONFIG_DIR=/home/luke/pm/helix/test-zed-config/config \
ZED_DATA_DIR=/home/luke/pm/helix/test-zed-config/data \
RUST_LOG=debug,external_websocket_sync=trace,agent_ui=debug \
./zed-build/zed > /tmp/zed-debug.log 2>&1 &

ZED_PID=$!
echo "✅ Zed started with PID: $ZED_PID"

# Wait for Zed to initialize
echo "⏳ Waiting for Zed to initialize..."
sleep 5

# Check if Zed is still running
if ! kill -0 $ZED_PID 2>/dev/null; then
    echo "❌ Zed crashed during startup!"
    echo "Last 20 lines of log:"
    tail -20 /tmp/zed-debug.log
    exit 1
fi

echo "✅ Zed is running"

# Now run a simple WebSocket test
echo "🔌 Testing WebSocket connection..."
timeout 15 go run integration-test/zed-websocket/integration_websocket_sync.go 2>&1 | head -30

echo ""
echo "📊 Checking results:"
echo "==================="

# Check Zed logs for key events
echo "🔍 Zed initialization logs:"
grep -E "(external.*websocket|WebSocket.*sync|thread.*creation)" /tmp/zed-debug.log | head -10

echo ""
echo "🔍 WebSocket connection logs:"
grep -E "(connect|handshake|established)" /tmp/zed-debug.log | head -5

echo ""
echo "🔍 Thread creation logs:"
grep -E "(CreateThread|thread.*creation|pending.*request)" /tmp/zed-debug.log | head -10

echo ""
echo "📊 Final thread count in database:"
if [ -f "test-zed-config/zed/threads/threads.db" ]; then
    sqlite3 test-zed-config/zed/threads/threads.db "SELECT COUNT(*) as thread_count FROM threads;" 2>/dev/null || echo "Database query failed"
else
    echo "No threads database found"
fi

# Clean up
echo ""
echo "🧹 Cleaning up..."
kill $ZED_PID 2>/dev/null || true
echo "✅ Debug complete!"
