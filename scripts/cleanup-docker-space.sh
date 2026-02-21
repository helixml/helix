#!/bin/bash
# cleanup-docker-space.sh — Hourly cron job to prevent /var/lib/docker zvol from filling up.
#
# Architecture: Host Docker → sandbox-nvidia (DinD) → desktop containers (DinDinD)
# Each desktop session creates a docker-data-{sessionID} volume inside the sandbox's
# Docker daemon. These volumes are never cleaned up when sessions end, accumulating
# hundreds of GB over time.
#
# Install: crontab -e → 0 * * * * /prod/home/luke/pm/helix/scripts/cleanup-docker-space.sh >> /var/log/helix-docker-cleanup.log 2>&1
#
# Safe operations only — never touches:
#   - Running containers or their volumes
#   - The main sandbox-docker-storage volume
#   - Active buildkit/buildx state
#   - Currently tagged images in use
#
set -euo pipefail

COMPOSE_FILE="/prod/home/luke/pm/helix/docker-compose.dev.yaml"
LOG_PREFIX="[helix-docker-cleanup]"
DRY_RUN="${DRY_RUN:-false}"

log() { echo "$(date '+%Y-%m-%d %H:%M:%S') $LOG_PREFIX $*"; }

run_or_dry() {
    if [ "$DRY_RUN" = "true" ]; then
        log "DRY RUN: $*"
    else
        "$@"
    fi
}

# Check if sandbox is running
if ! docker compose -f "$COMPOSE_FILE" ps --format '{{.Name}}' 2>/dev/null | grep -q sandbox-nvidia; then
    log "sandbox-nvidia not running, skipping"
    exit 0
fi

SANDBOX_EXEC="docker compose -f $COMPOSE_FILE exec -T sandbox-nvidia"

log "=== Starting cleanup ==="

# Report current usage
USAGE=$(df -h /var/lib/docker | awk 'NR==2{print $5}')
AVAIL=$(df -h /var/lib/docker | awk 'NR==2{print $4}')
log "Current /var/lib/docker usage: $USAGE used, $AVAIL available"

###############################################################################
# 1. BIGGEST WIN: Remove orphaned docker-data-ses_* volumes inside sandbox
###############################################################################
log "--- Phase 1: Orphaned session volumes inside sandbox ---"

