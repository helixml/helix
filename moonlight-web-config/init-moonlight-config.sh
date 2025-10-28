#!/bin/bash

# Initialize moonlight-web data.json and config.json from templates

DATA_FILE="/app/server/data.json"
DATA_TEMPLATE="/app/server/data.json.template"
CONFIG_FILE="/app/server/config.json"
CONFIG_TEMPLATE="/app/server/config.json.template"

# Initialize data.json if it doesn't exist or is empty
if [ ! -f "$DATA_FILE" ] || [ ! -s "$DATA_FILE" ]; then
    echo "🔧 Initializing moonlight-web data.json from template..."
    cp "$DATA_TEMPLATE" "$DATA_FILE"
    echo "✅ moonlight-web data.json initialized"
else
    echo "ℹ️  moonlight-web data.json already exists, skipping initialization"
fi

# Initialize config.json with dynamic TURN server IP
if [ ! -f "$CONFIG_FILE" ] || [ ! -s "$CONFIG_FILE" ]; then
    echo "🔧 Initializing moonlight-web config.json from template..."

    # Auto-detect public IP if TURN_PUBLIC_IP not set
    if [ -z "$TURN_PUBLIC_IP" ]; then
        echo "⏳ Auto-detecting public IP for TURN server..."
        TURN_PUBLIC_IP=$(curl -s --max-time 2 https://api.ipify.org 2>/dev/null || echo "")

        if [ -z "$TURN_PUBLIC_IP" ]; then
            echo "❌ Could not auto-detect public IP. Please set TURN_PUBLIC_IP environment variable."
            exit 1
        fi

        echo "✅ Auto-detected public IP: $TURN_PUBLIC_IP"
    else
        echo "✅ Using configured TURN_PUBLIC_IP: $TURN_PUBLIC_IP"
    fi

    # Substitute {{TURN_PUBLIC_IP}} in template
    sed "s/{{TURN_PUBLIC_IP}}/$TURN_PUBLIC_IP/g" "$CONFIG_TEMPLATE" > "$CONFIG_FILE"
    echo "✅ moonlight-web config.json initialized with TURN server at $TURN_PUBLIC_IP"
else
    echo "ℹ️  moonlight-web config.json already exists, skipping initialization"
fi

# Start the web server in background
echo "🚀 Starting moonlight-web server..."
/app/web-server &
WEB_SERVER_PID=$!

# Wait for web server to be ready (poll until it responds)
echo "⏳ Waiting for moonlight-web server to be ready..."
for i in {1..30}; do
    if curl -s http://localhost:8080/ > /dev/null 2>&1; then
        echo "✅ moonlight-web server is ready"
        break
    fi
    if [ $i -eq 30 ]; then
        echo "❌ moonlight-web server failed to start within 30 seconds"
        exit 1
    fi
    sleep 1
done

# Auto-pair with Wolf if MOONLIGHT_INTERNAL_PAIRING_PIN is set and not already paired
if [ -n "$MOONLIGHT_INTERNAL_PAIRING_PIN" ]; then
    # Check if data.json has a "paired" section
    if ! grep -q '"paired"' "$DATA_FILE"; then
        echo "🔗 Auto-pairing moonlight-web with Wolf (MOONLIGHT_INTERNAL_PAIRING_PIN is set)..."

        # Wait for Wolf to be ready too
        sleep 2

        # Trigger pairing via internal API (Wolf will auto-accept with PIN)
        curl -X POST http://localhost:8080/api/pair \
            -H "Content-Type: application/json" \
            -H "Authorization: Bearer ${MOONLIGHT_CREDENTIALS:-helix}" \
            -d '{"host_id":0}' \
            > /tmp/pair-response.log 2>&1

        if grep -q '"Paired"' /tmp/pair-response.log; then
            echo "✅ Auto-pairing with Wolf completed successfully"
        else
            echo "⚠️  Auto-pairing may have failed, check logs: cat /tmp/pair-response.log"
        fi
    else
        echo "ℹ️  moonlight-web already paired with Wolf, skipping auto-pair"
    fi
fi

# Wait for web server process to keep container running
wait $WEB_SERVER_PID
