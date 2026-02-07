#!/bin/bash
set -e

# Helix Desktop VM - Inside Setup Script
# Run this INSIDE the VM after first boot via cloud-init.
#
# Installs and configures:
# 1. ZFS 2.4.0 with dedup (from arter97 PPA) - for efficient Docker storage
# 2. Go 1.25 - for building helix-drm-manager and desktop-bridge
# 3. helix-drm-manager - DRM lease daemon for multi-desktop scanout
# 4. GDM disabled - needed for DRM lease approach
#
# Prerequisites:
# - Ubuntu 25.10 ARM64 VM with cloud-init complete
# - Docker installed (from cloud-init)
# - Helix repo cloned at ~/helix

HELIX_DIR="${HELIX_DIR:-$HOME/helix}"
GO_VERSION="1.25.0"

echo "================================================"
echo "Helix Desktop VM - Inside Setup"
echo "================================================"
echo ""

# Check we're running inside the VM
if [ "$(uname -m)" != "aarch64" ]; then
    echo "ERROR: This script must be run inside the ARM64 VM"
    exit 1
fi

# Check helix repo exists
if [ ! -d "$HELIX_DIR" ]; then
    echo "ERROR: Helix repo not found at $HELIX_DIR"
    echo "Clone it first:"
    echo "  git clone https://github.com/helixml/helix.git ~/helix"
    echo "  cd ~/helix && git checkout feature/macos-arm-desktop-port"
    exit 1
fi

echo "--- Step 1: Install ZFS 2.4.0 with dedup ---"
echo ""

ZFS_VERSION=$(zfs --version 2>/dev/null | head -1 | grep -oP '[\d.]+' || echo "not installed")
echo "Current ZFS: $ZFS_VERSION"

if ! echo "$ZFS_VERSION" | grep -q "^2\.4"; then
    echo "Installing ZFS 2.4.0 from arter97 PPA..."
    echo "(ZFS 2.3.x has dedup regression bug PR #17120 - 2.4.0 required)"

    sudo add-apt-repository -y ppa:arter97/zfs
    sudo apt-get update
    sudo apt-get install -y zfsutils-linux zfs-dkms

    echo "ZFS installed. A reboot is required to load the new kernel module."
    echo "After reboot, run this script again to continue setup."
    echo ""
    echo "  sudo reboot"
    echo "  # After reboot:"
    echo "  bash ~/helix/for-mac/scripts/setup-vm-inside.sh"

    # Check if this is a fresh install or upgrade
    if ! lsmod | grep -q zfs; then
        echo ""
        echo "ZFS kernel module not loaded - reboot required."
        exit 0
    fi
fi

# Verify ZFS is working
if command -v zfs &>/dev/null; then
    echo "ZFS version: $(zfs --version | head -1)"

    # Create ZFS pool on the dedicated data disk
    # The provision-vm.sh creates a second virtio disk at /dev/vdc for this
    if ! sudo zpool list helix 2>/dev/null; then
        # Find the data disk (second virtio disk, not the root disk)
        ZFS_DEV=""
        for dev in /dev/vdc /dev/vdb; do
            if [ -b "$dev" ] && ! mount | grep -q "$dev"; then
                ZFS_DEV="$dev"
                break
            fi
        done

        if [ -n "$ZFS_DEV" ]; then
            echo "Creating ZFS pool on $ZFS_DEV..."
            sudo zpool create -f helix "$ZFS_DEV"
            sudo zfs create -o dedup=on -o compression=lz4 -o atime=off helix/workspaces
            sudo zfs create -o compression=lz4 -o atime=off helix/docker
            echo "ZFS pool created with dedup workspaces"

            # Move Docker to ZFS for compression benefits
            if [ ! -L /var/lib/docker ] && [ -d /var/lib/docker ]; then
                sudo systemctl stop docker 2>/dev/null || true
                sudo mv /var/lib/docker /var/lib/docker.ext4-backup
                sudo ln -s /helix/docker /var/lib/docker
                sudo systemctl start docker 2>/dev/null || true
                echo "Docker storage moved to ZFS"
            fi
        else
            echo "No unmounted disk found for ZFS. If using provision-vm.sh,"
            echo "the second disk should be at /dev/vdc."
        fi
    else
        echo "ZFS pool 'helix' exists:"
        sudo zpool list helix
        sudo zfs list -r helix 2>/dev/null | head -5
    fi
