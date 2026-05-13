#!/bin/sh
# Create and push a multi-arch manifest on ghcr.io/helixml (primary registry).
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
GHCR_REPO=$(echo "$OLD_REPO" | sed 's|registry.helixml.tech/helix|ghcr.io/helixml|')

echo "$GITHUB_TOKEN" | docker login ghcr.io -u helixml --password-stdin

# TEMP: arm64 leg removed while arm64 builds are disabled in .drone.yml.
# Restore the `"$GHCR_REPO:$VERSION-linux-arm64"` line below when arm64 is re-enabled.
docker manifest create --amend "$GHCR_REPO:$VERSION" \
  "$GHCR_REPO:$VERSION-linux-amd64"
docker manifest push "$GHCR_REPO:$VERSION"
echo "GHCR manifest pushed (amd64-only): $GHCR_REPO:$VERSION"
