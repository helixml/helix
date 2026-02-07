#!/bin/bash
set -e

# Build QEMU only, using pre-built dependencies in sysroot
# This is much faster than rebuilding all 28 dependencies from scratch
QEMU_SRC="$HOME/pm/UTM/build-macOS-arm64/qemu-10.0.2-utm"
SYSROOT="$HOME/pm/UTM/sysroot-macOS-arm64"
NCPU=$(sysctl -n hw.ncpu)

echo "🏗️  Building QEMU with helix-frame-export (fast mode - dependencies already built)"
echo ""

# Verify QEMU source exists with helix module
if [ ! -d "$QEMU_SRC/hw/display/helix" ]; then
    echo "❌ Helix module not found in QEMU source"
    echo "   Expected: $QEMU_SRC/hw/display/helix"
    exit 1
fi

# Verify sysroot exists with key dependencies
if [ ! -d "$SYSROOT" ]; then
    echo "❌ Sysroot not found at $SYSROOT"
    echo "   Run full build first to create dependencies"
    exit 1
fi

# Add Homebrew tools to PATH
export PATH="/opt/homebrew/opt/bison/bin:/opt/homebrew/bin:$PATH"
export PKG_CONFIG_PATH="$SYSROOT/lib/pkgconfig"

BUILD_DIR="$QEMU_SRC/build"

# Check if meson build directory exists (created by UTM's build_dependencies.sh)
if [ ! -d "$BUILD_DIR" ]; then
    echo "❌ Build directory not found: $BUILD_DIR"
    echo "   You need to run the full dependency build first:"
    echo "   cd ~/pm/UTM && ./Scripts/build_dependencies.sh -p macos -a arm64"
    echo ""
    echo "   This creates the meson configuration files needed to build QEMU"
    exit 1
fi

cd "$BUILD_DIR"

# Clean old build artifacts
echo "🧹 Cleaning old build..."
ninja clean

echo ""
echo "🔨 Building QEMU with ninja ($NCPU cores)..."
echo "   Using existing meson configuration from UTM's build"
echo "   This includes SPICE, virglrenderer, and all UTM patches"
echo ""

# Build with ninja (uses existing meson configuration)
ninja -j$NCPU || {
    echo "❌ Compilation failed"
    exit 1
}

echo ""
echo "📥 Installing QEMU to sysroot..."
ninja install || {
    echo "❌ Installation failed"
    exit 1
}

echo "✅ QEMU build complete!"
echo ""
echo "Next steps:"
echo "  1. ./stack patch-utm-app   # Patch production UTM.app with custom QEMU"
echo "  2. Start VM and test helix-frame-export functionality"
