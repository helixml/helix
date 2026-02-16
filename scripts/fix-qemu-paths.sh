#!/bin/bash
#
# Fix QEMU dylib library paths for UTM compatibility
#
# Problem: Custom QEMU builds link against sysroot paths like:
#   /Users/luke/pm/UTM/sysroot-macOS-arm64/lib/libpixman-1.0.dylib
#
# Solution: Rewrite to match UTM's framework layout:
#   @rpath/pixman-1.0.framework/Versions/A/pixman-1.0
#
# UTM 5.0.1+ bundles libraries as macOS frameworks, not loose dylibs.
# The mapping is: libFOO.dylib -> FOO.framework/Versions/A/FOO
# where FOO strips the "lib" prefix.

set -euo pipefail

QEMU_DYLIB="/Applications/UTM.app/Contents/Frameworks/qemu-aarch64-softmmu.framework/Versions/A/qemu-aarch64-softmmu"

if [ ! -f "$QEMU_DYLIB" ]; then
    echo "❌ QEMU dylib not found: $QEMU_DYLIB"
    exit 1
fi

echo "🔧 Fixing library paths in QEMU dylib..."

# Get all sysroot library paths (from our custom build)
SYSROOT_LIBS=$(otool -L "$QEMU_DYLIB" | grep "sysroot" | awk '{print $1}' || true)
# Also catch any that were already partially fixed to @rpath/libFOO.dylib format
RPATH_DYLIBS=$(otool -L "$QEMU_DYLIB" | grep "@rpath/lib.*\.dylib" | awk '{print $1}' || true)

ALL_LIBS=$(echo -e "${SYSROOT_LIBS}\n${RPATH_DYLIBS}" | sort -u | grep -v '^$' || true)

if [ -z "$ALL_LIBS" ]; then
    echo "✅ No paths need fixing"
    exit 0
fi

echo "Found $(echo "$ALL_LIBS" | wc -l | xargs) paths to fix:"
echo "$ALL_LIBS" | sed 's/^/  /'
echo ""

UTM_FRAMEWORKS="/Applications/UTM.app/Contents/Frameworks"

# Fix each library path to match UTM's framework layout
for lib_path in $ALL_LIBS; do
    lib_name=$(basename "$lib_path")

    # Strip "lib" prefix and ".dylib" suffix to get framework name
    # libpixman-1.0.dylib -> pixman-1.0
    # libglib-2.0.0.dylib -> glib-2.0.0
    framework_name="${lib_name#lib}"      # strip "lib" prefix
    framework_name="${framework_name%.dylib}"  # strip ".dylib" suffix

    # Check if matching framework exists in UTM
    framework_path="$UTM_FRAMEWORKS/${framework_name}.framework/Versions/A/${framework_name}"
    if [ -f "$framework_path" ]; then
        new_path="@rpath/${framework_name}.framework/Versions/A/${framework_name}"
        echo "  $lib_name -> $new_path"
        install_name_tool -change "$lib_path" "$new_path" "$QEMU_DYLIB"
    else
        # Fallback: check if loose dylib exists (older UTM versions)
        if [ -f "$UTM_FRAMEWORKS/$lib_name" ]; then
            new_path="@rpath/$lib_name"
            echo "  $lib_name -> $new_path (loose dylib)"
            install_name_tool -change "$lib_path" "$new_path" "$QEMU_DYLIB"
        else
            echo "  ⚠️  $lib_name: no matching framework or dylib found in UTM!"
        fi
    fi
done

# Update the dylib's own ID to match UTM's convention
echo ""
echo "Updating dylib ID..."
install_name_tool -id "@rpath/qemu-aarch64-softmmu.framework/qemu-aarch64-softmmu" "$QEMU_DYLIB"

echo ""
echo "✅ Library paths fixed!"
echo ""
echo "Verification:"
otool -L "$QEMU_DYLIB" | head -25
