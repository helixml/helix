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

mkdir -p "$BUILD_DIR"
cd "$QEMU_SRC"

echo "📝 Generating meson cross-compilation files..."

# Generate config-meson.cross (based on UTM's configuration)
cat > "$BUILD_DIR/config-meson.cross" << EOF
# Automatically generated - do not modify
[properties]
[built-in options]
c_args = ['-arch','arm64','-isysroot','/Applications/Xcode.app/Contents/Developer/Platforms/MacOSX.platform/Developer/SDKs/MacOSX.sdk','-I$SYSROOT/include','-F$SYSROOT/Frameworks']
cpp_args = ['-arch','arm64','-isysroot','/Applications/Xcode.app/Contents/Developer/Platforms/MacOSX.platform/Developer/SDKs/MacOSX.sdk','-I$SYSROOT/include','-F$SYSROOT/Frameworks','-target','arm64-apple-macos11.0']
objc_args = ['-arch','arm64','-isysroot','/Applications/Xcode.app/Contents/Developer/Platforms/MacOSX.platform/Developer/SDKs/MacOSX.sdk','-I$SYSROOT/include','-F$SYSROOT/Frameworks','-target','arm64-apple-macos11.0']
c_link_args = ['-arch','arm64','-isysroot','/Applications/Xcode.app/Contents/Developer/Platforms/MacOSX.platform/Developer/SDKs/MacOSX.sdk','-L$SYSROOT/lib','-F$SYSROOT/Frameworks','-target','arm64-apple-macos11.0']
cpp_link_args = ['-arch','arm64','-isysroot','/Applications/Xcode.app/Contents/Developer/Platforms/MacOSX.platform/Developer/SDKs/MacOSX.sdk','-L$SYSROOT/lib','-F$SYSROOT/Frameworks','-target','arm64-apple-macos11.0']
objc_link_args = ['-arch','arm64','-isysroot','/Applications/Xcode.app/Contents/Developer/Platforms/MacOSX.platform/Developer/SDKs/MacOSX.sdk','-L$SYSROOT/lib','-F$SYSROOT/Frameworks','-target','arm64-apple-macos11.0']

[binaries]
c = ['/Applications/Xcode.app/Contents/Developer/usr/bin/gcc','-target','arm64-apple-macos11.0']
cpp = ['/Applications/Xcode.app/Contents/Developer/usr/bin/g++','-target','arm64-apple-macos11.0']
objc = ['clang','-target','arm64-apple-macos11.0']
ar = ['/Applications/Xcode.app/Contents/Developer/Toolchains/XcodeDefault.xctoolchain/usr/bin/ar']
nm = ['/Applications/Xcode.app/Contents/Developer/Toolchains/XcodeDefault.xctoolchain/usr/bin/nm']
pkgconfig = ['$SYSROOT/host/bin/pkg-config']
pkg-config = ['$SYSROOT/host/bin/pkg-config']
ranlib = ['/Applications/Xcode.app/Contents/Developer/Toolchains/XcodeDefault.xctoolchain/usr/bin/ranlib']
strip = ['/Applications/Xcode.app/Contents/Developer/Toolchains/XcodeDefault.xctoolchain/usr/bin/strip']

[host_machine]
system = 'darwin'
cpu_family = 'aarch64'
cpu = 'aarch64'
endian = 'little'
EOF

# Generate config-meson.native
cat > "$BUILD_DIR/config-meson.native" << EOF
# Automatically generated - do not modify
[binaries]
c = ['cc']
EOF

echo "✅ Cross-compilation files generated"
echo ""

# Configure with meson (same options UTM uses)
echo "⚙️  Configuring QEMU with meson..."
echo "   This will auto-detect SPICE, virglrenderer, and other features"
echo ""

/opt/homebrew/bin/meson setup "$BUILD_DIR" \
    --prefix="$SYSROOT" \
    -Dshared_lib=true \
    -Dcocoa=disabled \
    -Db_pie=false \
    -Ddocs=disabled \
    -Dplugins=true \
    --cross-file="$BUILD_DIR/config-meson.cross" \
    --native-file="$BUILD_DIR/config-meson.native"

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
