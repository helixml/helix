#!/bin/bash
set -euo pipefail

# =============================================================================
# Helix Desktop VM - Lightweight Provisioning (Pre-built ARM64 Images)
# =============================================================================
#
# Creates a VM using install.sh + pre-built ARM64 images from registry.helixml.tech
# instead of building all Docker images from source.
#
# Savings vs provision-vm.sh:
#   - No Go/Rust/Node toolchain install (~3 GB)
#   - No Docker build cache (~42 GB)
#   - No source code cloning (Zed, qwen-code repos)
#   - Expected compressed disk: ~12-15 GB (vs ~29 GB with build-from-source)
#
# Prerequisites:
#   brew install qemu mtools
#
# Usage:
#   ./provision-vm-light.sh --helix-version 2.6.2-rc2 [--upload]
#   ./provision-vm-light.sh --helix-version 2.6.2-rc2 --upload --disk-size 128G

# =============================================================================
# Configuration
# =============================================================================

VM_NAME="helix-desktop"
VM_DIR="${HELIX_VM_DIR:-/Volumes/Big/helix-vm/${VM_NAME}}"
DISK_SIZE="128G"
CPUS=8
MEMORY_MB=32768
SSH_PORT="${HELIX_VM_SSH_PORT:-2223}"
VM_USER="ubuntu"
VM_PASS="helix"
HELIX_VERSION=""

# Ubuntu 25.10 (Questing) - kernel 6.17+ for virtio-gpu multi-scanout
UBUNTU_URL="https://cloud-images.ubuntu.com/questing/current/questing-server-cloudimg-arm64.img"
UBUNTU_IMG="ubuntu-cloud.img"

# UEFI firmware (from Homebrew QEMU)
EFI_CODE="/opt/homebrew/share/qemu/edk2-aarch64-code.fd"
EFI_VARS_TEMPLATE="/opt/homebrew/share/qemu/edk2-arm-vars.fd"

# Parse arguments
UPLOAD=false
RESUME=false
while [[ $# -gt 0 ]]; do
    case $1 in
        --helix-version) HELIX_VERSION="$2"; shift 2 ;;
        --disk-size) DISK_SIZE="$2"; shift 2 ;;
        --cpus) CPUS="$2"; shift 2 ;;
        --memory) MEMORY_MB="$2"; shift 2 ;;
        --upload) UPLOAD=true; shift ;;
        --resume) RESUME=true; shift ;;
        *) echo "Unknown option: $1"; exit 1 ;;
    esac
done

if [ -z "$HELIX_VERSION" ]; then
    echo "Usage: ./provision-vm-light.sh --helix-version VERSION [--upload] [--resume]"
    echo "Example: ./provision-vm-light.sh --helix-version 2.6.2-rc2"
    exit 1
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

# State tracking (for resumability)
STATE_FILE="${VM_DIR}/.provision-light-state"
mark_step() { echo "$1" >> "$STATE_FILE"; }
step_done() { grep -qx "$1" "$STATE_FILE" 2>/dev/null; }

log() { echo "[$(date +%H:%M:%S)] $*"; }

# =============================================================================
# Prerequisites
# =============================================================================

log "=== Helix Desktop VM - Lightweight Provisioning ==="
log "Helix version: $HELIX_VERSION"
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
# QEMU boot function (from provision-vm.sh)
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

