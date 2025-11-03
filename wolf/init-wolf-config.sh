#!/bin/bash

# Initialize Wolf config from template if config.toml doesn't exist or is empty

CONFIG_FILE="/etc/wolf/cfg/config.toml"
TEMPLATE_FILE="/etc/wolf/cfg/config.toml.template"

if [ ! -f "$CONFIG_FILE" ] || [ ! -s "$CONFIG_FILE" ]; then
    echo "🔧 Initializing Wolf config from template..."
    cp "$TEMPLATE_FILE" "$CONFIG_FILE"

    # Generate UUID if empty
    if grep -q "uuid = ''" "$CONFIG_FILE"; then
        WOLF_UUID=$(cat /proc/sys/kernel/random/uuid 2>/dev/null || uuidgen 2>/dev/null || echo "00000000-0000-0000-0000-$(date +%s)")
        sed -i "s/uuid = ''/uuid = '$WOLF_UUID'/" "$CONFIG_FILE"
        echo "🆔 Generated UUID: $WOLF_UUID"
    fi

    echo "✅ Wolf config initialized"
else
    echo "ℹ️  Wolf config already exists, skipping initialization"
fi
