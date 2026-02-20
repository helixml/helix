#!/bin/bash
set -euo pipefail

# =============================================================================
# Upload VM disk images to Cloudflare R2 for CDN distribution
# =============================================================================
#
# Compresses disk.qcow2 with zstd, then uploads to R2 organized by
# version (git short hash). The app downloads these on first launch.
#
# Prerequisites:
#   - AWS CLI: brew install awscli
#   - qemu-img: brew install qemu (or bundled in app)
#   - R2 credentials in for-mac/.env.r2
#   - VM images built: ~/Library/Application Support/Helix/vm/helix-desktop/
#
# Usage:
#   cd for-mac && ./scripts/upload-vm-images.sh
#   VM_VERSION=v1.0 ./scripts/upload-vm-images.sh   # Override version tag
#   SKIP_COMPRESS=1 ./scripts/upload-vm-images.sh    # Upload without compressing

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
FOR_MAC_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
REPO_ROOT="$(cd "$FOR_MAC_DIR/.." && pwd)"

# Load R2 credentials (from .env.r2 file or environment variables)
R2_ENV="${FOR_MAC_DIR}/.env.r2"
if [ -f "$R2_ENV" ]; then
    # shellcheck disable=SC1090
    source "$R2_ENV"
fi

# Verify required vars are set (from .env.r2 or environment)
for var in R2_ACCESS_KEY_ID R2_SECRET_ACCESS_KEY; do
    if [ -z "${!var:-}" ]; then
        echo "ERROR: $var not set. Either create for-mac/.env.r2 or export it."
        exit 1
    fi
done

# Derive R2_ENDPOINT from R2_ACCOUNT_ID if not set directly
if [ -z "${R2_ENDPOINT:-}" ]; then
    if [ -z "${R2_ACCOUNT_ID:-}" ]; then
        echo "ERROR: Either R2_ENDPOINT or R2_ACCOUNT_ID must be set"
        exit 1
    fi
    R2_ENDPOINT="https://${R2_ACCOUNT_ID}.r2.cloudflarestorage.com"
fi
R2_BUCKET="${R2_BUCKET:-helix-desktop}"

export AWS_ACCESS_KEY_ID="$R2_ACCESS_KEY_ID"
export AWS_SECRET_ACCESS_KEY="$R2_SECRET_ACCESS_KEY"

# VM image directory
VM_DIR="${VM_DIR:-/Volumes/Big/helix-vm/helix-desktop}"

# Version tag
VM_VERSION="${VM_VERSION:-$(git -C "$REPO_ROOT" rev-parse --short HEAD 2>/dev/null || echo "dev")}"

SKIP_COMPRESS="${SKIP_COMPRESS:-0}"

log() { echo "[$(date +%H:%M:%S)] $*"; }

# =============================================================================
# Verify prerequisites
# =============================================================================

if ! command -v aws &>/dev/null; then
    echo "ERROR: AWS CLI not found. Install with: brew install awscli"
    exit 1
fi

if ! command -v qemu-img &>/dev/null; then
    echo "ERROR: qemu-img not found. Install with: brew install qemu"
    exit 1
fi

if [ ! -f "${VM_DIR}/disk.qcow2" ]; then
    echo "ERROR: Missing VM image: ${VM_DIR}/disk.qcow2"
    echo "Provision the VM first, then try again."
    exit 1
fi




# =============================================================================
# Step 1: Compress disk image with zstd
# =============================================================================

# Use /Volumes/Big for temp if available (compressed image can be 5+ GB)
if [ -d /Volumes/Big ]; then
    UPLOAD_DIR=$(mktemp -d /Volumes/Big/helix-upload.XXXXXX)
else
    UPLOAD_DIR=$(mktemp -d)
fi
trap "rm -rf '$UPLOAD_DIR'" EXIT

ORIG_SIZE=$(stat -f%z "${VM_DIR}/disk.qcow2" 2>/dev/null || stat -c%s "${VM_DIR}/disk.qcow2" 2>/dev/null)

if [ "$SKIP_COMPRESS" = "1" ]; then
    log "Skipping compression (SKIP_COMPRESS=1)"
    DISK_PATH="${VM_DIR}/disk.qcow2"
    USE_ZSTD=false
