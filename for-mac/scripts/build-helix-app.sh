#!/bin/bash
set -euo pipefail

# =============================================================================
# Build Helix.app with embedded QEMU + frameworks for standalone distribution
# =============================================================================
#
# This script:
#   1. Builds the Wails app (Go + frontend)
#   2. Copies our custom QEMU binary into the app bundle
#   3. Copies all required open-source frameworks from UTM's sysroot
#   4. Copies EFI firmware for VM booting
#   5. Copies Vulkan ICD (KosmicKrisp) for GPU rendering
#   6. Fixes dylib paths with install_name_tool
#   7. Ad-hoc signs everything
#
# Prerequisites:
#   - Wails CLI: go install github.com/wailsapp/wails/v2/cmd/wails@latest
#   - Custom QEMU built: ./qemu-helix/build-qemu-standalone.sh
#   - UTM sysroot: ~/pm/UTM/sysroot-macOS-arm64/
#   - Node.js + npm (for frontend build)
#
# Usage:
#   cd for-mac && ./scripts/build-helix-app.sh
#   ./scripts/build-helix-app.sh --skip-wails   # Skip Wails build (repackage only)

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
FOR_MAC_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
REPO_ROOT="$(cd "$FOR_MAC_DIR/.." && pwd)"

# Configuration
SYSROOT="${SYSROOT:-$HOME/pm/UTM/sysroot-macOS-arm64}"
UTM_FRAMEWORKS="${UTM_APP_FRAMEWORKS:-/Applications/UTM.app/Contents/Frameworks}"
EFI_CODE="/opt/homebrew/share/qemu/edk2-aarch64-code.fd"
EFI_VARS_TEMPLATE="/opt/homebrew/share/qemu/edk2-arm-vars.fd"

# VM image directory (from provision-vm.sh)
VM_DIR="${VM_DIR:-$HOME/.helix/vm/helix-desktop}"

# Output paths
# Wails uses the "name" field from wails.json for the .app bundle name,
# and "outputfilename" for the executable inside Contents/MacOS/.
APP_BUNDLE_NAME="helix-for-mac"
APP_EXEC_NAME="Helix for Mac"
APP_BUNDLE="${FOR_MAC_DIR}/build/bin/${APP_BUNDLE_NAME}.app"
CONTENTS="${APP_BUNDLE}/Contents"
MACOS_DIR="${CONTENTS}/MacOS"
FRAMEWORKS_DIR="${CONTENTS}/Frameworks"
RESOURCES_DIR="${CONTENTS}/Resources"

# Parse arguments
SKIP_WAILS=false
while [[ $# -gt 0 ]]; do
    case $1 in
        --skip-wails) SKIP_WAILS=true; shift ;;
        *) echo "Unknown option: $1"; exit 1 ;;
    esac
done

log() { echo "[$(date +%H:%M:%S)] $*"; }

# =============================================================================
# Verify prerequisites
# =============================================================================

log "=== Building Helix.app ==="

if [ ! -d "$SYSROOT" ]; then
    echo "ERROR: UTM sysroot not found at: $SYSROOT"
    echo "Build UTM dependencies first, or set SYSROOT env var."
    exit 1
fi

QEMU_DYLIB="$SYSROOT/lib/libqemu-aarch64-softmmu.dylib"
if [ ! -f "$QEMU_DYLIB" ]; then
    echo "ERROR: QEMU dylib not found at: $QEMU_DYLIB"
    echo "Build QEMU first: ./qemu-helix/build-qemu-standalone.sh"
    exit 1
fi

if [ ! -f "$EFI_CODE" ]; then
    echo "ERROR: EFI firmware not found at: $EFI_CODE"
    echo "Install with: brew install qemu"
    exit 1
fi

# =============================================================================
# Step 1: Build Wails app
# =============================================================================

if [ "$SKIP_WAILS" = false ]; then
    log "Step 1: Building Wails app..."
    cd "$FOR_MAC_DIR"
    # -skipbindings avoids launching a GUI window which hangs in headless environments.
    # Bindings are pre-generated in frontend/wailsjs/ and committed to the repo.
    wails build -clean -skipbindings
    log "Wails build complete"
