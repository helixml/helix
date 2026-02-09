#!/bin/bash
set -euo pipefail

# =============================================================================
# Code Sign Helix.app for distribution
# =============================================================================
#
# This script signs the app bundle for macOS distribution.
#
# Without Developer ID cert (ad-hoc):
#   ./scripts/sign-app.sh
#   Users must use "Open Anyway" in Security settings.
#
# With Developer ID cert:
#   ./scripts/sign-app.sh --identity "Developer ID Application: Your Name (TEAMID)"
#   ./scripts/sign-app.sh --notarize   (reads .env.signing for identity)
#
# IMPORTANT: Sign inside-out, WITHOUT --deep.
# --deep creates a seal over all inner components. If you re-sign anything
# inside after --deep, the seal breaks and macOS kills the app with
# SIGKILL (Code Signature Invalid) / Taskgated Invalid Signature.
#
# Correct order:
#   1. Sign each framework individually
#   2. Sign QEMU dylib (with entitlements)
#   3. Sign QEMU wrapper executable (with entitlements)
#   4. Sign the main app bundle (seals everything, do NOT use --deep)

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
FOR_MAC_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

APP_BUNDLE_NAME="helix-for-mac"
APP_BUNDLE="${FOR_MAC_DIR}/build/bin/${APP_BUNDLE_NAME}.app"
ENTITLEMENTS="${FOR_MAC_DIR}/build/darwin/entitlements.plist"
IDENTITY="-"  # Ad-hoc by default
NOTARIZE=false
APPLE_ID=""
TEAM_ID=""
APP_PASSWORD=""

# Load signing config from .env.signing if it exists
SIGNING_ENV="${FOR_MAC_DIR}/.env.signing"
if [ -f "$SIGNING_ENV" ]; then
    # shellcheck disable=SC1090
    source "$SIGNING_ENV"
    if [ -n "${APPLE_SIGNING_IDENTITY:-}" ]; then
        IDENTITY="$APPLE_SIGNING_IDENTITY"
    fi
    if [ -n "${APPLE_TEAM_ID:-}" ]; then
        TEAM_ID="$APPLE_TEAM_ID"
    fi
    # APPLE_ID is set directly from the env file
fi

# Parse arguments (override .env.signing values)
while [[ $# -gt 0 ]]; do
    case $1 in
        --identity) IDENTITY="$2"; shift 2 ;;
        --notarize) NOTARIZE=true; shift ;;
        --apple-id) APPLE_ID="$2"; shift 2 ;;
        --team-id) TEAM_ID="$2"; shift 2 ;;
        --app-password) APP_PASSWORD="$2"; shift 2 ;;
        *) echo "Unknown option: $1"; exit 1 ;;
    esac
done

log() { echo "[$(date +%H:%M:%S)] $*"; }

if [ ! -d "$APP_BUNDLE" ]; then
    echo "ERROR: App bundle not found at: $APP_BUNDLE"
    echo "Run ./scripts/build-helix-app.sh first."
    exit 1
fi

if [ ! -f "$ENTITLEMENTS" ]; then
    echo "ERROR: Entitlements file not found at: $ENTITLEMENTS"
    exit 1
fi

CONTENTS="${APP_BUNDLE}/Contents"
MACOS_DIR="${CONTENTS}/MacOS"
FRAMEWORKS_DIR="${CONTENTS}/Frameworks"

# Determine signing options
SIGN_OPTS=(--force --sign "$IDENTITY" --timestamp=none)
if [ "$IDENTITY" != "-" ]; then
    # With a real cert, use hardened runtime and proper timestamp
    SIGN_OPTS=(--force --sign "$IDENTITY" --timestamp --options runtime)
fi

# =============================================================================
# Step 1: Sign frameworks (innermost first)
# =============================================================================

log "Step 1: Signing frameworks..."

