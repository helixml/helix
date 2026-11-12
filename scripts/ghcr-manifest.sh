#!/bin/sh
# Publish a multi-arch manifest on ghcr.io/helixml.
# Usage: scripts/ghcr-manifest.sh <old-repo> <version>
# Example: scripts/ghcr-manifest.sh registry.helixml.tech/helix/controlplane v1.0
#
# Requires GITHUB_TOKEN environment variable for authentication.
# Skips silently if GITHUB_TOKEN is not set (allows gradual rollout).
#
# Resilience:
#   ghcr.io/helixml is the customer-facing registry (helm charts + install.sh
#   pull from here since PR #1901). A missing :VERSION manifest on GHCR
#   breaks `helm install` and install.sh with "no matching manifest", so any
#   failure here fails the build (release-rollback then tears down the
#   half-published release on tag builds).
#
#   Transient errors are retried 6x with linear backoff (30s..150s, up to
#   ~7.5min); `docker manifest create --amend` is idempotent. A rejected
#   token is not retried: it fails immediately.
set -e

if [ -z "$GITHUB_TOKEN" ]; then
  echo "GITHUB_TOKEN not set, skipping GHCR manifest"
  exit 0
fi

OLD_REPO="$1"
VERSION="$2"
GHCR_REPO=$(echo "$OLD_REPO" | sed 's|registry.helixml.tech/helix|ghcr.io/helixml|')

LOG_TAG=ghcr-manifest
. "$(dirname "$0")/lib/image-publish.sh"

ghcr_manifest_create() {
  docker manifest create --amend "$GHCR_REPO:$VERSION" \
    "$GHCR_REPO:$VERSION-linux-amd64" \
    "$GHCR_REPO:$VERSION-linux-arm64"
}

ghcr_manifest_push() {
  docker manifest push "$GHCR_REPO:$VERSION"
}

ghcr_login

if ! retry 6 30 ghcr_manifest_create; then
  echo "[ghcr-manifest] ERROR: docker manifest create failed after 6 attempts for $GHCR_REPO:$VERSION" >&2
  exit 1
fi

if ! retry 6 30 ghcr_manifest_push; then
  echo "[ghcr-manifest] ERROR: docker manifest push failed after 6 attempts for $GHCR_REPO:$VERSION" >&2
  exit 1
fi

echo "GHCR manifest pushed: $GHCR_REPO:$VERSION"
