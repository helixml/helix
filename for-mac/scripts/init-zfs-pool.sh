#!/bin/bash
set -e

# =========================================================================
# Step 1: Import or create pool
# =========================================================================
if sudo zpool list helix 2>/dev/null; then
    echo 'ZFS pool helix already exists'
elif sudo zpool import -f helix 2>/dev/null; then
    echo 'Imported existing ZFS pool'
elif sudo zpool import 2>/dev/null | grep -q 'pool: helix'; then
    # Pool exists on a device but import failed for an unknown reason.
    # Refuse to fall through to zpool create, which would destroy user data.
    echo 'ERROR: ZFS pool helix exists but import failed. Manual intervention required.'
    exit 1
else
    # No existing pool — find the data disk and create a new one (first boot)
    DATA_DISK=""
    for disk in /dev/vdb /dev/vdc /dev/vdd; do
        if [ -b "$disk" ] && ! findmnt -rn -S "$disk" >/dev/null 2>&1; then
            DATA_DISK="$disk"
            break
        fi
    done
    if [ -z "$DATA_DISK" ]; then
        echo 'ERROR: No unmounted data disk found'
        exit 1
    fi
    echo "Creating ZFS pool on $DATA_DISK..."
    # Clear stale /helix from golden image (ZFS won't mount over non-empty dir)
    if mountpoint -q /helix 2>/dev/null; then
        echo 'ERROR: /helix is already a mountpoint, refusing to remove'
        exit 1
    fi
    sudo rm -rf /helix 2>/dev/null || true
    sudo mkdir -p /helix
    sudo zpool create -f -m /helix helix "$DATA_DISK"
fi

# Expand pool if disk was resized (no-op if already at full size)
sudo zpool online -e helix $(sudo zpool status -P helix 2>/dev/null | awk '/\/dev\//{print $1}' | head -1) 2>/dev/null || true

# =========================================================================
# Step 2: Create datasets
# =========================================================================

# Workspaces dataset (dedup + compression for user workspace data)
if ! sudo zfs list helix/workspaces 2>/dev/null; then
    echo 'Creating helix/workspaces dataset...'
    sudo zfs create -o dedup=on -o compression=lz4 -o atime=off -o mountpoint=/helix/workspaces helix/workspaces
fi

# Docker volumes dataset — persists user data (postgres, keycloak, etc.)
# across root disk upgrades. Mounted at /var/lib/docker/volumes/ so Docker
# named volumes survive while images stay on root disk (pre-baked).
if ! sudo zfs list helix/docker-volumes 2>/dev/null; then
    echo 'Creating helix/docker-volumes dataset...'
    sudo zfs create -o compression=lz4 -o atime=off -o mountpoint=/var/lib/docker/volumes helix/docker-volumes
fi
# Ensure mount exists even if dataset was already created (e.g., after reboot)
if ! mountpoint -q /var/lib/docker/volumes 2>/dev/null; then
    sudo mkdir -p /var/lib/docker/volumes
    sudo zfs mount helix/docker-volumes 2>/dev/null || true
fi

# Container Docker zvol — stores per-session inner dockerd data and BuildKit state.
# The sandbox's own Docker storage stays on the root disk (default named volume)
# so desktop images baked during provisioning persist without transfer.
# This zvol is for data that benefits from ZFS dedup+compression:
#   - Per-session inner dockerd (/helix/container-docker/sessions/{id}/docker/)
#   - BuildKit state (/helix/container-docker/buildkit/)
# Hydra bind-mounts these paths into desktop containers and the BuildKit container.
ZVOL_SIZE=200G
ZVOL_DEV=/dev/zvol/helix/container-docker
if ! sudo zfs list helix/container-docker 2>/dev/null; then
    # Migrate from old name if it exists
    if sudo zfs list helix/sandbox-docker 2>/dev/null; then
        echo "Renaming helix/sandbox-docker zvol to helix/container-docker..."
        sudo umount /helix/sandbox-docker 2>/dev/null || true
        sudo zfs rename helix/sandbox-docker helix/container-docker
    else
        echo "Creating helix/container-docker zvol (${ZVOL_SIZE}, dedup + compression)..."
        sudo zfs create -V "$ZVOL_SIZE" -s -o dedup=on -o compression=lz4 helix/container-docker
        # Wait for device node
        for i in $(seq 1 10); do [ -e "$ZVOL_DEV" ] && break; sleep 1; done
        if [ ! -e "$ZVOL_DEV" ]; then
            echo "ERROR: $ZVOL_DEV did not appear after 10s"
            exit 1
        fi
        echo 'Formatting container-docker zvol as ext4...'
        sudo mkfs.ext4 -q -L container-docker "$ZVOL_DEV"
    fi
