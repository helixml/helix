#!/bin/bash
#
# zfs-dedup-workspaces.sh - Set up ZFS deduplication for Helix workspaces
#
# This script migrates Helix workspace data from Docker volumes (on ext4) to a
# ZFS dataset with deduplication enabled. This can save 60%+ disk space when
# running many spec tasks (node_modules and .zed-state are highly duplicated).
#
# Usage:
#   ./scripts/zfs-dedup-workspaces.sh <zpool-name> [--migrate]
#
# Arguments:
#   zpool-name    Name of the ZFS pool to create the dataset in (e.g., "prod")
#   --migrate     Actually perform the migration (without this, runs in dry-run mode)
#
# Prerequisites:
#   - ZFS pool must already exist
#   - Must be run as root (uses sudo internally)
#   - Sandbox should be stopped before migration
#
# Works with both:
#   - Dev setups: helix_sandbox-data Docker volume
#   - Prod setups: sandbox-data volume in /opt/HelixML
#
# After migration, set HELIX_SANDBOX_DATA in .env to point to the ZFS dataset.
#
# Example:
#   # Dry run (see what would happen)
#   sudo ./scripts/zfs-dedup-workspaces.sh prod
#
#   # Actually migrate
#   sudo ./scripts/zfs-dedup-workspaces.sh prod --migrate
#

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

