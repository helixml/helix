#!/bin/bash
#
# Fix QEMU dylib library paths for UTM sandbox compatibility
#
# Problem: Custom QEMU builds have hardcoded sysroot paths like:
#   /Users/luke/pm/UTM/sysroot-macOS-arm64/lib/libpixman-1.0.dylib
#
# Solution: Rewrite to @rpath so UTM can find libs in its Frameworks directory
#

set -euo pipefail

QEMU_DYLIB="/Applications/UTM.app/Contents/Frameworks/qemu-aarch64-softmmu.framework/Versions/A/qemu-aarch64-softmmu"

if [ ! -f "$QEMU_DYLIB" ]; then
    echo "❌ QEMU dylib not found: $QEMU_DYLIB"
    exit 1
fi

echo "🔧 Fixing library paths in QEMU dylib..."

# Get all sysroot library paths
SYSROOT_LIBS=$(otool -L "$QEMU_DYLIB" | grep "sysroot" | awk '{print $1}' || true)

if [ -z "$SYSROOT_LIBS" ]; then
    echo "✅ No sysroot paths found - already fixed or not needed"
    exit 0
fi

echo "Found $(echo "$SYSROOT_LIBS" | wc -l | xargs) sysroot paths to fix:"
echo "$SYSROOT_LIBS" | sed 's/^/  /'
echo ""

# Fix each library path
for lib_path in $SYSROOT_LIBS; do
    lib_name=$(basename "$lib_path")
    echo "  Fixing: $lib_name"

    # Change absolute path to @rpath
    install_name_tool -change "$lib_path" "@rpath/$lib_name" "$QEMU_DYLIB"
done

# Update the dylib's own ID
echo ""
echo "Updating dylib ID..."
install_name_tool -id "@rpath/qemu-aarch64-softmmu" "$QEMU_DYLIB"

echo ""
echo "✅ Library paths fixed!"
echo ""
echo "Verification:"
otool -L "$QEMU_DYLIB" | head -10
