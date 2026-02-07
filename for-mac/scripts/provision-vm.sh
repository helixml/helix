#!/bin/bash
set -euo pipefail

# =============================================================================
# Helix Desktop VM - Automated Provisioning
# =============================================================================
#
# Creates a fully configured Ubuntu 25.10 ARM64 VM for Helix Desktop streaming.
# Runs on macOS with Homebrew QEMU for initial setup, then creates UTM bundle.
#
# What this script does:
#   1. Downloads Ubuntu 25.10 cloud image
#   2. Creates qcow2 disk + cloud-init seed
#   3. Boots headless VM with QEMU for provisioning
#   4. Waits for cloud-init, then SSHs in to run setup
#   5. Installs: Docker, Go 1.25, ZFS 2.4.0, helix-drm-manager
#   6. Clones helix repo, builds desktop Docker image
#   7. Sets up docker-compose for Helix control plane
#   8. Creates UTM .utm bundle ready to launch
#
# Prerequisites:
#   brew install qemu  (for initial provisioning)
#   Custom QEMU in UTM.app (for production with scanout pipeline)
#
# Usage:
#   ./provision-vm.sh [--disk-size 256G] [--cpus 8] [--memory 16384]
#   ./provision-vm.sh --resume   # Resume from last step if interrupted

# =============================================================================
# Configuration
# =============================================================================

VM_NAME="helix-desktop"
VM_DIR="${HOME}/.helix/vm/${VM_NAME}"
DISK_SIZE="256G"
CPUS=8
MEMORY_MB=16384
SSH_PORT="${HELIX_VM_SSH_PORT:-2223}"  # Use 2223 during provisioning to avoid conflicts
VM_USER="luke"
VM_PASS="helix"
GO_VERSION="1.25.0"

# Ubuntu 25.10 (Questing) - kernel 6.17+ for virtio-gpu multi-scanout
UBUNTU_URL="https://cloud-images.ubuntu.com/questing/current/questing-server-cloudimg-arm64.img"
UBUNTU_IMG="ubuntu-cloud.img"

# UEFI firmware (from Homebrew QEMU)
EFI_CODE="/opt/homebrew/share/qemu/edk2-aarch64-code.fd"
EFI_VARS_TEMPLATE="/opt/homebrew/share/qemu/edk2-arm-vars.fd"

# Parse arguments
RESUME=false
while [[ $# -gt 0 ]]; do
    case $1 in
        --disk-size) DISK_SIZE="$2"; shift 2 ;;
        --cpus) CPUS="$2"; shift 2 ;;
        --memory) MEMORY_MB="$2"; shift 2 ;;
        --resume) RESUME=true; shift ;;
        *) echo "Unknown option: $1"; exit 1 ;;
    esac
done

# State tracking (for resumability)
STATE_FILE="${VM_DIR}/.provision-state"
mark_step() { echo "$1" >> "$STATE_FILE"; }
step_done() { grep -qx "$1" "$STATE_FILE" 2>/dev/null; }

log() { echo "[$(date +%H:%M:%S)] $*"; }

# =============================================================================
# Step 1: Prerequisites
# =============================================================================

log "=== Helix Desktop VM Provisioning ==="
log "Disk: $DISK_SIZE, CPUs: $CPUS, Memory: ${MEMORY_MB}MB"

if ! command -v qemu-system-aarch64 &>/dev/null; then
    log "ERROR: qemu-system-aarch64 not found. Install with: brew install qemu"
    exit 1
fi

if [ ! -f "$EFI_CODE" ]; then
    log "ERROR: UEFI firmware not found at $EFI_CODE"
    log "Install with: brew install qemu"
    exit 1
fi

mkdir -p "$VM_DIR"
if [ "$RESUME" = false ]; then
    rm -f "$STATE_FILE"
fi

# =============================================================================
# Step 2: Download Ubuntu cloud image
# =============================================================================

if ! step_done "download"; then
    if [ ! -f "${VM_DIR}/${UBUNTU_IMG}" ]; then
        log "Downloading Ubuntu 25.10 ARM64 cloud image..."
        curl -L -o "${VM_DIR}/${UBUNTU_IMG}" "$UBUNTU_URL"
    else
        log "Ubuntu cloud image already downloaded"
    fi
    mark_step "download"