fi

echo ""
echo "--- Step 2: Install Go $GO_VERSION ---"
echo ""

GO_CURRENT=$(go version 2>/dev/null | grep -oP '[\d.]+' || echo "not installed")
echo "Current Go: $GO_CURRENT"

if ! echo "$GO_CURRENT" | grep -q "^1\.25"; then
    echo "Installing Go $GO_VERSION..."
    curl -L -o /tmp/go.tar.gz "https://go.dev/dl/go${GO_VERSION}.linux-arm64.tar.gz"
    sudo rm -rf /usr/local/go
    sudo tar -C /usr/local -xzf /tmp/go.tar.gz
    rm /tmp/go.tar.gz

    # Add to PATH
    if ! grep -q "/usr/local/go/bin" ~/.bashrc; then
        echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
    fi
    export PATH=$PATH:/usr/local/go/bin

    echo "Go installed: $(go version)"
fi

echo ""
echo "--- Step 3: Build and install helix-drm-manager ---"
echo ""

cd "$HELIX_DIR/api"
echo "Building helix-drm-manager..."
CGO_ENABLED=0 /usr/local/go/bin/go build -o /tmp/helix-drm-manager ./cmd/helix-drm-manager/
sudo cp /tmp/helix-drm-manager /usr/local/bin/helix-drm-manager
sudo chmod +x /usr/local/bin/helix-drm-manager
echo "Installed: /usr/local/bin/helix-drm-manager"

# Create systemd service for helix-drm-manager
echo "Creating systemd service..."
sudo tee /etc/systemd/system/helix-drm-manager.service > /dev/null << 'SVCEOF'
[Unit]
Description=Helix DRM Lease Manager
Documentation=https://github.com/helixml/helix
After=multi-user.target
Wants=multi-user.target

[Service]
Type=simple
ExecStart=/usr/local/bin/helix-drm-manager
Restart=on-failure
RestartSec=5
# DRM master requires root (or CAP_SYS_ADMIN)
User=root
# Log to journal
StandardOutput=journal
StandardError=journal
SyslogIdentifier=helix-drm-manager

[Install]
WantedBy=multi-user.target
SVCEOF

sudo systemctl daemon-reload
sudo systemctl enable helix-drm-manager
sudo systemctl restart helix-drm-manager

echo "helix-drm-manager service enabled and started"
sleep 2
sudo systemctl status helix-drm-manager --no-pager | head -10

echo ""
echo "--- Step 4: Disable GDM ---"
echo ""

if systemctl is-active gdm &>/dev/null; then
    echo "Disabling GDM (needed for DRM lease approach)..."
    sudo systemctl disable gdm
    sudo systemctl stop gdm
    echo "GDM disabled. VM console uses Virtual-1 (text mode)."
else
    echo "GDM already disabled."
fi

echo ""
echo "--- Step 5: Docker image build ---"
echo ""

echo "To build the helix-ubuntu desktop image:"
echo "  cd ~/helix && docker build -f Dockerfile.ubuntu-helix -t helix-ubuntu:latest ."
echo ""
echo "This builds the container with GNOME, Zed, desktop-bridge, logind-stub,"
echo "and mutter-lease-launcher for the scanout pipeline."

echo ""
echo "================================================"
echo "Setup complete!"
echo "================================================"
echo ""
echo "Verify:"
echo "  zfs --version             # Should show 2.4.0"
echo "  go version                # Should show 1.25.x"
echo "  systemctl status helix-drm-manager  # Should be active"
echo "  ls /run/helix-drm.sock    # Should exist"
echo ""
echo "Test DRM lease:"
echo "  cd ~/helix/api && go run ./cmd/helix-drm-manager/ &"
echo "  # In another terminal:"
echo "  cd ~/helix/api && go build -o /tmp/drm-test ./cmd/scanout-stream-test/"
echo "  /tmp/drm-test"
echo ""