# Get running container session IDs
RUNNING_SESSIONS=$($SANDBOX_EXEC sh -c '
    docker ps --format "{{.Names}}" 2>/dev/null | grep "ubuntu-external-" | sed "s/ubuntu-external-//"
' 2>/dev/null || true)

# Get all docker-data-ses volumes
ALL_SES_VOLUMES=$($SANDBOX_EXEC sh -c 'docker volume ls -q 2>/dev/null | grep "^docker-data-ses_"' 2>/dev/null || true)

ORPHANED_COUNT=0
ORPHANED_VOLUMES=""
while IFS= read -r vol; do
    [ -z "$vol" ] && continue
    SESSION_ID=$(echo "$vol" | sed 's/^docker-data-ses_//')
    if ! echo "$RUNNING_SESSIONS" | grep -qF "$SESSION_ID"; then
        ORPHANED_VOLUMES="$ORPHANED_VOLUMES $vol"
        ORPHANED_COUNT=$((ORPHANED_COUNT + 1))
    fi
done <<< "$ALL_SES_VOLUMES"

if [ "$ORPHANED_COUNT" -gt 0 ]; then
    log "Found $ORPHANED_COUNT orphaned session volumes to remove"
    for vol in $ORPHANED_VOLUMES; do
        log "  Removing: $vol"
        run_or_dry $SANDBOX_EXEC docker volume rm "$vol" 2>/dev/null || log "  WARN: failed to remove $vol (may be in use)"
    done
else
    log "No orphaned session volumes found"
fi

###############################################################################
# 2. Remove old registry:5000/helix-ubuntu tags (keep latest 2)
###############################################################################
log "--- Phase 2: Old helix-ubuntu image tags inside sandbox ---"

# Get the current tag in use by running containers
CURRENT_TAG=$($SANDBOX_EXEC sh -c '
    docker ps --format "{{.Image}}" 2>/dev/null | grep "helix-ubuntu:" | head -1 | sed "s/.*://"
' 2>/dev/null || true)

if [ -n "$CURRENT_TAG" ]; then
    # Get all registry helix-ubuntu tags, sorted by creation date (newest first)
    OLD_REGISTRY_IMAGES=$($SANDBOX_EXEC sh -c '
        docker images "registry:5000/helix-ubuntu" --format "{{.Tag}} {{.CreatedAt}}" 2>/dev/null | sort -k2 -r
    ' 2>/dev/null || true)

    # Keep the 2 newest, remove the rest
    SKIP=2
    while IFS= read -r line; do
        [ -z "$line" ] && continue
        TAG=$(echo "$line" | awk '{print $1}')
        if [ "$SKIP" -gt 0 ]; then
            SKIP=$((SKIP - 1))
            log "  Keeping: registry:5000/helix-ubuntu:$TAG"
            continue
        fi
        log "  Removing: registry:5000/helix-ubuntu:$TAG"
        run_or_dry $SANDBOX_EXEC docker rmi "registry:5000/helix-ubuntu:$TAG" 2>/dev/null || true
    done <<< "$OLD_REGISTRY_IMAGES"

    # Also remove the local helix-ubuntu images that aren't the current tag or "latest"
    OLD_LOCAL_UBUNTU=$($SANDBOX_EXEC sh -c '
        docker images "helix-ubuntu" --format "{{.Tag}}" 2>/dev/null | grep -v "latest"
    ' 2>/dev/null || true)
    while IFS= read -r tag; do
        [ -z "$tag" ] && continue
        [ "$tag" = "$CURRENT_TAG" ] && continue
        log "  Removing old local: helix-ubuntu:$tag"
        run_or_dry $SANDBOX_EXEC docker rmi "helix-ubuntu:$tag" 2>/dev/null || true
    done <<< "$OLD_LOCAL_UBUNTU"
fi

###############################################################################
# 3. Remove old helix-task-* and helix-ses_* images inside sandbox
###############################################################################
log "--- Phase 3: Old spectask/session images inside sandbox ---"

OLD_TASK_IMAGES=$($SANDBOX_EXEC sh -c '
    docker images --format "{{.Repository}}:{{.Tag}}" 2>/dev/null | grep -E "^helix-task-|^helix-ses_"
' 2>/dev/null || true)

TASK_IMG_COUNT=0
while IFS= read -r img; do
    [ -z "$img" ] && continue
    log "  Removing: $img"
    run_or_dry $SANDBOX_EXEC docker rmi "$img" 2>/dev/null || true
    TASK_IMG_COUNT=$((TASK_IMG_COUNT + 1))
done <<< "$OLD_TASK_IMAGES"
log "Removed $TASK_IMG_COUNT old task/session images"

###############################################################################
# 4. Remove unused misc images inside sandbox (not needed for current operations)
###############################################################################
log "--- Phase 4: Unused misc images inside sandbox ---"

# These are one-off build/test images that are no longer needed
for img in "test-ubuntu-go:latest" "qwen-code-builder:node20" "zed-builder:ubuntu25"; do
    if $SANDBOX_EXEC docker image inspect "$img" >/dev/null 2>&1; then
        log "  Removing: $img"
        run_or_dry $SANDBOX_EXEC docker rmi "$img" 2>/dev/null || true
    fi
done

###############################################################################
# 5. Sandbox-level dangling images
###############################################################################
log "--- Phase 5: Dangling images inside sandbox ---"
DANGLING=$($SANDBOX_EXEC sh -c 'docker images -f dangling=true -q 2>/dev/null | wc -l' 2>/dev/null || echo 0)
if [ "$DANGLING" -gt 0 ]; then
    log "Removing $DANGLING dangling images inside sandbox"
    run_or_dry $SANDBOX_EXEC docker image prune -f 2>/dev/null || true
fi

###############################################################################
# 6. Sandbox-level orphaned non-session volumes (anonymous hashes, old task volumes)
###############################################################################
log "--- Phase 6: Orphaned misc volumes inside sandbox ---"

ORPHANED_MISC=$($SANDBOX_EXEC sh -c '
    docker volume ls -f dangling=true -q 2>/dev/null | grep -v "^docker-data-ses_" | grep -v "^buildkit" | grep -v "^buildx"
' 2>/dev/null || true)

MISC_COUNT=0
while IFS= read -r vol; do
    [ -z "$vol" ] && continue
    # Skip zed build cache volumes — they speed up zed rebuilds
    case "$vol" in
        zed-*) continue ;;
    esac
    log "  Removing: $vol"
    run_or_dry $SANDBOX_EXEC docker volume rm "$vol" 2>/dev/null || true
    MISC_COUNT=$((MISC_COUNT + 1))
done <<< "$ORPHANED_MISC"
log "Removed $MISC_COUNT orphaned misc volumes"

###############################################################################
# 7. Host-level: dangling images
###############################################################################
log "--- Phase 7: Host dangling images ---"
HOST_DANGLING=$(docker images -f dangling=true -q 2>/dev/null | wc -l)
if [ "$HOST_DANGLING" -gt 0 ]; then
    log "Removing $HOST_DANGLING dangling images on host"
    run_or_dry docker image prune -f 2>/dev/null || true
fi

###############################################################################
# 8. Host-level: dangling volumes (old helix-task-* leftovers)
###############################################################################
log "--- Phase 8: Host dangling volumes ---"
HOST_DANGLING_VOLS=$(docker volume ls -f dangling=true -q 2>/dev/null || true)
while IFS= read -r vol; do
    [ -z "$vol" ] && continue
    # Only remove known-safe patterns: old task volumes, anonymous hashes
    case "$vol" in
        helix-task-*|helix-inner-*)
            log "  Removing: $vol"
            run_or_dry docker volume rm "$vol" 2>/dev/null || true
            ;;
        helix_sandbox-data|helix_filestore-data|helix_helix-kodit)
            # These are potentially stale named volumes from old compose runs
            # but could be recreated — safe to remove if dangling
            log "  Removing dangling: $vol"
            run_or_dry docker volume rm "$vol" 2>/dev/null || true
            ;;
        [0-9a-f][0-9a-f][0-9a-f][0-9a-f]*)
            # Anonymous hash volumes — safe to remove if dangling
            log "  Removing anonymous: $vol"
            run_or_dry docker volume rm "$vol" 2>/dev/null || true
            ;;
        *)
            log "  Skipping unknown: $vol"
            ;;
    esac
done <<< "$HOST_DANGLING_VOLS"

###############################################################################
# 9. Host-level build cache — SKIPPED by default
#    Build cache is critical for fast rebuilds of helix-sandbox, helix-ubuntu,
#    helix-api, helix-frontend. DO NOT prune unless explicitly opted in.
#    Set PRUNE_BUILD_CACHE=true to enable (e.g. during emergency disk recovery).
###############################################################################
if [ "${PRUNE_BUILD_CACHE:-false}" = "true" ]; then
    log "--- Phase 9: Host build cache (>7 days old, opted in) ---"
    CACHE_BEFORE=$(docker system df --format '{{.Size}}' 2>/dev/null | tail -1)
    run_or_dry docker builder prune -f --filter "until=168h" 2>/dev/null || true
    CACHE_AFTER=$(docker system df --format '{{.Size}}' 2>/dev/null | tail -1)
    log "Build cache: before=$CACHE_BEFORE after=$CACHE_AFTER"
else
    log "--- Phase 9: Host build cache — SKIPPED (set PRUNE_BUILD_CACHE=true to enable) ---"
fi

###############################################################################
# Done — report
###############################################################################
USAGE_AFTER=$(df -h /var/lib/docker | awk 'NR==2{print $5}')
AVAIL_AFTER=$(df -h /var/lib/docker | awk 'NR==2{print $4}')
log "=== Cleanup complete ==="
log "Before: $USAGE used, $AVAIL available"
log "After:  $USAGE_AFTER used, $AVAIL_AFTER available"