fi

# =============================================================================
# Step 3: Create disk image
# =============================================================================

if ! step_done "disk"; then
    log "Creating ${DISK_SIZE} qcow2 disk image..."
    qemu-img create -f qcow2 "${VM_DIR}/disk.qcow2" "$DISK_SIZE"

    # Expand the cloud image onto our disk
    log "Importing cloud image into disk..."
    # Use virt-resize if available, otherwise copy and resize
    cp "${VM_DIR}/${UBUNTU_IMG}" "${VM_DIR}/disk.qcow2.tmp"
    qemu-img resize "${VM_DIR}/disk.qcow2.tmp" "$DISK_SIZE"
    mv "${VM_DIR}/disk.qcow2.tmp" "${VM_DIR}/disk.qcow2"

    # Copy EFI vars template
    cp "$EFI_VARS_TEMPLATE" "${VM_DIR}/efi_vars.fd"

    mark_step "disk"
fi

# =============================================================================
# Step 4: Create cloud-init seed
# =============================================================================

if ! step_done "cloudinit"; then
    log "Creating cloud-init configuration..."

    SEED_DIR="${VM_DIR}/seed"
    mkdir -p "$SEED_DIR"

    # Detect SSH public key
    SSH_KEY=""
    for keyfile in ~/.ssh/id_ed25519.pub ~/.ssh/id_rsa.pub; do
        if [ -f "$keyfile" ]; then
            SSH_KEY=$(cat "$keyfile")
            log "Using SSH key: $keyfile"
            break
        fi
    done

    cat > "${SEED_DIR}/user-data" << USERDATA
#cloud-config
hostname: helix-vm
manage_etc_hosts: true

users:
  - name: ${VM_USER}
    sudo: ALL=(ALL) NOPASSWD:ALL
    shell: /bin/bash
    groups: [docker, video, render]
    lock_passwd: false
    plain_text_passwd: ${VM_PASS}
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
  - python3-websockets
  - openssh-server

runcmd:
  - systemctl enable docker
  - systemctl start docker
  - usermod -aG docker ${VM_USER}
  - systemctl disable gdm || true
  - systemctl stop gdm || true
  - growpart /dev/vda 1 || growpart /dev/vda 2 || true
  - resize2fs /dev/vda1 || resize2fs /dev/vda2 || true
  - touch /var/lib/cloud/instance/provision-ready

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
USERDATA

    cat > "${SEED_DIR}/meta-data" << METADATA
instance-id: helix-vm-$(date +%s)
local-hostname: helix-vm
METADATA

    # Create seed ISO using hdiutil (macOS native)
    # Cloud-init expects a volume labeled "cidata"
    log "Creating cloud-init seed ISO..."
    if command -v mkisofs &>/dev/null; then
        mkisofs -output "${VM_DIR}/seed.iso" -volid cidata -joliet -rock "$SEED_DIR"
    else
        # Use hdiutil on macOS
        hdiutil makehybrid -o "${VM_DIR}/seed.iso" \
            -hfs -joliet -iso \
            -default-volume-name cidata \
            "$SEED_DIR"
    fi

    mark_step "cloudinit"
fi

# =============================================================================
# Step 5: Boot VM for provisioning
# =============================================================================

QEMU_PID=""
cleanup() {
    if [ -n "$QEMU_PID" ] && kill -0 "$QEMU_PID" 2>/dev/null; then
        log "Shutting down QEMU (PID $QEMU_PID)..."
        kill "$QEMU_PID" 2>/dev/null || true
        wait "$QEMU_PID" 2>/dev/null || true
    fi
}
trap cleanup EXIT

