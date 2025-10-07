#!/bin/bash

set -e

echo "🔗 Testing Moonlight Pairing End-to-End"
echo "========================================"
echo ""

cd /home/luke/pm/helix
API_KEY="hl-_pwrvW_Foqw1mggPOs6lnnq0aS13ppQecIss-HG71WQ="

# Step 1: Check API logs to see if Wolf has pending requests
echo "1. Checking API logs for Wolf pairing activity..."
WOLF_LOG=$(docker compose -f docker-compose.dev.yaml logs api 2>&1 | grep "Retrieved pending Wolf pair" | tail -1)
if [ -z "$WOLF_LOG" ]; then
    echo "   ⚠️  No pairing activity in logs"
    echo "   Start pairing from Moonlight client first"
    exit 1
fi

echo "   Latest Wolf pairing log:"
echo "   $WOLF_LOG"

WOLF_COUNT=$(echo "$WOLF_LOG" | sed -n 's/.*pending_count=\([0-9]*\).*/\1/p')
echo "   Pending requests in Wolf: $WOLF_COUNT"

if [ "$WOLF_COUNT" -eq 0 ]; then
    echo "   ⚠️  No pending requests in Wolf"
    exit 1
fi

# Step 2: Call Helix API to get pending requests
echo ""
echo "2. Calling Helix API for pending requests..."
HELIX_RESPONSE=$(curl -s -H "Authorization: Bearer $API_KEY" "http://localhost:8080/api/v1/wolf/pairing/pending")

echo "   Helix API response:"
echo "$HELIX_RESPONSE" | jq '.'

# Step 3: Check if Helix returned the requests
HELIX_COUNT=$(echo "$HELIX_RESPONSE" | jq '. | length' 2>/dev/null || echo "0")
echo "   Requests returned by Helix: $HELIX_COUNT"

if [ "$HELIX_COUNT" -eq 0 ]; then
    echo "   ❌ Helix API returned empty array but Wolf has requests!"
    echo "   This indicates transformation issue in backend"
    exit 1
fi

# Step 4: Extract pair_secret and show it
PAIR_SECRET=$(echo "$HELIX_RESPONSE" | jq -r '.[0].uuid // .[0].pair_secret // empty')
CLIENT_IP=$(echo "$HELIX_RESPONSE" | jq -r '.[0].client_ip // .[0].client_name // empty')

echo ""
echo "✅ Helix API working!"
echo "   Pair Secret: $PAIR_SECRET"
echo "   Client IP: $CLIENT_IP"

echo ""
echo "========================================="
echo "📋 Summary"
echo "========================================="
echo "Wolf pending requests: $WOLF_COUNT"
echo "Helix API requests: $HELIX_COUNT"
echo ""
echo "If counts match: ✅ Backend working correctly"
echo "If not: ❌ Check transformation in personal_dev_environment_handlers.go"
echo ""
echo "Next: Check browser console when opening pairing dialog in UI"
echo ""
