#!/bin/bash
set -euo pipefail

# =============================================================================
# Create DMG from Helix.app bundle + optionally upload to R2
# =============================================================================
#
# Creates a distributable .dmg disk image containing:
#   - Helix for Mac.app (with embedded QEMU + frameworks)
#   - Symlink to /Applications for drag-and-drop install
#
# VM disk images are NOT included in the DMG. They are downloaded from
# Cloudflare R2 on first launch (~18GB). The DMG itself is ~300MB.
#
# Prerequisites:
#   Run build-helix-app.sh first to create the .app bundle.
#
# Usage:
#   cd for-mac && ./scripts/create-dmg.sh
#   ./scripts/create-dmg.sh --output ~/Desktop/Helix.dmg
#   ./scripts/create-dmg.sh --skip-styling              # Skip Finder styling (for CI/headless)
#   ./scripts/create-dmg.sh --upload                    # Upload DMG + VM images to R2
#   ./scripts/create-dmg.sh --upload --version v1.0.0   # Upload with specific version tag

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
FOR_MAC_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
REPO_ROOT="$(cd "$FOR_MAC_DIR/.." && pwd)"

APP_BUNDLE_NAME="Helix"
APP_BUNDLE="${FOR_MAC_DIR}/build/bin/${APP_BUNDLE_NAME}.app"
DMG_NAME="Helix-for-Mac"
DMG_VOLUME="Helix"

# VM image directory (source for R2 upload)
VM_DIR="${VM_DIR:-$HOME/Library/Application Support/Helix/vm/helix-desktop}"

# Default output location (can be overridden with --output or --build-dir)
DMG_OUTPUT="${FOR_MAC_DIR}/build/bin/${DMG_NAME}.dmg"
BUILD_DIR=""

NOTARIZE=false
UPLOAD=false
REBUILD_TEMPLATE=false
VERSION=""
R2_BUCKET=""

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --output) DMG_OUTPUT="$2"; shift 2 ;;
        --build-dir) BUILD_DIR="$2"; shift 2 ;;
        --notarize) NOTARIZE=true; shift ;;
        --upload) UPLOAD=true; shift ;;
        --skip-styling) log "WARNING: --skip-styling is deprecated (template approach doesn't need it)"; shift ;;
        --rebuild-template) REBUILD_TEMPLATE=true; shift ;;
        --version) VERSION="$2"; shift 2 ;;
        --r2-bucket) R2_BUCKET="$2"; shift 2 ;;
        *) echo "Unknown option: $1"; exit 1 ;;
    esac
done

# If --build-dir specified, use it for staging and output
if [ -n "$BUILD_DIR" ]; then
    DMG_OUTPUT="${BUILD_DIR}/${DMG_NAME}.dmg"
fi
DMG_TEMP="$(dirname "$DMG_OUTPUT")/dmg-staging"

# Default version from git hash
if [ -z "$VERSION" ]; then
    VERSION="$(git -C "$REPO_ROOT" rev-parse --short HEAD 2>/dev/null || echo "dev")"
fi

log() { echo "[$(date +%H:%M:%S)] $*"; }

# =============================================================================
# Rebuild DMG template (requires GUI session with Finder)
# =============================================================================

