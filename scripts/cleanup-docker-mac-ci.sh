#!/bin/bash
# Docker image cleanup for macOS CI node (Docker for Mac)
#
# This node builds helix images via Drone CI. Each build produces multiple
# tagged images across registries (ghcr.io, registry.helixml.tech, local).
# Without cleanup, these accumulate and fill the Docker virtual disk.
#
# Removes:
#   1. Old registry/ghcr images (keeps latest tag per repo)
#   2. Old local helix-sandbox builds (keeps latest)
#   3. Dangling (untagged) images
#   4. Build cache exceeding 100 GB
#
# NEVER removes:
#   - Images in use by running containers
#   - CI tool images (drone, alpine/git, docker, node)
#   - The drone runner image
#
# Install:
#   crontab -e
#   @daily  /Users/luke/pm/helix/scripts/cleanup-docker-mac-ci.sh >> /tmp/docker-ci-cleanup.log 2>&1
#
# WARNING: Do NOT add `docker system prune` as a separate cron entry.
# It nukes dangling build cache, including BuildKit cache mounts (cargo
# registry, target dir) that are critical for fast Zed rebuilds.
#
# Usage:
#   ./cleanup-docker-mac-ci.sh              # actually delete
#   ./cleanup-docker-mac-ci.sh --dry-run    # just log what would be deleted

set -uo pipefail

LOG_FILE="/tmp/docker-ci-cleanup.log"
DRY_RUN="${1:-}"

log() {
    echo "$(date '+%Y-%m-%d %H:%M:%S') [docker-mac-cleanup] $*"
}

log "=============================================="
log "Docker Mac CI cleanup started (${DRY_RUN:-live})"
log "=============================================="

# Report current state
docker system df >> "$LOG_FILE" 2>&1
log "Docker.raw size: $(du -h ~/Library/Containers/com.docker.docker/Data/vms/0/data/Docker.raw 2>/dev/null | awk '{print $1}')"

# Snapshot images in use by any container (running or stopped)
USED_IMAGES=$(docker ps -a --format '{{.Image}}' | sort -u)

REMOVE_LIST=$(mktemp)

add_for_removal() {
    local image="$1"

    # Never touch CI tool images
    case "$image" in
        drone/*|alpine/git*|docker:*|node:*)
            log "SKIP (CI tool): $image"
            return
            ;;
    esac

    # Skip if in use by a container
    if echo "$USED_IMAGES" | grep -qxF "$image"; then
        log "SKIP (in use): $image"
        return
    fi

    echo "$image" >> "$REMOVE_LIST"
}

###############################################################################
# 1. For each helix image repo, keep only the newest tag, remove the rest
###############################################################################
log "--- Phase 1: Old helix images (keep newest per repo) ---"

CLEANUP_REPOS=(
    "registry.helixml.tech/helix/helix-ubuntu"
    "registry.helixml.tech/helix/helix-sway"
    "registry.helixml.tech/helix/helix-sandbox"
    "registry.helixml.tech/helix/controlplane"
    "registry.helixml.tech/helix/haystack"
    "registry.helixml.tech/helix/typesense"
    "ghcr.io/helixml/helix-ubuntu"
    "ghcr.io/helixml/helix-sway"
    "ghcr.io/helixml/helix-sandbox"
    "ghcr.io/helixml/controlplane"
    "ghcr.io/helixml/haystack"
    "ghcr.io/helixml/typesense"
)

for repo in "${CLEANUP_REPOS[@]}"; do
    # List tags sorted by creation date (newest first)
    IMAGES=$(docker images "$repo" --format '{{.Repository}}:{{.Tag}} {{.CreatedAt}}' 2>/dev/null | \
        grep -v '<none>' | sort -k2 -r)

    [ -z "$IMAGES" ] && continue

    SKIP=1  # keep the newest
    while IFS= read -r line; do
        [ -z "$line" ] && continue
        IMAGE=$(echo "$line" | awk '{print $1}')
        if [ "$SKIP" -gt 0 ]; then
            SKIP=$((SKIP - 1))
            log "KEEP: $IMAGE"
            continue
        fi
        add_for_removal "$IMAGE"
    done <<< "$IMAGES"
done

###############################################################################
# 2. Old local helix-sandbox builds (keep newest)
###############################################################################
log "--- Phase 2: Old local helix-sandbox builds ---"

IMAGES=$(docker images "helix-sandbox" --format '{{.Repository}}:{{.Tag}} {{.CreatedAt}}' 2>/dev/null | \
    grep -v '<none>' | sort -k2 -r)

SKIP=1
while IFS= read -r line; do
    [ -z "$line" ] && continue
    IMAGE=$(echo "$line" | awk '{print $1}')
    if [ "$SKIP" -gt 0 ]; then
        SKIP=$((SKIP - 1))
        log "KEEP: $IMAGE"
        continue
    fi
    add_for_removal "$IMAGE"
done <<< "$IMAGES"

###############################################################################
# 3. Old zed-builder images (keep newest)
###############################################################################
log "--- Phase 3: Old zed-builder images ---"

IMAGES=$(docker images "zed-builder" --format '{{.Repository}}:{{.Tag}} {{.CreatedAt}}' 2>/dev/null | \
    grep -v '<none>' | sort -k2 -r)

SKIP=1
while IFS= read -r line; do
    [ -z "$line" ] && continue
    IMAGE=$(echo "$line" | awk '{print $1}')
    if [ "$SKIP" -gt 0 ]; then
        SKIP=$((SKIP - 1))
        log "KEEP: $IMAGE"
        continue
    fi
    add_for_removal "$IMAGE"
done <<< "$IMAGES"

###############################################################################
# 4. Remove collected images
###############################################################################
TOTAL=$(wc -l < "$REMOVE_LIST" | tr -d ' ')
REMOVED=0
ERRORS=0

log "Removing $TOTAL images..."

while read -r image; do
    [ -z "$image" ] && continue
    if [ "$DRY_RUN" = "--dry-run" ]; then
        log "DRY RUN would remove: $image"
        ((REMOVED++)) || true
    else
        if docker rmi "$image" >> "$LOG_FILE" 2>&1; then
            log "Removed: $image"
            ((REMOVED++)) || true
        else
            log "ERROR removing: $image"
            ((ERRORS++)) || true
        fi
    fi
done < "$REMOVE_LIST"

rm -f "$REMOVE_LIST"

###############################################################################
# 5. Dangling image prune
###############################################################################
log "--- Phase 5: Dangling image prune ---"
if [ "$DRY_RUN" = "--dry-run" ]; then
    log "DRY RUN: would prune dangling images"
else
    docker image prune --force >> "$LOG_FILE" 2>&1
fi

###############################################################################
# 6. Build cache prune (keep 50GB)
###############################################################################
log "--- Phase 6: Build cache prune (keep 50GB) ---"
if [ "$DRY_RUN" = "--dry-run" ]; then
    log "DRY RUN: would prune build cache to 50GB"
else
    docker builder prune --keep-storage=100G --force >> "$LOG_FILE" 2>&1
fi

###############################################################################
# Report
###############################################################################
log "=============================================="
log "Cleanup complete: removed=$REMOVED errors=$ERRORS"
docker system df 2>&1 | while read -r line; do log "$line"; done
log "Docker.raw size: $(du -h ~/Library/Containers/com.docker.docker/Data/vms/0/data/Docker.raw 2>/dev/null | awk '{print $1}')"
log "=============================================="
