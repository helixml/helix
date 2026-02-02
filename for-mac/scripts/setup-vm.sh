#!/bin/bash
set -e

# Helix Desktop VM Setup Script
# Downloads and configures an Ubuntu ARM64 VM for Helix Desktop

HELIX_DIR="$HOME/.helix"
VM_DIR="$HELIX_DIR/vm"
IMAGE_NAME="helix-ubuntu.qcow2"
IMAGE_PATH="$VM_DIR/$IMAGE_NAME"

# Ubuntu 24.04 ARM64 cloud image (more stable than 25.10 for now)
UBUNTU_URL="https://cloud-images.ubuntu.com/noble/current/noble-server-cloudimg-arm64.img"

echo "🚀 Helix Desktop VM Setup"
echo "========================="
echo ""

# Create directories
mkdir -p "$VM_DIR"

# Check if image already exists
if [ -f "$IMAGE_PATH" ]; then
    echo "⚠️  VM image already exists at $IMAGE_PATH"
    read -p "Do you want to replace it? [y/N] " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        echo "Keeping existing image."
        exit 0
    fi
    rm -f "$IMAGE_PATH"
fi

echo "📥 Downloading Ubuntu 24.04 ARM64 cloud image..."
curl -L -o "$VM_DIR/ubuntu-base.img" "$UBUNTU_URL"

echo "📦 Creating VM disk (32GB)..."
qemu-img create -f qcow2 -b "$VM_DIR/ubuntu-base.img" -F qcow2 "$IMAGE_PATH" 32G

echo "🔧 Creating cloud-init configuration..."

# Create cloud-init ISO for first boot configuration
CLOUD_INIT_DIR="$VM_DIR/cloud-init"
mkdir -p "$CLOUD_INIT_DIR"

# user-data
cat > "$CLOUD_INIT_DIR/user-data" << 'EOF'
#cloud-config
hostname: helix-vm
users:
  - name: helix
    sudo: ALL=(ALL) NOPASSWD:ALL
    shell: /bin/bash
    ssh_authorized_keys: []
    lock_passwd: false
    # Password: helix (change this!)
    passwd: $6$rounds=4096$random$QWJjZGVmZ2hpamtsbW5vcHFyc3R1dnd4eXo

package_update: true
package_upgrade: true

packages:
  - docker.io
  - docker-compose
  - curl
  - git
  - htop
  - net-tools

runcmd:
  - systemctl enable docker
  - systemctl start docker
  - usermod -aG docker helix
  - echo "Helix VM ready!" > /etc/motd

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
EOF

# meta-data
cat > "$CLOUD_INIT_DIR/meta-data" << EOF
instance-id: helix-vm-001
local-hostname: helix-vm
EOF

# Create cloud-init ISO
echo "💿 Creating cloud-init ISO..."
if command -v hdiutil &> /dev/null; then
    # macOS
    hdiutil makehybrid -o "$VM_DIR/cloud-init.iso" -hfs -joliet -iso -default-volume-name cidata "$CLOUD_INIT_DIR"
elif command -v mkisofs &> /dev/null; then
    mkisofs -output "$VM_DIR/cloud-init.iso" -volid cidata -joliet -rock "$CLOUD_INIT_DIR"
else
    echo "⚠️  Could not create cloud-init ISO. You may need to configure the VM manually."
fi

echo ""
echo "✅ VM setup complete!"
echo ""
echo "VM Image: $IMAGE_PATH"
echo "Cloud-init ISO: $VM_DIR/cloud-init.iso"
echo ""
echo "To start the VM manually with QEMU:"
echo ""
echo "  qemu-system-aarch64 \\"
echo "    -machine virt,accel=hvf \\"
echo "    -cpu host -smp 2 -m 4096 \\"
echo "    -drive file=$IMAGE_PATH,format=qcow2 \\"
echo "    -cdrom $VM_DIR/cloud-init.iso \\"
echo "    -device virtio-net-pci,netdev=net0 \\"
echo "    -netdev user,id=net0,hostfwd=tcp::2222-:22,hostfwd=tcp::8080-:8080 \\"
echo "    -nographic"
echo ""
echo "Or use UTM to create a VM with this disk image."
echo ""
echo "Default credentials:"
echo "  Username: helix"
echo "  Password: helix"
echo ""
