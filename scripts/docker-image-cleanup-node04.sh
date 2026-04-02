#!/bin/bash
# Docker image cleanup for CI server
#
# Removes:
#   1. ALL random-named CI images (16-char alphanumeric names from e2e tests)
#   2. Registry images older than 7 days (helix-ubuntu, helix-sway, helix-sandbox,
#      controlplane, haystack, typesense, demos)
#   3. Old local helix-sandbox builds (> 7 days)
#   4. Dangling (untagged, unreferenced) images
#   5. Build cache exceeding 10 GB
#
# NEVER removes:
#   - Runner images (base layers can't be rebuilt - R2 5GB layer limit)
#   - Images in use by containers
#   - CI tool images (golang, node, alpine, drone, rod, tika, etc.)
#
# Usage:
#   ./docker-image-cleanup.sh              # actually delete
#   ./docker-image-cleanup.sh --dry-run    # just log what would be deleted

set -uo pipefail

LOG_FILE="/var/log/docker-image-cleanup.log"
DRY_RUN="${1:-}"

log() {
    echo "$(date -Iseconds) $*" >> "$LOG_FILE"
}

log "=============================================="
log "Docker image cleanup started (${DRY_RUN:-live})"
log "=============================================="
log "Disk before cleanup:"
df -h / >> "$LOG_FILE" 2>&1
docker system df >> "$LOG_FILE" 2>&1

# Snapshot images in use by any container (running or stopped)
USED_IMAGES=$(docker ps -a --format '{{.Image}}' | sort -u)

# Build list of images to remove into a temp file (avoids subshell counter issues)
REMOVE_LIST=$(mktemp)
SKIP_COUNT=0

add_for_removal() {
    local image="$1"

    # Never touch runner images, no matter what
    if [[ "$image" == *"/runner:"* ]] || [[ "$image" == *"/runner"* ]]; then
        log "SAFETY SKIP (runner): $image"
        return
    fi

    # Skip if in use by a container
    if echo "$USED_IMAGES" | grep -qxF "$image"; then
        log "SKIP (in use): $image"
        return
    fi

    echo "$image" >> "$REMOVE_LIST"
}

# ---------------------------------------------------------------
# 1. Remove ALL random-named CI images (16-char alphanumeric repos)
#    These are ephemeral e2e test images, safe to remove entirely.
# ---------------------------------------------------------------
log "--- Phase 1: Random-named CI images ---"
while read -r image; do
    add_for_removal "$image"
done < <(docker images --format '{{.Repository}}:{{.Tag}}' | grep -E '^[a-z0-9]{16}:')

# ---------------------------------------------------------------
# 2. Remove old (>7 day) tagged images from specific registry repos
#    EXPLICITLY excludes runner images.
# ---------------------------------------------------------------
log "--- Phase 2: Old registry images (>7 days) ---"

CLEANUP_REPOS=(
    "registry.helixml.tech/helix/helix-ubuntu"
    "registry.helixml.tech/helix/helix-sway"
    "registry.helixml.tech/helix/helix-sandbox"
    "registry.helixml.tech/helix/controlplane"
    "registry.helixml.tech/helix/haystack"
    "registry.helixml.tech/helix/typesense"
    "registry.helixml.tech/helix/demos"
)

for repo in "${CLEANUP_REPOS[@]}"; do
    while read -r image; do
        add_for_removal "$image"
    done < <(docker images --format '{{.Repository}}:{{.Tag}}\t{{.CreatedSince}}' "$repo" 2>/dev/null | \
        grep -E '(weeks|months|years) ago' | \
        awk -F'\t' '{print $1}' | \
        grep -v '<none>')
done

# ---------------------------------------------------------------
# 3. Remove old local helix-sandbox builds (not registry-prefixed)
# ---------------------------------------------------------------
log "--- Phase 3: Old local helix-sandbox images ---"
while read -r image; do
    add_for_removal "$image"
done < <(docker images --format '{{.Repository}}:{{.Tag}}\t{{.CreatedSince}}' "helix-sandbox" 2>/dev/null | \
    grep -E '(weeks|months|years) ago' | \
    awk -F'\t' '{print $1}' | \
    grep -v '<none>')

# ---------------------------------------------------------------
# 4. Remove collected images
# ---------------------------------------------------------------
TOTAL=$(wc -l < "$REMOVE_LIST")
REMOVED=0
ERRORS=0

log "Removing $TOTAL images..."

while read -r image; do
    if [ "$DRY_RUN" = "--dry-run" ]; then
        log "DRY RUN would remove: $image"
        ((REMOVED++)) || true
    else
        if docker rmi "$image" >> "$LOG_FILE" 2>&1; then
            ((REMOVED++)) || true
        else
            log "ERROR removing: $image"
            ((ERRORS++)) || true
        fi
    fi
done < "$REMOVE_LIST"

rm -f "$REMOVE_LIST"

# ---------------------------------------------------------------
# 5. Prune dangling images (untagged AND unreferenced)
#    This catches old untagged builds left behind after tag reuse.
#    Does NOT use -a, so unused-but-tagged images are kept.
# ---------------------------------------------------------------
log "--- Phase 5: Dangling image prune ---"
if [ "$DRY_RUN" = "--dry-run" ]; then
    log "DRY RUN: would prune dangling images"
else
    docker image prune --force >> "$LOG_FILE" 2>&1
fi

# ---------------------------------------------------------------
# 6. Prune build cache, keeping up to 10 GB
#    Build cache grows as layers orphaned from deleted images get
#    reclassified. Cap it to prevent unbounded growth.
# ---------------------------------------------------------------
log "--- Phase 6: Build cache prune (keep 10GB) ---"
if [ "$DRY_RUN" = "--dry-run" ]; then
    log "DRY RUN: would prune build cache to 10GB"
else
    docker builder prune --keep-storage=100G --force >> "$LOG_FILE" 2>&1
fi

# ---------------------------------------------------------------
# Report
# ---------------------------------------------------------------
log "=============================================="
log "Cleanup complete: removed=$REMOVED errors=$ERRORS"
log "Disk after cleanup:"
df -h / >> "$LOG_FILE" 2>&1
docker system df >> "$LOG_FILE" 2>&1
log "=============================================="
