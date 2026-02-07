#!/bin/bash
set -e

# Fix library paths in custom QEMU binary for UTM.app
# This changes absolute paths to @rpath so UTM can find the frameworks

QEMU="/Applications/UTM.app/Contents/Frameworks/qemu-aarch64-softmmu.framework/Versions/A/qemu-aarch64-softmmu"
SYSROOT="/Users/luke/pm/UTM/sysroot-macOS-arm64/lib"

if [ ! -f "$QEMU" ]; then
    echo "❌ QEMU binary not found at: $QEMU"
    exit 1
fi

echo "🔧 Fixing library paths in QEMU binary..."

# Fix ID
sudo install_name_tool -id @rpath/qemu-aarch64-softmmu.framework/Versions/A/qemu-aarch64-softmmu "$QEMU"

# Fix sysroot dependencies - map to UTM's framework structure
sudo install_name_tool -change "$SYSROOT/libqemu-aarch64-softmmu.dylib" @rpath/qemu-aarch64-softmmu.framework/Versions/A/qemu-aarch64-softmmu "$QEMU" 2>/dev/null || true
sudo install_name_tool -change "$SYSROOT/libpixman-1.0.dylib" @rpath/pixman-1.0.framework/Versions/A/pixman-1.0 "$QEMU" 2>/dev/null || true
sudo install_name_tool -change "$SYSROOT/libpng16.16.dylib" @rpath/png16.16.framework/Versions/A/png16.16 "$QEMU" 2>/dev/null || true
sudo install_name_tool -change "$SYSROOT/libjpeg.62.dylib" @rpath/jpeg.62.framework/Versions/A/jpeg.62 "$QEMU" 2>/dev/null || true
sudo install_name_tool -change "$SYSROOT/libepoxy.0.dylib" @rpath/epoxy.0.framework/Versions/A/epoxy.0 "$QEMU" 2>/dev/null || true
sudo install_name_tool -change "$SYSROOT/libgio-2.0.0.dylib" @rpath/gio-2.0.0.framework/Versions/A/gio-2.0.0 "$QEMU" 2>/dev/null || true
sudo install_name_tool -change "$SYSROOT/libgobject-2.0.0.dylib" @rpath/gobject-2.0.0.framework/Versions/A/gobject-2.0.0 "$QEMU" 2>/dev/null || true
sudo install_name_tool -change "$SYSROOT/libglib-2.0.0.dylib" @rpath/glib-2.0.0.framework/Versions/A/glib-2.0.0 "$QEMU" 2>/dev/null || true
sudo install_name_tool -change "$SYSROOT/libzstd.1.dylib" @rpath/zstd.1.framework/Versions/A/zstd.1 "$QEMU" 2>/dev/null || true
sudo install_name_tool -change "$SYSROOT/libslirp.0.dylib" @rpath/slirp.0.framework/Versions/A/slirp.0 "$QEMU" 2>/dev/null || true
sudo install_name_tool -change "$SYSROOT/libspice-server.1.dylib" @rpath/spice-server.1.framework/Versions/A/spice-server.1 "$QEMU" 2>/dev/null || true
sudo install_name_tool -change "$SYSROOT/libvirglrenderer.1.dylib" @rpath/virglrenderer.1.framework/Versions/A/virglrenderer.1 "$QEMU" 2>/dev/null || true
sudo install_name_tool -change "$SYSROOT/libusbredirparser.1.dylib" @rpath/usbredirparser.1.framework/Versions/A/usbredirparser.1 "$QEMU" 2>/dev/null || true
sudo install_name_tool -change "$SYSROOT/libusb-1.0.0.dylib" @rpath/usb-1.0.0.framework/Versions/A/usb-1.0.0 "$QEMU" 2>/dev/null || true
sudo install_name_tool -change "$SYSROOT/libgmodule-2.0.0.dylib" @rpath/gmodule-2.0.0.framework/Versions/A/gmodule-2.0.0 "$QEMU" 2>/dev/null || true

# NOTE: Don't change Homebrew dependencies (capstone, gnutls) - UTM doesn't bundle them
# They must remain pointing to /opt/homebrew/ for QEMU to find them

echo "✅ Fixed all library paths to use @rpath"
