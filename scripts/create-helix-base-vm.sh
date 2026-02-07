#!/bin/bash
#
# Create reproducible Helix development VM base image for macOS ARM64
#
# This script creates a UTM VM with:
# - Ubuntu 25.10 Server ARM64 (automated installation)
# - ext4 root partition (200GB, for system + Docker)
# - ZFS 2.4.0+ pool with dedup (remaining space, for workspaces)
# - GPU drivers for virtio-gpu-gl-pci
# - Docker, Go, Node.js, and development tools
#
# Docker uses default ext4 storage. Set HELIX_SANDBOX_DATA=/helix/workspaces
# in .env to use ZFS dedup for workspace storage (17.89x compression ratio).
#
# Usage: ./scripts/create-helix-base-vm.sh [OPTIONS]
#
# Options:
#   --name NAME         VM name (default: "Helix Base")
#   --disk-size SIZE    Disk size in GB (default: 1000)
#   --ram SIZE          RAM in GB (default: 64)
#   --cpus COUNT        CPU cores (default: 20)
#   --output PATH       Output location for VM (default: ~/Downloads)
#

set -euo pipefail

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
BLUE='\033[0;34m'
NC='\033[0m'

# Defaults
VM_NAME="Helix Base"
DISK_SIZE_GB=1000
RAM_GB=64
CPU_CORES=20
OUTPUT_DIR="$HOME/Downloads"

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --name) VM_NAME="$2"; shift 2 ;;
        --disk-size) DISK_SIZE_GB="$2"; shift 2 ;;
        --ram) RAM_GB="$2"; shift 2 ;;
        --cpus) CPU_CORES="$2"; shift 2 ;;
        --output) OUTPUT_DIR="$2"; shift 2 ;;
        *) echo "Unknown option: $1"; exit 1 ;;
    esac
done

echo -e "${BLUE}╔════════════════════════════════════════════════════════════╗${NC}"
echo -e "${BLUE}║  Creating Helix Development VM Base Image                 ║${NC}"
echo -e "${BLUE}╚════════════════════════════════════════════════════════════╝${NC}"
echo ""
echo "Configuration:"
echo "  VM Name: $VM_NAME"
echo "  Disk: ${DISK_SIZE_GB}GB"
echo "  RAM: ${RAM_GB}GB"
echo "  CPUs: $CPU_CORES cores"
echo "  Output: $OUTPUT_DIR"
echo ""

# Check prerequisites
if [ ! -d "/Applications/UTM.app" ]; then
    echo -e "${RED}❌ UTM not found. Install from: https://mac.getutm.app/${NC}"
    exit 1
fi

if [ "$(uname -m)" != "arm64" ]; then
    echo -e "${RED}❌ This script requires Apple Silicon${NC}"
    exit 1
fi

# Ubuntu 25.10 (Plucky Puffin)
UBUNTU_VERSION="25.10"
UBUNTU_ISO="ubuntu-${UBUNTU_VERSION}-live-server-arm64.iso"
UBUNTU_URL="https://cdimage.ubuntu.com/releases/${UBUNTU_VERSION}/release/${UBUNTU_ISO}"

echo -e "${YELLOW}📥 Downloading Ubuntu ${UBUNTU_VERSION} Server ARM64...${NC}"
ISO_PATH="$OUTPUT_DIR/$UBUNTU_ISO"

if [ ! -f "$ISO_PATH" ]; then
    curl -L --progress-bar -o "$ISO_PATH" "$UBUNTU_URL"
    echo -e "${GREEN}✓ Downloaded: $ISO_PATH${NC}"
else
    echo -e "${GREEN}✓ Using existing ISO: $ISO_PATH${NC}"
fi

# Create cloud-init autoinstall configuration
CLOUD_INIT_DIR="$OUTPUT_DIR/helix-cloud-init"
mkdir -p "$CLOUD_INIT_DIR"

echo -e "${YELLOW}⚙️  Creating autoinstall configuration...${NC}"

# meta-data (empty but required)
cat > "$CLOUD_INIT_DIR/meta-data" <<'EOF'
instance-id: helix-base-vm
local-hostname: helix-dev
EOF

