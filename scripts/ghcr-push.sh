#!/bin/sh
# Mirror per-arch images onto ghcr.io/helixml (customer-facing registry).
# Usage: scripts/ghcr-push.sh <image1> [image2] ...
# Example: scripts/ghcr-push.sh registry.helixml.tech/helix/controlplane:v1.0-linux-amd64
#
# Requires GITHUB_TOKEN environment variable for authentication.
# Fails if it is not set.
#
# Adds org.opencontainers.image.source label so GHCR links the package
# to the helix repository automatically.
#
# Resilience:
#   ghcr.io/helixml is the customer-facing registry (helm charts +
#   install.sh pull from it since PR #1901). A missing per-arch tag on
#   GHCR breaks the subsequent multi-arch manifest publish for that
#   release, so any failure here fails the build.
#
#   Transient login/push errors are retried 6x with linear backoff (30s..150s,
#   up to ~7.5min). A rejected token is not retried: it fails immediately.
set -e

LOG_TAG=ghcr-push
. "$(dirname "$0")/lib/image-publish.sh"
require_images ghcr-push.sh "$@"

ghcr_login

for IMAGE in "$@"; do
  GHCR_IMAGE=$(echo "$IMAGE" | sed 's|registry.helixml.tech/helix|ghcr.io/helixml|')
  echo "Mirroring $IMAGE -> $GHCR_IMAGE"
  echo "FROM $IMAGE" | docker build --provenance=false \
    --label "org.opencontainers.image.source=https://github.com/helixml/helix" \
    -t "$GHCR_IMAGE" -
  if ! retry 6 30 docker push "$GHCR_IMAGE"; then
    echo "[ghcr-push] ERROR: docker push failed after 6 attempts for $GHCR_IMAGE" >&2
    exit 1
  fi
done