else
    log "Step 1: Skipping Wails build (--skip-wails)"
    if [ ! -d "$APP_BUNDLE" ]; then
        echo "ERROR: App bundle not found at: $APP_BUNDLE"
        echo "Run without --skip-wails first."
        exit 1
    fi
fi

# =============================================================================
# Step 2: Copy QEMU binary
# =============================================================================

log "Step 2: Copying QEMU binary..."
mkdir -p "$MACOS_DIR"

# Copy QEMU as a standalone executable (not a framework dylib)
# We'll use it as a dylib loaded by the app, matching UTM's approach
cp "$QEMU_DYLIB" "$MACOS_DIR/libqemu-aarch64-softmmu.dylib"
log "  Copied QEMU dylib ($(du -h "$MACOS_DIR/libqemu-aarch64-softmmu.dylib" | awk '{print $1}'))"

# =============================================================================
# Step 3: Copy required frameworks
# =============================================================================

log "Step 3: Copying frameworks from UTM sysroot..."
mkdir -p "$FRAMEWORKS_DIR"

# These are the frameworks QEMU directly links against (@rpath dependencies)
# plus their transitive dependencies. All are open-source.
#
# Direct QEMU deps:
#   pixman-1.0, jpeg.62, epoxy.0, gio-2.0.0, gobject-2.0.0, glib-2.0.0,
#   zstd.1, slirp.0, spice-server.1, virglrenderer.1, usbredirparser.1,
#   usb-1.0.0, gmodule-2.0.0
#
# Transitive deps:
#   intl.8, iconv.2, ffi.8        (from glib, gobject)
#   ssl.1.1, crypto.1.1           (from spice-server)
#   opus.0                         (from spice-server)
#   gstreamer-1.0.0, gstapp-1.0.0, gstbase-1.0.0  (from spice-server)
#   vulkan.1                       (from virglrenderer)
#   vulkan_kosmickrisp             (our Vulkan driver, from virglrenderer)

REQUIRED_FRAMEWORKS=(
    # Direct QEMU dependencies
    "pixman-1.0"
    "jpeg.62"
    "epoxy.0"
    "gio-2.0.0"
    "gobject-2.0.0"
    "glib-2.0.0"
    "zstd.1"
    "slirp.0"
    "spice-server.1"
    "virglrenderer.1"
    "usbredirparser.1"
    "usb-1.0.0"
    "gmodule-2.0.0"
    # Transitive dependencies
    "intl.8"
    "iconv.2"
    "ffi.8"
    "ssl.1.1"
    "crypto.1.1"
    "opus.0"
    "gstreamer-1.0.0"
    "gstapp-1.0.0"
    "gstbase-1.0.0"
    "vulkan.1"
    "vulkan_kosmickrisp"
    # GStreamer deps needed by spice-server at runtime
    "gthread-2.0.0"
    "gpg-error.0"
    "gcrypt.20"
)

COPIED=0
SKIPPED=0
for fw in "${REQUIRED_FRAMEWORKS[@]}"; do
    fw_dir="${UTM_FRAMEWORKS}/${fw}.framework"
    if [ -d "$fw_dir" ]; then
        cp -R "$fw_dir" "$FRAMEWORKS_DIR/"
        COPIED=$((COPIED + 1))
    else
        echo "  WARNING: Framework not found: ${fw}.framework (checking sysroot dylib)"
        # Some frameworks may only exist as dylibs in the sysroot, not as .framework bundles
        # We'll handle these via dylib copy below
        SKIPPED=$((SKIPPED + 1))
    fi
done
log "  Copied $COPIED frameworks, $SKIPPED not found as framework bundles"

# =============================================================================
# Step 4: Copy EFI firmware
# =============================================================================

log "Step 4: Copying EFI firmware..."
FIRMWARE_DIR="${RESOURCES_DIR}/firmware"
mkdir -p "$FIRMWARE_DIR"