if ! step_done "boot_setup"; then
    log "Booting VM with QEMU for provisioning..."
    log "(This uses homebrew QEMU - no GPU, headless mode)"

    # Create a FAT seed disk for cloud-init (more reliable than ISO on UEFI)
    # cloud-init NoCloud looks for a filesystem labeled "cidata"
    SEED_DISK="${VM_DIR}/seed-fat.img"
    if [ ! -f "$SEED_DISK" ]; then
        if ! command -v mformat &>/dev/null || ! command -v mcopy &>/dev/null; then
            log "ERROR: mtools required for FAT seed disk. Install with: brew install mtools"
            exit 1
        fi
        dd if=/dev/zero of="$SEED_DISK" bs=1k count=2048 2>/dev/null
        mformat -i "$SEED_DISK" -v CIDATA -t 2 -h 64 -s 32 ::
        mcopy -i "$SEED_DISK" "${VM_DIR}/seed/user-data" "::user-data"
        mcopy -i "$SEED_DISK" "${VM_DIR}/seed/meta-data" "::meta-data"
        log "Created FAT seed disk with cloud-init data"
    fi

    # Start QEMU in background
    qemu-system-aarch64 \
        -machine virt,accel=hvf \
        -cpu host \
        -smp "$CPUS" \
        -m "$MEMORY_MB" \
        -drive if=pflash,format=raw,file="$EFI_CODE",readonly=on \
        -drive if=pflash,format=raw,file="${VM_DIR}/efi_vars.fd" \
        -drive file="${VM_DIR}/disk.qcow2",format=qcow2,if=virtio \
        -drive file="${SEED_DISK}",format=raw,if=virtio,readonly=on \
        -device virtio-net-pci,netdev=net0 \
        -netdev user,id=net0,hostfwd=tcp::${SSH_PORT}-:22 \
        -smbios type=1,serial=ds=nocloud \
        -nographic \
        -serial mon:stdio \
        > "${VM_DIR}/qemu-boot.log" 2>&1 &
    QEMU_PID=$!

    log "QEMU started (PID $QEMU_PID), waiting for cloud-init..."

    # Wait for SSH to become available (cloud-init takes 2-5 minutes)
    MAX_WAIT=600
    ELAPSED=0
    while [ $ELAPSED -lt $MAX_WAIT ]; do
        if ssh -o ConnectTimeout=3 -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null \
               -p "$SSH_PORT" "${VM_USER}@localhost" "test -f /var/lib/cloud/instance/provision-ready" 2>/dev/null; then
            log "Cloud-init complete! VM is ready for setup."
            break
        fi
        sleep 10
        ELAPSED=$((ELAPSED + 10))
        if [ $((ELAPSED % 60)) -eq 0 ]; then
            log "Still waiting for cloud-init... (${ELAPSED}s)"
        fi
    done

    if [ $ELAPSED -ge $MAX_WAIT ]; then
        log "ERROR: Timed out waiting for cloud-init (${MAX_WAIT}s)"
        log "Check ${VM_DIR}/qemu-boot.log for details"
        exit 1
    fi

    mark_step "boot_setup"
fi

# =============================================================================
# Step 6: Run inside setup via SSH
# =============================================================================

SSH_CMD="ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -p $SSH_PORT ${VM_USER}@localhost"

run_ssh() {
    $SSH_CMD "$@"
}

if ! step_done "install_zfs"; then
    log "Installing ZFS 2.4.0 from arter97 PPA..."
    run_ssh "sudo add-apt-repository -y ppa:arter97/zfs && sudo apt-get update && sudo apt-get install -y zfsutils-linux" || {
        log "ZFS install may need a reboot for DKMS. Continuing..."
    }
    mark_step "install_zfs"
fi

if ! step_done "install_go"; then
    log "Installing Go ${GO_VERSION}..."
    run_ssh "curl -L -o /tmp/go.tar.gz 'https://go.dev/dl/go${GO_VERSION}.linux-arm64.tar.gz' && \
             sudo rm -rf /usr/local/go && \
             sudo tar -C /usr/local -xzf /tmp/go.tar.gz && \
             rm /tmp/go.tar.gz && \
             echo 'export PATH=\$PATH:/usr/local/go/bin' >> ~/.bashrc"
    run_ssh "/usr/local/go/bin/go version"
    mark_step "install_go"
fi

if ! step_done "clone_helix"; then
    log "Cloning helix repository..."
    run_ssh "git clone https://github.com/helixml/helix.git ~/helix 2>/dev/null || (cd ~/helix && git pull)"
    run_ssh "cd ~/helix && git checkout feature/macos-arm-desktop-port"
    mark_step "clone_helix"
fi