if [ "$REBUILD_TEMPLATE" = true ]; then
    DMG_BACKGROUND="${FOR_MAC_DIR}/assets/dmg-background.png"
    DMG_TEMPLATE="${FOR_MAC_DIR}/assets/dmg-template.dmg"

    if [ ! -f "$DMG_BACKGROUND" ]; then
        echo "ERROR: Background image not found. Run: swift scripts/create-dmg-background.swift"
        exit 1
    fi

    log "Rebuilding DMG template (requires Finder)..."
    TMPL_TEMP="/tmp/dmg-template-staging"
    rm -rf "$TMPL_TEMP"
    mkdir -p "$TMPL_TEMP/${APP_BUNDLE_NAME}.app/Contents/MacOS"
    touch "$TMPL_TEMP/${APP_BUNDLE_NAME}.app/Contents/MacOS/${APP_BUNDLE_NAME}"

    rm -f /tmp/dmg-template-rw.dmg
    hdiutil create -fs HFS+ -srcfolder "$TMPL_TEMP" -volname "$DMG_VOLUME" \
        -format UDRW -size 50m /tmp/dmg-template-rw.dmg
    rm -rf "$TMPL_TEMP"

    MOUNT_DIR=$(hdiutil attach /tmp/dmg-template-rw.dmg -readwrite -noverify -noautoopen | \
        grep "/Volumes/" | awk -F'\t' '{print $NF}' | head -1)
    VOLUME_NAME=$(basename "$MOUNT_DIR")
    log "  Mounted at: $MOUNT_DIR"

    # Create Finder alias (carries proper /Applications icon)
    osascript -e "tell application \"Finder\" to make new alias file at (POSIX file \"$MOUNT_DIR\" as alias) to (POSIX file \"/Applications\" as alias) with properties {name:\"Applications\"}"

    # Copy background
    mkdir -p "$MOUNT_DIR/.background"
    cp "$DMG_BACKGROUND" "$MOUNT_DIR/.background/background.png"

    # Apply Finder styling
    osascript <<APPLESCRIPT
    tell application "Finder"
        tell disk "$VOLUME_NAME"
            open
            set current view of container window to icon view
            set toolbar visible of container window to false
            set statusbar visible of container window to false
            set the bounds of container window to {100, 100, 760, 500}
            set viewOptions to the icon view options of container window
            set arrangement of viewOptions to not arranged
            set icon size of viewOptions to 128
            set background picture of viewOptions to file ".background:background.png"
            set position of item "${APP_BUNDLE_NAME}.app" of container window to {170, 175}
            set position of item "Applications" of container window to {490, 175}
            close
            open
            delay 2
            close
        end tell
    end tell
APPLESCRIPT

    sync
    hdiutil detach "$MOUNT_DIR" -quiet || hdiutil detach "$MOUNT_DIR" -force

    # Compress and save
    rm -f "$DMG_TEMPLATE"
    hdiutil convert /tmp/dmg-template-rw.dmg -format UDZO -o "$DMG_TEMPLATE"
    rm -f /tmp/dmg-template-rw.dmg

    log "Template saved: $DMG_TEMPLATE ($(du -h "$DMG_TEMPLATE" | awk '{print $1}'))"
    log "Commit this file to the repo."
    exit 0
fi

# Verify app bundle exists
if [ ! -d "$APP_BUNDLE" ]; then
    echo "ERROR: App bundle not found at: $APP_BUNDLE"
    echo "Run ./scripts/build-helix-app.sh first."
    exit 1
fi

# =============================================================================
# Sign the app bundle before packaging
# =============================================================================
# build-helix-app.sh only does ad-hoc signing. If .env.signing exists with a
# Developer ID certificate, re-sign properly before creating the DMG.

SIGN_SCRIPT="${SCRIPT_DIR}/sign-app.sh"
SIGNING_ENV="${FOR_MAC_DIR}/.env.signing"

if [ -f "$SIGNING_ENV" ] && [ -f "$SIGN_SCRIPT" ]; then
    log "Found .env.signing — re-signing app with Developer ID before packaging..."
    # Don't pass --notarize here — the DMG itself gets notarized below,
    # which covers the app inside it. We just need the Developer ID signature.
    bash "$SIGN_SCRIPT"
else
    log "No .env.signing found — app will be packaged with ad-hoc signature"
    log "Users must use System Settings > Privacy & Security > Open Anyway"
fi

# Clean up previous artifacts
rm -f "$DMG_OUTPUT"
rm -rf "$DMG_TEMP"

