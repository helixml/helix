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
SKIP_STYLING=false
VERSION=""
R2_BUCKET=""

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --output) DMG_OUTPUT="$2"; shift 2 ;;
        --build-dir) BUILD_DIR="$2"; shift 2 ;;
        --notarize) NOTARIZE=true; shift ;;
        --upload) UPLOAD=true; shift ;;
        --skip-styling) SKIP_STYLING=true; shift ;;
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
# Create staging directory with app + Applications symlink
# =============================================================================

log "Creating DMG staging area..."
mkdir -p "$DMG_TEMP"

# Copy app bundle to staging
cp -R "$APP_BUNDLE" "$DMG_TEMP/"

# Create Applications symlink for drag-and-drop install
ln -s /Applications "$DMG_TEMP/Applications"

# =============================================================================
# Create DMG with styled Finder window
# =============================================================================
#
# Flow: UDRW (read-write) → mount → copy background + AppleScript styling
#       → unmount → convert to ULFO (compressed, read-only)
#
# Use --skip-styling for headless/CI builds that can't run AppleScript.

DMG_BACKGROUND="${FOR_MAC_DIR}/assets/dmg-background.png"
DMG_RW="${DMG_OUTPUT%.dmg}-rw.dmg"

if [ "$SKIP_STYLING" = true ] || [ ! -f "$DMG_BACKGROUND" ]; then
    # Unstyled path: single-step ULFO creation (for CI or missing background)
    if [ ! -f "$DMG_BACKGROUND" ]; then
        log "WARNING: Background image not found at $DMG_BACKGROUND"
        log "  Run: swift scripts/create-dmg-background.swift"
        log "  Falling back to unstyled DMG"
    fi
    log "Creating DMG (ULFO/lzfse, unstyled)..."
    hdiutil create \
        -fs HFS+ \
        -srcfolder "$DMG_TEMP" \
        -volname "$DMG_VOLUME" \
        -format ULFO \
        "$DMG_OUTPUT"
    rm -rf "$DMG_TEMP"
else
    # Styled path: create read-write, style with AppleScript, convert to ULFO
    log "Creating read-write DMG..."
    rm -f "$DMG_RW"

    # Calculate size: app bundle + 20MB headroom for background + .DS_Store
    APP_SIZE_KB=$(du -sk "$DMG_TEMP" | awk '{print $1}')
    DMG_SIZE_MB=$(( (APP_SIZE_KB / 1024) + 20 ))

    hdiutil create \
        -fs HFS+ \
        -srcfolder "$DMG_TEMP" \
        -volname "$DMG_VOLUME" \
        -format UDRW \
        -size "${DMG_SIZE_MB}m" \
        "$DMG_RW"

    rm -rf "$DMG_TEMP"

    # Mount the read-write DMG
    log "Mounting DMG for styling..."
    MOUNT_DIR=$(hdiutil attach "$DMG_RW" -readwrite -noverify -noautoopen | \
        grep "/Volumes/" | awk -F'\t' '{print $NF}' | head -1)

    if [ -z "$MOUNT_DIR" ] || [ ! -d "$MOUNT_DIR" ]; then
        echo "ERROR: Failed to mount DMG. Mount output:"
        hdiutil attach "$DMG_RW" -readwrite -noverify -noautoopen
        exit 1
    fi
    log "  Mounted at: $MOUNT_DIR"

    # Extract the actual volume name (might be "Helix 1" if another Helix is mounted)
    VOLUME_NAME=$(basename "$MOUNT_DIR")
    log "  Volume name: $VOLUME_NAME"

    # Copy background image into hidden .background directory
    mkdir -p "$MOUNT_DIR/.background"
    cp "$DMG_BACKGROUND" "$MOUNT_DIR/.background/background.png"

    # Style the Finder window with AppleScript
    log "Applying Finder styling..."
    osascript <<APPLESCRIPT
    tell application "Finder"
        tell disk "$VOLUME_NAME"
            open
            set current view of container window to icon view
            set toolbar visible of container window to false
            set statusbar visible of container window to false

            -- Window size: 660x400 to match background image
            set the bounds of container window to {100, 100, 760, 500}

            set viewOptions to the icon view options of container window
            set arrangement of viewOptions to not arranged
            set icon size of viewOptions to 128
            set background picture of viewOptions to file ".background:background.png"

            -- Position icons: app on left, Applications on right
            set position of item "$APP_BUNDLE_NAME.app" of container window to {170, 175}
            set position of item "Applications" of container window to {490, 175}

            close
            open
            -- Let Finder write .DS_Store
            delay 2
            close
        end tell
    end tell
APPLESCRIPT

    # Flush filesystem
    sync

    # Unmount
    log "Unmounting DMG..."
    hdiutil detach "$MOUNT_DIR" -quiet || hdiutil detach "$MOUNT_DIR" -force

    # Convert to compressed read-only ULFO
    log "Converting to ULFO (lzfse compressed)..."
    hdiutil convert "$DMG_RW" \
        -format ULFO \
        -o "$DMG_OUTPUT"

    # Clean up intermediate image
    rm -f "$DMG_RW"
fi

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
