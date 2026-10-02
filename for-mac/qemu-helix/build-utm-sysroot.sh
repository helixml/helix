#!/bin/bash
# Build the UTM sysroot (virglrenderer, SPICE, glib, ...) that QEMU compiles
# against, from the utmapp/UTM commit pinned in UTM_VERSION. A no-op when the
# sysroot was already built from that commit; takes ~1-1.5h otherwise.
#
# Usage:
#   build-utm-sysroot.sh           # build if needed
#   build-utm-sysroot.sh --check   # exit 1 unless the sysroot matches the pin
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
UTM_RELEASE="$(sed -n 's/^UTM_RELEASE=//p' "$HERE/UTM_VERSION")"
UTM_COMMIT="$(sed -n 's/^UTM_COMMIT=//p' "$HERE/UTM_VERSION")"
UTM_DIR="${UTM_DIR:-$HOME/pm/UTM}"
SYSROOT="$UTM_DIR/sysroot-macOS-arm64"  # where UTM's build_dependencies.sh puts it
STAMP="$SYSROOT/.helix-utm-commit"

if [ "$(cat "$STAMP" 2>/dev/null)" = "$UTM_COMMIT" ]; then
    echo "UTM sysroot matches $UTM_RELEASE ($UTM_COMMIT): $SYSROOT"
    exit 0
fi
if [ "${1:-}" = "--check" ]; then
    echo "ERROR: $SYSROOT was not built from UTM $UTM_COMMIT ($UTM_RELEASE)." >&2
    echo "       Run for-mac/qemu-helix/build-utm-sysroot.sh" >&2
    exit 1
fi

if [ ! -d "$UTM_DIR/.git" ]; then
    git clone -q https://github.com/utmapp/UTM "$UTM_DIR"
fi
if [ -n "$(git -C "$UTM_DIR" status --porcelain --untracked-files=no)" ]; then
    echo "ERROR: $UTM_DIR has local changes; refusing to build from a modified UTM." >&2
    exit 1
fi
git -C "$UTM_DIR" fetch -q origin "$UTM_COMMIT"
git -C "$UTM_DIR" checkout -q --detach "$UTM_COMMIT"

# Build requirements, as UTM's .github/workflows/build.yml installs them.
# (UTM's check_env tests `brew --prefix`, which succeeds even when a formula
# is not installed, so a missing llvm only fails an hour in, in mesa.)
brew install --quiet bison pkg-config gettext glib-utils libgpg-error nasm make meson cmake \
    llvm spirv-llvm-translator libxcb libxrandr
pip3 install --quiet --break-system-packages --user six pyparsing pyyaml setuptools distlib mako

echo "Building UTM sysroot from $UTM_COMMIT ($UTM_RELEASE) into $SYSROOT"
(cd "$UTM_DIR" && PATH="$(brew --prefix bison)/bin:$PATH" \
    scripts/build_dependencies.sh -p macos -a arm64)
echo "$UTM_COMMIT" > "$STAMP"