boot_provisioning_vm() {
    log "Booting VM with QEMU for provisioning..."
    log "(This uses homebrew QEMU - no GPU, headless mode)"
    log "SSH port: ${SSH_PORT} (dev VM on 2222 is untouched)"

    SEED_DISK="${VM_DIR}/seed-fat.img"

    QEMU_ARGS=(
        -machine virt,accel=hvf
        -cpu host
        -smp "$CPUS"
        -m "$MEMORY_MB"
        -drive if=pflash,format=raw,file="$EFI_CODE",readonly=on
        -drive if=pflash,format=raw,file="${VM_DIR}/efi_vars.fd"
        -drive file="${VM_DIR}/disk.qcow2",format=qcow2,if=virtio
        -device virtio-net-pci,netdev=net0
        -netdev user,id=net0,hostfwd=tcp::${SSH_PORT}-:22
        -nographic
        -serial mon:stdio
    )
    # Attach cloud-init seed disk if it exists (fresh provision only)
    if [ -f "$SEED_DISK" ]; then
        QEMU_ARGS+=(-drive file="${SEED_DISK}",format=raw,if=virtio,readonly=on)
        QEMU_ARGS+=(-smbios type=1,serial=ds=nocloud)
    fi

    qemu-system-aarch64 "${QEMU_ARGS[@]}" > "${VM_DIR}/qemu-boot.log" 2>&1 &
    QEMU_PID=$!

    log "QEMU started (PID $QEMU_PID), waiting for SSH..."

    MAX_WAIT=600
    ELAPSED=0
    while [ $ELAPSED -lt $MAX_WAIT ]; do
        if ssh -o ConnectTimeout=3 -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null \
               -p "$SSH_PORT" "${VM_USER}@localhost" "echo ready" 2>/dev/null; then
            log "SSH is ready!"
            break
        fi
        sleep 10
        ELAPSED=$((ELAPSED + 10))
        if [ $((ELAPSED % 60)) -eq 0 ]; then
            log "Still waiting for SSH... (${ELAPSED}s)"
        fi
    done

    if [ $ELAPSED -ge $MAX_WAIT ]; then
        log "ERROR: Timed out waiting for SSH (${MAX_WAIT}s)"
        log "Check ${VM_DIR}/qemu-boot.log for details"
        exit 1
    fi
}

# =============================================================================
# Step 1: Download Ubuntu cloud image
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
# Step 2: Create disk image
# =============================================================================

if ! step_done "disk"; then
    log "Creating root disk (${DISK_SIZE})..."
    cp "${VM_DIR}/${UBUNTU_IMG}" "${VM_DIR}/disk.qcow2.tmp"
    qemu-img resize "${VM_DIR}/disk.qcow2.tmp" "$DISK_SIZE"
    mv "${VM_DIR}/disk.qcow2.tmp" "${VM_DIR}/disk.qcow2"

    # Copy EFI vars template
    cp "$EFI_VARS_TEMPLATE" "${VM_DIR}/efi_vars.fd"

    mark_step "disk"
fi

# =============================================================================
# Step 3: Create cloud-init seed
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

locale: en_US.UTF-8

packages:
  - ca-certificates
  - curl
  - git
  - htop
  - net-tools
  - build-essential
  - linux-headers-generic
  - dkms
  - python3-websockets
  - openssh-server
  - locales

