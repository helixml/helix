#!/bin/bash
# Create and push a multi-arch manifest on ghcr.io/helixml.
# Usage: scripts/ghcr-manifest.sh <old-repo> <version>
# Example: scripts/ghcr-manifest.sh registry.helixml.tech/helix/controlplane v1.0
#
# Requires GITHUB_TOKEN environment variable for authentication.
# Skips silently if GITHUB_TOKEN is not set (allows gradual rollout).
set -e

if [ -z "$GITHUB_TOKEN" ]; then
  echo "GITHUB_TOKEN not set, skipping GHCR manifest"
  exit 0
fi

OLD_REPO="$1"
VERSION="$2"
GHCR_REPO="${OLD_REPO/registry.helixml.tech\/helix/ghcr.io\/helixml}"

echo "$GITHUB_TOKEN" | docker login ghcr.io -u helixml --password-stdin

docker manifest create --amend "$GHCR_REPO:$VERSION" \
  "$GHCR_REPO:$VERSION-linux-amd64" \
  "$GHCR_REPO:$VERSION-linux-arm64"
docker manifest push "$GHCR_REPO:$VERSION"
echo "GHCR multi-arch manifest pushed: $GHCR_REPO:$VERSION"