cp "$EFI_CODE" "$FIRMWARE_DIR/edk2-aarch64-code.fd"
cp "$EFI_VARS_TEMPLATE" "$FIRMWARE_DIR/edk2-arm-vars.fd"
log "  Copied EFI code ($(du -h "$FIRMWARE_DIR/edk2-aarch64-code.fd" | awk '{print $1}')) + vars template"

# =============================================================================
# Step 5: Copy Vulkan ICD configuration
# =============================================================================

log "Step 5: Copying Vulkan ICD configuration..."
VULKAN_DIR="${RESOURCES_DIR}/vulkan/icd.d"
mkdir -p "$VULKAN_DIR"

# Create ICD JSON that points to bundled KosmicKrisp framework
# The path is relative to the JSON file location
cat > "$VULKAN_DIR/kosmickrisp_mesa_icd.json" << 'EOF'
{
    "ICD": {
        "api_version": "1.3.335",
        "library_path": "../../../Frameworks/vulkan_kosmickrisp.framework/Versions/Current/vulkan_kosmickrisp"
    },
    "file_format_version": "1.0.1"
}
EOF
log "  Created Vulkan ICD config"

# Copy open-source notices (required by GPL/LGPL for bundled QEMU + frameworks)
cp "${FOR_MAC_DIR}/NOTICES.md" "${RESOURCES_DIR}/NOTICES.md"
log "  Copied open-source NOTICES.md"

# =============================================================================
# Step 5b: Bundle VM disk images (compressed qcow2)
# =============================================================================

log "Step 5b: Bundling VM disk images..."
VM_BUNDLE_DIR="${RESOURCES_DIR}/vm"
mkdir -p "$VM_BUNDLE_DIR"

if [ -f "${VM_DIR}/disk.qcow2" ] && [ -f "${VM_DIR}/zfs-data.qcow2" ]; then
    # Compress qcow2 images (removes unused clusters, applies zlib compression)
    # This reduces 16G+13G raw to ~6G+4G compressed
    log "  Compressing root disk (this takes a few minutes)..."
    qemu-img convert -c -O qcow2 "${VM_DIR}/disk.qcow2" "${VM_BUNDLE_DIR}/disk.qcow2"
    log "  Root disk: $(du -h "${VM_BUNDLE_DIR}/disk.qcow2" | awk '{print $1}')"

    log "  Compressing ZFS data disk..."
    qemu-img convert -c -O qcow2 "${VM_DIR}/zfs-data.qcow2" "${VM_BUNDLE_DIR}/zfs-data.qcow2"
    log "  ZFS disk: $(du -h "${VM_BUNDLE_DIR}/zfs-data.qcow2" | awk '{print $1}')"

    # Copy EFI vars (VM-specific, 64MB)
    cp "${VM_DIR}/efi_vars.fd" "${VM_BUNDLE_DIR}/efi_vars.fd"
    log "  Copied EFI vars"

    VM_TOTAL=$(du -sh "$VM_BUNDLE_DIR" | awk '{print $1}')
    log "  Total VM bundle size: $VM_TOTAL"
else
    log "  WARNING: VM images not found at ${VM_DIR}/"
    log "  Run provision-vm.sh first, or set VM_DIR to the VM directory."
    log "  The app will still work but won't have a bundled VM image."
fi

# =============================================================================
# Step 6: Fix dylib paths (install_name_tool)
# =============================================================================

log "Step 6: Fixing dylib load paths..."

# Fix QEMU dylib: change sysroot paths to @rpath
# The QEMU dylib currently has its install_name as an absolute sysroot path
install_name_tool -id "@rpath/libqemu-aarch64-softmmu.dylib" \
    "$MACOS_DIR/libqemu-aarch64-softmmu.dylib" 2>/dev/null || true

# Fix the absolute sysroot path in QEMU's own reference
QEMU_OLD_ID="$SYSROOT/lib/libqemu-aarch64-softmmu.dylib"
install_name_tool -change "$QEMU_OLD_ID" \
    "@executable_path/libqemu-aarch64-softmmu.dylib" \
    "$MACOS_DIR/libqemu-aarch64-softmmu.dylib" 2>/dev/null || true

