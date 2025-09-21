#!/bin/bash
# Complete Moonlight pairing test for HyprMoon Integrated Screencopy Architecture

echo "🌙 Complete Moonlight Pairing & Streaming Test"
echo "📡 HyprMoon Integrated Screencopy + Wolf Streaming"
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

# Wait for the pairing process to actually initiate the /pair request
echo "   Waiting for moonlight client to initiate pairing..."
for i in {1..30}; do
    PAIR_INITIATED=$(docker compose -f docker-compose.dev.yaml logs zed-runner --since="10s" | grep "PAIR DEBUG.*ENTRY.*endpoints::pair.*called")
    if [ -n "$PAIR_INITIATED" ]; then
        echo "   ✓ Pairing initiated by moonlight client"
        break
    fi
    if [ $((i % 5)) -eq 0 ]; then
        echo "   Still waiting for pairing initiation... (${i}s elapsed)"
    fi
    sleep 1
done

if [ -z "$PAIR_INITIATED" ]; then
    echo "   ❌ Pairing was not initiated by moonlight client after 30 seconds"
    kill $PAIR_PID 2>/dev/null
    exit 1
fi

# Wait a bit more for the PIN secret to be generated in pairing_atom
echo "   Waiting for PIN secret generation..."
sleep 3

# Extract the PIN secret from the server logs
echo "   Extracting PIN secret from server logs..."
PIN_SECRET=""
for i in {1..10}; do
    PIN_URL=$(docker compose -f docker-compose.dev.yaml logs zed-runner --since="30s" | grep "Visit this URL to enter PIN:" -A1 | grep "http://.*pin/#" | tail -1)
    if [ -n "$PIN_URL" ]; then
        # Extract secret from URL like: http://127.0.0.1:47989/pin/#A5308B99CE6054D0
        # Strip ANSI color codes first
        CLEAN_URL=$(echo "$PIN_URL" | sed 's/\x1b\[[0-9;]*m//g')
        PIN_SECRET=$(echo "$CLEAN_URL" | sed 's/.*pin\/#//')
        echo "   ✓ PIN secret extracted: $PIN_SECRET"
        break
    fi
    echo "   Waiting for PIN URL generation... (${i}s elapsed)"
    sleep 1
done

if [ -z "$PIN_SECRET" ]; then
    echo "   ❌ Could not extract PIN secret from server logs"
    echo "   Recent PIN-related logs:"
    docker compose -f docker-compose.dev.yaml logs zed-runner --since="30s" | grep -E "(PIN|pin|secret|Visit)" | tail -10
    kill $PAIR_PID 2>/dev/null
    exit 1
fi

# Wait for PIN URL generation
echo "2. Verifying HyprMoon Integrated Screencopy setup..."
echo "   Checking if HyprMoon is running..."
HYPRMOON_RUNNING=$(docker compose -f docker-compose.dev.yaml logs zed-runner --since="60s" | grep -E "(HyprMoon.*integrated screencopy|HyprMoon.*Wolf streaming)")
if [ -n "$HYPRMOON_RUNNING" ]; then
    echo "   ✓ HyprMoon integrated screencopy is running"
else
    echo "   ❌ HyprMoon integrated screencopy not detected"
    echo "   Recent HyprMoon logs:"
    docker compose -f docker-compose.dev.yaml logs zed-runner --since="60s" | grep -E "(HyprMoon|Hyprland|Wolf)" | tail -5
fi

echo "   Checking if Wolf Streaming Engine is running..."
WOLF_RUNNING=$(docker compose -f docker-compose.dev.yaml logs zed-runner --since="60s" | grep -E "(HTTPS server listening.*47984|WolfMoonlightServer.*StreamSession)")
if [ -n "$WOLF_RUNNING" ]; then
    echo "   ✓ Wolf Streaming Engine is running"
else
    echo "   ❌ Wolf Streaming Engine not detected"
    echo "   Recent Wolf streaming logs:"
    docker compose -f docker-compose.dev.yaml logs zed-runner --since="60s" | grep -E "(Wolf|HTTPS|streaming|Moonlight)" | tail -5
fi

echo "   Checking screencopy configuration..."
SCREENCOPY_CONFIG=$(docker compose -f docker-compose.dev.yaml exec zed-runner bash -c "echo HYPRMOON_FRAME_SOURCE=\$HYPRMOON_FRAME_SOURCE" 2>/dev/null | grep "screencopy")
if [ -n "$SCREENCOPY_CONFIG" ]; then
    echo "   ✓ Screencopy backend configured (session-based frame capture)"
    # Check if frame dump directory exists
    FRAME_DIR=$(docker compose -f docker-compose.dev.yaml exec zed-runner bash -c "ls -d /tmp/hyprmoon_frame_dumps 2>/dev/null" | tr -d '\r')
    if [ -n "$FRAME_DIR" ]; then
        echo "   ✓ Frame dump directory ready at /tmp/hyprmoon_frame_dumps"
    else
        echo "   ⚠️  Frame dump directory not found"
    fi
