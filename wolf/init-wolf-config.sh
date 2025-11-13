#!/bin/bash

# Initialize Wolf config from template if config.toml doesn't exist or is empty

CONFIG_FILE="/etc/wolf/cfg/config.toml"
TEMPLATE_FILE="/opt/wolf-defaults/config.toml.template"

if [ ! -f "$CONFIG_FILE" ] || [ ! -s "$CONFIG_FILE" ]; then
    echo "🔧 Initializing Wolf config from template..."
    cp "$TEMPLATE_FILE" "$CONFIG_FILE"

    # Generate UUID if empty
    if grep -q "uuid = ''" "$CONFIG_FILE"; then
        WOLF_UUID=$(cat /proc/sys/kernel/random/uuid 2>/dev/null || uuidgen 2>/dev/null || echo "00000000-0000-0000-0000-$(date +%s)")
        sed -i "s/uuid = ''/uuid = '$WOLF_UUID'/" "$CONFIG_FILE"
        echo "🆔 Generated UUID: $WOLF_UUID"
    fi

    # Set hostname from env var (default: "local" if not set)
    HELIX_HOSTNAME=${HELIX_HOSTNAME:-local}
    sed -i "s/{{HELIX_HOSTNAME}}/$HELIX_HOSTNAME/g" "$CONFIG_FILE"
    echo "🏷️  Set hostname: Helix ($HELIX_HOSTNAME)"

    # Set pairing PIN from env var if provided
    if [ ! -z "$MOONLIGHT_INTERNAL_PAIRING_PIN" ]; then
        # Wolf stores PIN as 4-digit array in config
        # Convert "1234" to [1, 2, 3, 4]
        PIN_ARRAY="[${MOONLIGHT_INTERNAL_PAIRING_PIN:0:1}, ${MOONLIGHT_INTERNAL_PAIRING_PIN:1:1}, ${MOONLIGHT_INTERNAL_PAIRING_PIN:2:1}, ${MOONLIGHT_INTERNAL_PAIRING_PIN:3:1}]"
        sed -i "s/pin = .*/pin = $PIN_ARRAY/" "$CONFIG_FILE"
        echo "🔐 Set pairing PIN from MOONLIGHT_INTERNAL_PAIRING_PIN"
    fi

    # Set GOP size (keyframe interval) from env var
    # Default: 120 (keyframe every 2 seconds at 60fps)
    GOP_SIZE=${GOP_SIZE:-120}
    sed -i "s/gop-size=[0-9-]*/gop-size=$GOP_SIZE/g" "$CONFIG_FILE"
    echo "🎬 Set GOP size (keyframe interval): $GOP_SIZE frames"

    echo "✅ Wolf config initialized"
else
    echo "ℹ️  Wolf config already exists, skipping initialization"
fi