if ! step_done "build_drm_manager"; then
    log "Building and installing helix-drm-manager..."
    run_ssh "cd ~/helix/api && CGO_ENABLED=0 /usr/local/go/bin/go build -o /tmp/helix-drm-manager ./cmd/helix-drm-manager/ && \
             sudo cp /tmp/helix-drm-manager /usr/local/bin/helix-drm-manager && \
             sudo chmod +x /usr/local/bin/helix-drm-manager"

    # Create systemd service
    run_ssh "sudo tee /etc/systemd/system/helix-drm-manager.service > /dev/null << 'SVCEOF'
[Unit]
Description=Helix DRM Lease Manager
After=multi-user.target

[Service]
Type=simple
ExecStart=/usr/local/bin/helix-drm-manager
Restart=on-failure
RestartSec=5
User=root

[Install]
WantedBy=multi-user.target
SVCEOF"

    run_ssh "sudo systemctl daemon-reload && sudo systemctl enable helix-drm-manager"
    mark_step "build_drm_manager"
fi

if ! step_done "build_desktop_image"; then
    log "Building helix-ubuntu desktop Docker image (this will take a while)..."
    run_ssh "cd ~/helix && docker build -f Dockerfile.ubuntu-helix -t helix-ubuntu:latest . 2>&1 | tail -10"
    mark_step "build_desktop_image"
fi

if ! step_done "setup_compose"; then
    log "Setting up docker-compose for Helix control plane..."

    # Create a minimal .env for the VM
    run_ssh "cd ~/helix && cat > .env.vm << 'ENVEOF'
# Helix VM configuration for macOS ARM desktop streaming
# Control plane only - no GPU runner needed

# API
API_HOST=0.0.0.0:8080
HELIX_URL=http://localhost:8080

# Database
POSTGRES_HOST=helix-postgres
POSTGRES_PORT=5432
POSTGRES_DB=postgres
POSTGRES_USER=postgres
POSTGRES_PASSWORD=postgres

# Auth (development mode)
KEYCLOAK_ADMIN_PASSWORD=admin

# Runner token
RUNNER_TOKEN=oh-hallo-insecure-token

# Frontend
FRONTEND_URL=http://localhost:5173

# Sandbox (runs directly in VM Docker, not DinD)
GPU_VENDOR=virtio

# Desktop image
HELIX_DESKTOP_IMAGE=helix-ubuntu:latest
ENVEOF"
    mark_step "setup_compose"
fi

# =============================================================================
# Step 7: Shut down VM
# =============================================================================

if ! step_done "shutdown"; then
    log "Shutting down provisioning VM..."
    run_ssh "sudo shutdown -h now" 2>/dev/null || true
    sleep 5

    # Kill QEMU if still running
    if [ -n "$QEMU_PID" ] && kill -0 "$QEMU_PID" 2>/dev/null; then
        kill "$QEMU_PID" 2>/dev/null || true
        wait "$QEMU_PID" 2>/dev/null || true
    fi
    QEMU_PID=""
    mark_step "shutdown"
fi

# =============================================================================
# Step 8: Create UTM bundle
# =============================================================================