runcmd:
  # Install Docker CE from official repository (includes buildx + compose)
  - install -m 0755 -d /etc/apt/keyrings
  - curl -fsSL https://download.docker.com/linux/ubuntu/gpg -o /etc/apt/keyrings/docker.asc
  - chmod a+r /etc/apt/keyrings/docker.asc
  - echo "deb [arch=arm64 signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/ubuntu noble stable" > /etc/apt/sources.list.d/docker.list
  - apt-get update
  - apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
  - systemctl disable docker
  - systemctl disable docker.socket
  - usermod -aG docker ${VM_USER}
  - systemctl disable gdm || true
  - systemctl stop gdm || true
  # Enable helix-storage-init service (runs before Docker on every boot)
  - systemctl daemon-reload
  - systemctl enable helix-storage-init.service
  # Disable cloud-init for subsequent boots (root swap shouldn't re-provision)
  - touch /etc/cloud/cloud-init.disabled
  # Limit Virtual-1 (VM console) to 1080p so the text console isn't painfully slow at 5K.
  - sed -i 's/GRUB_CMDLINE_LINUX_DEFAULT="[^"]*"/GRUB_CMDLINE_LINUX_DEFAULT="quiet splash console=tty0 video=Virtual-1:1920x1080"/' /etc/default/grub
  - update-grub
  - growpart /dev/vda 1 || growpart /dev/vda 2 || true
  - resize2fs /dev/vda1 || resize2fs /dev/vda2 || true
  # Set up UTF-8 locale for serial console
  - locale-gen en_US.UTF-8
  - update-locale LANG=en_US.UTF-8
  # Configure serial-getty on ttyAMA0 for clean output (no DSR queries, UTF-8)
  - mkdir -p /etc/systemd/system/serial-getty@ttyAMA0.service.d
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
  # Serial console getty override: UTF-8 mode, no DSR terminal probes, clean login prompt
  - path: /etc/systemd/system/serial-getty@ttyAMA0.service.d/override.conf
    content: |
      [Service]
      ExecStart=
      ExecStart=-/sbin/agetty -8 --noclear --noissue --nohints %I 115200 linux
      Environment=LANG=en_US.UTF-8
  - path: /etc/systemd/system/helix-storage-init.service
    content: |
      [Unit]
      Description=Helix ZFS Storage Initialization
      DefaultDependencies=no
      After=zfs-mount.service zfs-import-cache.service
      Before=docker.service containerd.service
      Wants=zfs-mount.service

      [Service]
      Type=oneshot
      ExecStart=/usr/local/bin/helix-storage-init.sh
      RemainAfterExit=yes

      [Install]
      WantedBy=multi-user.target
  - path: /usr/local/bin/helix-storage-init.sh
    permissions: '0755'
    content: |
      #!/bin/bash
      set -e
      LOG_TAG="helix-storage-init"
      log() { logger -t "\$LOG_TAG" "\$*"; echo "\$*"; }

      # Import ZFS pool if not already imported
      if ! zpool list helix 2>/dev/null; then
          log "Importing ZFS pool..."
          # Try each possible data disk
          for disk in /dev/vdb /dev/vdc /dev/vdd; do
              if [ -b "\$disk" ]; then
                  if zpool import -f -d "\$disk" helix 2>/dev/null; then
                      log "Imported ZFS pool from \$disk"
                      break
                  fi
              fi
          done
      fi

      # Exit early if pool still not available (host app will create it via SSH)
      if ! zpool list helix 2>/dev/null; then
          log "ZFS pool not found — waiting for host app initialization"
          exit 0
      fi

      # Expand pool if disk was resized
      zpool online -e helix \$(zpool list -vHP helix 2>/dev/null | awk '/dev/{print \$1}' | head -1) 2>/dev/null || true

      # Mount Docker volumes dataset if it exists.
      # Host Docker images stay on root disk (pre-baked during provisioning).
      # Only named volumes (postgres data, etc.) persist on ZFS across upgrades.
      if zfs list helix/docker-volumes 2>/dev/null; then
          if ! mountpoint -q /var/lib/docker/volumes 2>/dev/null; then
              log "Mounting Docker volumes dataset..."
              mkdir -p /var/lib/docker/volumes
              zfs mount helix/docker-volumes 2>/dev/null || {
                  log "Docker volumes dataset already mounted or mount failed"
              }
          else
              log "Docker volumes already mounted"
          fi
      fi

      # Mount sandbox-docker zvol if it exists (dedup-enabled ext4 for sandbox DinD)
      ZVOL_DEV=/dev/zvol/helix/sandbox-docker
      if zfs list helix/sandbox-docker 2>/dev/null && [ -e "\$ZVOL_DEV" ]; then
          if ! mountpoint -q /helix/sandbox-docker 2>/dev/null; then
              log "Mounting sandbox-docker zvol..."
              mkdir -p /helix/sandbox-docker
              mount "\$ZVOL_DEV" /helix/sandbox-docker || {
                  log "sandbox-docker zvol mount failed"
              }
          else
              log "sandbox-docker already mounted"
          fi
      fi

      # Restore persistent config if available
      if [ -d /helix/config/ssh ]; then
          log "Restoring SSH host keys from /helix/config/ssh/..."
          cp /helix/config/ssh/ssh_host_* /etc/ssh/ 2>/dev/null || true
          chmod 600 /etc/ssh/ssh_host_*_key 2>/dev/null || true
          chmod 644 /etc/ssh/ssh_host_*_key.pub 2>/dev/null || true
          systemctl restart sshd 2>/dev/null || true
          if [ -f /helix/config/ssh/authorized_keys ]; then
              mkdir -p /home/ubuntu/.ssh
              cp /helix/config/ssh/authorized_keys /home/ubuntu/.ssh/authorized_keys
              chmod 600 /home/ubuntu/.ssh/authorized_keys
              chown ubuntu:ubuntu /home/ubuntu/.ssh/authorized_keys
          fi
      fi

      if [ -f /helix/config/machine-id ]; then
          cp /helix/config/machine-id /etc/machine-id
          systemd-machine-id-commit 2>/dev/null || true
      fi

      if [ -f /helix/config/env.vm ] && [ ! -f /home/ubuntu/helix/.env.vm ]; then
          mkdir -p /home/ubuntu/helix
          cp /helix/config/env.vm /home/ubuntu/helix/.env.vm
          chown ubuntu:ubuntu /home/ubuntu/helix/.env.vm
      fi

      # Docker starts automatically via systemd ordering (Before=docker.service).
      # Do NOT call "systemctl start docker" here — it deadlocks because this
      # service must complete before docker.service can start.

      log "Helix storage initialization complete"
