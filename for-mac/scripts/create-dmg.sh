#!/bin/bash
set -euo pipefail

# =============================================================================
# Create DMG from Helix.app bundle
# =============================================================================
#
# Creates a distributable .dmg disk image containing:
#   - Helix for Mac.app (with embedded QEMU + frameworks)
#   - Symlink to /Applications for drag-and-drop install
#
# Prerequisites:
#   Run build-helix-app.sh first to create the .app bundle.
#
# Usage:
#   cd for-mac && ./scripts/create-dmg.sh
#   ./scripts/create-dmg.sh --output ~/Desktop/Helix.dmg

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
FOR_MAC_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

APP_BUNDLE_NAME="helix-for-mac"
APP_BUNDLE="${FOR_MAC_DIR}/build/bin/${APP_BUNDLE_NAME}.app"
DMG_NAME="Helix-for-Mac"
DMG_OUTPUT="${FOR_MAC_DIR}/build/bin/${DMG_NAME}.dmg"
DMG_VOLUME="Helix"
DMG_TEMP="${FOR_MAC_DIR}/build/bin/dmg-staging"

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --output) DMG_OUTPUT="$2"; shift 2 ;;
        *) echo "Unknown option: $1"; exit 1 ;;
    esac
done

log() { echo "[$(date +%H:%M:%S)] $*"; }

# Verify app bundle exists
if [ ! -d "$APP_BUNDLE" ]; then
    echo "ERROR: App bundle not found at: $APP_BUNDLE"
    echo "Run ./scripts/build-helix-app.sh first."
    exit 1
fi

# Clean up previous artifacts
rm -f "$DMG_OUTPUT"
rm -rf "$DMG_TEMP"

# =============================================================================
# Create staging directory with app + Applications symlink
# =============================================================================

log "Creating DMG staging area..."
mkdir -p "$DMG_TEMP"

# Copy app bundle to staging
cp -R "$APP_BUNDLE" "$DMG_TEMP/"

# Create Applications symlink for drag-and-drop install
ln -s /Applications "$DMG_TEMP/Applications"

# =============================================================================
# Create DMG
# =============================================================================

log "Creating DMG (compressed UDZO format)..."

# Create the DMG from the staging directory
hdiutil create \
    -fs HFS+ \
    -srcfolder "$DMG_TEMP" \
    -volname "$DMG_VOLUME" \
    -format UDZO \
    -imagekey zlib-level=9 \
    "$DMG_OUTPUT"

# Clean up staging
rm -rf "$DMG_TEMP"

DMG_SIZE=$(du -h "$DMG_OUTPUT" | awk '{print $1}')

log ""
log "================================================"
log "DMG created successfully!"
log "================================================"
log ""
log "Output: $DMG_OUTPUT"
log "Size:   $DMG_SIZE"
log ""
log "To install:"
log "  1. Double-click the .dmg to mount it"
log "  2. Drag 'Helix for Mac' to the Applications folder"
log "  3. Eject the disk image"
log "  4. Launch from Applications"
log ""
log "NOTE: Without Apple Developer signing, users must:"
log "  System Settings > Privacy & Security > scroll down > 'Open Anyway'"
log "  (macOS Sequoia removed the old right-click bypass)"
