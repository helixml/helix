#!/bin/bash
set -euo pipefail

# =============================================================================
# Upload VM disk images to Cloudflare R2 for CDN distribution
# =============================================================================
#
# Compresses disk.qcow2 with zstd, then uploads to R2 organized by
# version (git short hash). The app downloads these on first launch.
#
# Incremental update support:
#   - Generates xdelta3 binary patches from the previous version's disk to
#     this version's disk, then uploads to vm/{FROM}_to_{TO}/patch.xdelta3.zst
#   - Updates the manifest with a "patches" array so clients can download
#     only the delta instead of the full 7.8 GB disk.
#   - For Docker-image-only releases, set DOCKER_ONLY_UPDATE=1 to skip disk
#     upload and set docker_only_update=true in the manifest.
#
# Prerequisites:
#   - AWS CLI: brew install awscli
#   - qemu-img: brew install qemu (or bundled in app)
#   - xdelta3: brew install xdelta  (for patch generation)
#   - R2 credentials in for-mac/.env.r2
#   - VM images built: ~/Library/Application Support/Helix/vm/helix-desktop/
#
# Usage:
#   cd for-mac && ./scripts/upload-vm-images.sh
#   VM_VERSION=v1.0 ./scripts/upload-vm-images.sh        # Override version tag
#   SKIP_COMPRESS=1 ./scripts/upload-vm-images.sh         # Upload without compressing
#   DOCKER_ONLY_UPDATE=1 ./scripts/upload-vm-images.sh    # Service images only, no disk
#   SKIP_PATCH=1 ./scripts/upload-vm-images.sh            # Skip patch generation
#   PATCH_VERSIONS=5 ./scripts/upload-vm-images.sh        # Keep patches for N versions

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
DOCKER_ONLY_UPDATE="${DOCKER_ONLY_UPDATE:-0}"
SKIP_PATCH="${SKIP_PATCH:-0}"
# Number of previous versions to keep patches for; older patches are pruned.
PATCH_VERSIONS="${PATCH_VERSIONS:-3}"

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

if [ "$DOCKER_ONLY_UPDATE" != "1" ] && [ ! -f "${VM_DIR}/disk.qcow2" ]; then
    echo "ERROR: Missing VM image: ${VM_DIR}/disk.qcow2"
    echo "Provision the VM first, then try again."
    echo "For a docker-only release (no disk change), set DOCKER_ONLY_UPDATE=1."
    exit 1
fi




# =============================================================================
# Docker-only update: skip disk upload, generate manifest with docker_only_update=true
# =============================================================================

if [ "$DOCKER_ONLY_UPDATE" = "1" ]; then
    log ""
    log "=== Docker-only update mode ==="
    log "Skipping disk upload — only Helix service images changed."
    log "Version: ${VM_VERSION}"

    MANIFEST_PATH="${FOR_MAC_DIR}/vm-manifest.json"
    cat > "$MANIFEST_PATH" << MANIFEST_EOF
{
  "version": "${VM_VERSION}",
  "base_url": "https://dl.helix.ml/vm",
  "files": [],
  "docker_only_update": true,
  "patches": []
}
MANIFEST_EOF

    log "Uploading docker-only manifest to CDN..."
    aws s3 cp "$MANIFEST_PATH" "s3://${R2_BUCKET}/vm/${VM_VERSION}/manifest.json" \
        --endpoint-url "$R2_ENDPOINT" \
        --content-type "application/json" \
        --cache-control "no-cache, max-age=0" \
        --no-progress 2>&1
    log "Done."
    log ""
    log "The vm-manifest.json has been written to ${MANIFEST_PATH}"
    log "Run ./scripts/build-helix-app.sh to embed it in the app."
    exit 0
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
  ],
  "docker_only_update": false,
  "patches": []
}
MANIFEST_EOF
else
    cat > "$MANIFEST_PATH" << MANIFEST_EOF
{
  "version": "${VM_VERSION}",
  "base_url": "https://dl.helix.ml/vm",
  "files": [
    {"name": "disk.qcow2", "size": ${DISK_SIZE}, "sha256": "${DISK_SHA256}"}
  ],
  "docker_only_update": false,
  "patches": []
}
MANIFEST_EOF
fi

log "  Written to: ${MANIFEST_PATH}"
cat "$MANIFEST_PATH"

# Upload manifest to CDN so the in-app updater can fetch it at
# https://dl.helix.ml/vm/{VERSION}/manifest.json
log ""
log "Uploading vm-manifest.json to CDN..."
aws s3 cp "$MANIFEST_PATH" "s3://${R2_BUCKET}/vm/${VM_VERSION}/manifest.json" \
    --endpoint-url "$R2_ENDPOINT" \
    --content-type "application/json" \
    --cache-control "no-cache, max-age=0" \
    --no-progress 2>&1
log "  Done."