USERDATA

    cat > "${SEED_DIR}/meta-data" << METADATA
instance-id: helix-vm-$(date +%s)
local-hostname: helix-vm
METADATA

    # Create FAT12 seed disk for cloud-init (ISO doesn't work reliably on UEFI)
    log "Creating cloud-init seed disk (FAT12 with label CIDATA)..."
    if ! command -v mformat &>/dev/null || ! command -v mcopy &>/dev/null; then
        log "ERROR: mtools required. Install with: brew install mtools"
        exit 1
    fi
    SEED_DISK="${VM_DIR}/seed-fat.img"
    rm -f "$SEED_DISK"
    dd if=/dev/zero of="$SEED_DISK" bs=1k count=2048 2>/dev/null
    mformat -i "$SEED_DISK" -v CIDATA -t 2 -h 64 -s 32 ::
    mcopy -i "$SEED_DISK" "${SEED_DIR}/user-data" "::user-data"
    mcopy -i "$SEED_DISK" "${SEED_DIR}/meta-data" "::meta-data"

    mark_step "cloudinit"
fi

# =============================================================================
# Step 4: Boot VM for provisioning
# =============================================================================

# Check if there are remaining steps that need SSH access
NEED_SSH=false
for step in install_zfs install_drm_manager run_install_sh setup_dirs configure_env patch_sandbox fix_arm64_images prime_stack cleanup shutdown; do
    if ! step_done "$step"; then
        NEED_SSH=true
        break
    fi
done

if ! step_done "boot_setup"; then
    boot_provisioning_vm

    # First boot: wait for cloud-init to complete
    log "Waiting for cloud-init to finish..."
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
elif [ "$NEED_SSH" = true ]; then
    # Resuming: boot VM again for remaining steps
    boot_provisioning_vm
fi

# =============================================================================
# SSH helper
# =============================================================================

SSH_CMD="ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -p $SSH_PORT ${VM_USER}@localhost"
run_ssh() { $SSH_CMD "$@"; }

# =============================================================================
# Step 5: Install ZFS
# =============================================================================

if ! step_done "install_zfs"; then
    log "Installing ZFS 2.4.0 from arter97 PPA..."
    run_ssh "sudo add-apt-repository -y ppa:arter97/zfs && sudo apt-get update && sudo apt-get install -y zfsutils-linux" || {
        log "ZFS install may need a reboot for DKMS. Continuing..."
    }
    mark_step "install_zfs"
fi

# =============================================================================
# Step 6: Cross-compile and install helix-drm-manager
# =============================================================================

if ! step_done "install_drm_manager"; then
    log "Cross-compiling helix-drm-manager for linux/arm64..."
    (cd "$REPO_ROOT/api" && GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o /tmp/helix-drm-manager ./cmd/helix-drm-manager/)

    log "Uploading helix-drm-manager to VM..."
    scp -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null \
        -P "$SSH_PORT" /tmp/helix-drm-manager "${VM_USER}@localhost:/tmp/"
    run_ssh "sudo cp /tmp/helix-drm-manager /usr/local/bin/ && sudo chmod +x /usr/local/bin/helix-drm-manager"

    # Create systemd service
    run_ssh "sudo tee /etc/systemd/system/helix-drm-manager.service > /dev/null << 'SVCEOF'
