#!/bin/bash
set -euo pipefail

# =============================================================================
# Release Rollback Script
# =============================================================================
# Rolls back a failed release by cleaning up partially-published artifacts.
# Idempotent — safe to run multiple times.
#
# Usage:
#   scripts/release-rollback.sh VERSION
#   scripts/release-rollback.sh 2.9.14
#
# In CI: VERSION is read from DRONE_TAG if not provided.
#
# Required environment variables:
#   GITHUB_TOKEN              - GitHub PAT for gh CLI (release + tag deletion)
#
# Optional environment variables (skip cleanup if not set):
#   GCS_SERVICE_ACCOUNT_KEY   - Base64-encoded GCS service account JSON
#   R2_ACCESS_KEY_ID          - Cloudflare R2 credentials
#   R2_SECRET_ACCESS_KEY      - Cloudflare R2 credentials
#   SLACK_WEBHOOK_URL         - Slack incoming webhook for notifications
# =============================================================================

VERSION="${1:-${DRONE_TAG:-}}"
if [ -z "$VERSION" ]; then
  echo "Usage: $0 VERSION"
  echo "Example: $0 2.9.14"
  exit 1
fi

# Strip leading 'v' for semver (helm charts use bare semver)
SEMVER="${VERSION#v}"

REPO="helixml/helix"
R2_ENDPOINT="https://f0150e619c6dc08f55aea6d2248b1c6c.r2.cloudflarestorage.com"
R2_BUCKET="helix-desktop"
GCS_BUCKET="gs://charts.helixml.tech"

log() {
  echo "[rollback] $*"
}

warn() {
  echo "[rollback] WARNING: $*" >&2
}

# --- 1. Delete GitHub Release ---
rollback_github_release() {
  if [ -z "${GITHUB_TOKEN:-}" ]; then
    warn "GITHUB_TOKEN not set, skipping GitHub release cleanup"
    return 0
  fi

  log "Deleting GitHub release for $VERSION..."
  if gh release view "$VERSION" --repo "$REPO" >/dev/null 2>&1; then
    gh release delete "$VERSION" --repo "$REPO" --yes
    log "GitHub release $VERSION deleted"
  else
    log "GitHub release $VERSION not found (already deleted or never created)"
  fi
}

# --- 2. Delete Remote Git Tag ---
rollback_git_tag() {
  if [ -z "${GITHUB_TOKEN:-}" ]; then
    warn "GITHUB_TOKEN not set, skipping git tag cleanup"
    return 0
  fi

  log "Deleting remote git tag $VERSION..."
  if gh api "repos/$REPO/git/refs/tags/$VERSION" >/dev/null 2>&1; then
    gh api --method DELETE "repos/$REPO/git/refs/tags/$VERSION"
    log "Remote git tag $VERSION deleted"
  else
    log "Remote git tag $VERSION not found (already deleted or never created)"
  fi
}

# --- 3. Delete Helm Chart Packages from GCS ---
rollback_helm_charts() {
  if [ -z "${GCS_SERVICE_ACCOUNT_KEY:-}" ]; then
    # Fall back to ambient credentials (e.g. developer's local gcloud auth)
    if ! gsutil ls "$GCS_BUCKET" >/dev/null 2>&1; then
      warn "No GCS credentials available, skipping helm chart cleanup"
      return 0
    fi
  else
    echo "$GCS_SERVICE_ACCOUNT_KEY" | base64 -d > /tmp/gcs-key.json
    gcloud auth activate-service-account --key-file=/tmp/gcs-key.json --quiet
    rm -f /tmp/gcs-key.json
  fi

  log "Cleaning up helm charts for version $SEMVER..."

  CHART_NAMES="helix-controlplane helix-runner helix-sandbox"
  CHARTS_REMOVED=0

  for chart in $CHART_NAMES; do
    TGZ="${chart}-${SEMVER}.tgz"
    if gsutil stat "${GCS_BUCKET}/${TGZ}" >/dev/null 2>&1; then
      gsutil rm "${GCS_BUCKET}/${TGZ}"
      log "Deleted helm chart: $TGZ"
      CHARTS_REMOVED=$((CHARTS_REMOVED + 1))
    else
      log "Helm chart $TGZ not found in bucket (skipping)"
    fi
  done

  if [ "$CHARTS_REMOVED" -gt 0 ]; then
    log "Regenerating helm index.yaml..."

    HELM_TEMP=$(mktemp -d)
    trap 'rm -rf "$HELM_TEMP"' RETURN

    gsutil -m rsync "$GCS_BUCKET" "$HELM_TEMP/" 2>/dev/null || true

    # Remove old index before regenerating
    rm -f "$HELM_TEMP/index.yaml"

    helm repo index --url "https://charts.helixml.tech" "$HELM_TEMP/"
    gsutil cp "$HELM_TEMP/index.yaml" "${GCS_BUCKET}/index.yaml"
    gsutil setmeta -h "Cache-Control:no-cache,no-store,max-age=0" "${GCS_BUCKET}/index.yaml"
    log "Helm index.yaml regenerated and uploaded"
  else
    log "No helm charts were removed, index.yaml unchanged"
  fi
}

