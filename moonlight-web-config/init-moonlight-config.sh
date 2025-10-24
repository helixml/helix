#!/bin/bash

# Initialize moonlight-web data.json from template if it doesn't exist or is empty

DATA_FILE="/app/server/data.json"
TEMPLATE_FILE="/app/server/data.json.template"

if [ ! -f "$DATA_FILE" ] || [ ! -s "$DATA_FILE" ]; then
    echo "🔧 Initializing moonlight-web data.json from template..."
    cp "$TEMPLATE_FILE" "$DATA_FILE"
    echo "✅ moonlight-web data.json initialized"
else
    echo "ℹ️  moonlight-web data.json already exists, skipping initialization"
fi
