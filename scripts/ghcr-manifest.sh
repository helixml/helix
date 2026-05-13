#!/bin/sh
# Mirror a multi-arch manifest from registry.helixml.tech onto ghcr.io/helixml.
# Usage: scripts/ghcr-manifest.sh <old-repo> <version>
# Example: scripts/ghcr-manifest.sh registry.helixml.tech/helix/controlplane v1.0
#
# Requires GITHUB_TOKEN environment variable for authentication.
# Skips silently if GITHUB_TOKEN is not set (allows gradual rollout).
#
# Resilience:
#   GHCR is a downstream mirror; registry.helixml.tech is the source of truth.
#   Each ghcr.io call is retried 3x with backoff. If all retries fail (e.g.
#   transient ghcr.io 5xx/timeout), the script logs loudly and exits 0 so a
#   single mirror blip does not fail the build and trigger release-rollback.
#   Re-running scripts/ghcr-manifest.sh against the same version safely
#   re-creates the manifest on the mirror.
set -e

if [ -z "$GITHUB_TOKEN" ]; then
  echo "GITHUB_TOKEN not set, skipping GHCR manifest"
  exit 0
fi

OLD_REPO="$1"
VERSION="$2"
GHCR_REPO=$(echo "$OLD_REPO" | sed 's|registry.helixml.tech/helix|ghcr.io/helixml|')

# retry <max-attempts> <sleep-secs-base> -- <cmd...>
# Runs the command, retries on failure with linear backoff (base, base*2, ...).
# Returns the command's final exit code.
retry() {
  max="$1"; base="$2"; shift 2
  attempt=1
  while :; do
    if "$@"; then
      return 0
    fi
    rc=$?
    if [ "$attempt" -ge "$max" ]; then
      return "$rc"
    fi
    delay=$((base * attempt))
    echo "[ghcr-manifest] attempt $attempt/$max failed (rc=$rc), retrying in ${delay}s: $*" >&2
    sleep "$delay"
    attempt=$((attempt + 1))
  done
}

ghcr_login() {
  echo "$GITHUB_TOKEN" | docker login ghcr.io -u helixml --password-stdin
}

ghcr_manifest_create() {
  # TEMP: arm64 leg removed while arm64 builds are disabled in .drone.yml.
  # Restore the `"$GHCR_REPO:$VERSION-linux-arm64"` line below when arm64 is re-enabled.
  docker manifest create --amend "$GHCR_REPO:$VERSION" \
    "$GHCR_REPO:$VERSION-linux-amd64"
}

ghcr_manifest_push() {
  docker manifest push "$GHCR_REPO:$VERSION"
}

# Three retries with 5s, 10s, 15s backoff. Each operation is independent;
# `docker manifest create --amend` is idempotent.
if ! retry 3 5 ghcr_login; then
  echo "[ghcr-manifest] WARNING: ghcr.io login failed after retries; skipping mirror for $GHCR_REPO:$VERSION (registry.helixml.tech is the source of truth)" >&2
  exit 0
fi

if ! retry 3 5 ghcr_manifest_create; then
  echo "[ghcr-manifest] WARNING: docker manifest create failed after retries; skipping mirror for $GHCR_REPO:$VERSION" >&2
  exit 0
fi

if ! retry 3 5 ghcr_manifest_push; then
  echo "[ghcr-manifest] WARNING: docker manifest push failed after retries; skipping mirror for $GHCR_REPO:$VERSION" >&2
  exit 0
fi

echo "GHCR manifest pushed (amd64-only): $GHCR_REPO:$VERSION"
