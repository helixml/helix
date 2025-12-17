#!/bin/bash
# Auto-pair Moonlight Web with Wolf
# Based on original moonlight-web-stream/server-templates/init-moonlight-config.sh logic

DATA_FILE="/opt/moonlight-web/server/data.json"

# Check if already paired (skip if paired section exists in data.json)
if grep -q '"paired"' "$DATA_FILE" 2>/dev/null; then
    echo "ℹ️  moonlight-web already paired with Wolf, skipping auto-pair"
    exit 0
fi

echo "🔗 Auto-pairing moonlight-web with Wolf..."

# Wait for Wolf to be ready (check if Wolf's Moonlight port 47989 is accepting connections)
# Original script checked TCP port, not API endpoint (API takes longer to init)
echo "⏳ Waiting for Wolf to be ready..."
for i in {1..60}; do
    if timeout 1 bash -c 'cat < /dev/null > /dev/tcp/localhost/47989' 2>/dev/null; then
        echo "✅ Wolf port 47989 is responding"
        # Wait additional 5 seconds for HTTPS endpoint to fully initialize
        # Wolf's TCP port responds before HTTPS is ready, causing pairing failures
        echo "⏳ Waiting 5s for Wolf HTTPS endpoint to initialize..."
        sleep 5
        echo "✅ Wolf is ready for pairing"
        break
    fi
    if [ $i -eq 60 ]; then
        echo "❌ Wolf failed to start within 60 seconds, skipping auto-pair"
        exit 0  # Don't fail the container, just skip pairing
    fi
    sleep 1
done

# Trigger pairing via Moonlight Web API (which will connect to Wolf and complete pairing)
CREDS="${MOONLIGHT_CREDENTIALS:-helix}"
PAIR_RESPONSE=$(curl -s -X POST http://localhost:8080/api/pair \
    -H "Authorization: Bearer $CREDS" \
    -H "Content-Type: application/json" \
    -d '{"host_id":0}' 2>&1)

echo "📡 Pairing response: $PAIR_RESPONSE"

# Wait a moment for pairing to complete and data.json to be updated
sleep 1

# Check if pairing succeeded by looking for paired section in data.json
# (more reliable than parsing chunked HTTP response)
if grep -q '"paired"' "$DATA_FILE"; then
    echo "✅ Auto-pairing with Wolf completed successfully"
else
    echo "❌ Auto-pairing failed - paired section not found in data.json"
    echo "   This may be expected if Wolf is not fully initialized yet"
    echo "   Pairing can be completed manually via Moonlight Web UI"
fi