# Verify manifest is accessible
MANIFEST_HTTP=$(curl -sI -o /dev/null -w "%{http_code}" "https://dl.helix.ml/vm/${VM_VERSION}/manifest.json")
if [ "$MANIFEST_HTTP" = "200" ]; then
    log "CDN check: https://dl.helix.ml/vm/${VM_VERSION}/manifest.json → 200 OK"
else
    log "WARNING: manifest CDN check returned HTTP ${MANIFEST_HTTP} (may need DNS propagation)"
fi

# =============================================================================
# Step 5: Generate binary delta patch from the previous version (if available)
# =============================================================================
#
# Produces vm/{PREV}_to_{CURR}/patch.xdelta3.zst for each of the last
# PATCH_VERSIONS previous releases. Clients download this instead of the full
# disk when upgrading from a matching version.
#
# Prerequisites: xdelta3 (brew install xdelta), zstd, aws CLI
# Skip with: SKIP_PATCH=1

if [ "$DOCKER_ONLY_UPDATE" = "1" ]; then
    log "DOCKER_ONLY_UPDATE=1 — skipping patch generation (no disk change)"
elif [ "$SKIP_PATCH" = "1" ]; then
    log "SKIP_PATCH=1 — skipping patch generation"
elif ! command -v xdelta3 &>/dev/null; then
    log "WARNING: xdelta3 not found — skipping patch generation. Install with: brew install xdelta"
elif [ "$USE_ZSTD" != "true" ]; then
    log "WARNING: patch generation requires zstd mode — skipping"
