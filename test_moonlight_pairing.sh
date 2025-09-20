#!/bin/bash
# Automated Moonlight pairing test script

echo "🌙 Automated Moonlight Pairing Test"
echo "=================================="

# Kill any existing moonlight processes
pkill -f moonlight 2>/dev/null || true
sleep 1

# Start pairing in background (this will trigger PIN generation on server)
echo "1. Starting Moonlight pairing to trigger PIN generation..."
timeout 30 moonlight pair localhost > /tmp/moonlight_pair.log 2>&1 &
PAIR_PID=$!
echo "   Pairing process started (PID: $PAIR_PID)"

# Wait a moment for the pairing request to reach the server
sleep 3

# Extract PIN from server logs
echo "2. Extracting PIN from server logs..."
PIN_URL=$(docker compose -f docker-compose.dev.yaml logs zed-runner --since="30s" | grep "Visit this URL to enter PIN" | tail -1 | awk '{print $NF}')

if [ -z "$PIN_URL" ]; then
    echo "❌ ERROR: No PIN URL found in server logs"
    echo "Server might not be responding or pairing request didn't reach it"
    kill $PAIR_PID 2>/dev/null
    exit 1
fi

# Extract the PIN secret from the URL (format: http://127.0.0.1:47989/pin/#SECRET)
PIN_SECRET=$(echo "$PIN_URL" | cut -d'#' -f2)
echo "   PIN URL: $PIN_URL"
echo "   PIN Secret: $PIN_SECRET"

# Generate a 4-digit PIN (in real usage, user would provide this)
PIN="1234"
echo "3. Using PIN: $PIN"

# Submit PIN to server via HTTP POST
echo "4. Submitting PIN to server..."
PIN_RESPONSE=$(curl -s -X POST "http://localhost:47989/pin/" \
    -H "Content-Type: application/x-www-form-urlencoded" \
    -d "pin=${PIN}&secret=${PIN_SECRET}")

if [ $? -eq 0 ]; then
    echo "   ✓ PIN submitted successfully"
    echo "   Response: $PIN_RESPONSE"
else
    echo "   ❌ Failed to submit PIN"
fi

# Wait for pairing to complete
echo "5. Waiting for pairing completion..."
sleep 5

# Kill the background pairing process
kill $PAIR_PID 2>/dev/null || true
wait $PAIR_PID 2>/dev/null || true

# Check final results
echo "6. Checking pairing results..."
echo "   Moonlight output:"
cat /tmp/moonlight_pair.log | tail -5

echo "   Server logs (last few lines):"
docker compose -f docker-compose.dev.yaml logs zed-runner --since="30s" | grep -E "(Phase.*response|Successfully saved|CRITICAL.*paired_clients)" | tail -5

# Check if server is still running (no crash)
if docker compose -f docker-compose.dev.yaml ps zed-runner | grep -q "Up"; then
    echo "   ✅ Server still running (no crash)"
else
    echo "   ❌ Server crashed during pairing"
fi

echo ""
echo "🌙 Pairing test completed!"