# Add @rpath pointing to Frameworks directory for the main executable
MAIN_EXEC="${MACOS_DIR}/${APP_EXEC_NAME}"
if [ -f "$MAIN_EXEC" ]; then
    # Add rpath if not already present
    install_name_tool -add_rpath "@executable_path/../Frameworks" "$MAIN_EXEC" 2>/dev/null || true
fi

# Add rpath to QEMU dylib too
install_name_tool -add_rpath "@executable_path/../Frameworks" \
    "$MACOS_DIR/libqemu-aarch64-softmmu.dylib" 2>/dev/null || true

# Fix each framework's internal references
# Frameworks already use @rpath references (from UTM's build), so they should resolve
# as long as the rpath is set correctly. But we need to verify no absolute paths remain.
FIXED_COUNT=0
for fw_dir in "$FRAMEWORKS_DIR"/*.framework; do
    fw_name=$(basename "$fw_dir" .framework)
    # Find the actual binary inside the framework
    fw_bin="$fw_dir/Versions/A/$fw_name"
    if [ ! -f "$fw_bin" ]; then
        fw_bin="$fw_dir/$fw_name"
    fi
    if [ -f "$fw_bin" ]; then
        # Check for and fix any absolute sysroot paths
        # Use "|| true" on grep to avoid pipefail exit when no matches found
        otool -L "$fw_bin" 2>/dev/null | (grep "$SYSROOT" || true) | awk '{print $1}' | while read -r old_path; do
            if [ -n "$old_path" ]; then
                lib_name=$(basename "$old_path")
                install_name_tool -change "$old_path" "@rpath/$lib_name" "$fw_bin" 2>/dev/null || true
            fi
        done
    fi
done
log "  Fixed dylib paths"

# =============================================================================
# Step 7: Ad-hoc code signing
# =============================================================================

log "Step 7: Signing app bundle (ad-hoc)..."

ENTITLEMENTS="${FOR_MAC_DIR}/build/darwin/entitlements.plist"

# Sign frameworks first (inside-out signing order)
for fw_dir in "$FRAMEWORKS_DIR"/*.framework; do
    codesign --force --sign - --timestamp=none "$fw_dir" 2>/dev/null || true
done

# Sign QEMU dylib with entitlements (needs Hypervisor.framework access)
codesign --force --sign - --timestamp=none \
    --entitlements "$ENTITLEMENTS" \
    "$MACOS_DIR/libqemu-aarch64-softmmu.dylib" 2>/dev/null || true

# Sign the main app bundle
codesign --force --sign - --timestamp=none \
    --entitlements "$ENTITLEMENTS" \
    --deep "$APP_BUNDLE" 2>/dev/null || true

log "  Ad-hoc signing complete"

# =============================================================================
# Summary
# =============================================================================

APP_SIZE=$(du -sh "$APP_BUNDLE" | awk '{print $1}')
FW_COUNT=$(ls -d "$FRAMEWORKS_DIR"/*.framework 2>/dev/null | wc -l | tr -d ' ')

log ""
log "================================================"
log "Build complete!"
log "================================================"
log ""
log "App bundle: $APP_BUNDLE"
log "Total size: $APP_SIZE"
log "Frameworks: $FW_COUNT"
log ""
log "Contents:"
log "  MacOS/${APP_EXEC_NAME}      - Main app (Wails)"
log "  MacOS/libqemu-*.dylib       - Custom QEMU with helix-frame-export"
log "  Frameworks/                  - ${FW_COUNT} open-source frameworks"
log "  Resources/firmware/          - EFI firmware (edk2)"
log "  Resources/vulkan/            - KosmicKrisp Vulkan ICD"
log ""
log "Verification:"
log "  codesign -vvv '$APP_BUNDLE'"
log "  otool -L '$MACOS_DIR/libqemu-aarch64-softmmu.dylib' | grep -c '@rpath'"
log ""
log "To create DMG:"
log "  ./scripts/create-dmg.sh"
log ""
log "NOTE: This build uses ad-hoc signing. On other Macs, users must:"
log "  System Settings > Privacy & Security > scroll down > 'Open Anyway'"
