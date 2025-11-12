#!/bin/bash
set -e

# Helper script to run Moonlight stress tests with automatic token management

# Get admin password from environment or .env file
if [ -z "$ADMIN_USER_PASSWORD" ]; then
    if [ -f .env ]; then
        export $(grep ADMIN_USER_PASSWORD .env | xargs)
    fi
fi

if [ -z "$ADMIN_USER_PASSWORD" ]; then
    echo "❌ ADMIN_USER_PASSWORD not set"
    echo "Set it in .env or export it: export ADMIN_USER_PASSWORD=your-password"
    exit 1
fi

# Get admin user email (default to luke.marsden@gmail.com)
ADMIN_EMAIL=${ADMIN_EMAIL:-luke.marsden@gmail.com}

echo "🔐 Logging in as $ADMIN_EMAIL to get auth token..."

# Login and get token
ADMIN_TOKEN=$(curl -s http://localhost:8080/api/v1/auth/login \
    -H "Content-Type: application/json" \
    -d "{\"username\":\"$ADMIN_EMAIL\",\"password\":\"$ADMIN_USER_PASSWORD\"}" \
    | jq -r '.token')

if [ -z "$ADMIN_TOKEN" ] || [ "$ADMIN_TOKEN" == "null" ]; then
    echo "❌ Failed to get auth token. Is the API running?"
    exit 1
fi

echo "✅ Got auth token (expires in 24h)"

# Export for tests
export ADMIN_TOKEN
export HELIX_API_URL=http://localhost:8080

# Run tests
cd api/pkg/server

if [ "$1" == "health" ]; then
    echo "🏥 Running health check only..."
    go test -run TestMoonlightHealthCheck -v
elif [ "$1" == "quick" ]; then
    echo "⚡ Running quick tests (scenarios 1-2)..."
    go test -run "TestMoonlightStressScenario[12]|TestMoonlightHealthCheck" -v -timeout 5m
elif [ "$1" == "leak" ]; then
    echo "🔍 Running memory leak detection..."
    go test -run TestMoonlightMemoryLeak -v -timeout 10m
elif [ -n "$1" ]; then
    echo "🎯 Running test: $1"
    go test -run "$1" -v -timeout 10m
else
    echo "🚀 Running all Moonlight stress tests..."
    go test -run TestMoonlight -v -timeout 15m
fi
