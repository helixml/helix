#!/bin/bash
set -e
echo "🌙 Initializing Moonlight Web config..."

DATA_FILE="/opt/moonlight-web/server/data.json"
CONFIG_FILE="/opt/moonlight-web/server/config.json"
DATA_TEMPLATE="/opt/moonlight-web/templates/data.json.template"
CONFIG_TEMPLATE="/opt/moonlight-web/templates/config.json.template"

mkdir -p /opt/moonlight-web/server

# Initialize data.json if it doesn't exist or is empty
if [ ! -f "$DATA_FILE" ] || [ ! -s "$DATA_FILE" ]; then
    echo "🔧 Initializing moonlight-web data.json from template..."
    # Wolf is on 127.0.0.1 in unified sandbox container
    # Must use 127.0.0.1 NOT localhost - Wolf only listens on IPv4, and "localhost"
    # resolves to IPv6 first on modern Linux, causing "LikelyOffline" errors
    sed 's/"address": "wolf"/"address": "127.0.0.1"/' "$DATA_TEMPLATE" > "$DATA_FILE"
    # Use WOLF_INSTANCE_ID for consistency with Wolf hostname
    WOLF_INSTANCE_ID=${WOLF_INSTANCE_ID:-local}
    sed -i "s/{{HELIX_HOSTNAME}}/$WOLF_INSTANCE_ID/g" "$DATA_FILE"
    echo "✅ moonlight-web data.json initialized"
fi

# Initialize config.json with dynamic TURN server IP
if [ ! -f "$CONFIG_FILE" ] || [ ! -s "$CONFIG_FILE" ]; then
    echo "🔧 Initializing moonlight-web config.json from template..."

    # Use environment variables or defaults for dev mode
    MOONLIGHT_CREDENTIALS=${MOONLIGHT_CREDENTIALS:-helix:helix}
    TURN_PASSWORD=${TURN_PASSWORD:-helix-turn-secret}
    TURN_PUBLIC_IP=${TURN_PUBLIC_IP:-}

    # Auto-detect public IP if TURN_PUBLIC_IP not set
    if [ -z "$TURN_PUBLIC_IP" ]; then
        echo "⏳ Auto-detecting public IP for TURN server..."
        TURN_PUBLIC_IP=$(curl -s --max-time 2 https://api.ipify.org 2>/dev/null || echo "127.0.0.1")
        echo "✅ Using TURN IP: $TURN_PUBLIC_IP"
    fi

    # Substitute template variables
    sed -e "s/{{TURN_PUBLIC_IP}}/$TURN_PUBLIC_IP/g" \
        -e "s/{{MOONLIGHT_CREDENTIALS}}/$MOONLIGHT_CREDENTIALS/g" \
        -e "s/{{TURN_PASSWORD}}/$TURN_PASSWORD/g" \
        "$CONFIG_TEMPLATE" > "$CONFIG_FILE"
    echo "✅ moonlight-web config.json initialized"
fi

# NOTE: Moonlight Web startup moved to startup-app.sh to ensure Wolf is running first
# This avoids the "LikelyOffline" error caused by Moonlight trying to connect before Wolf
echo "✅ Moonlight Web config ready (startup deferred to after Wolf)"
