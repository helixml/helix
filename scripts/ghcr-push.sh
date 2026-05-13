#!/bin/sh
# Mirror per-arch images onto ghcr.io/helixml (customer-facing registry).
# Usage: scripts/ghcr-push.sh <image1> [image2] ...
# Example: scripts/ghcr-push.sh registry.helixml.tech/helix/controlplane:v1.0-linux-amd64
#
# Requires GITHUB_TOKEN environment variable for authentication.
# Skips silently if GITHUB_TOKEN is not set (allows gradual rollout).
#
# Adds org.opencontainers.image.source label so GHCR links the package
# to the helix repository automatically.
#
# Resilience:
#   ghcr.io/helixml is the customer-facing registry (helm charts +
#   install.sh pull from it since PR #1901). A missing per-arch tag on
#   GHCR breaks the subsequent multi-arch manifest publish for that
#   release, so we cannot exit 0 on push failure.
#
#   docker login + docker push are retried 6x with linear backoff (30s,
#   60s, 90s, 120s, 150s = up to ~7.5min total). `docker push` is
#   idempotent for already-pushed layers. If all retries exhaust, exit 1
#   so the build fails (and release-rollback runs on tag builds).
set -e

if [ -z "$GITHUB_TOKEN" ]; then
  echo "GITHUB_TOKEN not set, skipping GHCR push"
  exit 0
fi

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
    echo "[ghcr-push] attempt $attempt/$max failed (rc=$rc), retrying in ${delay}s: $*" >&2
    sleep "$delay"
    attempt=$((attempt + 1))
  done
}

ghcr_login() {
  echo "$GITHUB_TOKEN" | docker login ghcr.io -u helixml --password-stdin
}

if ! retry 6 30 ghcr_login; then
  echo "[ghcr-push] ERROR: ghcr.io login failed after 6 retries" >&2
  echo "[ghcr-push] ERROR: ghcr.io/helixml is the customer-facing registry; cannot continue" >&2
  exit 1
fi

for IMAGE in "$@"; do
  GHCR_IMAGE=$(echo "$IMAGE" | sed 's|registry.helixml.tech/helix|ghcr.io/helixml|')
  echo "Mirroring $IMAGE -> $GHCR_IMAGE"
  echo "FROM $IMAGE" | docker build --provenance=false \
    --label "org.opencontainers.image.source=https://github.com/helixml/helix" \
    -t "$GHCR_IMAGE" -
  if ! retry 6 30 docker push "$GHCR_IMAGE"; then
    echo "[ghcr-push] ERROR: docker push failed after 6 retries for $GHCR_IMAGE" >&2
    echo "[ghcr-push] ERROR: ghcr.io/helixml is the customer-facing registry; cannot continue" >&2
    exit 1
  fi
done
