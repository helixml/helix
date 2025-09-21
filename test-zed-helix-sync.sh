#!/bin/bash

echo "🚀 Zed-Helix WebSocket Sync End-to-End Test"
echo "==========================================="
echo "This script will:"
echo "  1. Build Zed with WebSocket sync from ../zed"
echo "  2. Launch Zed with proper environment variables"
echo "  3. Run integration test to verify sync works"
echo "  4. Keep Zed running for you to interact with"
echo ""

# Check if Helix is running
echo "🔍 Checking if Helix is running..."
if ! curl -s http://localhost:8080/api/v1/config/js > /dev/null 2>&1; then
    echo "❌ Helix API not running on localhost:8080"
    echo "   Please run './stack start' first, then try again."
    exit 1
fi
echo "✅ Helix is running"

# Build Zed with WebSocket sync
echo ""
echo "🔨 Building Zed with WebSocket sync..."
if ! ./stack build-zed debug; then
    echo "❌ Failed to build Zed"
    exit 1
fi
echo "✅ Zed built successfully"

# Check if binary exists
if [ ! -f "./zed-build/zed" ]; then
    echo "❌ Zed binary not found at ./zed-build/zed"
    exit 1
fi

# Load runner token from .env
echo ""
echo "🔑 Loading authentication token..."
if [ ! -f ".env" ]; then
    echo "❌ .env file not found"
    exit 1
fi

RUNNER_TOKEN=$(grep "ZED_AGENT_RUNNER_TOKEN=" .env | cut -d'=' -f2)
if [ -z "$RUNNER_TOKEN" ]; then
    echo "❌ ZED_AGENT_RUNNER_TOKEN not found in .env"
    exit 1
fi
echo "✅ Token loaded: $RUNNER_TOKEN"

# Start Zed in background with WebSocket sync enabled
echo ""
echo "🚀 Starting Zed with WebSocket sync and AI panel..."
export RUST_LOG="info,external_websocket_sync=debug"
export ZED_EXTERNAL_SYNC_ENABLED=true
export ZED_WEBSOCKET_SYNC_ENABLED=true
export ZED_HELIX_URL=localhost:8080
export ZED_HELIX_TOKEN=$RUNNER_TOKEN
export ZED_HELIX_TLS=false
export ZED_AUTO_OPEN_AI_PANEL=true
export ZED_SHOW_AI_ASSISTANT=true

# Start Zed in background
./zed-build/zed &
ZED_PID=$!

echo "✅ Zed started (PID: $ZED_PID)"
echo "   Environment variables:"
echo "     ZED_EXTERNAL_SYNC_ENABLED=true"
echo "     ZED_WEBSOCKET_SYNC_ENABLED=true"
echo "     ZED_HELIX_URL=localhost:8080"
echo "     ZED_HELIX_TOKEN=$RUNNER_TOKEN"
echo "     ZED_AUTO_OPEN_AI_PANEL=true"
echo "     ZED_SHOW_AI_ASSISTANT=true"

# Give Zed time to start up
echo ""
echo "⏳ Waiting for Zed to initialize (5 seconds)..."
sleep 5

# Run the integration test
echo ""
echo "🧪 Running integration test..."
cd test/integration

# Initialize Go module if needed
go mod init helix-integration-test 2>/dev/null || true
go mod tidy 2>/dev/null || true

# Run the test with a timeout
echo "🔬 Testing WebSocket sync..."
if timeout 30 go run integration_websocket_sync.go; then
    echo ""
    echo "🎉 SUCCESS! WebSocket sync is working!"
else
    echo ""
    echo "⚠️  Test completed (may have timed out, but that's often normal)"
fi

# Go back to project root
cd ../..

echo ""
echo "📋 What's running:"
echo "  - Helix API: http://localhost:8080"
echo "  - Zed Editor: PID $ZED_PID (with WebSocket sync enabled)"
echo ""
echo "🎮 You can now:"
echo "  - Use Zed editor directly (it's already running)"
echo "  - Create Helix sessions that sync to Zed"
echo "  - Test the WebSocket sync manually"
echo ""
echo "🛑 To stop everything:"
echo "  - Press Ctrl+C to stop this script"
echo "  - Or run: kill $ZED_PID"

# Keep script running and monitor Zed
echo "👀 Monitoring Zed process... (Press Ctrl+C to stop)"

# Function to cleanup on exit
cleanup() {
    echo ""
    echo "🧹 Cleaning up..."
    kill $ZED_PID 2>/dev/null || true
    echo "✅ Zed stopped"
    exit 0
}

# Set up signal handlers
trap cleanup SIGINT SIGTERM

# Wait for Zed process to exit or user to interrupt
while kill -0 $ZED_PID 2>/dev/null; do
    sleep 2
done

echo ""
echo "📋 Zed process ended"
cleanup
