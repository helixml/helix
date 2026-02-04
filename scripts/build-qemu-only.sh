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

cd "$QEMU_SRC"

# Clean old build artifacts
echo "🧹 Cleaning old build..."
make distclean 2>/dev/null || true

echo "📦 Configuring QEMU..."
# Use same flags as UTM's build_dependencies.sh
./configure \
    --prefix="$SYSROOT" \
    --host=aarch64-apple-darwin \
    --cross-prefix="" \
    --enable-shared-lib \
    --disable-cocoa \
    --cpu=aarch64 \
    --target-list=aarch64-softmmu \
    || {
        echo "❌ Configure failed"
        tail -50 config.log
        exit 1
    }

echo "🔨 Compiling QEMU with $NCPU cores..."
make -j$NCPU || {
    echo "❌ Compilation failed"
    exit 1
}

echo "📥 Installing QEMU to sysroot..."
make install || {
    echo "❌ Installation failed"
    exit 1
}

echo "✅ QEMU build complete!"
echo ""
echo "Next steps:"
echo "  1. ./stack patch-utm-app   # Patch production UTM.app with custom QEMU"
echo "  2. Start VM and test helix-frame-export functionality"