fi
# Mount the zvol
if ! mountpoint -q /helix/container-docker 2>/dev/null; then
    sudo mkdir -p /helix/container-docker
    if [ -e "$ZVOL_DEV" ]; then
        sudo mount "$ZVOL_DEV" /helix/container-docker
    fi
fi
# Create subdirectories for Hydra
sudo mkdir -p /helix/container-docker/sessions
sudo mkdir -p /helix/container-docker/buildkit

# Config dataset (persistent state surviving root disk swaps)
if ! sudo zfs list helix/config 2>/dev/null; then
    echo 'Creating helix/config dataset...'
    sudo zfs create -o compression=lz4 -o mountpoint=/helix/config helix/config
fi

# =========================================================================
# Step 3: Persist / restore config (SSH keys, machine-id, authorized_keys)
# =========================================================================

# SSH host keys
if [ ! -d /helix/config/ssh ]; then
    # First boot: copy keys TO config
    echo 'Persisting SSH host keys to /helix/config/ssh/...'
    sudo mkdir -p /helix/config/ssh
    sudo cp /etc/ssh/ssh_host_* /helix/config/ssh/
    # Also persist authorized_keys if they exist
    if [ -f /home/ubuntu/.ssh/authorized_keys ]; then
        sudo cp /home/ubuntu/.ssh/authorized_keys /helix/config/ssh/authorized_keys
    fi
else
    # Upgrade boot: restore keys FROM config
    echo 'Restoring SSH host keys from /helix/config/ssh/...'
    sudo cp /helix/config/ssh/ssh_host_* /etc/ssh/
    sudo chmod 600 /etc/ssh/ssh_host_*_key
    sudo chmod 644 /etc/ssh/ssh_host_*_key.pub
    sudo systemctl restart sshd 2>/dev/null || true
    # Restore authorized_keys
    if [ -f /helix/config/ssh/authorized_keys ]; then
        mkdir -p /home/ubuntu/.ssh
        sudo cp /helix/config/ssh/authorized_keys /home/ubuntu/.ssh/authorized_keys
        sudo chmod 600 /home/ubuntu/.ssh/authorized_keys
        sudo chown ubuntu:ubuntu /home/ubuntu/.ssh/authorized_keys
    fi
fi

# Machine ID
if [ ! -f /helix/config/machine-id ]; then
    sudo cp /etc/machine-id /helix/config/machine-id
else
    sudo cp /helix/config/machine-id /etc/machine-id
    sudo systemd-machine-id-commit 2>/dev/null || true
fi

# Helix .env.vm
if [ -f /home/ubuntu/helix/.env.vm ] && [ ! -f /helix/config/env.vm ]; then
    sudo cp /home/ubuntu/helix/.env.vm /helix/config/env.vm
elif [ -f /helix/config/env.vm ] && [ ! -f /home/ubuntu/helix/.env.vm ]; then
    sudo mkdir -p /home/ubuntu/helix
    sudo cp /helix/config/env.vm /home/ubuntu/helix/.env.vm
    sudo chown ubuntu:ubuntu /home/ubuntu/helix/.env.vm
fi

# Sandbox Docker storage — bind mount on root disk (NOT a Docker named volume)
# so pre-baked desktop images survive the ZFS mount over /var/lib/docker/volumes/.
sudo mkdir -p /var/lib/helix-sandbox-docker

# =========================================================================
# Step 4: Ensure Docker is running
# =========================================================================
# Host Docker runs on root disk (images pre-baked during provisioning).
# Only sandbox inner Docker and workspace data use ZFS.
if ! systemctl is-active docker >/dev/null 2>&1; then
    echo 'Starting Docker...'
    sudo systemctl start docker
else
    echo 'Docker already running'
fi

echo 'ZFS storage ready'
