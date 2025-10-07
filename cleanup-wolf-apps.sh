#!/bin/bash

# Remove default Wolf apps we don't need

echo "🧹 Cleaning up default Wolf apps..."

# Apps to remove (keep only Wolf UI, Helix Lab 3D, Test ball)
APPS_TO_REMOVE=(
    "Firefox"
    "RetroArch"
    "Steam"
    "Pegasus"
    "Lutris"
    "Prismlauncher"
    "Desktop (xfce)"
    "EmulationStation"
    "Kodi"
    "Test Integration"
)

for APP_TITLE in "${APPS_TO_REMOVE[@]}"; do
    echo "Removing: $APP_TITLE"
    # Note: This would require curl in API container or direct Wolf API access
    # For now, manually remove from config.toml or use Wolf UI to remove
done

echo ""
echo "To manually remove apps:"
echo "1. Restart Wolf (clears dynamic apps)"
echo "2. Or use Wolf UI to manage apps"
echo "3. Or edit wolf/config.toml (but it gets overwritten)"
echo ""
echo "Better solution: Set default Wolf config to minimal apps only"