if ! step_done "utm_bundle"; then
    log "Creating UTM bundle..."

    UTM_DIR="${VM_DIR}/${VM_NAME}.utm"
    UTM_DOCS="${HOME}/Library/Containers/com.utmapp.UTM/Data/Documents"

    mkdir -p "${UTM_DIR}/Data"

    # Copy disk image to UTM bundle
    DISK_UUID=$(uuidgen)
    cp "${VM_DIR}/disk.qcow2" "${UTM_DIR}/Data/${DISK_UUID}.qcow2"
    cp "${VM_DIR}/efi_vars.fd" "${UTM_DIR}/Data/efi_vars.fd"

    # Generate VM UUID
    VM_UUID=$(uuidgen)

    # Create UTM config.plist
    cat > "${UTM_DIR}/config.plist" << PLISTEOF
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
            <key>DownscalingFilter</key>
            <string>Linear</string>
            <key>DynamicResolution</key>
            <true/>
            <key>Hardware</key>
            <string>virtio-gpu-gl-pci</string>
            <key>NativeResolution</key>
            <false/>
            <key>UpscalingFilter</key>
            <string>Nearest</string>
        </dict>
    </array>
    <key>Drive</key>
    <array>
        <dict>
            <key>Identifier</key>
            <string>${DISK_UUID}</string>
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
        <key>Icon</key>
        <string>linux</string>
        <key>IconCustom</key>
        <false/>
        <key>Name</key>
        <string>Helix Desktop</string>
        <key>UUID</key>
        <string>${VM_UUID}</string>
    </dict>
    <key>Input</key>
    <dict>
        <key>MaximumUsbShare</key>
        <integer>3</integer>
        <key>UsbBusSupport</key>
        <string>3.0</string>
        <key>UsbSharing</key>
        <false/>
    </dict>
    <key>Network</key>
    <array>
        <dict>
            <key>Hardware</key>
            <string>virtio-net-pci</string>
            <key>IsolateFromHost</key>
            <false/>
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
                <dict>
                    <key>GuestAddress</key>
                    <string>10.0.2.15</string>
                    <key>GuestPort</key>
                    <integer>8080</integer>
                    <key>HostAddress</key>
                    <string>127.0.0.1</string>
                    <key>HostPort</key>
                    <integer>8080</integer>
                    <key>Protocol</key>
                    <string>TCP</string>
                </dict>
            </array>
        </dict>
    </array>
    <key>QEMU</key>
    <dict>
        <key>AdditionalArguments</key>
        <array/>
        <key>BalloonDevice</key>
        <false/>
        <key>DebugLog</key>
        <false/>
        <key>Hypervisor</key>
        <true/>
        <key>PS2Controller</key>
        <false/>
        <key>RNGDevice</key>
        <true/>
        <key>RTCLocalTime</key>
        <false/>
        <key>TPMDevice</key>
        <false/>
        <key>TSO</key>
        <false/>
        <key>UEFIBoot</key>
        <true/>
    </dict>
    <key>Serial</key>
    <array/>
    <key>Sharing</key>
    <dict>
        <key>ClipboardSharing</key>
        <true/>
        <key>DirectoryShareMode</key>
        <string>None</string>
        <key>DirectoryShareReadOnly</key>
        <false/>
    </dict>
    <key>Sound</key>
    <array>
        <dict>
            <key>Hardware</key>
            <string>intel-hda</string>
        </dict>
    </array>
    <key>System</key>
    <dict>
        <key>Architecture</key>
        <string>aarch64</string>
        <key>CPU</key>
        <string>default</string>
        <key>CPUCount</key>
        <integer>${CPUS}</integer>
        <key>CPUFlagsAdd</key>
        <array/>
        <key>CPUFlagsRemove</key>
        <array/>
        <key>ForceMulticore</key>
        <false/>
        <key>JITCacheSize</key>
        <integer>0</integer>
        <key>MemorySize</key>
        <integer>${MEMORY_MB}</integer>
        <key>Target</key>
        <string>virt</string>
    </dict>
</dict>
</plist>
PLISTEOF

    # Symlink into UTM documents directory
    if [ -d "$UTM_DOCS" ]; then
        ln -sf "${UTM_DIR}" "${UTM_DOCS}/${VM_NAME}.utm"
        log "UTM bundle linked at: ${UTM_DOCS}/${VM_NAME}.utm"
    else
        log "UTM documents directory not found. Manually import: ${UTM_DIR}"
    fi

    mark_step "utm_bundle"
fi

# =============================================================================
# Done
# =============================================================================

log ""
log "================================================"
log "VM provisioning complete!"
log "================================================"
log ""
log "UTM bundle: ${VM_DIR}/${VM_NAME}.utm"
log "Disk image: ${VM_DIR}/disk.qcow2"
log ""
log "To use with UTM:"
log "  1. Make sure custom QEMU is installed in UTM.app"
log "     (run: ./for-mac/qemu-helix/build-qemu-standalone.sh)"
log "  2. Open UTM - the VM should appear as 'Helix Desktop'"
log "  3. Start the VM"
log "  4. SSH in: ssh -p ${SSH_PORT} ${VM_USER}@localhost"
log ""
log "helix-drm-manager starts automatically on boot."
log "To start the Helix stack:"
log "  ssh helix-vm"
log "  cd ~/helix"
log "  docker compose -f docker-compose.dev.yaml up -d api postgres frontend"
log ""