# user-data (autoinstall configuration)
cat > "$CLOUD_INIT_DIR/user-data" <<'EOF'
#cloud-config
autoinstall:
  version: 1

  # Locale and keyboard
  locale: en_US.UTF-8
  keyboard:
    layout: us

  # Network (DHCP on first interface)
  network:
    version: 2
    ethernets:
      enp0s1:
        dhcp4: true

  # Storage layout: ext4 root + ZFS pool
  storage:
    layout:
      name: custom
    config:
      # Primary disk
      - type: disk
        id: disk0
        ptable: gpt
        path: /dev/vda
        wipe: superblock

      # BIOS boot partition
      - type: partition
        id: partition-bios
        device: disk0
        size: 1M
        flag: bios_grub

      # EFI partition
      - type: partition
        id: partition-efi
        device: disk0
        size: 512M
        flag: boot

      - type: format
        id: format-efi
        volume: partition-efi
        fstype: fat32

      - type: mount
        id: mount-efi
        device: format-efi
        path: /boot/efi

      # Root partition (ext4, 200GB)
      - type: partition
        id: partition-root
        device: disk0
        size: 200G

      - type: format
        id: format-root
        volume: partition-root
        fstype: ext4

      - type: mount
        id: mount-root
        device: format-root
        path: /

      # ZFS partition (remaining space)
      - type: partition
        id: partition-zfs
        device: disk0
        size: -1

      # Note: ZFS setup happens in late-commands (can't create pool in autoinstall)

  # Identity
  identity:
    hostname: helix-dev
    username: helix
    password: "$6$rounds=4096$SaltSaltSalt$YhQjWwrAFxIj6VqDlKjXcE9x8nUQqKC0rM5p0cGMJ5K7k4gx1z2y3P4Q5R6S7T8U9V0W1X2Y3Z4a5b6c7d8e9f0."  # "helix"

  # SSH
  ssh:
    install-server: true
    allow-pw: true

  # Packages to install
  packages:
    - build-essential
    - git
    - curl
    - wget
    - vim
    - tmux
    - htop
    - iotop
    - net-tools
    - dnsutils
    - ca-certificates
    - gnupg
    - lsb-release
    - software-properties-common
    # GPU/Graphics
    - mesa-utils
    - mesa-vulkan-drivers
    - vulkan-tools
    - libgl1-mesa-dri
    - libglx-mesa0
    - libglapi-mesa
    - libgbm1
    # Will install ZFS 2.4.0 in late-commands

  # Commands to run after installation
  late-commands:
    # Add arter97 ZFS PPA and install ZFS 2.4.0
    - curtin in-target --target=/target -- add-apt-repository -y ppa:arter97/zfs
    - curtin in-target --target=/target -- apt-get update
    - curtin in-target --target=/target -- apt-get install -y zfsutils-linux zfs-dkms

    # Create ZFS pool on the partition
    - curtin in-target --target=/target -- zpool create -f -o ashift=12 helix /dev/vda3
    - curtin in-target --target=/target -- zfs set compression=lz4 helix
    - curtin in-target --target=/target -- zfs set atime=off helix

    # Create ZFS dataset for workspaces with deduplication
    - curtin in-target --target=/target -- zfs create -o dedup=on -o compression=lz4 helix/workspaces

    # Set ZFS to auto-mount on boot
    - curtin in-target --target=/target -- zfs set mountpoint=/helix helix
    - curtin in-target --target=/target -- systemctl enable zfs-import-cache.service zfs-mount.service

    # Install Docker (uses default ext4 storage)
    - curtin in-target --target=/target -- sh -c 'curl -fsSL https://download.docker.com/linux/ubuntu/gpg | gpg --dearmor -o /usr/share/keyrings/docker-archive-keyring.gpg'
    - curtin in-target --target=/target -- sh -c 'echo "deb [arch=arm64 signed-by=/usr/share/keyrings/docker-archive-keyring.gpg] https://download.docker.com/linux/ubuntu $(lsb_release -cs) stable" > /etc/apt/sources.list.d/docker.list'
    - curtin in-target --target=/target -- apt-get update
    - curtin in-target --target=/target -- apt-get install -y docker-ce docker-ce-cli containerd.io docker-compose-plugin
    - curtin in-target --target=/target -- usermod -aG docker helix

    # Install Go (latest)
    - curtin in-target --target=/target -- sh -c 'curl -L https://go.dev/dl/go1.23.6.linux-arm64.tar.gz | tar -C /usr/local -xzf -'
    - curtin in-target --target=/target -- sh -c 'echo "export PATH=\$PATH:/usr/local/go/bin" >> /etc/profile.d/go.sh'

    # Install Node.js (LTS)
    - curtin in-target --target=/target -- sh -c 'curl -fsSL https://deb.nodesource.com/setup_lts.x | bash -'
    - curtin in-target --target=/target -- apt-get install -y nodejs

    # Create helix workspace directory
    - curtin in-target --target=/target -- mkdir -p /helix/workspaces
    - curtin in-target --target=/target -- chown helix:helix /helix/workspaces

    # Cleanup
    - curtin in-target --target=/target -- apt-get clean
    - curtin in-target --target=/target -- rm -rf /var/lib/apt/lists/*

  # Power off after installation (we'll create an image from this)
  shutdown: poweroff
EOF

echo -e "${GREEN}✓ Created cloud-init configuration${NC}"

# Create cloud-init ISO
CLOUD_INIT_ISO="$OUTPUT_DIR/helix-cloud-init.iso"
echo -e "${YELLOW}💿 Creating cloud-init ISO...${NC}"

if command -v genisoimage &> /dev/null; then
    genisoimage -output "$CLOUD_INIT_ISO" -volid cidata -joliet -rock "$CLOUD_INIT_DIR/user-data" "$CLOUD_INIT_DIR/meta-data"
elif command -v mkisofs &> /dev/null; then
    mkisofs -output "$CLOUD_INIT_ISO" -volid cidata -joliet -rock "$CLOUD_INIT_DIR/user-data" "$CLOUD_INIT_DIR/meta-data"
else
    echo -e "${RED}❌ Neither genisoimage nor mkisofs found. Install with: brew install cdrtools${NC}"
    exit 1
fi

echo -e "${GREEN}✓ Created: $CLOUD_INIT_ISO${NC}"

# Generate UUIDs
VM_UUID=$(uuidgen)
DISK_UUID=$(uuidgen)
UBUNTU_ISO_UUID=$(uuidgen)
CLOUD_INIT_UUID=$(uuidgen)

# Create VM directory
VM_DIR="$OUTPUT_DIR/${VM_NAME}.utm"
DATA_DIR="$VM_DIR/Data"
IMAGES_DIR="$VM_DIR/Images"

if [ -d "$VM_DIR" ]; then
    echo -e "${RED}❌ VM already exists: $VM_DIR${NC}"
    echo "Remove it first or choose a different name"
    exit 1
fi

mkdir -p "$DATA_DIR" "$IMAGES_DIR"

# Create disk image
DISK_PATH="$DATA_DIR/${DISK_UUID}.qcow2"
echo -e "${YELLOW}💾 Creating ${DISK_SIZE_GB}GB disk image...${NC}"
/Applications/UTM.app/Contents/Frameworks/qemu-img.framework/Versions/A/qemu-img create -f qcow2 "$DISK_PATH" "${DISK_SIZE_GB}G" > /dev/null
echo -e "${GREEN}✓ Created: $DISK_PATH${NC}"

# Copy EFI vars
EFI_VARS_DST="$DATA_DIR/efi_vars.fd"
if [ -f "/Applications/UTM.app/Contents/Resources/qemu/edk2-arm-vars.fd" ]; then
    cp "/Applications/UTM.app/Contents/Resources/qemu/edk2-arm-vars.fd" "$EFI_VARS_DST"
fi

# Generate MAC address
MAC_ADDRESS=$(printf '52:42:%02X:%02X:%02X:%02X' $((RANDOM%256)) $((RANDOM%256)) $((RANDOM%256)) $((RANDOM%256)))

# Create config.plist
echo -e "${YELLOW}⚙️  Creating VM configuration...${NC}"
cat > "$VM_DIR/config.plist" <<PLIST_EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Backend</key>
	<string>QEMU</string>
	<key>ConfigurationVersion</key>
	<integer>4</integer>
	<key>Display</key>
	<array>
		<dict>
			<key>Hardware</key>
			<string>virtio-gpu-gl-pci</string>
			<key>DynamicResolution</key>
			<true/>
		</dict>
	</array>
	<key>Drive</key>
	<array>
		<dict>
			<key>Identifier</key>
			<string>$UBUNTU_ISO_UUID</string>
			<key>ImageType</key>
			<string>CD</string>
			<key>Interface</key>
			<string>USB</string>
			<key>InterfaceVersion</key>
			<integer>1</integer>
			<key>ReadOnly</key>
			<true/>
		</dict>
		<dict>
			<key>Identifier</key>
			<string>$CLOUD_INIT_UUID</string>
			<key>ImageType</key>
			<string>CD</string>
			<key>Interface</key>
			<string>USB</string>
			<key>InterfaceVersion</key>
			<integer>1</integer>
			<key>ReadOnly</key>
			<true/>
		</dict>
		<dict>
			<key>Identifier</key>
			<string>$DISK_UUID</string>
			<key>ImageName</key>
			<string>${DISK_UUID}.qcow2</string>
			<key>ImageType</key>
			<string>Disk</string>
			<key>Interface</key>
			<string>VirtIO</string>
			<key>InterfaceVersion</key>
			<integer>1</integer>
			<key>ReadOnly</key>
			<false/>
		</dict>
	</array>
	<key>Information</key>
	<dict>
		<key>Name</key>
		<string>$VM_NAME</string>
		<key>UUID</key>
		<string>$VM_UUID</string>
	</dict>
	<key>Network</key>
	<array>
		<dict>
			<key>Hardware</key>
			<string>virtio-net-pci</string>
			<key>MacAddress</key>
			<string>$MAC_ADDRESS</string>
			<key>Mode</key>
			<string>Emulated</string>
			<key>PortForward</key>
			<array>
				<dict>
					<key>GuestAddress</key>
					<string>10.0.2.15</string>
					<key>GuestPort</key>
					<integer>22</integer>
					<key>HostAddress</key>
					<string>127.0.0.1</string>
					<key>HostPort</key>
					<integer>2222</integer>
					<key>Protocol</key>
					<string>TCP</string>
				</dict>
			</array>
		</dict>
	</array>
	<key>QEMU</key>
	<dict>
		<key>Hypervisor</key>
		<true/>
		<key>UEFIBoot</key>
		<true/>
	</dict>
	<key>Sharing</key>
	<dict>
		<key>ClipboardSharing</key>
		<true/>
	</dict>
	<key>System</key>
	<dict>
		<key>Architecture</key>
		<string>aarch64</string>
		<key>CPUCount</key>
		<integer>$CPU_CORES</integer>
		<key>MemorySize</key>
		<integer>$(($RAM_GB * 1024))</integer>
		<key>Target</key>
		<string>aarch64</string>
	</dict>
</dict>
</plist>
PLIST_EOF

# Create ISO bookmarks
for iso_data in "$UBUNTU_ISO_UUID:$ISO_PATH" "$CLOUD_INIT_UUID:$CLOUD_INIT_ISO"; do
    iso_uuid="${iso_data%%:*}"
    iso_path="${iso_data##*:}"
    cat > "$IMAGES_DIR/${iso_uuid}.plist" <<BOOKMARK_EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>IsExternal</key>
	<true/>
	<key>Path</key>
	<string>$iso_path</string>
</dict>
</plist>
BOOKMARK_EOF
done

echo -e "${GREEN}✓ VM created: $VM_DIR${NC}"
echo ""
echo -e "${BLUE}╔════════════════════════════════════════════════════════════╗${NC}"
echo -e "${BLUE}║  Installation Instructions                                ║${NC}"
echo -e "${BLUE}╚════════════════════════════════════════════════════════════╝${NC}"
echo ""
echo "1. Open UTM.app and import the VM from:"
echo "   $VM_DIR"
echo ""
echo "2. Start the VM - Ubuntu will install automatically (15-20 min)"
echo "   - Username: helix"
echo "   - Password: helix"
echo ""
echo "3. After installation completes, the VM will power off"
echo ""
echo "4. Remove the ISOs from the VM (boot from disk only)"
echo ""
echo "5. Start the VM and verify ZFS setup:"
echo "   ssh -p 2222 helix@localhost"
echo "   zfs list"
echo "   zpool status"
echo ""
echo "Expected ZFS layout:"
echo "  helix             - Root pool"
echo "  helix/workspaces  - Dedup enabled (for Helix workspace storage)"
echo ""
echo "6. Configure Helix to use ZFS workspaces:"
echo "   Add to .env: HELIX_SANDBOX_DATA=/helix/workspaces"
echo ""
echo "7. This is now your base image - create snapshots/clones as needed"
echo ""