# =============================================================================
# Create DMG from pre-built template
# =============================================================================
#
# Uses a pre-built template DMG (assets/dmg-template.dmg) that contains:
#   - Finder alias to /Applications (with proper icon — requires GUI to create)
#   - Background image with arrow
#   - .DS_Store with Finder layout (icon positions, window size, icon view)
#
# Flow: template (UDZO) → convert to UDRW → mount → swap app bundle
#       → unmount → convert to ULFO (compressed, read-only)
#
# To regenerate the template (requires GUI session with Finder):
#   ./scripts/create-dmg.sh --rebuild-template
#
# The --skip-styling flag is no longer needed — the template already has
# all Finder styling baked in.

DMG_TEMPLATE="${FOR_MAC_DIR}/assets/dmg-template.dmg"
DMG_RW="${DMG_OUTPUT%.dmg}-rw.dmg"

if [ ! -f "$DMG_TEMPLATE" ]; then
    echo "ERROR: DMG template not found at: $DMG_TEMPLATE"
    echo "Run: ./scripts/create-dmg.sh --rebuild-template (requires GUI session)"
    exit 1
fi

# Convert compressed template to read-write
log "Preparing DMG from template..."
rm -f "$DMG_RW"
hdiutil convert "$DMG_TEMPLATE" -format UDRW -o "$DMG_RW"

# Resize to fit the app bundle + headroom
APP_SIZE_KB=$(du -sk "$APP_BUNDLE" | awk '{print $1}')
DMG_SIZE_MB=$(( (APP_SIZE_KB / 1024) + 20 ))
hdiutil resize -size "${DMG_SIZE_MB}m" "$DMG_RW" 2>/dev/null || true

# Mount
log "Mounting DMG..."
MOUNT_DIR=$(hdiutil attach "$DMG_RW" -readwrite -noverify -noautoopen | \
    grep "/Volumes/" | awk -F'\t' '{print $NF}' | head -1)
if [ -z "$MOUNT_DIR" ] || [ ! -d "$MOUNT_DIR" ]; then
    echo "ERROR: Failed to mount DMG."
    exit 1
fi
log "  Mounted at: $MOUNT_DIR"

# Swap placeholder app with real app
rm -rf "$MOUNT_DIR/${APP_BUNDLE_NAME}.app"
cp -R "$APP_BUNDLE" "$MOUNT_DIR/"

sync
log "Unmounting DMG..."
hdiutil detach "$MOUNT_DIR" -quiet || hdiutil detach "$MOUNT_DIR" -force

# Convert to compressed read-only ULFO
log "Converting to ULFO (lzfse compressed)..."
hdiutil convert "$DMG_RW" \
    -format ULFO \
    -o "$DMG_OUTPUT"
rm -f "$DMG_RW"

DMG_SIZE=$(du -h "$DMG_OUTPUT" | awk '{print $1}')

# =============================================================================
# Notarize DMG (optional)
# =============================================================================

if [ "$NOTARIZE" = true ]; then
    log "Notarizing DMG (uploading to Apple)..."
    xcrun notarytool submit "$DMG_OUTPUT" \
        --keychain-profile "helix-notarize" \
        --wait

    log "Stapling notarization ticket to DMG..."
    xcrun stapler staple "$DMG_OUTPUT"
    log "DMG notarized and stapled"
fi

# =============================================================================
# Upload to R2 (optional)
# =============================================================================

