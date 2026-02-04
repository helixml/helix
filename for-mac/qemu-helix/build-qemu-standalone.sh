#!/bin/bash
set -e

# Standalone QEMU build script for helix-frame-export
# This replicates UTM's QEMU build configuration without needing UTM checkout

echo "🏗️  Building QEMU with helix-frame-export (standalone mode)"
echo ""

# Configuration
QEMU_SRC="${QEMU_SRC:-$HOME/pm/qemu-utm}"
SYSROOT="${SYSROOT:-$HOME/pm/UTM/sysroot-macOS-arm64}"
BUILD_DIR="$QEMU_SRC/build"
NCPU=$(sysctl -n hw.ncpu)

# Verify prerequisites
if [ ! -d "$QEMU_SRC" ]; then
    echo "❌ QEMU source not found at: $QEMU_SRC"
    echo "   Clone with: git clone https://github.com/helixml/qemu-utm $QEMU_SRC"
    exit 1
fi

if [ ! -d "$SYSROOT" ]; then
    echo "❌ Sysroot not found at: $SYSROOT"
    echo "   You need to build dependencies first with UTM's build_dependencies.sh"
    echo "   This creates the sysroot with virglrenderer, SPICE, etc."
    exit 1
fi

if [ ! -f "$SYSROOT/host/bin/pkg-config" ]; then
    echo "❌ Custom pkg-config not found in sysroot"
    echo "   The sysroot needs UTM's custom pkg-config at: $SYSROOT/host/bin/pkg-config"
    exit 1
fi

echo "✅ QEMU source: $QEMU_SRC"
echo "✅ Sysroot: $SYSROOT"
echo "✅ Helix module: $([ -d "$QEMU_SRC/hw/display/helix" ] && echo "present" || echo "MISSING")"
echo ""

# Clean old build
if [ -d "$BUILD_DIR" ]; then
    echo "🧹 Cleaning old build..."
    rm -rf "$BUILD_DIR"
fi

cd "$QEMU_SRC"

# Step 1: Set environment to use sysroot's pkg-config
echo "📝 Setting up build environment..."
export PKG_CONFIG="$SYSROOT/host/bin/pkg-config"
export PKG_CONFIG_PATH="$SYSROOT/lib/pkgconfig"
export PATH="$SYSROOT/host/bin:$PATH"
echo "   PKG_CONFIG: $PKG_CONFIG"
echo "   PKG_CONFIG_PATH: $PKG_CONFIG_PATH"
echo ""

# Step 2: Run configure script
echo "🔧 Running QEMU configure script..."
echo "   This will auto-detect SPICE, virglrenderer via pkg-config"
echo ""

# Configure creates build dir and calls meson internally
# It will use our PKG_CONFIG to find sysroot libraries
./configure \
    --prefix="$SYSROOT" \
    --target-list=aarch64-softmmu \
    -Dshared_lib=true \
    -Dcocoa=disabled \
    -Db_pie=false \
    -Ddocs=disabled \
    -Dplugins=true

if [ $? -ne 0 ]; then
    echo "❌ Configure failed"
    exit 1
fi

echo "✅ configure completed"
echo ""

# Check if SPICE was detected
if grep -q "spice protocol support.*YES" "$BUILD_DIR/meson-logs/meson-log.txt" 2>/dev/null; then
    echo ""
    echo "✅ SPICE support: ENABLED"
else
    echo ""
    echo "⚠️  SPICE support: DISABLED (this will cause -spice: invalid option error)"
    echo "   Check that spice-protocol and spice-server are in $SYSROOT/lib/pkgconfig/"
    exit 1
fi

# Build with ninja
echo ""
echo "🔨 Building QEMU ($NCPU cores)..."
echo "   This takes ~5-10 minutes (2974 compilation steps)"
echo ""

cd "$BUILD_DIR"
ninja -j$NCPU

# Install to sysroot
echo ""
echo "📥 Installing to sysroot..."
ninja install

echo ""
echo "✅ QEMU build complete!"
echo ""
echo "Output:"
echo "  • QEMU dylib: $SYSROOT/lib/libqemu-aarch64-softmmu.dylib"
echo ""
echo "Next steps:"
echo "  1. Install into UTM.app:"
echo "     sudo cp $SYSROOT/lib/libqemu-aarch64-softmmu.dylib \\"
echo "          /Applications/UTM.app/Contents/Frameworks/qemu-aarch64-softmmu.framework/Versions/A/qemu-aarch64-softmmu"
echo ""
echo "  2. Fix library paths:"
echo "     ~/pm/helix/scripts/fix-qemu-paths.sh"
echo ""
echo "  3. Start VM:"
echo "     /Applications/UTM.app/Contents/MacOS/utmctl start <UUID>"
