#!/bin/bash
# Build virgl_render_server from the virglrenderer of the UTM release pinned in
# UTM_VERSION, with the fixes in patches/virglrenderer applied, the way UTM's
# build_dependencies.sh builds it, and print the path to the binary. Cached per
# virglrenderer commit + patch set. Needs the UTM sysroot (build-utm-sysroot.sh).
#
# UTM.app's render server rejects shared buffers on macOS, where dma_buf is
# emulated with shm, which kills the Venus context of any guest that renders into
# them (Chromium on Zink). See README-QEMU-BUILD.md "Patches on top of UTM".
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
UTM_COMMIT="$(sed -n 's/^UTM_COMMIT=//p' "$HERE/UTM_VERSION")"
UTM_DIR="${UTM_DIR:-$HOME/pm/UTM}"
PATCHES="$HERE/patches/virglrenderer"
CROSS_FILE="$UTM_DIR/build-macOS-arm64/meson-darwin.cross"

"$HERE/build-utm-sysroot.sh" --check >&2
[ -f "$CROSS_FILE" ] || { echo "ERROR: $CROSS_FILE missing; run build-utm-sysroot.sh" >&2; exit 1; }

SOURCES="$(git -C "$UTM_DIR" show "$UTM_COMMIT:patches/sources")"
VIRGL_REPO="$(sed -n 's/^VIRGLRENDERER_REPO="\(.*\)"$/\1/p' <<<"$SOURCES")"
VIRGL_COMMIT="$(sed -n 's/^VIRGLRENDERER_COMMIT="\(.*\)"$/\1/p' <<<"$SOURCES")"
[ -n "$VIRGL_REPO" ] && [ -n "$VIRGL_COMMIT" ] || { echo "ERROR: no virglrenderer pin in UTM $UTM_COMMIT patches/sources" >&2; exit 1; }

PATCH_HASH="$(cat "$PATCHES"/*.patch | shasum -a 256 | cut -c1-12)"
CACHE="${RENDER_SERVER_CACHE:-$HOME/Library/Caches/helix/render-server}/${VIRGL_COMMIT:0:12}-$PATCH_HASH"
BIN="$CACHE/virgl_render_server"
if [ -f "$CACHE/.complete" ]; then
    echo "$BIN"
    exit 0
fi

rm -rf "$CACHE"
mkdir -p "$CACHE"
SRC="$CACHE/src"
echo "Building virgl_render_server ${VIRGL_COMMIT:0:12} with $(ls "$PATCHES" | wc -l | tr -d ' ') patches..." >&2
{
    git clone -q --filter=blob:none "$VIRGL_REPO" "$SRC"
    git -C "$SRC" checkout -q --detach "$VIRGL_COMMIT"
    git -C "$SRC" -c user.name=helix -c user.email=noreply@helix.ml am -q "$PATCHES"/*.patch
    # Options as in UTM's build_dependencies.sh for macOS.
    meson setup "$SRC/build" "$SRC" --buildtype=release --cross-file "$CROSS_FILE" \
        -Dtests=false -Dcheck-gl-errors=false -Dvenus=true -Dneptune=true \
        -Dvulkan-dload=false -Drender-server-mode=process
    meson compile -C "$SRC/build" server/virgl_render_server
} > "$CACHE/build.log" 2>&1 || { echo "ERROR: virgl_render_server build failed, see $CACHE/build.log" >&2; exit 1; }

cp "$SRC/build/server/virgl_render_server" "$BIN"
# Link the Vulkan loader through the bundled framework, like UTM's QEMURenderServer.
install_name_tool -change @rpath/libvulkan.1.dylib @rpath/vulkan.1.framework/Versions/A/vulkan.1 "$BIN"
if otool -L "$BIN" | tail -n +2 | grep -vE '^\s+(/usr/lib/|/System/|@rpath/vulkan\.1\.framework/)' | grep -q .; then
    echo "ERROR: $BIN links unexpected libraries:" >&2
    otool -L "$BIN" >&2
    exit 1
fi
rm -rf "$SRC"
touch "$CACHE/.complete"
echo "$BIN"
