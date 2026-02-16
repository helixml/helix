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
#   ./scripts/sign-app.sh --identity "Developer ID Application: Your Name (TEAMID)" --notarize
#
# Signing order matters: sign inside-out (frameworks first, then app)

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
FOR_MAC_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

APP_BUNDLE_NAME="Helix"
APP_BUNDLE="${FOR_MAC_DIR}/build/bin/${APP_BUNDLE_NAME}.app"
APP_ENTITLEMENTS="${FOR_MAC_DIR}/build/darwin/entitlements-app.plist"
QEMU_ENTITLEMENTS="${FOR_MAC_DIR}/build/darwin/entitlements.plist"
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
    if [ -n "${APPLE_ID:-}" ]; then
        # APPLE_ID already set from env file
        true
    fi
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

if [ ! -f "$QEMU_ENTITLEMENTS" ]; then
    echo "ERROR: QEMU entitlements file not found at: $QEMU_ENTITLEMENTS"
    exit 1
fi
if [ ! -f "$APP_ENTITLEMENTS" ]; then
    echo "ERROR: App entitlements file not found at: $APP_ENTITLEMENTS"
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
# Step 1: Sign frameworks (inside-out)
# =============================================================================

log "Step 1: Signing frameworks..."

FRAMEWORK_COUNT=0
for fw_dir in "$FRAMEWORKS_DIR"/*.framework; do
    if [ -d "$fw_dir" ]; then
        codesign "${SIGN_OPTS[@]}" "$fw_dir" 2>/dev/null || {
            echo "  WARNING: Failed to sign $(basename "$fw_dir")"
        }
        FRAMEWORK_COUNT=$((FRAMEWORK_COUNT + 1))
    fi
done
log "  Signed $FRAMEWORK_COUNT frameworks"

# =============================================================================
# Step 2: Sign main app bundle
# =============================================================================

log "Step 2: Signing main app bundle..."

codesign "${SIGN_OPTS[@]}" --entitlements "$APP_ENTITLEMENTS" --deep "$APP_BUNDLE"
log "  Signed app bundle (minimal entitlements)"

# =============================================================================
# Step 3: Re-sign QEMU dylib with entitlements
# =============================================================================

# QEMU must be signed AFTER --deep because --deep strips inner entitlements.
# The wrapper executable (qemu-system-aarch64) is the process that needs
# Hypervisor.framework access, JIT, and unsigned memory entitlements.
log "Step 3: Re-signing QEMU binaries (with entitlements, after --deep)..."

QEMU_DYLIB="${MACOS_DIR}/libqemu-aarch64-softmmu.dylib"
QEMU_WRAPPER="${MACOS_DIR}/qemu-system-aarch64"

if [ -f "$QEMU_DYLIB" ]; then
    codesign "${SIGN_OPTS[@]}" --entitlements "$QEMU_ENTITLEMENTS" "$QEMU_DYLIB"
    log "  Signed QEMU dylib"
fi
if [ -f "$QEMU_WRAPPER" ]; then
    codesign "${SIGN_OPTS[@]}" --entitlements "$QEMU_ENTITLEMENTS" "$QEMU_WRAPPER"
    log "  Signed QEMU wrapper with entitlements"
else
    log "  WARNING: QEMU wrapper executable not found"
fi

QEMU_IMG="${MACOS_DIR}/qemu-img"
if [ -f "$QEMU_IMG" ]; then
    codesign "${SIGN_OPTS[@]}" "$QEMU_IMG"
    log "  Signed qemu-img"
fi

# =============================================================================
# Step 4: Verify
# =============================================================================

log "Step 4: Verifying signature..."
codesign -vvv "$APP_BUNDLE" 2>&1 | tail -2

# =============================================================================
# Step 5: Notarize (optional, requires Developer ID)
# =============================================================================

if [ "$NOTARIZE" = true ]; then
    if [ "$IDENTITY" = "-" ]; then
        echo "ERROR: Cannot notarize with ad-hoc signing. Provide --identity."
        exit 1
    fi
    if [ -z "$APPLE_ID" ] || [ -z "$TEAM_ID" ]; then
        echo "ERROR: Notarization requires --apple-id and --team-id"
        echo "Usage: $0 --identity 'Developer ID...' --notarize --apple-id you@email.com --team-id XXXXX"
        exit 1
    fi

    log "Step 5: Notarizing..."

    # Create a zip for notarization
    NOTARIZE_ZIP="/tmp/helix-notarize.zip"
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

    # Staple the ticket
    log "  Stapling notarization ticket..."
    xcrun stapler staple "$APP_BUNDLE"

    rm -f "$NOTARIZE_ZIP"
    log "  Notarization complete!"
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
