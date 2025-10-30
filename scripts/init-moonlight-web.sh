#!/bin/bash
# Initialize moonlight-web and pair with Wolf automatically
# This script runs during Helix startup to ensure moonlight-web is ready

set -e

MOONLIGHT_WEB_URL="http://localhost:8081"
WOLF_URL="http://localhost:47989"
DATA_FILE="./moonlight-web-config/data.json"

echo "🚀 Initializing moonlight-web integration..."

# Wait for services to be ready
echo "⏳ Waiting for moonlight-web..."
timeout 30 bash -c 'until curl -sf http://localhost:8081/ > /dev/null; do sleep 1; done'

echo "⏳ Waiting for Wolf..."
timeout 30 bash -c 'until curl -sf http://localhost:47989/serverinfo > /dev/null 2>&1; do sleep 1; done'

echo "✅ Services are ready"

# Check if Wolf is already paired
PAIRED=$(docker compose -f docker-compose.dev.yaml exec -T moonlight-web \
  cat /app/server/data.json 2>/dev/null | \
  jq -r '.hosts[0].client_private_key != null' || echo "false")

if [ "$PAIRED" = "true" ]; then
  echo "✅ Wolf is already paired with moonlight-web"
  exit 0
fi

echo "📋 Wolf is not paired yet"
echo ""
echo "╔════════════════════════════════════════════════════════════╗"
echo "║  MANUAL PAIRING REQUIRED (One-Time Setup)                  ║"
echo "╚════════════════════════════════════════════════════════════╝"
echo ""
echo "To pair Wolf with moonlight-web:"
echo ""
echo "1. Open: http://localhost:8080/moonlight/"
echo "2. Click on the Wolf host"
echo "3. When prompted, check Wolf logs for the pairing PIN:"
echo ""
echo "   docker compose -f docker-compose.dev.yaml logs wolf | grep -i pin"
echo ""
echo "4. Enter the 4-digit PIN in the moonlight-web UI"
echo "5. Pairing is saved permanently in moonlight-web-config/data.json"
echo ""
echo "Once paired, streaming will work automatically for all sessions."
echo ""
echo "NOTE: This is a one-time setup per deployment."
echo "      The pairing persists in the Docker volume."
echo ""

# Alternative: Attempt automatic pairing if Wolf provides API
# (This would require Wolf enhancement to expose pairing PIN)
#
# WOLF_PIN=$(curl -s http://localhost:47989/api/pairing-pin 2>/dev/null || echo "")
# if [ -n "$WOLF_PIN" ]; then
#   echo "🔑 Found Wolf pairing PIN: $WOLF_PIN"
#   echo "🔗 Attempting automatic pairing..."
#
#   # Call moonlight-web pairing API
#   curl -X POST "$MOONLIGHT_WEB_URL/api/pair" \
#     -u "helix:helix" \
#     -H "Content-Type: application/json" \
#     -d "{\"host_id\": 0, \"pin\": \"$WOLF_PIN\"}"
#
#   echo "✅ Automatic pairing complete"
# fi

exit 1  # Exit with error to indicate manual pairing needed
