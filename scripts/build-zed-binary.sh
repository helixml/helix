#!/bin/sh
# Build the release Zed binary from external/zed-source into zed-build/.
# Usage: scripts/build-zed-binary.sh <cache-dir>
# Binaries are cached per Zed commit in <cache-dir>, which concurrent builds
# share. Run from the repository root.
set -eu
src=external/zed-source
sha=$(git -C "$src" rev-parse HEAD)
cache="$1/zed-bin-$sha"
mkdir -p zed-build
if [ -f "$cache/zed" ]; then
  cp "$cache/app-icon.png" "$cache/zed" zed-build/
  echo "Using cached Zed binary for $sha"
  exit 0
fi
docker build --provenance=false \
  --build-arg BUILD_TYPE=release \
  --build-arg ZED_COMMIT_SHA="$sha" \
  --output type=local,dest=zed-build \
  -f Dockerfile.zed-build "$src"
cp "$src/crates/zed/resources/app-icon.png" zed-build/
# The zed file marks a cache hit, so it is published last, and each file via
# an atomic rename so a concurrent build never reads a partial copy. The temp
# name comes from mktemp, not $$: concurrent CI builds of the same commit run
# in separate containers where the shell often has the same PID, so a
# PID-named temp file collides and one build's rename fails.
mkdir -p "$cache"
for f in app-icon.png:644 zed:755; do
  mode=${f#*:}
  f=${f%:*}
  tmp=$(mktemp "$cache/$f.XXXXXX")
  cp "zed-build/$f" "$tmp"
  chmod "$mode" "$tmp"
  mv "$tmp" "$cache/$f"
done
echo "Built and cached Zed binary for $sha"