[Unit]
Description=Helix DRM Lease Manager
After=systemd-udev-settle.service
Wants=systemd-udev-settle.service

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
    mark_step "install_drm_manager"
fi

# =============================================================================
# Step 7: Run install.sh (downloads pre-built ARM64 images)
# =============================================================================

if ! step_done "run_install_sh"; then
    log "Running install.sh to set up Helix with pre-built ARM64 images..."
    log "Version: ${HELIX_VERSION}"

    run_ssh "curl -sL https://get.helix.ml/install.sh -o /tmp/install.sh && chmod +x /tmp/install.sh"

    # Use --controlplane --sandbox to get compose file, .env, sandbox.sh, and system config.
    # The --sandbox flag may warn about missing GPU but creates sandbox.sh anyway.
    # || true because sandbox GPU detection may produce a non-zero exit.
    run_ssh "/tmp/install.sh --controlplane --sandbox -y --helix-version $HELIX_VERSION --api-host http://localhost:8080 2>&1" || {
        log "WARNING: install.sh completed with warnings (expected for headless VM without GPU)"
    }

    # install.sh uses "sudo curl" which can fail silently in some VM environments.
    # Verify docker-compose.yaml was downloaded; if not, download it directly.
    COMPOSE_SIZE=$(run_ssh "wc -c < /opt/HelixML/docker-compose.yaml 2>/dev/null || echo 0")
    if [ "${COMPOSE_SIZE:-0}" -lt 100 ]; then
        log "docker-compose.yaml is missing or empty — downloading directly..."
        run_ssh "curl -sL 'https://get.helixml.tech/helixml/helix/releases/download/${HELIX_VERSION}/docker-compose.yaml' -o /opt/HelixML/docker-compose.yaml"
        COMPOSE_SIZE=$(run_ssh "wc -c < /opt/HelixML/docker-compose.yaml 2>/dev/null || echo 0")
        if [ "${COMPOSE_SIZE:-0}" -lt 100 ]; then
            log "ERROR: Failed to download docker-compose.yaml"
            exit 1
        fi
        log "docker-compose.yaml downloaded (${COMPOSE_SIZE} bytes)"
    fi

    mark_step "run_install_sh"
fi

# =============================================================================
# Step 8: Set up directory structure
# =============================================================================

if ! step_done "setup_dirs"; then
    log "Setting up directory structure..."

    # install.sh installs to /opt/HelixML — symlink ~/helix for vm.go compatibility
    run_ssh "sudo chown -R ubuntu:ubuntu /opt/HelixML"
    run_ssh "ln -sf /opt/HelixML ~/helix"

    # Stop whatever install.sh started (we'll restart with our config)
    log "Stopping install.sh's services for reconfiguration..."
    run_ssh "cd ~/helix && docker compose down 2>/dev/null || true"
    run_ssh "docker stop helix-sandbox 2>/dev/null; docker rm helix-sandbox 2>/dev/null || true"

    mark_step "setup_dirs"
fi

# =============================================================================
# Step 9: Apply macOS-specific configuration
# =============================================================================

if ! step_done "configure_env"; then
    log "Applying macOS-specific configuration to .env..."

    # Strip any null bytes from .env (install.sh may pad the file on some platforms)
    run_ssh "cd ~/helix && tr -d '\0' < .env > .env.tmp && mv .env.tmp .env"

    # Remove keys we want to override (install.sh may have set them differently)
    run_ssh "cd ~/helix && sed -i '/^GPU_VENDOR=/d; /^HELIX_DESKTOP_IMAGE=/d; /^HELIX_EDITION=/d; /^ADMIN_USER_IDS=/d; /^ENABLE_CUSTOM_USER_PROVIDERS=/d; /^PROJECTS_FREE_MAX_CONCURRENT_DESKTOPS=/d; /^CONTAINER_DOCKER_PATH=/d; /^HELIX_SANDBOX_DATA=/d' .env"

    # Append macOS-specific overrides (using individual echo to avoid heredoc issues via SSH)
    run_ssh "cd ~/helix && {
