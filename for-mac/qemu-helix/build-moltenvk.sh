#!/bin/bash
# Build the MoltenVK of the UTM release pinned in UTM_VERSION, with the fixes in
# patches/moltenvk and patches/spirv-cross applied, and print the path to the
# resulting MoltenVK.framework. Cached per MoltenVK commit + patch set; a
# build takes ~10 minutes.
#
# UTM.app's MoltenVK can't compile the shaders of wgpu apps (Zed, and wgpu's
# own validation shaders) to Metal, which kills the guest's Venus context. See
# README-QEMU-BUILD.md "Patches on top of UTM". Drop the patches (and this
# script) once a UTM release ships the fixes.
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
UTM_COMMIT="$(sed -n 's/^UTM_COMMIT=//p' "$HERE/UTM_VERSION")"
UTM_DIR="${UTM_DIR:-$HOME/pm/UTM}"
PATCHES="$HERE/patches/spirv-cross"
MVK_PATCHES="$HERE/patches/moltenvk"

if [ ! -d "$UTM_DIR/.git" ]; then
    git clone -q https://github.com/utmapp/UTM "$UTM_DIR" >&2
fi
git -C "$UTM_DIR" cat-file -e "$UTM_COMMIT" 2>/dev/null || git -C "$UTM_DIR" fetch -q origin "$UTM_COMMIT" >&2
SOURCES="$(git -C "$UTM_DIR" show "$UTM_COMMIT:patches/sources")"
MOLTENVK_REPO="$(sed -n 's/^MOLTENVK_REPO="\(.*\)"$/\1/p' <<<"$SOURCES")"
MOLTENVK_COMMIT="$(sed -n 's/^MOLTENVK_COMMIT="\(.*\)"$/\1/p' <<<"$SOURCES")"
[ -n "$MOLTENVK_REPO" ] && [ -n "$MOLTENVK_COMMIT" ] || { echo "ERROR: no MoltenVK pin in UTM $UTM_COMMIT patches/sources" >&2; exit 1; }

# Metal private API: MoltenVK uses it for VK_EXT_provoking_vertex (last-vertex
# flat shading, which GL needs; without it Zink renders Chromium's vector icons
# wrong), logicOp and wide lines. It only bars App Store distribution, which
# Helix for Mac doesn't use (UTM leaves it off because UTM is on the App Store).
MAKE_ARGS="MVK_USE_METAL_PRIVATE_API=1"
PATCH_HASH="$({ cat "$MVK_PATCHES"/*.patch "$PATCHES"/*.patch; echo "$MAKE_ARGS"; } | shasum -a 256 | cut -c1-12)"
CACHE="${MOLTENVK_CACHE:-$HOME/Library/Caches/helix/moltenvk}/${MOLTENVK_COMMIT:0:12}-$PATCH_HASH"
FRAMEWORK="$CACHE/MoltenVK.framework"
if [ -f "$CACHE/.complete" ]; then
    echo "$FRAMEWORK"
    exit 0
fi

rm -rf "$CACHE"
mkdir -p "$CACHE"
SRC="$CACHE/src"
echo "Building MoltenVK ${MOLTENVK_COMMIT:0:12} ($MAKE_ARGS) with $(ls "$MVK_PATCHES" | wc -l | tr -d ' ') MoltenVK and $(ls "$PATCHES" | wc -l | tr -d ' ') SPIRV-Cross patches..." >&2
{
    git clone -q --filter=blob:none "$MOLTENVK_REPO" "$SRC"
    git -C "$SRC" checkout -q --detach "$MOLTENVK_COMMIT"
    git -C "$SRC" -c user.name=helix -c user.email=noreply@helix.ml am -q "$MVK_PATCHES"/*.patch
    cd "$SRC"
    # Fetch the external sources only, so SPIRV-Cross can be patched before it is built.
    env -i PATH="$PATH" HOME="$HOME" LANG=en_US.UTF-8 ./fetchDependencies --none
    git -C External/SPIRV-Cross -c user.name=helix -c user.email=noreply@helix.ml am -q "$PATCHES"/*.patch
    git -C External/SPIRV-Cross rev-parse HEAD > ExternalRevisions/SPIRV-Cross_repo_revision
    env -i PATH="$PATH" HOME="$HOME" LANG=en_US.UTF-8 ./fetchDependencies --macos
    env -i PATH="$PATH" HOME="$HOME" LANG=en_US.UTF-8 make macos $MAKE_ARGS
} > "$CACHE/build.log" 2>&1 || { echo "ERROR: MoltenVK build failed, see $CACHE/build.log" >&2; exit 1; }

cp -R "$SRC/Package/Release/MoltenVK/dynamic/MoltenVK.xcframework/macos-arm64_x86_64/MoltenVK.framework" "$FRAMEWORK"
rm -rf "$SRC"
touch "$CACHE/.complete"
echo "$FRAMEWORK"
