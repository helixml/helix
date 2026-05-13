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
#   breaks `helm install` and install.sh with "no matching manifest", so we
#   cannot exit 0 on failure here.
#
#   Each ghcr.io call is retried 6x with linear backoff (30s, 60s, 90s,
#   120s, 150s = up to ~7.5min total) to ride out transient ghcr.io
#   timeouts and 5xx. `docker manifest create --amend` is idempotent so
#   retrying a half-completed push is safe.
#
#   If all retries exhaust, exit 1 to fail the build. release-rollback
#   then tears down half-published artifacts (correct behavior given the
#   release is genuinely incomplete on the customer registry).
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

# Six retries with 30s, 60s, 90s, 120s, 150s backoff (up to ~7.5min total).
# `docker manifest create --amend` is idempotent, so retrying after a partial
# push is safe.
if ! retry 6 30 ghcr_login; then
  echo "[ghcr-manifest] ERROR: ghcr.io login failed after 6 retries for $GHCR_REPO:$VERSION" >&2
  echo "[ghcr-manifest] ERROR: ghcr.io/helixml is the customer-facing registry; cannot continue" >&2
  exit 1
fi

if ! retry 6 30 ghcr_manifest_create; then
  echo "[ghcr-manifest] ERROR: docker manifest create failed after 6 retries for $GHCR_REPO:$VERSION" >&2
  echo "[ghcr-manifest] ERROR: ghcr.io/helixml is the customer-facing registry; cannot continue" >&2
  exit 1
fi

if ! retry 6 30 ghcr_manifest_push; then
  echo "[ghcr-manifest] ERROR: docker manifest push failed after 6 retries for $GHCR_REPO:$VERSION" >&2
  echo "[ghcr-manifest] ERROR: ghcr.io/helixml is the customer-facing registry; cannot continue" >&2
  exit 1
fi

echo "GHCR manifest pushed (amd64-only): $GHCR_REPO:$VERSION"