if [ "$UPLOAD" = true ]; then
    log "Uploading to Cloudflare R2..."

    # Load R2 credentials from .env.r2
    R2_ENV="${FOR_MAC_DIR}/.env.r2"
    if [ -f "$R2_ENV" ]; then
        set -a
        source "$R2_ENV"
        set +a
    fi

    # Validate required env vars
    : "${R2_ACCOUNT_ID:?Set R2_ACCOUNT_ID in .env.r2 or environment}"
    : "${R2_ACCESS_KEY_ID:?Set R2_ACCESS_KEY_ID in .env.r2 or environment}"
    : "${R2_SECRET_ACCESS_KEY:?Set R2_SECRET_ACCESS_KEY in .env.r2 or environment}"
    R2_BUCKET="${R2_BUCKET:-${R2_BUCKET_NAME:-helix-releases}}"
    R2_PUBLIC_URL="${R2_PUBLIC_URL:-https://dl.helix.ml}"

    R2_ENDPOINT="https://${R2_ACCOUNT_ID}.r2.cloudflarestorage.com"

    # Use aws CLI for S3-compatible uploads
    if ! command -v aws &>/dev/null; then
        echo "ERROR: aws CLI not found. Install with: brew install awscli"
        exit 1
    fi

    export AWS_ACCESS_KEY_ID="$R2_ACCESS_KEY_ID"
    export AWS_SECRET_ACCESS_KEY="$R2_SECRET_ACCESS_KEY"

    upload_file() {
        local src="$1"
        local dest="$2"
        local filename
        filename=$(basename "$src")
        local size
        size=$(du -h "$src" | awk '{print $1}')
        log "  Uploading ${filename} (${size}) -> s3://${R2_BUCKET}/${dest}"
        aws s3 cp "$src" "s3://${R2_BUCKET}/${dest}" \
            --endpoint-url "$R2_ENDPOINT" \
            --no-progress
    }

    # 1. Upload DMG
    upload_file "$DMG_OUTPUT" "desktop/${VERSION}/${DMG_NAME}.dmg"

    # 2. Upload VM images (from source directory, not bundle)
    if [ -f "${VM_DIR}/disk.qcow2" ]; then
        upload_file "${VM_DIR}/disk.qcow2" "vm/${VERSION}/disk.qcow2"
    else
        log "  WARNING: disk.qcow2 not found at ${VM_DIR}, skipping"
    fi

    if [ -f "${VM_DIR}/zfs-data.qcow2" ]; then
        upload_file "${VM_DIR}/zfs-data.qcow2" "vm/${VERSION}/zfs-data.qcow2"
    else
        log "  WARNING: zfs-data.qcow2 not found at ${VM_DIR}, skipping"
    fi

    if [ -f "${VM_DIR}/efi_vars.fd" ]; then
        upload_file "${VM_DIR}/efi_vars.fd" "vm/${VERSION}/efi_vars.fd"
    fi

    # 3. Upload manifest from app bundle
    MANIFEST="${APP_BUNDLE}/Contents/Resources/vm/vm-manifest.json"
    if [ -f "$MANIFEST" ]; then
        upload_file "$MANIFEST" "vm/${VERSION}/manifest.json"
    fi

    # 4. Update latest.json pointer
    LATEST_JSON=$(mktemp)
    cat > "$LATEST_JSON" << EOF
{
  "version": "${VERSION}",
  "url": "${R2_PUBLIC_URL}/desktop/${VERSION}/${DMG_NAME}.dmg",
  "vm_manifest": "${R2_PUBLIC_URL}/vm/${VERSION}/manifest.json"
}
EOF
    upload_file "$LATEST_JSON" "vm/latest.json"
    upload_file "$LATEST_JSON" "desktop/latest.json"
    rm -f "$LATEST_JSON"

    log "  Upload complete!"
    log "  DMG: ${R2_PUBLIC_URL}/desktop/${VERSION}/${DMG_NAME}.dmg"
    log "  VM:  ${R2_PUBLIC_URL}/vm/${VERSION}/"
fi

log ""
log "================================================"
log "DMG created successfully!"
log "================================================"
log ""
log "Output: $DMG_OUTPUT"
log "Size:   $DMG_SIZE"
if [ "$NOTARIZE" = true ]; then
    log "Notarized: YES (Gatekeeper will accept on any Mac)"
fi
if [ "$UPLOAD" = true ]; then
    log "Uploaded:  YES (version: ${VERSION})"
fi
log ""
log "To install:"
log "  1. Double-click the .dmg to mount it"
log "  2. Drag 'Helix for Mac' to the Applications folder"
log "  3. Eject the disk image"
log "  4. Launch from Applications"
log "  5. VM images will be downloaded on first launch (~18GB)"