echo ''
echo '# === macOS Desktop overrides ==='
echo 'GPU_VENDOR=virtio'
echo 'HELIX_DESKTOP_IMAGE=helix-ubuntu:latest'
echo 'HELIX_EDITION=mac-desktop'
echo 'ADMIN_USER_IDS=all'
echo 'ENABLE_CUSTOM_USER_PROVIDERS=true'
echo 'PROJECTS_FREE_MAX_CONCURRENT_DESKTOPS=15'
echo 'CONTAINER_DOCKER_PATH=/helix/container-docker'
echo 'HELIX_SANDBOX_DATA=/helix/workspaces'
} >> .env"

    mark_step "configure_env"
fi

# =============================================================================
# Step 10: Patch sandbox.sh for macOS
# =============================================================================

if ! step_done "patch_sandbox"; then
    log "Patching sandbox.sh for macOS/virtio GPU..."
    if run_ssh "test -f ~/helix/sandbox.sh"; then
        # Set GPU_VENDOR to virtio (no GPU device flags needed)
        run_ssh "cd ~/helix && sed -i 's/^GPU_VENDOR=.*/GPU_VENDOR=\"virtio\"/' sandbox.sh" || true
        # Use helix-ubuntu instead of helix-sway for desktop sessions
        run_ssh "cd ~/helix && sed -i 's|helix-sway|helix-ubuntu|g' sandbox.sh" || true
        # Remove NVIDIA-specific docker flags (not needed for virtio)
        run_ssh "cd ~/helix && sed -i 's/--gpus all//g; s/--runtime=nvidia//g' sandbox.sh" || true
        log "sandbox.sh patched"
    else
        log "WARNING: sandbox.sh not found — install.sh may have skipped sandbox setup"
    fi

    # Disable Docker on boot (vm.go controls startup via ZFS init)
    run_ssh "sudo systemctl disable docker docker.socket" || true

    mark_step "patch_sandbox"
fi

# =============================================================================
# Step 11: Fix ARM64 image tags (multi-arch manifest workaround)
# =============================================================================

if ! step_done "fix_arm64_images"; then
    log "Fixing ARM64 image tags (multi-arch manifest workaround)..."

    # Some images (typesense, sandbox) don't have multi-arch manifests due to a
    # Drone CI bug. They're published with -linux-arm64 suffix tags instead.
    # Pull the arm64-specific tags and retag to match what compose expects.
    run_ssh "sudo systemctl start docker" || true
    for image in typesense helix-sandbox; do
        FULL="registry.helixml.tech/helix/${image}:${HELIX_VERSION}"
        ARM64="registry.helixml.tech/helix/${image}:${HELIX_VERSION}-linux-arm64"
        log "  Checking ${image}..."
        if ! run_ssh "docker pull ${FULL} 2>/dev/null"; then
            log "  Multi-arch pull failed for ${image}, trying arm64-specific tag..."
            if run_ssh "docker pull ${ARM64} 2>&1"; then
                run_ssh "docker tag ${ARM64} ${FULL}"
                log "  Retagged ${ARM64} → ${FULL}"
            else
                log "  WARNING: Could not pull ${image} at all"
            fi
        fi
    done

    mark_step "fix_arm64_images"
fi

# =============================================================================
# Step 12: Prime stack (start, verify, transfer desktop image, stop)
# =============================================================================

