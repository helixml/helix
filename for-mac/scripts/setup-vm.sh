#!/bin/bash
set -e

# Helix Desktop VM Setup Script
# Creates an Ubuntu ARM64 VM image for macOS ARM Helix Desktop
#
# This script:
# 1. Downloads Ubuntu 25.10 ARM64 cloud image (needed for kernel 6.17+ virtio-gpu)
# 2. Creates a qcow2 disk image
# 3. Generates cloud-init config for first boot
#
# After first boot, run setup-vm-inside.sh inside the VM to install:
# - Docker, Go 1.25, ZFS 2.4.0 with dedup
# - helix-drm-manager (DRM lease daemon for multi-desktop scanout)
# - GDM disabled (needed for DRM lease approach)

HELIX_DIR="$HOME/.helix"
VM_DIR="$HELIX_DIR/vm"
IMAGE_NAME="helix-ubuntu.qcow2"
IMAGE_PATH="$VM_DIR/$IMAGE_NAME"
DISK_SIZE="${1:-256G}"  # Default 256GB, pass arg to override

# Ubuntu 25.10 ARM64 cloud image
# Kernel 6.17+ required for virtio-gpu multi-scanout support
UBUNTU_URL="https://cloud-images.ubuntu.com/questing/current/questing-server-cloudimg-arm64.img"

echo "Helix Desktop VM Setup"
echo "======================"
echo ""
echo "Disk size: $DISK_SIZE"
echo ""

# Create directories
mkdir -p "$VM_DIR"

# Check if image already exists
if [ -f "$IMAGE_PATH" ]; then
    echo "VM image already exists at $IMAGE_PATH"
    read -p "Do you want to replace it? [y/N] " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        echo "Keeping existing image."
        exit 0
    fi
    rm -f "$IMAGE_PATH"
fi

echo "Downloading Ubuntu 25.10 ARM64 cloud image..."
curl -L -o "$VM_DIR/ubuntu-base.img" "$UBUNTU_URL"

echo "Creating VM disk ($DISK_SIZE)..."
qemu-img create -f qcow2 -b "$VM_DIR/ubuntu-base.img" -F qcow2 "$IMAGE_PATH" "$DISK_SIZE"

echo "Creating cloud-init configuration..."

# Create cloud-init ISO for first boot configuration
CLOUD_INIT_DIR="$VM_DIR/cloud-init"
mkdir -p "$CLOUD_INIT_DIR"

# Detect SSH public key
SSH_KEY=""
for key in ~/.ssh/id_ed25519.pub ~/.ssh/id_rsa.pub; do
    if [ -f "$key" ]; then
        SSH_KEY=$(cat "$key")
        echo "Found SSH key: $key"
        break
    fi
done

# user-data
cat > "$CLOUD_INIT_DIR/user-data" << EOF
#cloud-config
hostname: helix-vm
manage_etc_hosts: true

users:
  - name: luke
    sudo: ALL=(ALL) NOPASSWD:ALL
    shell: /bin/bash
    groups: [docker, video, render]
    lock_passwd: false
    # Password: helix
    passwd: \$6\$rounds=4096\$helix\$uLHzj8JQHP0jHVKhgUgvHEq2oLm2BPxNJKBOGJ6u9t.H5/5Gz6w3Cq4YJx0qF5tN/g9TgVMHmjV/JcM.FN81
    ssh_authorized_keys:
      - ${SSH_KEY}

package_update: true
package_upgrade: false

packages:
  - docker.io
  - docker-compose-v2
  - curl
  - git
  - htop
  - net-tools
  - build-essential
  - linux-headers-generic
  - dkms
  - modetest
  - libdrm-dev

runcmd:
  # Docker setup
  - systemctl enable docker
  - systemctl start docker
  - usermod -aG docker luke
  # Disable GDM (needed for DRM lease approach - VM console uses Virtual-1)
  - systemctl disable gdm || true
  - systemctl stop gdm || true
  # Expand filesystem if disk was resized
  - growpart /dev/vda 2 || true
  - resize2fs /dev/vda2 || true
  # Create workspace directories
  - mkdir -p /tmp/workspace /tmp/work
  - chown luke:luke /tmp/workspace /tmp/work
  # Set MOTD
  - echo "Helix Desktop VM - run ~/helix/for-mac/scripts/setup-vm-inside.sh for full setup" > /etc/motd

write_files:
  - path: /etc/docker/daemon.json
    content: |
      {
        "storage-driver": "overlay2",
        "log-driver": "json-file",
        "log-opts": {
          "max-size": "10m",
          "max-file": "3"
        }
      }
  - path: /etc/ssh/sshd_config.d/helix.conf
    content: |
      PasswordAuthentication yes
      PubkeyAuthentication yes
      AcceptEnv HELIX_* GPU_*
EOF

# meta-data
cat > "$CLOUD_INIT_DIR/meta-data" << EOF
instance-id: helix-vm-001
local-hostname: helix-vm
EOF

# Create cloud-init ISO
echo "Creating cloud-init ISO..."
if command -v hdiutil &> /dev/null; then
    # macOS
    hdiutil makehybrid -o "$VM_DIR/cloud-init.iso" -hfs -joliet -iso -default-volume-name cidata "$CLOUD_INIT_DIR"
elif command -v mkisofs &> /dev/null; then
    mkisofs -output "$VM_DIR/cloud-init.iso" -volid cidata -joliet -rock "$CLOUD_INIT_DIR"
else
    echo "Could not create cloud-init ISO (install cdrtools or use hdiutil on macOS)"
    exit 1
fi

# Add helix-vm to SSH config if not already there
if ! grep -q "Host helix-vm" ~/.ssh/config 2>/dev/null; then
    echo "Adding helix-vm to ~/.ssh/config..."
    cat >> ~/.ssh/config << 'SSHEOF'

Host helix-vm
    HostName localhost
    Port 2222
    User luke
    StrictHostKeyChecking no
    UserKnownHostsFile /dev/null
    LogLevel ERROR
SSHEOF
fi

echo ""
echo "VM setup complete!"
echo ""
echo "VM Image: $IMAGE_PATH"
echo "Cloud-init ISO: $VM_DIR/cloud-init.iso"
echo "Disk size: $DISK_SIZE"
echo ""
echo "Next steps:"
echo "  1. Create a VM in UTM using the disk image and cloud-init ISO"
echo "     - CPU: 8+ cores, RAM: 16+ GB"
echo "     - Network: virtio-net-pci with port forward 2222->22"
echo "     - GPU: virtio-gpu-gl-pci with max_outputs=16"
echo "  2. Boot the VM (first boot takes a few minutes for cloud-init)"
echo "  3. SSH in: ssh helix-vm"
echo "  4. Clone helix and run the inside setup script:"
echo "     git clone https://github.com/helixml/helix.git ~/helix"
echo "     cd ~/helix && git checkout feature/macos-arm-desktop-port"
echo "     bash for-mac/scripts/setup-vm-inside.sh"
echo ""
echo "Default credentials:"
echo "  Username: luke"
echo "  Password: helix"
echo ""
