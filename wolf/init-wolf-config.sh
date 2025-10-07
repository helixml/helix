#!/bin/bash

# Initialize Wolf config from template if config.toml doesn't exist or is empty

CONFIG_FILE="/etc/wolf/cfg/config.toml"
TEMPLATE_FILE="/etc/wolf/cfg/config.toml.template"

if [ ! -f "$CONFIG_FILE" ] || [ ! -s "$CONFIG_FILE" ]; then
    echo "🔧 Initializing Wolf config from template..."
    cp "$TEMPLATE_FILE" "$CONFIG_FILE"
    echo "✅ Wolf config initialized"
else
    echo "ℹ️  Wolf config already exists, skipping initialization"
fi