if ! step_done "prime_stack"; then
    log "Priming Helix stack..."

    # Ensure Docker is running
    run_ssh "sudo systemctl start docker" || true

    # Pull compose images individually to work around multi-arch manifest failures.
    # docker compose pull aborts ALL pulls if any single image fails (e.g. typesense
    # missing multi-arch manifest), so we pull each service separately.
    log "Pulling compose images..."
    COMPOSE_SERVICES=$(run_ssh "cd ~/helix && docker compose config --services 2>/dev/null" 2>/dev/null || echo "")
    for svc in $COMPOSE_SERVICES; do
        log "  Pulling $svc..."
        run_ssh "cd ~/helix && docker compose pull $svc 2>&1" || {
            log "  WARNING: Failed to pull $svc (may already be cached locally)"
        }
    done

    # Start compose services (prod compose file from install.sh)
    run_ssh "cd ~/helix && docker compose up -d 2>&1" || {
        log "WARNING: docker compose up failed — checking status..."
        run_ssh "cd ~/helix && docker compose ps 2>&1" || true
    }

    # Start sandbox separately (prod mode uses sandbox.sh, not compose profile)
    if run_ssh "test -f ~/helix/sandbox.sh"; then
        run_ssh "cd ~/helix && nohup bash sandbox.sh > /tmp/sandbox.log 2>&1 &" || true
    fi

    # Wait for API health check (up to 5 minutes)
    log "Waiting for API health..."
    ELAPSED=0
    MAX_WAIT=300
    while [ $ELAPSED -lt $MAX_WAIT ]; do
        if run_ssh "curl -s -o /dev/null -w '%{http_code}' http://localhost:8080/api/v1/status 2>/dev/null" 2>/dev/null | grep -qE '^[1234]'; then
            log "API is healthy!"
            break
        fi
        sleep 10
        ELAPSED=$((ELAPSED + 10))
        if [ $((ELAPSED % 60)) -eq 0 ]; then
            log "Waiting for API... (${ELAPSED}s)"
        fi
    done

    if [ $ELAPSED -ge $MAX_WAIT ]; then
        log "WARNING: API did not become healthy within ${MAX_WAIT}s"
        run_ssh "cd ~/helix && docker compose ps 2>&1" || true
        run_ssh "cd ~/helix && docker compose logs api --tail 20 2>&1" || true
    fi

    # Wait for sandbox's inner dockerd to be ready
    log "Waiting for sandbox's inner dockerd..."
    SANDBOX_READY=0
    for i in $(seq 1 30); do
        if run_ssh "docker exec helix-sandbox docker info >/dev/null 2>&1"; then
            SANDBOX_READY=1
            log "Sandbox dockerd is ready!"
            break
        fi
        sleep 2
    done

    if [ "$SANDBOX_READY" = "0" ]; then
        log "WARNING: Sandbox dockerd not ready after 60s"
        run_ssh "docker ps --format '{{.Names}} {{.Status}}' 2>&1 | grep sandbox" || true
    fi

    # Pull helix-ubuntu into sandbox's inner dockerd
    if [ "$SANDBOX_READY" = "1" ]; then
        log "Pulling helix-ubuntu into sandbox's inner dockerd..."
        # Try multi-arch tag first, fall back to arm64-specific tag
        PULL_TAG=""
        if run_ssh "docker exec helix-sandbox docker pull registry.helixml.tech/helix/helix-ubuntu:${HELIX_VERSION} 2>&1"; then
            PULL_TAG="${HELIX_VERSION}"
        elif run_ssh "docker exec helix-sandbox docker pull registry.helixml.tech/helix/helix-ubuntu:${HELIX_VERSION}-linux-arm64 2>&1"; then
            PULL_TAG="${HELIX_VERSION}-linux-arm64"
        else
            log "WARNING: Failed to pull helix-ubuntu into sandbox"
        fi

        if [ -n "$PULL_TAG" ]; then
            # Tag as latest for Hydra to find
            run_ssh "docker exec helix-sandbox docker tag registry.helixml.tech/helix/helix-ubuntu:${PULL_TAG} helix-ubuntu:latest" || true
            run_ssh "docker exec helix-sandbox docker tag registry.helixml.tech/helix/helix-ubuntu:${PULL_TAG} helix-ubuntu:${HELIX_VERSION}" || true
            log "helix-ubuntu tagged as latest and ${HELIX_VERSION}"
        fi
    fi

    # Stop the stack but keep named volumes (sandbox-docker-storage contains
    # the desktop images we just transferred into the sandbox's nested dockerd)
    log "Stopping Helix stack (keeping volumes)..."
    run_ssh "cd ~/helix && docker compose down 2>&1" || true
    run_ssh "docker stop helix-sandbox 2>/dev/null; docker rm helix-sandbox 2>/dev/null" || true

    # Stop Docker (will be started by the desktop app on user's first boot)
    run_ssh "sudo systemctl stop docker"

    # List cached images for verification
    run_ssh "sudo systemctl start docker && docker images --format 'table {{.Repository}}\t{{.Tag}}\t{{.Size}}' && sudo systemctl stop docker"

    mark_step "prime_stack"
