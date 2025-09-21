#!/bin/bash

echo "🧪 Zed-Helix WebSocket Sync Integration Test"
echo "============================================"

# Check if Zed binary exists
if [ ! -f "./zed-build/zed" ]; then
    echo "❌ Zed binary not found. Please run './stack build-zed' first."
    exit 1
fi

echo "✅ Zed binary found at ./zed-build/zed"

# Check if Helix is running
if ! curl -s http://localhost:8080/api/v1/bootstrap > /dev/null 2>&1; then
    echo "❌ Helix API not running. Please run './stack start' first."
    exit 1
fi

echo "✅ Helix detected running on localhost:8080"

# Build and run the integration test
echo "🔨 Building integration test..."
cd test/integration
go mod init helix-integration-test 2>/dev/null || true
go mod tidy 2>/dev/null || true

echo "🚀 Running integration test..."
go run integration_websocket_sync.go