# --- 4. Delete R2 Assets (DMG, VM disk) ---
rollback_r2_assets() {
  if [ -z "${R2_ACCESS_KEY_ID:-}" ] || [ -z "${R2_SECRET_ACCESS_KEY:-}" ]; then
    warn "R2 credentials not set, skipping R2 cleanup"
    return 0
  fi

  export AWS_ACCESS_KEY_ID="$R2_ACCESS_KEY_ID"
  export AWS_SECRET_ACCESS_KEY="$R2_SECRET_ACCESS_KEY"

  log "Deleting R2 assets for $VERSION..."

  # Delete DMG
  if aws s3 rm "s3://${R2_BUCKET}/desktop/${VERSION}/Helix-for-Mac.dmg" \
    --endpoint-url "$R2_ENDPOINT" 2>/dev/null; then
    log "Deleted DMG: desktop/${VERSION}/Helix-for-Mac.dmg"
  else
    log "DMG not found (skipping): desktop/${VERSION}/Helix-for-Mac.dmg"
  fi

  # Delete VM disk and manifest
  if aws s3 rm "s3://${R2_BUCKET}/vm/${VERSION}/" \
    --endpoint-url "$R2_ENDPOINT" \
    --recursive 2>/dev/null; then
    log "Deleted VM assets: vm/${VERSION}/"
  else
    log "VM assets not found (skipping): vm/${VERSION}/"
  fi
}

# --- 5. Revert latest.json ---
rollback_latest_json() {
  if [ -z "${R2_ACCESS_KEY_ID:-}" ] || [ -z "${R2_SECRET_ACCESS_KEY:-}" ]; then
    warn "R2 credentials not set, skipping latest.json revert"
    return 0
  fi
  if [ -z "${GITHUB_TOKEN:-}" ]; then
    warn "GITHUB_TOKEN not set, cannot determine previous release for latest.json"
    return 0
  fi

  export AWS_ACCESS_KEY_ID="$R2_ACCESS_KEY_ID"
  export AWS_SECRET_ACCESS_KEY="$R2_SECRET_ACCESS_KEY"

  log "Checking if latest.json points to $VERSION..."

  CURRENT_LATEST=$(aws s3 cp "s3://${R2_BUCKET}/desktop/latest.json" - \
    --endpoint-url "$R2_ENDPOINT" 2>/dev/null || echo "")

  if [ -z "$CURRENT_LATEST" ]; then
    log "latest.json not found on R2 (nothing to revert)"
    return 0
  fi

  CURRENT_VERSION=$(echo "$CURRENT_LATEST" | jq -r '.version // empty' 2>/dev/null || echo "")
  if [ "$CURRENT_VERSION" != "$VERSION" ]; then
    log "latest.json points to $CURRENT_VERSION (not $VERSION), no revert needed"
    return 0
  fi

  # Find the previous good release
  PREV_VERSION=$(gh release list --repo "$REPO" --limit 10 \
    --json tagName --jq '.[].tagName' | grep -v "^${VERSION}$" | head -1)

  if [ -z "$PREV_VERSION" ]; then
    warn "No previous release found to revert latest.json to"
    return 0
  fi

  log "Reverting latest.json to previous release: $PREV_VERSION"
  printf '{"version":"%s","dmg_url":"https://dl.helix.ml/desktop/%s/Helix-for-Mac.dmg","vm_url":"https://dl.helix.ml/vm/%s/disk.qcow2.zst"}' \
    "$PREV_VERSION" "$PREV_VERSION" "$PREV_VERSION" | \
    aws s3 cp - "s3://${R2_BUCKET}/desktop/latest.json" \
      --endpoint-url "$R2_ENDPOINT" \
      --content-type "application/json" \
      --no-progress
  log "latest.json reverted to $PREV_VERSION"
}

# --- 6. Send Slack Notification ---
send_slack_notification() {
  if [ -z "${SLACK_WEBHOOK_URL:-}" ]; then
    return 0
  fi

  DRONE_LINK=""
  if [ -n "${DRONE_BUILD_NUMBER:-}" ]; then
    DRONE_LINK=" (<https://drone.lukemarsden.net/helixml/helix/${DRONE_BUILD_NUMBER}|Build #${DRONE_BUILD_NUMBER}>)"
  fi

  curl -sf -X POST -H 'Content-type: application/json' \
    --data "{\"text\":\":rotating_light: *Release rollback completed for ${VERSION}*${DRONE_LINK}\"}" \
    "$SLACK_WEBHOOK_URL" || warn "Failed to send Slack notification"
}

# === Execute rollback ===
log "Starting release rollback for $VERSION"
log "============================================"

rollback_github_release || warn "GitHub release cleanup encountered errors"
rollback_git_tag        || warn "Git tag cleanup encountered errors"
rollback_helm_charts    || warn "Helm chart cleanup encountered errors"
rollback_r2_assets      || warn "R2 asset cleanup encountered errors"
rollback_latest_json    || warn "latest.json revert encountered errors"
send_slack_notification || warn "Slack notification encountered errors"

log "============================================"
log "Release rollback for $VERSION complete"