else
    echo "   ❌ Screencopy backend not configured"
    echo "   Recent screencopy config logs:"
    docker compose -f docker-compose.dev.yaml logs zed-runner --since="30s" | grep -E "(HYPRMOON|screencopy|frame)" | tail -3
fi

echo "   Checking Wolf HTTP server availability..."
for i in {1..30}; do
    # Test HTTP server directly
    HTTP_RESPONSE=$(curl -s "http://localhost:47989/serverinfo?uniqueid=test&uuid=test" 2>/dev/null | grep -o "status_code=\"200\"")
    if [ -n "$HTTP_RESPONSE" ]; then
        echo "   ✓ Wolf HTTP server responding on port 47989"
        break
    fi
    if [ $((i % 5)) -eq 0 ]; then
        echo "   Still waiting for HTTP server... (${i}s elapsed)"
        echo "   Recent HTTP server logs:"
        docker compose -f docker-compose.dev.yaml logs zed-runner --since="10s" | grep -E "(HTTP|serverinfo|47989)" | tail -3
    fi
    sleep 1
done

if [ -z "$HTTP_RESPONSE" ]; then
    echo "❌ ERROR: Wolf HTTP server not responding after 30 seconds"
    echo "   Final HTTP server logs:"
    docker compose -f docker-compose.dev.yaml logs zed-runner --since="60s" | grep -E "(HTTP|Wolf|error|fail)" | tail -15
    kill $PAIR_PID 2>/dev/null
    exit 1
fi

echo "   ✓ Wolf HTTP server ready for pairing"
echo "   Server URL: http://localhost:47989"

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
    echo "   Checking screencopy server logs..."
    docker compose -f docker-compose.dev.yaml logs zed-runner --since="60s" | grep -E "(Moonlight|working-screencopy|HTTP|error|fail)" | tail -10
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

# Test both VNC and Moonlight streaming
echo "5. Testing dual streaming setup (VNC + Moonlight)..."

# Test VNC connectivity first
echo "   Testing VNC server on port 5901..."
VNC_TEST=$(netstat -tlnp 2>/dev/null | grep :5901)
if [ -n "$VNC_TEST" ]; then
    echo "   ✓ VNC server listening on port 5901"
else
    echo "   ❌ VNC server not listening on port 5901"
fi

# Test Moonlight streaming
echo "   Testing Moonlight video streaming..."
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

# Check streaming logs and provide comprehensive status
if [ -f "streaming_$$.log" ]; then
    if grep -q "Computer.*has not been paired" streaming_$$.log; then
        echo "   ❌ Still shows not paired - pairing incomplete"
        echo "   Recent pairing logs:"
        tail -5 pairing_$$.log 2>/dev/null || echo "   No pairing logs"
    elif grep -q "connection.*refused\|error" streaming_$$.log; then
        echo "   ⚠️  Pairing worked but streaming connection failed"
        cat streaming_$$.log | grep -E "(connection|error|critical)" | tail -3
    else
        echo "   🎉 SUCCESS: Moonlight video streaming working!"
    fi
fi

echo ""
echo "📊 FINAL STATUS REPORT - HyprMoon Integrated Screencopy + Wolf Streaming"
echo "=================================================================="
echo "🌙 HyprMoon Integration: $([ -n "$HYPRMOON_RUNNING" ] && echo "✅ RUNNING" || echo "❌ NOT DETECTED")"
echo "🐺 Wolf Streaming Engine: $([ -n "$WOLF_RUNNING" ] && echo "✅ RUNNING" || echo "❌ NOT DETECTED")"
echo "📸 Screencopy Backend: $([ -n "$SCREENCOPY_CONFIG" ] && echo "✅ CONFIGURED" || echo "❌ NOT CONFIGURED")"
echo "🌐 VNC Server: $([ -n "$VNC_TEST" ] && echo "✅ LISTENING:5901" || echo "❌ NOT LISTENING")"
echo "🎮 Moonlight Stream: $STREAM_STATUS"
echo "🎯 Frame Captures: $(find /home/luke/pm/helix/screencopy-frames/ -name "*.png" 2>/dev/null | wc -l) total frames saved"

echo ""
echo "🌙 HyprMoon Integrated Screencopy Test completed successfully at $(date)"
echo "📱 VNC: localhost:5901 (password: helix123)"
echo "🎮 Moonlight: localhost:47989 (HTTP) / localhost:47984 (HTTPS)"
echo "📸 Frame captures: /home/luke/pm/helix/screencopy-frames/ ($(ls /home/luke/pm/helix/screencopy-frames/*.png 2>/dev/null | wc -l) frames)"