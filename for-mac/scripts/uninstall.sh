#!/bin/bash
# Completely uninstall Helix for Mac and all associated data.
# Usage: ./uninstall.sh [--keep-vm]
#   --keep-vm   Remove the app but keep VM disk images (they're large and slow to re-download)

set -euo pipefail

KEEP_VM=false
for arg in "$@"; do
    case "$arg" in
        --keep-vm) KEEP_VM=true ;;
        -h|--help)
            echo "Usage: $0 [--keep-vm]"
            echo "  --keep-vm   Remove everything except VM disk images"
            exit 0
            ;;
        *) echo "Unknown option: $arg"; exit 1 ;;
    esac
done

APP_PATH="/Applications/Helix for Mac.app"
DATA_DIR="$HOME/Library/Application Support/Helix"
BUNDLE_ID="com.helixml.Helix"
SPICE_SOCK="/tmp/helix-spice.sock"

echo "=== Helix for Mac Uninstaller ==="
echo ""
echo "This will remove:"
echo "  - $APP_PATH"
echo "  - $DATA_DIR (settings, SSH keys, updates)"
if [ "$KEEP_VM" = true ]; then
    echo "  - VM disk images will be KEPT (--keep-vm)"
else
    echo "  - VM disk images (~20GB+)"
fi
echo "  - WebKit caches and preferences"
echo "  - $SPICE_SOCK"
echo ""
read -p "Are you sure? [y/N] " confirm
if [[ ! "$confirm" =~ ^[Yy]$ ]]; then
    echo "Aborted."
    exit 0
fi

echo ""

# 1. Kill the app if running
if pgrep -f "Helix for Mac" >/dev/null 2>&1; then
    echo "Stopping Helix for Mac..."
    pkill -f "Helix for Mac" || true
    sleep 2
fi

# 2. Remove the app bundle
if [ -d "$APP_PATH" ]; then
    echo "Removing $APP_PATH..."
    rm -rf "$APP_PATH"
else
    echo "App not found at $APP_PATH (skipping)"
fi

# 3. Remove data directory
if [ -d "$DATA_DIR" ]; then
    if [ "$KEEP_VM" = true ]; then
        echo "Removing app data (keeping VM images)..."
        # Remove everything except vm/
        find "$DATA_DIR" -mindepth 1 -maxdepth 1 ! -name "vm" -exec rm -rf {} +
    else
        echo "Removing $DATA_DIR..."
        rm -rf "$DATA_DIR"
    fi
else
    echo "Data directory not found (skipping)"
fi

# 4. Remove WebKit/Wails caches
echo "Removing caches and preferences..."
rm -rf "$HOME/Library/Caches/$BUNDLE_ID" 2>/dev/null || true
rm -rf "$HOME/Library/WebKit/$BUNDLE_ID" 2>/dev/null || true
defaults delete "$BUNDLE_ID" 2>/dev/null || true

# 5. Remove SPICE socket
rm -f "$SPICE_SOCK" 2>/dev/null || true

echo ""
echo "Done. Helix for Mac has been uninstalled."
if [ "$KEEP_VM" = true ]; then
    echo "VM images kept at: $DATA_DIR/vm/"
fi
