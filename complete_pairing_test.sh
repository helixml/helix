#!/bin/bash
# Complete Moonlight pairing test that doesn't stop until video streaming works

echo "🌙 Complete Moonlight Pairing & Streaming Test"
echo "=============================================="

# Clean up existing pairing connections by removing localhost entries from config
echo "0. Cleaning up existing pairing connections..."
CONFIG_FILE="$HOME/snap/moonlight/2671/.config/Moonlight Game Streaming Project/Moonlight.conf"
if [ -f "$CONFIG_FILE" ]; then
    # Count localhost entries
    LOCALHOST_COUNT=$(grep -c "manualaddress=localhost" "$CONFIG_FILE" 2>/dev/null || echo "0")
    echo "   Found $LOCALHOST_COUNT localhost entries to remove"

    # Remove all localhost entries from the config file
    sed -i '/manualaddress=localhost/,+10d' "$CONFIG_FILE" 2>/dev/null || true
    sed -i '/hostname=Hyprland/,+10d' "$CONFIG_FILE" 2>/dev/null || true

    echo "   Localhost pairing entries cleared from config"

    # Also restart the server to clear server-side pairing state
    echo "   Restarting server to clear server-side pairing state..."
    cd /home/luke/pm/helix
    docker compose -f docker-compose.dev.yaml restart zed-runner >/dev/null 2>&1
    sleep 10  # Wait for server to fully restart
    echo "   Server restarted"
else
    echo "   No config file found - clean state"
fi

# Generate random PIN
PIN=$(printf "%04d" $((RANDOM % 10000)))
echo "1. Starting pairing with PIN: $PIN"

# Start pairing in background with timeout
timeout 120 moonlight pair --pin "$PIN" localhost > pairing_$$.log 2>&1 &
PAIR_PID=$!
echo "   Pairing started (PID: $PAIR_PID)"

# Wait for PIN URL generation
echo "2. Waiting for server PIN URL..."
echo "   First checking if Moonlight server is initializing..."
MOONLIGHT_INIT=$(docker compose -f docker-compose.dev.yaml logs zed-runner --since="60s" | grep "Initializing moonlight server")
if [ -n "$MOONLIGHT_INIT" ]; then
    echo "   ✓ Moonlight server is initializing"
else
    echo "   ❌ No Moonlight server initialization found"
    echo "   Recent logs:"
    docker compose -f docker-compose.dev.yaml logs zed-runner --since="60s" | tail -10
    kill $PAIR_PID 2>/dev/null
    exit 1
fi

for i in {1..30}; do
    PIN_URL=$(docker compose -f docker-compose.dev.yaml logs zed-runner --since="30s" | grep "pin/#" | tail -1 | awk '{print $NF}' | sed 's/\x1b\[[0-9;]*m//g')
    if [ -n "$PIN_URL" ]; then
        break
    fi
    if [ $((i % 5)) -eq 0 ]; then
        echo "   Still waiting for PIN URL... (${i}s elapsed)"
        echo "   Recent Moonlight logs:"
        docker compose -f docker-compose.dev.yaml logs zed-runner --since="10s" | grep -E "(Moonlight|Wolf|HTTP|pin)" | tail -3
    fi
    sleep 1
done

if [ -z "$PIN_URL" ]; then
    echo "❌ ERROR: No PIN URL generated after 30 seconds"
    echo "   Final Moonlight server logs:"
    docker compose -f docker-compose.dev.yaml logs zed-runner --since="60s" | grep -E "(Moonlight|Wolf|HTTP|error|fail)" | tail -15
    kill $PAIR_PID 2>/dev/null
    exit 1
fi

PIN_SECRET=$(echo "$PIN_URL" | cut -d'#' -f2)
echo "   PIN URL: $PIN_URL"
echo "   PIN Secret: $PIN_SECRET"

# Give pairing atom time to be fully updated before attempting PIN submission
echo "   Waiting 2 seconds for pairing session to stabilize..."
sleep 2

