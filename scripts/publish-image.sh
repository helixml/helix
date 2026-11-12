#!/bin/sh
# Push per-arch images to registry.helixml.tech, then mirror them to ghcr.io.
# Usage: scripts/publish-image.sh <image1> [image2] ...
# Caller must already be logged in to registry.helixml.tech.
#
# registry.helixml.tech is backed by Cloudflare R2; a single R2 5xx/HEAD error
# used to fail the whole build (and, on tags, roll back the release), so each
# push is retried. Retrying is safe: layers already uploaded are skipped.
set -e

LOG_TAG=publish-image
. "$(dirname "$0")/lib/image-publish.sh"

for IMAGE in "$@"; do
  if ! retry 4 20 docker push "$IMAGE"; then
    echo "[publish-image] ERROR: docker push failed after 4 attempts for $IMAGE" >&2
    exit 1
  fi
done

"$(dirname "$0")/ghcr-push.sh" "$@"
