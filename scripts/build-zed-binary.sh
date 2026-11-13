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
# an atomic rename so a concurrent build never reads a partial copy.
mkdir -p "$cache"
for f in app-icon.png zed; do
  cp "zed-build/$f" "$cache/$f.$$"
  mv "$cache/$f.$$" "$cache/$f"
done
echo "Built and cached Zed binary for $sha"