else
    if ! command -v zstd &>/dev/null; then
        log "ERROR: zstd not found. Install with: brew install zstd"
        exit 1
    fi

    # First ensure the qcow2 is uncompressed (for best zstd compression)
    log "Step 1a: Creating uncompressed qcow2 for optimal zstd compression..."
    UNCOMPRESSED="${UPLOAD_DIR}/disk.qcow2"
    qemu-img convert -O qcow2 "${VM_DIR}/disk.qcow2" "$UNCOMPRESSED"
    UNCOMPRESSED_SIZE=$(stat -f%z "$UNCOMPRESSED" 2>/dev/null || stat -c%s "$UNCOMPRESSED" 2>/dev/null)
    log "  Uncompressed qcow2: $(echo "$UNCOMPRESSED_SIZE" | awk '{printf "%.1f GB", $1/1073741824}')"

    log "Step 1b: Compressing with zstd -3 (multithreaded)..."
    DISK_PATH="${UPLOAD_DIR}/disk.qcow2.zst"
    zstd -T0 -3 "$UNCOMPRESSED" -o "$DISK_PATH" --force 2>&1
    COMP_SIZE=$(stat -f%z "$DISK_PATH" 2>/dev/null || stat -c%s "$DISK_PATH" 2>/dev/null)
    RATIO=$(echo "scale=1; 100 - $COMP_SIZE * 100 / $UNCOMPRESSED_SIZE" | bc)
    log "  Compressed: $(echo "$COMP_SIZE" | awk '{printf "%.1f GB", $1/1073741824}') (${RATIO}% smaller than uncompressed)"

    USE_ZSTD=true
fi

# =============================================================================
# Step 2: Upload files
# =============================================================================

log ""
log "=== Uploading VM images to R2 ==="
log "Version:  ${VM_VERSION}"
log "Bucket:   ${R2_BUCKET}"
log "CDN URL:  https://dl.helix.ml/vm/${VM_VERSION}/"
log ""

DISK_SIZE=$(stat -f%z "$DISK_PATH" 2>/dev/null || stat -c%s "$DISK_PATH" 2>/dev/null)

# Determine the upload filename
if [ "$USE_ZSTD" = true ]; then
    DISK_UPLOAD_NAME="disk.qcow2.zst"
else
    DISK_UPLOAD_NAME="disk.qcow2"
fi

log "  ${DISK_UPLOAD_NAME}:  $(echo "$DISK_SIZE" | awk '{printf "%.1f GB", $1/1073741824}')"
log ""

log "Uploading ${DISK_UPLOAD_NAME}..."
aws s3 cp "$DISK_PATH" "s3://${R2_BUCKET}/vm/${VM_VERSION}/${DISK_UPLOAD_NAME}" \
    --endpoint-url "$R2_ENDPOINT" \
    --no-progress 2>&1
log "  Done."

# =============================================================================
# Step 3: Verify
# =============================================================================

log ""
log "Verifying uploads..."
aws s3 ls "s3://${R2_BUCKET}/vm/${VM_VERSION}/" \
    --endpoint-url "$R2_ENDPOINT"

# Quick download test
HTTP_CODE=$(curl -sI -o /dev/null -w "%{http_code}" "https://dl.helix.ml/vm/${VM_VERSION}/${DISK_UPLOAD_NAME}")
if [ "$HTTP_CODE" = "200" ]; then
    log "CDN check: https://dl.helix.ml/vm/${VM_VERSION}/${DISK_UPLOAD_NAME} → 200 OK"
else
    log "WARNING: CDN check returned HTTP ${HTTP_CODE} (may need DNS propagation)"
fi

# =============================================================================
# Step 4: Generate vm-manifest.json for the app bundle
# =============================================================================

log ""
log "Generating vm-manifest.json..."

DISK_SHA256=$(shasum -a 256 "$DISK_PATH" | awk '{print $1}')

MANIFEST_PATH="${FOR_MAC_DIR}/vm-manifest.json"

if [ "$USE_ZSTD" = true ]; then
    DECOMPRESSED_SIZE=$(stat -f%z "$UNCOMPRESSED" 2>/dev/null || stat -c%s "$UNCOMPRESSED" 2>/dev/null)
    cat > "$MANIFEST_PATH" << MANIFEST_EOF
{
  "version": "${VM_VERSION}",
  "base_url": "https://dl.helix.ml/vm",
  "files": [
    {
      "name": "disk.qcow2.zst",
      "size": ${DISK_SIZE},
      "sha256": "${DISK_SHA256}",
      "compression": "zstd",
      "decompressed_name": "disk.qcow2",
      "decompressed_size": ${DECOMPRESSED_SIZE}
    }
  ]
}
MANIFEST_EOF
else
    cat > "$MANIFEST_PATH" << MANIFEST_EOF
{
  "version": "${VM_VERSION}",
  "base_url": "https://dl.helix.ml/vm",
  "files": [
    {"name": "disk.qcow2", "size": ${DISK_SIZE}, "sha256": "${DISK_SHA256}"}
  ]
}
MANIFEST_EOF
fi

log "  Written to: ${MANIFEST_PATH}"
cat "$MANIFEST_PATH"

log ""
log "================================================"
log "Upload complete!"
log "================================================"
log ""
log "Version:     ${VM_VERSION}"
log "Download:    https://dl.helix.ml/vm/${VM_VERSION}/${DISK_UPLOAD_NAME}"
log "Disk size:   $(echo "$DISK_SIZE" | awk '{printf "%.1f GB", $1/1073741824}') (from $(echo "$ORIG_SIZE" | awk '{printf "%.1f GB", $1/1073741824}') on disk)"
log ""
log "The vm-manifest.json has been written to ${MANIFEST_PATH}"
log "Run ./scripts/build-helix-app.sh to embed it in the app."
