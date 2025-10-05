#!/bin/bash

echo "🧪 Zed WebSocket Integration End-to-End Test"
echo "==========================================="
echo ""

cd /home/luke/pm/helix

# Ensure latest binary is built
echo "1. Building latest Zed..."
./stack build-zed > /tmp/zed_build.log 2>&1
if [ $? -ne 0 ]; then
    echo "❌ Build failed"
    tail -20 /tmp/zed_build.log
    exit 1
fi
echo "✅ Zed built successfully"

# Check Helix API
echo ""
echo "2. Checking Helix API..."
HEALTH=$(curl -s http://localhost:8080/health 2>&1 | head -1)
if [ -z "$HEALTH" ]; then
    echo "❌ Helix API not responding"
    exit 1
fi
echo "✅ Helix API is running"

# Create or find Zed agent app
echo ""
echo "3. Finding Zed external agent app..."
ZED_APP=$(curl -s -H "Authorization: Bearer $(cat .test_runner_token)" \
    "http://localhost:8080/api/v1/apps?owner=admin&model_name=zed" | \
    jq -r '.[] | select(.config.helix.assistants[0].name == "zed") | .id' | head -1)

if [ -z "$ZED_APP" ]; then
    echo "❌ No Zed agent app found"
    echo "   Create one via Helix UI first"
    exit 1
fi
echo "✅ Found Zed app: $ZED_APP"

# Create new session with Zed
echo ""
echo "4. Creating new Helix session with Zed agent..."
SESSION_RESPONSE=$(curl -s -X POST \
    -H "Authorization: Bearer $(cat .test_runner_token)" \
    -H "Content-Type: application/json" \
    -d '{
        "session_id": "",
        "session_type": "text",
        "session_mode": "inference", 
        "model_name": "zed",
        "lora_dir": "",
        "type": "text",
        "user_interact": {
            "files": [],
            "message": "Hello from automated test - what is 2+2?"
        }
    }' \
    "http://localhost:8080/api/v1/sessions")

SESSION_ID=$(echo "$SESSION_RESPONSE" | jq -r '.id')

if [ "$SESSION_ID" == "null" ] || [ -z "$SESSION_ID" ]; then
    echo "❌ Failed to create session"
    echo "Response: $SESSION_RESPONSE"
    exit 1
fi
echo "✅ Created session: $SESSION_ID"

# Wait for Zed to start
echo ""
echo "5. Waiting for Zed container to start..."
sleep 10

# Find Zed container
ZED_CONTAINER=$(docker ps --format '{{.Names}}' | grep "zed-external.*${SESSION_ID:0:20}")
if [ -z "$ZED_CONTAINER" ]; then
    echo "❌ Zed container not found"
    docker ps | grep zed
    exit 1
fi
echo "✅ Found Zed container: $ZED_CONTAINER"

# Check Zed logs for WebSocket activity
echo ""
echo "6. Checking Zed logs for WebSocket initialization..."
docker logs "$ZED_CONTAINER" 2>&1 | grep -E "WEBSOCKET|THREAD_SERVICE|ZED.*WebSocket" | tail -20

# Wait for response
echo ""
echo "7. Waiting 15 seconds for AI response..."
sleep 15

# Check session for response
echo ""
echo "8. Checking session for AI response..."
SESSION_DATA=$(curl -s -H "Authorization: Bearer $(cat .test_runner_token)" \
    "http://localhost:8080/api/v1/sessions/$SESSION_ID")

INTERACTIONS=$(echo "$SESSION_DATA" | jq -r '.interactions | length')
echo "   Session has $INTERACTIONS interactions"

if [ "$INTERACTIONS" -gt 0 ]; then
    RESPONSE=$(echo "$SESSION_DATA" | jq -r '.interactions[0].response')
    if [ "$RESPONSE" != "null" ] && [ -n "$RESPONSE" ]; then
        echo "✅ Got response from Zed!"
        echo "   Response: ${RESPONSE:0:100}..."
    else
        echo "❌ No response in interaction"
    fi
else
    echo "❌ No interactions in session"
fi

echo ""
echo "9. Full diagnostic logs:"
echo "========================"
echo "Zed container logs (WebSocket):"
docker logs "$ZED_CONTAINER" 2>&1 | grep -i websocket | tail -30
echo ""
echo "Zed container logs (Thread Service):"
docker logs "$ZED_CONTAINER" 2>&1 | grep -i thread_service | tail -30
echo ""
echo "Helix API logs (external agent):"
docker logs helix-api 2>&1 | grep -i "external.*agent\|zed.*sync" | tail -20

