#!/bin/bash
# Test Wolf Background Session for Personal Dev Environments

echo "🧪 Testing Wolf Background Session with Personal Dev Environment"
echo "================================================================"

echo "1. Checking Wolf service status..."
docker compose -f docker-compose.dev.yaml ps wolf

echo ""
echo "2. Testing Wolf HTTP server availability..."
HTTP_RESPONSE=$(curl -s "http://localhost:47989/serverinfo?uniqueid=test&uuid=test" 2>/dev/null)
if echo "$HTTP_RESPONSE" | grep -q "status_code=\"200\""; then
    echo "   ✅ Wolf HTTP server responding on port 47989"
else
    echo "   ❌ Wolf HTTP server not responding"
    echo "   Response: $HTTP_RESPONSE"
fi

echo ""
echo "3. Checking our Personal Dev environment with background session..."
PERSONAL_DEV=$(curl -s -H "Authorization: Bearer oh-hallo-insecure-token" \
  "http://localhost:8080/api/v1/personal-dev-environments" 2>/dev/null)

if [ -n "$PERSONAL_DEV" ] && [ "$PERSONAL_DEV" != "[]" ]; then
    echo "   ✅ Personal Dev environment found:"
    echo "$PERSONAL_DEV" | jq '.[0] | {instanceID, wolf_session_id, stream_url, status}'

    # Extract session ID for further testing
    SESSION_ID=$(echo "$PERSONAL_DEV" | jq -r '.[0].wolf_session_id // empty')
    if [ -n "$SESSION_ID" ]; then
        echo "   🎯 Wolf Session ID: $SESSION_ID"
    fi
else
    echo "   ❌ No Personal Dev environments found"
    echo "   Creating test environment..."
    curl -X POST http://localhost:8080/api/v1/personal-dev-environments \
        -H "Content-Type: application/json" \
        -H "Authorization: Bearer oh-hallo-insecure-token" \
        -d '{"environment_name": "moonlight-test", "app_id": "app-moonlight-test"}' \
        | jq .
fi

echo ""
echo "4. Testing Wolf paired clients..."
CLIENTS_RESPONSE=$(curl -s "http://localhost:47989/api/v1/clients" 2>/dev/null)
echo "   Wolf clients response: $CLIENTS_RESPONSE"

echo ""
echo "5. Checking Wolf session activity in logs..."
echo "   Recent Wolf session logs:"
docker compose -f docker-compose.dev.yaml logs wolf --since="10m" 2>/dev/null | grep -E "(session|RTSP|client|video)" | tail -5

echo ""
echo "6. Testing basic Moonlight pairing (quick test)..."
echo "   Generating random PIN for pairing test..."
PIN=$(printf "%04d" $((RANDOM % 10000)))
echo "   Testing PIN: $PIN"

echo "   Starting moonlight pair command in background..."
timeout 15 moonlight pair --pin "$PIN" localhost > moonlight_test.log 2>&1 &
PAIR_PID=$!

echo "   Waiting 10 seconds for pairing to initiate..."
sleep 10

echo "   Checking if pairing was initiated..."
if ps -p $PAIR_PID > /dev/null 2>&1; then
    echo "   ✅ Moonlight pairing process still running"
    kill $PAIR_PID 2>/dev/null
else
    echo "   ⚠️  Moonlight pairing process ended"
fi

if [ -f "moonlight_test.log" ]; then
    echo "   Moonlight output:"
    cat moonlight_test.log | head -10
    rm -f moonlight_test.log
fi

echo ""
echo "🎯 SUMMARY"
echo "=========="
echo "Wolf HTTP Server: $(curl -s "http://localhost:47989/serverinfo?uniqueid=test&uuid=test" 2>/dev/null | grep -q "status_code=\"200\"" && echo "✅ WORKING" || echo "❌ NOT RESPONDING")"
echo "Personal Dev Env: $([ -n "$PERSONAL_DEV" ] && [ "$PERSONAL_DEV" != "[]" ] && echo "✅ FOUND" || echo "❌ NOT FOUND")"
echo "Background Sessions: $([ -n "$SESSION_ID" ] && echo "✅ ACTIVE ($SESSION_ID)" || echo "❌ NONE")"
echo ""
echo "🌐 Access Points:"
echo "   Personal Dev Stream: http://localhost:8090/?session=$SESSION_ID"
echo "   Wolf HTTP Server: http://localhost:47989/serverinfo"
echo "   VNC (if available): localhost:5901"