fi

# =============================================================================
# Step 12: Cleanup
# =============================================================================

if ! step_done "cleanup"; then
    log "Cleaning up to shrink root disk..."
    # apt package cache
    run_ssh "sudo apt-get clean && sudo rm -rf /var/lib/apt/lists/*" || true
    # Dangling images
    run_ssh "sudo systemctl start docker && docker image prune -f 2>/dev/null && sudo systemctl stop docker" || true
    # Temp files
    run_ssh "sudo rm -rf /tmp/* /var/tmp/*" || true
    # Cloud-init logs and seed data
    run_ssh "sudo rm -rf /var/lib/cloud/instances /var/log/cloud-init*" || true
    mark_step "cleanup"
fi

# =============================================================================
# Step 13: Shutdown
# =============================================================================

if ! step_done "shutdown"; then
    log "Trimming free space for qcow2 compaction..."
    run_ssh "sudo fstrim -av 2>/dev/null || true" 2>/dev/null || true

    log "Shutting down provisioning VM..."
    run_ssh "sudo shutdown -h now" 2>/dev/null || true
    sleep 5

    # Kill QEMU if still running
    if [ -n "$QEMU_PID" ] && kill -0 "$QEMU_PID" 2>/dev/null; then
        kill "$QEMU_PID" 2>/dev/null || true
        wait "$QEMU_PID" 2>/dev/null || true
    fi
    QEMU_PID=""

    # Compact the qcow2 image (removes sparse regions after fstrim).
    # No -c flag: upload-vm-images.sh will compress with zstd externally,
    # which achieves better compression on uncompressed qcow2 data.
    log "Compacting qcow2 disk image (uncompressed for zstd)..."
    ORIG_SIZE=$(stat -f%z "${VM_DIR}/disk.qcow2" 2>/dev/null || stat -c%s "${VM_DIR}/disk.qcow2" 2>/dev/null)
    qemu-img convert -O qcow2 "${VM_DIR}/disk.qcow2" "${VM_DIR}/disk-compact.qcow2"
    mv "${VM_DIR}/disk-compact.qcow2" "${VM_DIR}/disk.qcow2"
    COMPACT_SIZE=$(stat -f%z "${VM_DIR}/disk.qcow2" 2>/dev/null || stat -c%s "${VM_DIR}/disk.qcow2" 2>/dev/null)
    log "  Compacted: $(echo "$ORIG_SIZE" | awk '{printf "%.1f GB", $1/1073741824}') → $(echo "$COMPACT_SIZE" | awk '{printf "%.1f GB", $1/1073741824}')"

    mark_step "shutdown"
fi

# =============================================================================
# Step 14: Upload to R2 CDN
# =============================================================================

if [ "$UPLOAD" = true ]; then
    if ! step_done "upload"; then
        log ""
        log "=== Uploading VM image to R2 CDN ==="
        VM_DIR="$VM_DIR" bash "${SCRIPT_DIR}/upload-vm-images.sh"
        mark_step "upload"
    fi
fi

# =============================================================================
# Done
# =============================================================================

log ""
log "================================================"
log "Lightweight VM provisioning complete!"
log "================================================"
log ""
log "Disk image: ${VM_DIR}/disk.qcow2"
log ""
if [ "$UPLOAD" = true ]; then
    log "VM image uploaded to R2. Manifest updated at for-mac/vm-manifest.json."
    log "Commit and push to deploy."
else
    log "To upload to R2: ./provision-vm-light.sh --helix-version $HELIX_VERSION --resume --upload"
fi
log ""
log "To test the first-launch flow:"
log "  cd for-mac && wails dev"
log ""
