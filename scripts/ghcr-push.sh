#!/bin/sh
# Mirror images from old registry to ghcr.io/helixml (primary).
# Usage: scripts/ghcr-push.sh <image1> [image2] ...
# Example: scripts/ghcr-push.sh registry.helixml.tech/helix/controlplane:v1.0-linux-amd64
#
# Requires GITHUB_TOKEN environment variable for authentication.
# Skips silently if GITHUB_TOKEN is not set (allows gradual rollout).
#
# Adds org.opencontainers.image.source label so GHCR links the package
# to the helix repository automatically.
set -e

if [ -z "$GITHUB_TOKEN" ]; then
  echo "GITHUB_TOKEN not set, skipping GHCR push"
  exit 0
fi

echo "$GITHUB_TOKEN" | docker login ghcr.io -u helixml --password-stdin

for IMAGE in "$@"; do
  GHCR_IMAGE=$(echo "$IMAGE" | sed 's|registry.helixml.tech/helix|ghcr.io/helixml|')
  echo "Mirroring $IMAGE -> $GHCR_IMAGE"
  echo "FROM $IMAGE" | docker build --provenance=false \
    --label "org.opencontainers.image.source=https://github.com/helixml/helix" \
    -t "$GHCR_IMAGE" -
  docker push "$GHCR_IMAGE"
done
