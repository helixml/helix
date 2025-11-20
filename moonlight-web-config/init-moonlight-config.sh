#!/bin/bash

# Initialize moonlight-web data.json and config.json from templates

DATA_FILE="/app/server/data.json"
DATA_TEMPLATE="/app/templates/data.json.template"
CONFIG_FILE="/app/server/config.json"
CONFIG_TEMPLATE="/app/templates/config.json.template"

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

    # Validate required credentials are set (no insecure defaults!)
    if [ -z "$MOONLIGHT_CREDENTIALS" ]; then
        echo "❌ ERROR: MOONLIGHT_CREDENTIALS environment variable is required but not set."
        echo "This should be set by install.sh or docker-compose environment."
        exit 1
    fi

    if [ -z "$TURN_PASSWORD" ]; then
        echo "❌ ERROR: TURN_PASSWORD environment variable is required but not set."
        echo "This should be set by install.sh or docker-compose environment."
        exit 1
    fi

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

    # Substitute all template variables
    sed -e "s/{{TURN_PUBLIC_IP}}/$TURN_PUBLIC_IP/g" \
        -e "s/{{MOONLIGHT_CREDENTIALS}}/$MOONLIGHT_CREDENTIALS/g" \
        -e "s/{{TURN_PASSWORD}}/$TURN_PASSWORD/g" \
        "$CONFIG_TEMPLATE" > "$CONFIG_FILE"
    echo "✅ moonlight-web config.json initialized with TURN server at $TURN_PUBLIC_IP"
else
    echo "ℹ️  moonlight-web config.json already exists, skipping initialization"
fi

# ALWAYS sync credentials from environment variables (fixes upgrade issues)
# On upgrades, install.sh preserves existing .env credentials but config.json may have old values
if [ -f "$CONFIG_FILE" ]; then
    echo "🔄 Syncing credentials from environment variables to config.json..."

    if [ -n "$MOONLIGHT_CREDENTIALS" ]; then
        # Update credentials in config.json (using | as delimiter since credentials don't contain |)
        sed -i "s|\"credentials\": \"[^\"]*\"|\"credentials\": \"$MOONLIGHT_CREDENTIALS\"|g" "$CONFIG_FILE"
        echo "✅ Updated MOONLIGHT_CREDENTIALS in config.json"
    fi

    if [ -n "$TURN_PASSWORD" ]; then
        # Update TURN credential in config.json
        sed -i "s|\"credential\": \"[^\"]*\"|\"credential\": \"$TURN_PASSWORD\"|g" "$CONFIG_FILE"
        echo "✅ Updated TURN_PASSWORD in config.json"
    fi

    if [ -n "$TURN_PUBLIC_IP" ]; then
        # Update TURN server URLs in config.json
        sed -i "s|turn:[^:]*:3478|turn:$TURN_PUBLIC_IP:3478|g" "$CONFIG_FILE"
        echo "✅ Updated TURN_PUBLIC_IP in config.json"
    fi

    echo "✅ Credentials synced successfully"
fi

# Start the web server in background
echo "🚀 Starting moonlight-web server..."
/app/web-server &
WEB_SERVER_PID=$!

# Wait for web server to be ready (poll until it responds)
echo "⏳ Waiting for moonlight-web server to be ready..."
for i in {1..30}; do
    # Check if port 8080 is accepting connections (using bash built-in /dev/tcp)
    if timeout 1 bash -c 'cat < /dev/null > /dev/tcp/localhost/8080' 2>/dev/null; then
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

        # Wait for Wolf to be ready (check if port 47989 is accepting connections)
        echo "⏳ Waiting for Wolf to be ready..."
        for i in {1..120}; do
            if timeout 1 bash -c 'cat < /dev/null > /dev/tcp/wolf/47989' 2>/dev/null; then
                echo "✅ Wolf port is responding"
                # Wait additional 10 seconds for HTTPS endpoint to fully initialize
                # Wolf's TCP port responds before HTTPS is ready, causing pairing failures
                echo "⏳ Waiting 10s for Wolf HTTPS endpoint to initialize..."
                sleep 10
                echo "✅ Wolf is ready for pairing"
                break
            fi
            if [ $i -eq 120 ]; then
                echo "❌ Wolf failed to start within 120 seconds, skipping auto-pair"
                exit 0  # Don't fail the container, just skip pairing
            fi
            sleep 1
        done

        # Trigger pairing via internal API with retries (Wolf will auto-accept with PIN)
        # Use bash /dev/tcp since curl is not available in container
        # Store credentials in variable to ensure proper expansion
        CREDS="${MOONLIGHT_CREDENTIALS:-helix}"

        PAIRED=false
        for attempt in {1..5}; do
            echo "🔗 Pairing attempt $attempt/5..."

            exec 3<>/dev/tcp/localhost/8080
            {
                echo -ne "POST /api/pair HTTP/1.1\r\n"
                echo -ne "Host: localhost:8080\r\n"
                echo -ne "Content-Type: application/json\r\n"
                echo -ne "Authorization: Bearer $CREDS\r\n"
                echo -ne "Content-Length: 13\r\n"
                echo -ne "\r\n"
                echo -ne '{"host_id":0}'
            } >&3
            cat <&3 > /tmp/pair-response.log
            exec 3<&-
            exec 3>&-

            if grep -q '"Paired"' /tmp/pair-response.log; then
                echo "✅ Auto-pairing with Wolf completed successfully on attempt $attempt"
                PAIRED=true
                break
            else
                echo "⚠️  Pairing attempt $attempt failed, check logs: cat /tmp/pair-response.log"
                if [ $attempt -lt 5 ]; then
                    echo "⏳ Waiting 3 seconds before retry..."
                    sleep 3
                fi
            fi
        done

        if [ "$PAIRED" = false ]; then
            echo "❌ Auto-pairing failed after 5 attempts"
            echo "Moonlight-web will start but may not be able to stream until manually paired"
        fi
    else
        echo "ℹ️  moonlight-web already paired with Wolf, skipping auto-pair"
    fi
fi

# Wait for web server process to keep container running
wait $WEB_SERVER_PID