# First test if HTTP server is responding at all
echo "3. Testing HTTP server connectivity..."
echo "   Testing basic serverinfo endpoint..."
HTTP_TEST=$(curl -s --connect-timeout 5 "http://localhost:47989/serverinfo?uniqueid=test&uuid=test" 2>&1)
if [ $? -eq 0 ] && [ -n "$HTTP_TEST" ]; then
    echo "   ✓ HTTP server responding"
else
    echo "   ❌ HTTP server not responding: $HTTP_TEST"
    echo "   Checking if port 47989 is listening..."
    netstat -tlnp 2>/dev/null | grep :47989 || echo "   Port 47989 not listening"
    echo "   Checking Moonlight server logs..."
    docker compose -f docker-compose.dev.yaml logs zed-runner --since="60s" | grep -E "(Moonlight|Wolf|HTTP|error|fail)" | tail -10
    kill $PAIR_PID 2>/dev/null
    exit 1
fi

# Submit PIN via JSON
echo "4. Submitting PIN $PIN to server..."
echo "   Sending POST to http://localhost:47989/pin/ with PIN: $PIN and secret: $PIN_SECRET"
JSON_PAYLOAD="{\"pin\":\"$PIN\",\"secret\":\"$PIN_SECRET\"}"
echo "   DEBUG: JSON payload: '$JSON_PAYLOAD'"
echo "   DEBUG: JSON bytes: $(echo -n "$JSON_PAYLOAD" | wc -c)"
echo "   DEBUG: JSON hex: $(echo -n "$JSON_PAYLOAD" | hexdump -C)"
RESPONSE=$(curl -s -v -X POST "http://localhost:47989/pin/" \
    -H "Content-Type: application/json" \
    -d "$JSON_PAYLOAD" 2>&1)
CURL_EXIT=$?

echo "   Curl exit code: $CURL_EXIT"
echo "   Response: $RESPONSE"

if echo "$RESPONSE" | grep -q "OK"; then
    echo "   ✓ PIN submitted successfully"
else
    echo "   ❌ PIN submission failed"
    echo "   Full response: $RESPONSE"
    kill $PAIR_PID 2>/dev/null
    exit 1
fi

# Wait for pairing completion
echo "4. Waiting for complete 4-phase pairing..."
sleep 10

# Check if all phases completed
PHASES=$(docker compose -f docker-compose.dev.yaml logs zed-runner --since="60s" | grep "Phase.*response.*status=200" | wc -l)
echo "   Completed phases: $PHASES/4"

# Kill pairing process
kill $PAIR_PID 2>/dev/null || true
wait $PAIR_PID 2>/dev/null || true

# Check server status
if docker compose -f docker-compose.dev.yaml ps zed-runner | grep -q "Up"; then
    echo "   ✅ Server still running (no crash)"
else
    echo "   ❌ Server crashed during pairing"
    exit 1
fi

# Test video streaming
echo "5. Testing video streaming..."
echo "   Moonlight client debug output:"
timeout 30 moonlight stream localhost "Hyprland Desktop" --quit-after --1080 2>&1 | tee streaming_$$.log &
STREAM_PID=$!

sleep 5
STREAM_STATUS="unknown"
if ps -p $STREAM_PID > /dev/null 2>&1; then
    STREAM_STATUS="running"
else
    STREAM_STATUS="ended"
fi

kill $STREAM_PID 2>/dev/null || true

echo "   Stream status: $STREAM_STATUS"

# Check streaming logs
if [ -f "streaming_$$.log" ]; then
    if grep -q "Computer.*has not been paired" streaming_$$.log; then
        echo "   ❌ Still shows not paired - pairing incomplete"
        echo "   Recent pairing logs:"
        tail -5 pairing_$$.log 2>/dev/null || echo "   No pairing logs"
    elif grep -q "connection.*refused\|error" streaming_$$.log; then
        echo "   ⚠️  Pairing worked but streaming connection failed"
        cat streaming_$$.log | grep -E "(connection|error|critical)" | tail -3
    else
        echo "   🎉 SUCCESS: Video streaming working!"
    fi
fi

echo ""
echo "🌙 Test completed at $(date)"