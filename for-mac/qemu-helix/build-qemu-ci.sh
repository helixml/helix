#!/bin/bash
# Build QEMU for the release app from the helixml/qemu-utm commit pinned in
# QEMU_UTM_COMMIT. Builds are cached by commit + build-script hash under
# $QEMU_CACHE, and installed outside the shared UTM sysroot.
#
# Usage:
#   build-qemu-ci.sh            # build (or reuse) and verify
#   build-qemu-ci.sh --prefix   # print the install prefix (for QEMU_PREFIX)
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
COMMIT="$(tr -d '[:space:]' < "$HERE/QEMU_UTM_COMMIT")"
KEY="${COMMIT}-$(cat "$HERE/build-qemu-standalone.sh" "$HERE/UTM_VERSION" | shasum -a 256 | cut -c1-12)"
CACHE="${QEMU_CACHE:-/Volumes/Big/qemu-builds}"
DIR="$CACHE/$KEY"
PREFIX="$DIR/prefix"

if [ "${1:-}" = "--prefix" ]; then
    echo "$PREFIX"
    exit 0
fi

# QEMU must compile against the sysroot of the UTM release we follow
"$HERE/build-utm-sysroot.sh" --check

if [ -f "$DIR/.complete" ]; then
    echo "Reusing cached QEMU build: $PREFIX"
else
    echo "Building QEMU from helixml/qemu-utm@$COMMIT into $PREFIX"
    rm -rf "$DIR"
    mkdir -p "$DIR/src"
    git -C "$DIR/src" init -q
    git -C "$DIR/src" fetch -q --depth 1 https://github.com/helixml/qemu-utm "$COMMIT"
    git -C "$DIR/src" checkout -q FETCH_HEAD
    QEMU_SRC="$DIR/src" QEMU_PREFIX="$PREFIX" INSTALL_TO_UTM=false \
        bash "$HERE/build-qemu-standalone.sh"
    rm -rf "$DIR/src"
    touch "$DIR/.complete"
fi

# libslirp must be the vendored, statically linked copy (IPv6 DNS scope fix)
if otool -L "$PREFIX/lib/libqemu-aarch64-softmmu.dylib" | tail -n +2 | grep -qi slirp; then
    echo "ERROR: QEMU links an external libslirp"
    exit 1
fi
echo "QEMU ready: $PREFIX"
