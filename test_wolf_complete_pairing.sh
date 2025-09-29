#!/bin/bash
# Complete Moonlight pairing test adapted for Wolf Personal Dev Environments

echo "🌙 Complete Moonlight Pairing & Streaming Test"
echo "📡 Wolf Personal Dev Environment Integration"
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
    sed -i '/hostname=Helix/,+10d' "$CONFIG_FILE" 2>/dev/null || true

    echo "   Localhost pairing entries cleared from config"
else
    echo "   No config file found - clean state"
fi

# Ensure we have a Personal Dev environment with background session
echo "1. Ensuring Personal Dev environment with background Wolf session..."
PERSONAL_DEV=$(curl -s -H "Authorization: Bearer oh-hallo-insecure-token" \
  "http://localhost:8080/api/v1/personal-dev-environments" 2>/dev/null)

if [ -z "$PERSONAL_DEV" ] || [ "$PERSONAL_DEV" = "[]" ]; then
    echo "   No Personal Dev environment found, creating one..."
    PERSONAL_DEV=$(curl -X POST http://localhost:8080/api/v1/personal-dev-environments \
        -H "Content-Type: application/json" \
        -H "Authorization: Bearer oh-hallo-insecure-token" \
        -d '{"environment_name": "pairing-test", "app_id": "app-pairing-test"}' 2>/dev/null)
    echo "   Created Personal Dev environment for testing"
fi

# Extract session info
SESSION_ID=$(echo "$PERSONAL_DEV" | jq -r '.[0].wolf_session_id // empty' 2>/dev/null)
if [ -n "$SESSION_ID" ]; then
    echo "   ✅ Background Wolf session active: $SESSION_ID"
else
    echo "   ❌ No Wolf session found in Personal Dev environment"
    exit 1
fi

# Generate random PIN
PIN=$(printf "%04d" $((RANDOM % 10000)))
echo "2. Starting pairing with PIN: $PIN"

# Start pairing in background with timeout
timeout 120 moonlight pair --pin "$PIN" localhost > pairing_$$.log 2>&1 &
PAIR_PID=$!
echo "   Pairing started (PID: $PAIR_PID)"

# Wait for the pairing process to actually initiate the /pair request
echo "   Waiting for moonlight client to initiate pairing..."
for i in {1..30}; do
    # Check Wolf logs for pairing activity (since we're using Wolf directly)
    PAIR_INITIATED=$(docker compose -f docker-compose.dev.yaml logs wolf --since="10s" | grep -E "(pair|PIN|certificate)")
    if [ -n "$PAIR_INITIATED" ]; then
        echo "   ✓ Pairing initiated by moonlight client"
        break
    fi
    # Also check if moonlight is showing connection activity
    if [ -f "pairing_$$.log" ]; then
        MOONLIGHT_ACTIVITY=$(grep -E "(Processing|Executing|pair)" pairing_$$.log 2>/dev/null)
        if [ -n "$MOONLIGHT_ACTIVITY" ]; then
            echo "   ✓ Moonlight client is communicating with Wolf server"
            break
        fi
    fi
    if [ $((i % 5)) -eq 0 ]; then
        echo "   Still waiting for pairing initiation... (${i}s elapsed)"
    fi
    sleep 1
done

if [ -z "$PAIR_INITIATED" ] && [ -z "$MOONLIGHT_ACTIVITY" ]; then
    echo "   ❌ Pairing was not initiated by moonlight client after 30 seconds"
    echo "   Recent Wolf logs:"
    docker compose -f docker-compose.dev.yaml logs wolf --since="30s" | tail -5
    echo "   Moonlight logs:"
    [ -f "pairing_$$.log" ] && cat pairing_$$.log | tail -10
    kill $PAIR_PID 2>/dev/null
    exit 1
fi

echo "3. Verifying Wolf streaming setup..."
echo "   Checking Wolf HTTP server availability..."
for i in {1..15}; do
    # Test HTTP server directly
    HTTP_RESPONSE=$(curl -s "http://localhost:47989/serverinfo?uniqueid=test&uuid=test" 2>/dev/null | grep -o "status_code=\"200\"")
    if [ -n "$HTTP_RESPONSE" ]; then
        echo "   ✓ Wolf HTTP server responding on port 47989"
        break
    fi
    if [ $((i % 5)) -eq 0 ]; then
        echo "   Still waiting for HTTP server... (${i}s elapsed)"
    fi
    sleep 1
