#!/bin/bash
# Print the path to the UTM.app of the release pinned in UTM_VERSION,
# downloading and caching it on first use. Helix bundles its frameworks and
# QEMURenderServer, which must match the utm-edition QEMU we build.
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
UTM_RELEASE="$(sed -n 's/^UTM_RELEASE=//p' "$HERE/UTM_VERSION")"
UTM_COMMIT="$(sed -n 's/^UTM_COMMIT=//p' "$HERE/UTM_VERSION")"
CACHE="${UTM_APP_CACHE:-$HOME/Library/Caches/helix/utm}/$UTM_RELEASE"
APP="$CACHE/UTM.app"

if [ ! -f "$CACHE/.complete" ]; then
    rm -rf "$CACHE"
    mkdir -p "$CACHE"
    echo "Downloading UTM $UTM_RELEASE..." >&2
    curl -fsSL -o "$CACHE/UTM.dmg" \
        "https://github.com/utmapp/UTM/releases/download/$UTM_RELEASE/UTM.dmg"
    MOUNT="$(mktemp -d)"
    hdiutil attach "$CACHE/UTM.dmg" -nobrowse -noautoopen -readonly -mountpoint "$MOUNT" >/dev/null
    trap 'hdiutil detach "$MOUNT" -force >/dev/null 2>&1 || true' EXIT
    ditto "$MOUNT/UTM.app" "$APP"
    rm -f "$CACHE/UTM.dmg"
    touch "$CACHE/.complete"
fi

VERSION="$(defaults read "$APP/Contents/Info.plist" CFBundleShortVersionString)"
if [ "v$VERSION" != "$UTM_RELEASE" ]; then
    echo "ERROR: $APP is UTM $VERSION, expected $UTM_RELEASE" >&2
    exit 1
fi
echo "$APP"