FRAMEWORK_COUNT=0
for fw_dir in "$FRAMEWORKS_DIR"/*.framework; do
    if [ -d "$fw_dir" ]; then
        codesign "${SIGN_OPTS[@]}" "$fw_dir" || {
            echo "  WARNING: Failed to sign $(basename "$fw_dir")"
        }
        FRAMEWORK_COUNT=$((FRAMEWORK_COUNT + 1))
    fi
done
log "  Signed $FRAMEWORK_COUNT frameworks"

# =============================================================================
# Step 2: Sign QEMU binaries (with entitlements)
# =============================================================================

log "Step 2: Signing QEMU binaries (with entitlements)..."

QEMU_DYLIB="${MACOS_DIR}/libqemu-aarch64-softmmu.dylib"
QEMU_WRAPPER="${MACOS_DIR}/qemu-system-aarch64"

# Sign dylib first (loaded by wrapper)
if [ -f "$QEMU_DYLIB" ]; then
    codesign "${SIGN_OPTS[@]}" --entitlements "$ENTITLEMENTS" "$QEMU_DYLIB"
    log "  Signed QEMU dylib"
else
    log "  WARNING: QEMU dylib not found"
fi

# Sign wrapper executable (this is what gets exec'd, needs hypervisor entitlement)
if [ -f "$QEMU_WRAPPER" ]; then
    codesign "${SIGN_OPTS[@]}" --entitlements "$ENTITLEMENTS" "$QEMU_WRAPPER"
    log "  Signed QEMU wrapper"
else
    log "  WARNING: QEMU wrapper executable not found"
fi

# =============================================================================
# Step 3: Sign main app bundle (seals everything — MUST be last)
# =============================================================================
#
# DO NOT use --deep here. --deep recursively re-signs everything inside,
# which would overwrite the QEMU entitlements we just applied. Instead,
# signing just the app bundle creates a CodeResources seal that covers
# all the already-signed inner components.

log "Step 3: Signing main app bundle (creating seal)..."

codesign "${SIGN_OPTS[@]}" --entitlements "$ENTITLEMENTS" "$APP_BUNDLE"
log "  Signed app bundle"

# =============================================================================
# Step 4: Verify
# =============================================================================

log "Step 4: Verifying signature..."

# Verify the overall bundle (checks seal integrity)
if ! codesign --verify --deep --strict "$APP_BUNDLE" 2>&1; then
    echo "ERROR: App bundle signature verification failed!"
    codesign -vvv "$APP_BUNDLE" 2>&1
    exit 1
fi
log "  Bundle signature: valid"

# Verify QEMU wrapper has entitlements
if [ -f "$QEMU_WRAPPER" ]; then
    if codesign -d --entitlements - "$QEMU_WRAPPER" 2>&1 | grep -q "com.apple.security.hypervisor"; then
        log "  QEMU entitlements: hypervisor OK"
    else
        echo "ERROR: QEMU wrapper missing hypervisor entitlement!"
        exit 1
    fi
fi

# Show signing identity
codesign -dvv "$APP_BUNDLE" 2>&1 | grep -E "Authority|TeamIdentifier|Identifier" || true

# =============================================================================
# Step 5: Notarize (optional, requires Developer ID)
# =============================================================================

if [ "$NOTARIZE" = true ]; then
    if [ "$IDENTITY" = "-" ]; then
        echo "ERROR: Cannot notarize with ad-hoc signing. Provide --identity or .env.signing."
        exit 1
    fi

    log "Step 5: Notarizing app..."

    # Create a zip for notarization
    NOTARIZE_ZIP="/tmp/helix-notarize.zip"
    rm -f "$NOTARIZE_ZIP"
    log "  Creating zip for upload..."
    ditto -c -k --keepParent "$APP_BUNDLE" "$NOTARIZE_ZIP"

    # Submit for notarization
    log "  Submitting to Apple notary service..."
    if [ -n "$APP_PASSWORD" ]; then
        xcrun notarytool submit "$NOTARIZE_ZIP" \
            --apple-id "$APPLE_ID" \
            --team-id "$TEAM_ID" \
            --password "$APP_PASSWORD" \
            --wait
    else
        # Use keychain profile (set up with: xcrun notarytool store-credentials)
        xcrun notarytool submit "$NOTARIZE_ZIP" \
            --keychain-profile "helix-notarize" \
            --wait
    fi

    # Staple the ticket to the app
    log "  Stapling notarization ticket..."
    xcrun stapler staple "$APP_BUNDLE"

    rm -f "$NOTARIZE_ZIP"
    log "  App notarization complete!"
fi

log ""
log "================================================"
log "Signing complete!"
log "================================================"
log ""
if [ "$IDENTITY" = "-" ]; then
    log "Signed with: ad-hoc (local testing only)"
    log "Users on other Macs must: Settings > Privacy & Security > Open Anyway"
else
    log "Signed with: $IDENTITY"
    if [ "$NOTARIZE" = true ]; then
        log "Notarized: YES (Gatekeeper will accept on any Mac)"
    else
        log "Notarized: NO (run with --notarize for full Gatekeeper approval)"
    fi
fi
log ""
log "Next steps:"
log "  ./scripts/create-dmg.sh --notarize    # Create and notarize DMG"