# Parse arguments
if [ $# -lt 1 ]; then
    echo "Usage: $0 <zpool-name> [--migrate]"
    echo ""
    echo "Arguments:"
    echo "  zpool-name    Name of the ZFS pool (e.g., 'prod', 'tank')"
    echo "  --migrate     Actually perform the migration (default: dry-run)"
    echo ""
    echo "Example:"
    echo "  sudo $0 prod              # Dry run"
    echo "  sudo $0 prod --migrate    # Actually migrate"
    exit 1
fi

ZPOOL_NAME="$1"
MIGRATE=false
if [ "${2:-}" = "--migrate" ]; then
    MIGRATE=true
fi

# Configuration
ZFS_DATASET="${ZPOOL_NAME}/helix-workspaces"
ZFS_MOUNTPOINT="/${ZPOOL_NAME}/helix-workspaces"

# Detect environment (dev vs prod)
detect_environment() {
    # Check for dev setup (helix_sandbox-data volume)
    if docker volume inspect helix_sandbox-data &>/dev/null; then
        DOCKER_VOLUME_NAME="helix_sandbox-data"
        DOCKER_VOLUME_PATH=$(docker volume inspect helix_sandbox-data --format '{{ .Mountpoint }}')
        SOURCE_PATH="${DOCKER_VOLUME_PATH}"
        ENVIRONMENT="dev"
        return 0
    fi

    # Check for prod setup (sandbox-data volume - prefixed with directory name)
    for prefix in "helixmlhelix" "helix" "opt_helixml"; do
        vol_name="${prefix}_sandbox-data"
        if docker volume inspect "$vol_name" &>/dev/null; then
            DOCKER_VOLUME_NAME="$vol_name"
            DOCKER_VOLUME_PATH=$(docker volume inspect "$vol_name" --format '{{ .Mountpoint }}')
            SOURCE_PATH="${DOCKER_VOLUME_PATH}"
            ENVIRONMENT="prod"
            return 0
        fi
    done

    log_error "Could not detect Helix installation (no sandbox-data volume found)"
    exit 1
}

# Check prerequisites
check_prerequisites() {
    log_info "Checking prerequisites..."

    # Check for root/sudo
    if [ "$EUID" -ne 0 ]; then
        log_error "This script must be run as root (use sudo)"
        exit 1
    fi

    # Check ZFS is available
    if ! command -v zfs &>/dev/null; then
        log_error "ZFS not found. Please install ZFS first."
        exit 1
    fi

    # Check zpool exists
    if ! zpool list "$ZPOOL_NAME" &>/dev/null; then
        log_error "ZFS pool '$ZPOOL_NAME' not found"
        echo "Available pools:"
        zpool list
        exit 1
    fi

    # Check if dataset already exists
    if zfs list "$ZFS_DATASET" &>/dev/null; then
        log_warn "ZFS dataset '$ZFS_DATASET' already exists"
        DATASET_EXISTS=true
    else
        DATASET_EXISTS=false
    fi

    # Check sandbox status
    SANDBOX_RUNNING=false
    if docker ps --format '{{.Names}}' | grep -qE '(sandbox-nvidia|sandbox)$'; then
        SANDBOX_RUNNING=true
        log_warn "Sandbox container is currently running"
    fi

    log_success "Prerequisites check passed"
}

# Calculate current disk usage
calculate_usage() {
    log_info "Analyzing current workspace disk usage..."

    if [ -d "$SOURCE_PATH" ]; then
        CURRENT_SIZE=$(du -sh "$SOURCE_PATH" 2>/dev/null | cut -f1)
        log_info "Current sandbox-data size: $CURRENT_SIZE"

        # Check for .zed-state (huge dedup potential)
        ZED_STATE_COUNT=$(find "$SOURCE_PATH" -type d -name ".zed-state" 2>/dev/null | wc -l)
        if [ "$ZED_STATE_COUNT" -gt 0 ]; then
            ZED_STATE_SIZE=$(find "$SOURCE_PATH" -type d -name ".zed-state" -exec du -s {} \; 2>/dev/null | awk '{sum+=$1} END {printf "%.1fG", sum/1024/1024}')
            log_info "Found $ZED_STATE_COUNT .zed-state directories totaling ${ZED_STATE_SIZE} (very high dedup potential)"
        fi

        # Check for node_modules
        NODE_MODULES_COUNT=$(find "$SOURCE_PATH" -type d -name "node_modules" 2>/dev/null | wc -l)
        if [ "$NODE_MODULES_COUNT" -gt 0 ]; then
            log_info "Found $NODE_MODULES_COUNT node_modules directories (high dedup potential)"
        fi

        # Check for .git directories
        GIT_COUNT=$(find "$SOURCE_PATH" -type d -name ".git" 2>/dev/null | wc -l)
        if [ "$GIT_COUNT" -gt 0 ]; then
            log_info "Found $GIT_COUNT git repositories (high dedup potential for clones)"
        fi
    else
        log_warn "Source path not found at $SOURCE_PATH"
        CURRENT_SIZE="0"
    fi
}

# Show migration plan
show_plan() {
    echo ""
    echo "========================================"
    echo "ZFS Dedup Migration Plan"
    echo "========================================"
    echo ""
    echo "Environment:        $ENVIRONMENT"
    echo "Docker volume:      $DOCKER_VOLUME_NAME"
    echo "Source path:        $SOURCE_PATH"
    echo "Current size:       ${CURRENT_SIZE:-unknown}"
    echo ""
    echo "ZFS pool:           $ZPOOL_NAME"
    echo "ZFS dataset:        $ZFS_DATASET"
    echo "ZFS mountpoint:     $ZFS_MOUNTPOINT"
    echo "ZFS options:        dedup=on, compression=lz4"
    echo ""
    echo "Steps that will be performed:"
    echo "  1. Stop sandbox container (if running)"
    echo "  2. Create ZFS dataset with dedup=on, compression=lz4"
    echo "  3. Copy data from Docker volume to ZFS dataset"
    echo "  4. Show dedup/compression ratios"
    echo ""
    echo "After migration, you need to:"
    echo "  1. Add to .env:  HELIX_SANDBOX_DATA=${ZFS_MOUNTPOINT}"
    echo "  2. Restart sandbox: docker compose -f docker-compose.dev.yaml up -d sandbox-nvidia"
    echo ""

    if [ "$DATASET_EXISTS" = true ]; then
        echo "NOTE: Dataset already exists, will skip creation step"
        echo ""
    fi

    if [ "$MIGRATE" = false ]; then
        echo -e "${YELLOW}DRY RUN MODE - No changes will be made${NC}"
        echo "Run with --migrate flag to perform actual migration"
        echo ""
    fi
}

# Perform migration
perform_migration() {
    if [ "$MIGRATE" = false ]; then
        log_info "Dry run complete. No changes made."
        return 0
    fi

    echo ""
    log_info "Starting migration..."

    # Step 1: Stop sandbox
    if [ "$SANDBOX_RUNNING" = true ]; then
        log_info "Stopping sandbox container..."
        # Try both dev and prod compose files
        if [ -f "docker-compose.dev.yaml" ]; then
            docker compose -f docker-compose.dev.yaml stop sandbox-nvidia sandbox 2>/dev/null || true
        fi
        if [ -f "/opt/HelixML/docker-compose.yaml" ]; then
            docker compose -f /opt/HelixML/docker-compose.yaml stop sandbox 2>/dev/null || true
        fi
        sleep 2
    fi

    # Step 2: Create ZFS dataset
    if [ "$DATASET_EXISTS" = false ]; then
        log_info "Creating ZFS dataset ${ZFS_DATASET} with dedup + lz4 compression..."
        zfs create -o dedup=on -o compression=lz4 -o mountpoint="${ZFS_MOUNTPOINT}" "$ZFS_DATASET"
        log_success "ZFS dataset created"
    else
        log_info "Using existing ZFS dataset ${ZFS_DATASET}"
        # Ensure dedup and compression are enabled
        zfs set dedup=on "$ZFS_DATASET"
        zfs set compression=lz4 "$ZFS_DATASET"
    fi

    # Step 3: Copy data
    if [ -d "$SOURCE_PATH" ] && [ "$(ls -A "$SOURCE_PATH" 2>/dev/null)" ]; then
        log_info "Copying data from ${SOURCE_PATH} to ${ZFS_MOUNTPOINT}..."
        log_info "This may take a while depending on data size (${CURRENT_SIZE})..."
        echo ""

        rsync -av --info=progress2 "$SOURCE_PATH/" "$ZFS_MOUNTPOINT/"

        echo ""
        log_success "Data copy complete"

        # Verify copy
        log_info "Verifying data integrity..."
        SRC_COUNT=$(find "$SOURCE_PATH" -type f 2>/dev/null | wc -l)
        DST_COUNT=$(find "$ZFS_MOUNTPOINT" -type f 2>/dev/null | wc -l)

        if [ "$SRC_COUNT" -eq "$DST_COUNT" ]; then
            log_success "Verification passed: $SRC_COUNT files copied"
        else
            log_warn "File count mismatch: source has $SRC_COUNT files, destination has $DST_COUNT"
            log_warn "This may be normal if files changed during copy. Please verify manually."
        fi
    else
        log_info "No existing data to migrate - dataset is ready for use"
    fi

    # Show results
    echo ""
    log_success "Migration complete!"
    echo ""
    show_dedup_status

    echo ""
    echo "========================================"
    echo "NEXT STEPS"
    echo "========================================"
    echo ""
    echo "1. Add to your .env file:"
    echo ""
    echo "   HELIX_SANDBOX_DATA=${ZFS_MOUNTPOINT}"
    echo ""
    echo "2. Restart the sandbox:"
    echo ""
    echo "   docker compose -f docker-compose.dev.yaml up -d sandbox-nvidia"
    echo ""
    echo "3. Verify it's working, then optionally remove old Docker volume data:"
    echo ""
    echo "   # After confirming everything works:"
    echo "   docker volume rm helix_sandbox-data  # or just leave it"
    echo ""
}

# Show current dedup status
show_dedup_status() {
    if zfs list "$ZFS_DATASET" &>/dev/null; then
        log_info "ZFS dedup/compression status:"
        echo ""
        zfs get dedupratio,compressratio,used,logicalused,referenced "$ZFS_DATASET"
        echo ""

        # Calculate savings
        USED=$(zfs get -Hp -o value used "$ZFS_DATASET")
        LOGICAL=$(zfs get -Hp -o value logicalused "$ZFS_DATASET")
        if [ "$LOGICAL" -gt 0 ]; then
            SAVINGS=$((LOGICAL - USED))
            SAVINGS_GB=$((SAVINGS / 1024 / 1024 / 1024))
            LOGICAL_GB=$((LOGICAL / 1024 / 1024 / 1024))
            USED_GB=$((USED / 1024 / 1024 / 1024))
            log_info "Space savings: ${SAVINGS_GB}GB saved (${LOGICAL_GB}GB logical -> ${USED_GB}GB actual)"
        fi
    fi
}

# Main
main() {
    echo "========================================"
    echo "Helix ZFS Dedup Workspaces Setup"
    echo "========================================"
    echo ""

    detect_environment
    check_prerequisites
    calculate_usage
    show_plan
    perform_migration
}

main "$@"