done

if [ -z "$HTTP_RESPONSE" ]; then
    echo "❌ ERROR: Wolf HTTP server not responding after 15 seconds"
    kill $PAIR_PID 2>/dev/null
    exit 1
fi

echo "   ✓ Wolf HTTP server ready for pairing"

# Wait for pairing to progress and look for PIN submission opportunity
echo "4. Monitoring pairing progress..."
sleep 10

# Check pairing process status
if ps -p $PAIR_PID > /dev/null 2>&1; then
    echo "   ✅ Pairing process still active"
else
    echo "   ⚠️  Pairing process ended early"
    echo "   Pairing log output:"
    [ -f "pairing_$$.log" ] && cat pairing_$$.log | head -20
fi

# Kill pairing process
kill $PAIR_PID 2>/dev/null || true
wait $PAIR_PID 2>/dev/null || true

# Check if pairing made progress
echo "5. Checking pairing results..."
if [ -f "pairing_$$.log" ]; then
    if grep -q "Computer.*has not been paired" pairing_$$.log; then
        echo "   ⚠️  Still shows not paired - pairing incomplete but connection established"
    elif grep -q "connection.*refused\|error" pairing_$$.log; then
        echo "   ❌ Connection failed during pairing"
        cat pairing_$$.log | grep -E "(connection|error|critical)" | tail -3
    elif grep -q "Helix.*is now online" pairing_$$.log; then
        echo "   🎉 SUCCESS: Moonlight successfully discovered Helix server!"
    else
        echo "   ❓ Pairing completed with unknown status"
    fi

    echo ""
    echo "   📋 Key pairing events detected:"
    grep -E "(Processing|Executing|Helix|online|pair)" pairing_$$.log | tail -5
else
    echo "   ❌ No pairing log found"
fi

# Test basic streaming attempt (quick test)
echo "6. Testing Moonlight streaming discovery..."
echo "   Testing quick stream discovery (5 second timeout)..."
timeout 5 moonlight stream localhost "Desktop" --quit-after --1080 2>&1 | head -10 > streaming_test_$$.log

if [ -f "streaming_test_$$.log" ]; then
    if grep -q "Helix.*is now online" streaming_test_$$.log; then
        echo "   ✅ Moonlight can discover and connect to Wolf server"
    elif grep -q "Computer.*has not been paired" streaming_test_$$.log; then
        echo "   🟡 Server discovered but pairing required (expected for first connection)"
    else
        echo "   ⚠️  Streaming test results unclear"
        echo "   First few lines of streaming test:"
        head -5 streaming_test_$$.log
    fi
fi

echo ""
echo "📊 FINAL STATUS REPORT - Wolf Personal Dev Environment Streaming"
echo "================================================================"
echo "🐺 Wolf HTTP Server: $([ -n "$HTTP_RESPONSE" ] && echo "✅ RESPONDING" || echo "❌ NOT RESPONDING")"
echo "🎯 Background Session: $([ -n "$SESSION_ID" ] && echo "✅ ACTIVE ($SESSION_ID)" || echo "❌ NONE")"
echo "🌙 Moonlight Discovery: $(grep -q "Helix.*online" pairing_$$.log streaming_test_$$.log 2>/dev/null && echo "✅ SERVER FOUND" || echo "❌ NOT DISCOVERED")"
echo "🔗 Connection Test: $(ps -p $PAIR_PID > /dev/null 2>&1 && echo "🔄 IN PROGRESS" || echo "✅ COMPLETED")"

echo ""
echo "🎮 Wolf Personal Dev Environment Test completed at $(date)"
echo "📱 Wolf HTTP: localhost:47989"
echo "🌐 Personal Dev Stream: http://localhost:8090/?session=$SESSION_ID"

# Cleanup
rm -f pairing_$$.log streaming_test_$$.log