else
    log ""
    log "=== Generating delta patches ==="

    # List all versions currently in the CDN bucket (one directory per version)
    log "Fetching version list from CDN bucket..."
    PREV_VERSIONS=$(aws s3 ls "s3://${R2_BUCKET}/vm/" \
        --endpoint-url "$R2_ENDPOINT" 2>/dev/null \
        | awk '{print $NF}' \
        | sed 's|/$||' \
        | grep -v "^${VM_VERSION}$" \
        | tail -n "${PATCH_VERSIONS}" \
        || echo "")

    if [ -z "$PREV_VERSIONS" ]; then
        log "No previous versions found in CDN — skipping patch generation (first release?)"
    else
        log "Will generate patches from: $(echo "$PREV_VERSIONS" | tr '\n' ' ')"

        PATCHES_JSON="[]"

        for PREV_VERSION in $PREV_VERSIONS; do
            PATCH_KEY="${PREV_VERSION}_to_${VM_VERSION}"
            PREV_DISK_URL="https://dl.helix.ml/vm/${PREV_VERSION}/disk.qcow2.zst"
            PATCH_DIR="${UPLOAD_DIR}/${PATCH_KEY}"
            mkdir -p "$PATCH_DIR"

            log ""
            log "--- Patch: ${PREV_VERSION} → ${VM_VERSION} ---"

            # Download previous version's compressed disk
            PREV_COMPRESSED="${PATCH_DIR}/prev_disk.qcow2.zst"
            log "  Downloading previous disk (${PREV_VERSION})..."
            if ! curl -fsSL -o "$PREV_COMPRESSED" "$PREV_DISK_URL" 2>&1; then
                log "  WARNING: Could not download ${PREV_DISK_URL} — skipping patch for ${PREV_VERSION}"
                continue
            fi

            # Decompress previous disk
            PREV_DISK="${PATCH_DIR}/prev_disk.qcow2"
            log "  Decompressing previous disk..."
            if ! zstd -d "$PREV_COMPRESSED" -o "$PREV_DISK" --force 2>&1; then
                log "  WARNING: Decompression failed for ${PREV_VERSION} — skipping"
                continue
            fi
            rm -f "$PREV_COMPRESSED"

            # Record SHA256 of previous (decompressed) disk — this is applies_to_sha256
            PREV_SHA256=$(shasum -a 256 "$PREV_DISK" | awk '{print $1}')
            log "  Previous disk SHA256: ${PREV_SHA256}"

            # New disk is already decompressed as $UNCOMPRESSED
            NEW_SHA256=$(shasum -a 256 "$UNCOMPRESSED" | awk '{print $1}')
            log "  New disk SHA256:      ${NEW_SHA256}"

            # Generate xdelta3 patch
            PATCH_RAW="${PATCH_DIR}/patch.xdelta3"
            log "  Generating xdelta3 patch (this takes 5-30 minutes)..."
            if ! xdelta3 -e -s "$PREV_DISK" "$UNCOMPRESSED" "$PATCH_RAW" 2>&1; then
                log "  WARNING: xdelta3 failed for ${PREV_VERSION} — skipping"
                rm -f "$PREV_DISK" "$PATCH_RAW"
                continue
            fi
            rm -f "$PREV_DISK"

            # Compress patch with zstd
            PATCH_ZST="${PATCH_DIR}/patch.xdelta3.zst"
            log "  Compressing patch with zstd..."
            zstd -T0 -3 "$PATCH_RAW" -o "$PATCH_ZST" --force 2>&1
            rm -f "$PATCH_RAW"

            PATCH_SIZE=$(stat -f%z "$PATCH_ZST" 2>/dev/null || stat -c%s "$PATCH_ZST" 2>/dev/null)
            PATCH_SHA256=$(shasum -a 256 "$PATCH_ZST" | awk '{print $1}')
            log "  Patch size: $(echo "$PATCH_SIZE" | awk '{printf "%.1f MB", $1/1048576}')"
            log "  Patch SHA256: ${PATCH_SHA256}"

            # Upload patch to CDN
            log "  Uploading patch to CDN: vm/${PATCH_KEY}/patch.xdelta3.zst"
            aws s3 cp "$PATCH_ZST" "s3://${R2_BUCKET}/vm/${PATCH_KEY}/patch.xdelta3.zst" \
                --endpoint-url "$R2_ENDPOINT" \
                --no-progress 2>&1
            log "  Uploaded."

            # Append to patches JSON array
            PATCH_ENTRY="{\"from_version\":\"${PREV_VERSION}\",\"name\":\"patch.xdelta3.zst\",\"size\":${PATCH_SIZE},\"sha256\":\"${PATCH_SHA256}\",\"applies_to_sha256\":\"${PREV_SHA256}\",\"result_sha256\":\"${NEW_SHA256}\"}"
            if [ "$PATCHES_JSON" = "[]" ]; then
                PATCHES_JSON="[${PATCH_ENTRY}]"
            else
                PATCHES_JSON="${PATCHES_JSON%]},${PATCH_ENTRY}]"
            fi
        done

        # Update manifest.json with patch metadata
        if [ "$PATCHES_JSON" != "[]" ]; then
            log ""
            log "Updating manifest with ${PATCH_VERSIONS} patch entries..."
            # Rewrite manifest with patches array populated
            if [ "$USE_ZSTD" = true ]; then
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
  ],
  "docker_only_update": false,
  "patches": ${PATCHES_JSON}
}
MANIFEST_EOF
            fi

            # Re-upload updated manifest
            aws s3 cp "$MANIFEST_PATH" "s3://${R2_BUCKET}/vm/${VM_VERSION}/manifest.json" \
                --endpoint-url "$R2_ENDPOINT" \
                --content-type "application/json" \
                --cache-control "no-cache, max-age=0" \
                --no-progress 2>&1
            log "Manifest re-uploaded with patch metadata."
        fi
    fi

    # ==========================================================================
    # Prune old patches: delete patch directories older than PATCH_VERSIONS+1
    # ==========================================================================
    log ""
    log "=== Pruning old patches (keeping last ${PATCH_VERSIONS} versions) ==="
    ALL_VERSIONS=$(aws s3 ls "s3://${R2_BUCKET}/vm/" \
        --endpoint-url "$R2_ENDPOINT" 2>/dev/null \
        | awk '{print $NF}' \
        | sed 's|/$||' \
        | grep -v '_to_' \
        | sort \
        || echo "")
    PRUNE_BEFORE=$(echo "$ALL_VERSIONS" | head -n -$((PATCH_VERSIONS + 1)) || true)
    for OLD_VER in $PRUNE_BEFORE; do
        log "  Checking for stale patches pointing to ${OLD_VER}..."
        STALE_KEYS=$(aws s3 ls "s3://${R2_BUCKET}/vm/" \
            --endpoint-url "$R2_ENDPOINT" 2>/dev/null \
            | awk '{print $NF}' \
            | sed 's|/$||' \
            | grep "^${OLD_VER}_to_" \
            || true)
        for STALE in $STALE_KEYS; do
            log "  Deleting stale patch directory: vm/${STALE}/"
            aws s3 rm "s3://${R2_BUCKET}/vm/${STALE}/" \
                --recursive \
                --endpoint-url "$R2_ENDPOINT" \
                --no-progress 2>&1 || true
        done
    done
fi

log ""
log "================================================"
log "Upload complete!"
log "================================================"
log ""
log "Version:     ${VM_VERSION}"
log "Download:    https://dl.helix.ml/vm/${VM_VERSION}/${DISK_UPLOAD_NAME}"
log "Manifest:    https://dl.helix.ml/vm/${VM_VERSION}/manifest.json"
log "Disk size:   $(echo "$DISK_SIZE" | awk '{printf "%.1f GB", $1/1073741824}') (from $(echo "$ORIG_SIZE" | awk '{printf "%.1f GB", $1/1073741824}') on disk)"
log ""
log "The vm-manifest.json has been written to ${MANIFEST_PATH}"
log "Run ./scripts/build-helix-app.sh to embed it in the app."
