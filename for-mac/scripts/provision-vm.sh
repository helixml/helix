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
#   8. Primes the stack (pull/build/start/verify/stop)
#   9. Creates UTM .utm bundle ready to launch
#  10. Optionally compresses + uploads disk image to R2 CDN
#
# Prerequisites:
#   brew install qemu mtools  (for initial provisioning)
#   Custom QEMU in UTM.app (for production with scanout pipeline)
#
# Usage:
#   ./provision-vm.sh [--disk-size 256G] [--cpus 8] [--memory 16384]
#   ./provision-vm.sh --resume    # Resume from last step if interrupted
#   ./provision-vm.sh --upload    # Also compress + upload to R2 after provisioning
#   ./provision-vm.sh --update [--upload]  # Update existing image (pull code, rebuild, re-prime)

# =============================================================================
# Configuration
# =============================================================================

VM_NAME="helix-desktop"
VM_DIR="${HELIX_VM_DIR:-/Volumes/Big/helix-vm/${VM_NAME}}"
DISK_SIZE="128G"      # Root disk (OS only — Docker data lives on ZFS data disk)
CPUS=8
MEMORY_MB=32768
SSH_PORT="${HELIX_VM_SSH_PORT:-2223}"  # Use 2223 during provisioning to avoid conflicts
VM_USER="ubuntu"
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
UPLOAD=false
UPDATE=false
while [[ $# -gt 0 ]]; do
    case $1 in
        --disk-size) DISK_SIZE="$2"; shift 2 ;;
        --cpus) CPUS="$2"; shift 2 ;;
        --memory) MEMORY_MB="$2"; shift 2 ;;
        --resume) RESUME=true; shift ;;
        --upload) UPLOAD=true; shift ;;
        --update) UPDATE=true; shift ;;
        *) echo "Unknown option: $1"; exit 1 ;;
    esac
done

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

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
if [ "$RESUME" = false ] && [ "$UPDATE" = false ]; then
    rm -f "$STATE_FILE"
fi

# =============================================================================
# QEMU boot function (used by both fresh provisioning and update mode)
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

    # Start QEMU in background
    # Disks: vda=root (ext4), vdb=cloud-init seed (if exists)
    # ZFS data disk is NOT attached during provisioning — it's created on first boot.
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
# Update mode: boot existing golden image, pull code, rebuild, re-prime, upload
# =============================================================================

if [ "$UPDATE" = true ]; then
    if [ ! -f "${VM_DIR}/disk.qcow2" ]; then
        log "ERROR: No existing disk.qcow2 found at ${VM_DIR}/disk.qcow2"
        log "Run without --update first to create a fresh image."
        exit 1
    fi

    BRANCH=$(git -C "$(cd "$SCRIPT_DIR/../.." && pwd)" rev-parse --abbrev-ref HEAD 2>/dev/null || echo "main")
    log "=== Incremental Update Mode ==="
    log "Branch: $BRANCH"
    log "Existing image: ${VM_DIR}/disk.qcow2"

    boot_provisioning_vm

    SSH_CMD="ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -p $SSH_PORT ${VM_USER}@localhost"
    run_ssh() { $SSH_CMD "$@"; }

    # Fix the helix-storage-init.sh deadlock: the old script calls "systemctl start docker"
    # but it has Before=docker.service, creating a circular dependency where docker.service
    # waits for helix-storage-init.service to complete, but the init script waits for docker.
    #
    # Fix: (1) patch the script, (2) kill the stuck child, (3) wait for the init service
    # to finish (which unblocks docker.service), (4) then start Docker.
    log "Fixing storage init script and starting Docker..."
    run_ssh "sudo sed -i '/^systemctl start docker/d' /usr/local/bin/helix-storage-init.sh" || true
    run_ssh "CHILD=\$(pgrep -f 'systemctl start docker' 2>/dev/null) && sudo kill \$CHILD 2>/dev/null" || true

    # Wait for helix-storage-init to finish (it should exit quickly after we killed the stuck child)
    INIT_WAIT=0
    while [ $INIT_WAIT -lt 30 ]; do
        INIT_STATE=$(run_ssh "systemctl is-active helix-storage-init 2>/dev/null" 2>/dev/null || echo "unknown")
        if [ "$INIT_STATE" = "active" ] || [ "$INIT_STATE" = "inactive" ] || [ "$INIT_STATE" = "failed" ]; then
            log "Storage init service: $INIT_STATE"
            break
        fi
        sleep 2
        INIT_WAIT=$((INIT_WAIT + 2))
    done

    # Now Docker should be unblocked — start it explicitly
    run_ssh "sudo systemctl start docker" || true
    DOCKER_WAIT=0
    while [ $DOCKER_WAIT -lt 60 ]; do
        if run_ssh "systemctl is-active docker" 2>/dev/null | grep -q "^active"; then
            log "Docker is running"
            break
        fi
        sleep 3
        DOCKER_WAIT=$((DOCKER_WAIT + 3))
        if [ $((DOCKER_WAIT % 15)) -eq 0 ]; then
            log "Waiting for Docker... (${DOCKER_WAIT}s)"
        fi
    done
    if ! run_ssh "systemctl is-active docker" 2>/dev/null | grep -q "^active"; then
        log "ERROR: Docker did not start within 60 seconds"
        run_ssh "sudo systemctl status helix-storage-init docker 2>&1; sudo journalctl -u docker --no-pager -n 20 2>&1" || true
        exit 1
    fi

    # Pull latest code. If the repo is missing (previous failed run deleted it)
    # or git refs are corrupted (non-clean shutdown), handle gracefully.
    log "Pulling latest code (branch: ${BRANCH})..."
    if ! run_ssh "[ -d ~/helix/.git ]"; then
        log "Helix repo not found — cloning fresh..."
        # Directory may exist without .git (e.g., .env.vm, docker-compose files from install.sh).
        # Back up .env.vm, remove the non-git dir, clone, then restore .env.vm.
        run_ssh "cp ~/helix/.env.vm /tmp/env.vm.bak 2>/dev/null || true"
        run_ssh "rm -rf ~/helix"
        run_ssh "git clone -b ${BRANCH} https://github.com/helixml/helix.git ~/helix"
        run_ssh "cp /tmp/env.vm.bak ~/helix/.env.vm 2>/dev/null || true"
    elif ! run_ssh "cd ~/helix && git fetch origin 2>&1"; then
        log "Git fetch failed — resetting remote refs..."
        run_ssh "cd ~/helix && rm -rf .git/refs/remotes/origin && git fetch origin 2>&1"
        run_ssh "cd ~/helix && git checkout ${BRANCH} && git pull origin ${BRANCH}"
    else
        run_ssh "cd ~/helix && git checkout ${BRANCH} && git pull origin ${BRANCH}"
    fi
    log "Helix at: $(run_ssh 'cd ~/helix && git log --oneline -1' 2>/dev/null)"

    # Ensure build dependencies are cloned (cleaned up after full provision)
    log "Ensuring Zed and Qwen Code repos..."
    run_ssh "[ -d ~/zed ] || git clone https://github.com/helixml/zed.git ~/zed" || true
    run_ssh "[ -d ~/qwen-code ] || git clone https://github.com/helixml/qwen-code.git ~/qwen-code" || true

    # Rebuild desktop image
    log "Rebuilding desktop image..."
    run_ssh "cd ~/helix && PROJECTS_ROOT=~ SKIP_DESKTOP_TRANSFER=1 DOCKER_BUILDKIT=1 bash stack build-ubuntu 2>&1" || {
        log "WARNING: Desktop image build failed."
        exit 1
    }

    # Rebuild sandbox image (Docker-in-Docker container host)
    log "Rebuilding sandbox image for arm64..."
    run_ssh "cd ~/helix && DOCKER_BUILDKIT=1 docker build -f Dockerfile.sandbox -t helix-sandbox:latest . 2>&1" || {
        log "WARNING: Sandbox image build failed."
    }

    # Re-prime the stack
    log "Re-priming Helix stack..."
    run_ssh "cd ~/helix && [ ! -e .env ] && ln -s .env.vm .env || true"

    # Ensure critical env vars are present in .env.vm (may be missing from older provisions)
    run_ssh "grep -q '^COMPOSE_PROFILES=' ~/helix/.env.vm 2>/dev/null || echo 'COMPOSE_PROFILES=code-macos' >> ~/helix/.env.vm"
    run_ssh "grep -q '^CONTAINER_DOCKER_PATH=' ~/helix/.env.vm 2>/dev/null || echo 'CONTAINER_DOCKER_PATH=/helix/container-docker' >> ~/helix/.env.vm"
    run_ssh "grep -q '^ENABLE_CUSTOM_USER_PROVIDERS=' ~/helix/.env.vm 2>/dev/null || echo 'ENABLE_CUSTOM_USER_PROVIDERS=true' >> ~/helix/.env.vm"

    run_ssh "cd ~/helix && docker compose -f docker-compose.dev.yaml pull 2>&1" || true
    run_ssh "cd ~/helix && docker compose -f docker-compose.dev.yaml build 2>&1" || true

    # Ensure sandbox Docker bind mount directory exists on root disk
    run_ssh "sudo mkdir -p /var/lib/helix-sandbox-docker"

    log "Starting stack (with sandbox) to verify and load desktop images..."
    run_ssh "cd ~/helix && docker compose -f docker-compose.dev.yaml up -d 2>&1"

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

    # Wait for sandbox's inner dockerd to be ready before transferring images.
    # The sandbox runs Docker-in-Docker — its dockerd takes a few seconds to start.
    log "Waiting for sandbox's inner dockerd..."
    SANDBOX_READY=0
    for i in $(seq 1 30); do
        if run_ssh "cd ~/helix && docker compose -f docker-compose.dev.yaml exec -T sandbox-macos docker info >/dev/null 2>&1"; then
            SANDBOX_READY=1
            break
        fi
        sleep 2
    done
    if [ "$SANDBOX_READY" = "0" ]; then
        log "WARNING: Sandbox dockerd not ready after 60s. Checking status..."
        run_ssh "cd ~/helix && docker compose -f docker-compose.dev.yaml ps sandbox-macos 2>&1" || true
    fi

    # Transfer desktop image into sandbox's nested dockerd via local registry.
    # The sandbox runs its own Docker daemon (DinD), so desktop images built on
    # the host Docker must be pushed through the local registry (port 5000) and
    # pulled inside the sandbox. Without this, Hydra gets "No such image" errors.
    log "Transferring desktop image to sandbox..."
    run_ssh "cd ~/helix && bash stack transfer-ubuntu-to-sandbox 2>&1" || {
        log "WARNING: Desktop image transfer failed. Will retry on first boot."
    }

    # Stop the stack but keep data — sandbox Docker storage is a bind mount at
    # /var/lib/helix-sandbox-docker on the root disk, so desktop images persist.
    log "Stopping stack..."
    run_ssh "cd ~/helix && docker compose -f docker-compose.dev.yaml down 2>&1"

    # Cleanup
    run_ssh "rm -rf ~/.cache/go-build" || true
    # NOTE: Keep ~/zed and ~/qwen-code — they're reused by --update mode
    run_ssh "sudo apt-get clean && sudo rm -rf /var/lib/apt/lists/*" || true
    # NOTE: Do NOT run `docker builder prune` here — it destroys BuildKit cache
    # mounts (cargo registry, rustup toolchain, Rust build artifacts) that make
    # subsequent --update rebuilds fast. Only prune dangling images.
    run_ssh "docker image prune -f 2>/dev/null" || true
    run_ssh "sudo rm -rf /tmp/* /var/tmp/*" || true

    # Stop Docker
    run_ssh "sudo systemctl stop docker"

    # Trim and shutdown
    log "Trimming free space..."
    run_ssh "sudo fstrim -av 2>/dev/null || true" 2>/dev/null || true
    log "Shutting down VM..."
    run_ssh "sudo shutdown -h now" 2>/dev/null || true
    sleep 5
    if [ -n "$QEMU_PID" ] && kill -0 "$QEMU_PID" 2>/dev/null; then
        kill "$QEMU_PID" 2>/dev/null || true
        wait "$QEMU_PID" 2>/dev/null || true
    fi
    QEMU_PID=""

    # Upload if requested
    if [ "$UPLOAD" = true ]; then
        log ""
        log "=== Uploading updated VM image to R2 CDN ==="
        VM_DIR="$VM_DIR" bash "${SCRIPT_DIR}/upload-vm-images.sh"
    fi

    log ""
    log "================================================"
    log "Incremental update complete!"
    log "================================================"
    log "Disk image: ${VM_DIR}/disk.qcow2"
    if [ "$UPLOAD" = true ]; then
        log "Uploaded to R2. Commit and push vm-manifest.json to deploy."
    else
        log "To upload: ./provision-vm.sh --update --upload"
    fi
    exit 0
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
    log "Creating root disk (${DISK_SIZE})..."
    cp "${VM_DIR}/${UBUNTU_IMG}" "${VM_DIR}/disk.qcow2.tmp"
    qemu-img resize "${VM_DIR}/disk.qcow2.tmp" "$DISK_SIZE"
    mv "${VM_DIR}/disk.qcow2.tmp" "${VM_DIR}/disk.qcow2"

    # ZFS data disk is no longer created during provisioning.
    # It's created on first boot by the desktop app (vm.go createEmptyQcow2).

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
  # EDID advertises 5K as preferred mode, but only DRM lease connectors (Virtual-2+) need it.
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
# Step 5: Boot VM for provisioning
# =============================================================================

# Check if there are remaining steps that need SSH access
NEED_SSH=false
for step in install_zfs setup_zfs_pool install_go clone_helix build_drm_manager clone_deps build_desktop_image setup_compose prime_stack cleanup shutdown; do
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
    # Resuming: VM was previously booted but there are remaining steps.
    # Boot it again for SSH access.
    boot_provisioning_vm
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

if ! step_done "setup_zfs_pool"; then
    log "ZFS pool setup skipped during provisioning."
    log "ZFS data disk is created on first boot by the desktop app (vm.go)."
    log "Docker stays on ext4 overlay2 (default, configured in cloud-init daemon.json)."
    mark_step "setup_zfs_pool"
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
    BRANCH="feature/macos-arm-desktop-port-working2"
    log "Setting up helix repository (branch: ${BRANCH})..."
    run_ssh "git clone https://github.com/helixml/helix.git ~/helix 2>/dev/null || true"
    run_ssh "cd ~/helix && git fetch origin && git checkout ${BRANCH} && git pull origin ${BRANCH}"
    log "Helix at: $(run_ssh 'cd ~/helix && git log --oneline -1' 2>/dev/null)"
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
    mark_step "build_drm_manager"
fi

if ! step_done "clone_deps"; then
    log "Cloning Zed and Qwen Code repositories..."
    # Zed IDE fork (needed for external_websocket_sync)
    run_ssh "git clone https://github.com/helixml/zed.git ~/zed 2>/dev/null || (cd ~/zed && git fetch && git pull)" || true
    # Qwen Code agent
    run_ssh "git clone https://github.com/helixml/qwen-code.git ~/qwen-code 2>/dev/null || (cd ~/qwen-code && git fetch && git pull)" || true
    mark_step "clone_deps"
fi

if ! step_done "build_desktop_image"; then
    log "Building full desktop stack (Zed + Qwen Code + helix-ubuntu image)..."
    log "This builds Zed (Rust), Qwen Code (Node.js), and the GNOME desktop image."
    log "First run takes 30-60 minutes. Subsequent builds use Docker cache."
    # PROJECTS_ROOT tells ./stack where to find zed/ and qwen-code/ repos
    # SKIP_DESKTOP_TRANSFER=1 skips pushing to local registry (no sandbox running)
    run_ssh "cd ~/helix && PROJECTS_ROOT=~ SKIP_DESKTOP_TRANSFER=1 DOCKER_BUILDKIT=1 bash stack build-ubuntu 2>&1" || {
        log "WARNING: Desktop image build failed."
        log "Retry manually: ssh helix-vm 'cd ~/helix && PROJECTS_ROOT=~ SKIP_DESKTOP_TRANSFER=1 bash stack build-ubuntu'"
    }
    # Build sandbox image (Docker-in-Docker container host for desktop sessions)
    log "Building sandbox image for arm64..."
    run_ssh "cd ~/helix && DOCKER_BUILDKIT=1 docker build -f Dockerfile.sandbox -t helix-sandbox:latest . 2>&1" || {
        log "WARNING: Sandbox image build failed."
        log "Retry manually: ssh helix-vm 'cd ~/helix && docker build -f Dockerfile.sandbox -t helix-sandbox:latest .'"
    }

    # Verify the image was actually created — don't mark step done if it wasn't
    if run_ssh "docker images helix-ubuntu:latest --format '{{.Size}}'" 2>/dev/null | grep -q .; then
        log "Desktop image built successfully: $(run_ssh 'docker images helix-ubuntu:latest --format "{{.Size}}"' 2>/dev/null)"
        mark_step "build_desktop_image"
    else
        log "ERROR: Desktop image not available. Build failed."
        log "The provisioning VM is still running. SSH in to debug:"
        log "  ssh -p ${SSH_PORT} ${VM_USER}@localhost"
        log "Then retry: cd ~/helix && PROJECTS_ROOT=~ SKIP_DESKTOP_TRANSFER=1 bash stack build-ubuntu"
        exit 1
    fi
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

# Runner token
RUNNER_TOKEN=oh-hallo-insecure-token

# Frontend — Vite dev server inside the frontend container on port 8081
FRONTEND_URL=http://frontend:8081

# Sandbox (runs directly in VM Docker, not DinD)
GPU_VENDOR=virtio

# Desktop image
HELIX_DESKTOP_IMAGE=helix-ubuntu:latest

# Workspace storage on ZFS with dedup (saves disk space on Mac)
HELIX_SANDBOX_DATA=/helix/workspaces

# Desktop edition identifier (used by Launchpad telemetry)
HELIX_EDITION=mac-desktop

# All desktop users are admins (single-user environment)
ADMIN_USER_IDS=all

# Allow users to configure their own inference providers
ENABLE_CUSTOM_USER_PROVIDERS=true

# Max concurrent desktops (hard limit: 15 QEMU video outputs)
PROJECTS_FREE_MAX_CONCURRENT_DESKTOPS=15

# Enable sandbox-macos compose profile so sandbox starts with docker compose up -d
COMPOSE_PROFILES=code-macos
ENVEOF"
    mark_step "setup_compose"
fi

if ! step_done "prime_stack"; then
    log "Priming Helix stack (pulling/building all Docker images)..."
    log "This ensures first user boot doesn't need to pull images."

    # Create .env symlink so docker compose picks up config
    run_ssh "cd ~/helix && [ ! -e .env ] && ln -s .env.vm .env || true"

    # Start Docker (it's disabled on boot — we need it for compose)
    run_ssh "sudo systemctl start docker"

    # Pull all pre-built images and build locally-built images
    run_ssh "cd ~/helix && docker compose -f docker-compose.dev.yaml pull 2>&1" || {
        log "WARNING: Some images failed to pull (may be optional services)"
    }
    run_ssh "cd ~/helix && docker compose -f docker-compose.dev.yaml build 2>&1" || {
        log "WARNING: Some images failed to build"
    }

    # Start the stack and wait for API health check
    log "Starting Helix stack to verify all services work..."
    run_ssh "cd ~/helix && docker compose -f docker-compose.dev.yaml up -d 2>&1"

    # Wait for API to become healthy (up to 5 minutes)
    ELAPSED=0
    MAX_WAIT=300
    while [ $ELAPSED -lt $MAX_WAIT ]; do
        # API returns 401 without auth cookie — that's fine, it means the API is up.
        # Match vm.go checkAPIHealth: status < 500 means healthy.
        if run_ssh "curl -s -o /dev/null -w '%{http_code}' http://localhost:8080/api/v1/status 2>/dev/null" 2>/dev/null | grep -qE '^[1234]'; then
            log "API is healthy! Stack priming complete."
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
        log "Checking container status..."
        run_ssh "cd ~/helix && docker compose -f docker-compose.dev.yaml ps 2>&1" || true
        run_ssh "cd ~/helix && docker compose -f docker-compose.dev.yaml logs api --tail 20 2>&1" || true
    fi

    # Wait for sandbox's inner dockerd to be ready before transferring images.
    log "Waiting for sandbox's inner dockerd..."
    SANDBOX_READY=0
    for i in $(seq 1 30); do
        if run_ssh "cd ~/helix && docker compose -f docker-compose.dev.yaml exec -T sandbox-macos docker info >/dev/null 2>&1"; then
            SANDBOX_READY=1
            break
        fi
        sleep 2
    done
    if [ "$SANDBOX_READY" = "0" ]; then
        log "WARNING: Sandbox dockerd not ready after 60s. Checking status..."
        run_ssh "cd ~/helix && docker compose -f docker-compose.dev.yaml ps sandbox-macos 2>&1" || true
    fi

    # Transfer desktop image into sandbox's nested dockerd via local registry.
    # The sandbox runs Docker-in-Docker, so images built on the host Docker must
    # be pushed through the local registry and pulled inside the sandbox.
    log "Transferring desktop image to sandbox..."
    run_ssh "cd ~/helix && bash stack transfer-ubuntu-to-sandbox 2>&1" || {
        log "WARNING: Desktop image transfer failed. Will retry on first boot."
    }

    # Stop the stack. Sandbox Docker storage is a bind mount at
    # /var/lib/helix-sandbox-docker on the root disk — desktop images persist.
    log "Stopping Helix stack..."
    run_ssh "cd ~/helix && docker compose -f docker-compose.dev.yaml down 2>&1"

    # Stop Docker (it will be started by the desktop app on user's first boot)
    run_ssh "sudo systemctl stop docker"

    # List cached images for verification
    run_ssh "sudo systemctl start docker && docker images --format 'table {{.Repository}}\t{{.Tag}}\t{{.Size}}' && sudo systemctl stop docker"

    mark_step "prime_stack"
fi

# =============================================================================
# Step 7: Shut down VM
# =============================================================================

if ! step_done "cleanup"; then
    log "Cleaning up build artifacts to shrink root disk..."
    # NOTE: Keep ~/zed and ~/qwen-code — the golden image is used for development,
    # so `./stack build-ubuntu` needs these repos available for rebuilding.
    # Go build cache
    run_ssh "rm -rf ~/.cache/go-build /tmp/go*.tar.gz" || true
    # apt package cache
    run_ssh "sudo apt-get clean && sudo rm -rf /var/lib/apt/lists/*" || true
    # NOTE: Do NOT run `docker builder prune` — it destroys BuildKit cache mounts
    # (cargo registry, rustup toolchain, Rust build artifacts) that make subsequent
    # --update rebuilds fast (minutes instead of 30-60 min Rust compile).
    # Dangling images (untagged intermediates)
    run_ssh "docker image prune -f 2>/dev/null" || true
    # Temp files
    run_ssh "sudo rm -rf /tmp/* /var/tmp/*" || true
    # Cloud-init logs and seed data (no longer needed)
    run_ssh "sudo rm -rf /var/lib/cloud/instances /var/log/cloud-init*" || true
    mark_step "cleanup"
fi

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

    # Link disk images into UTM bundle (hardlink to avoid doubling disk usage)
    DISK_UUID=$(uuidgen)
    ln -f "${VM_DIR}/disk.qcow2" "${UTM_DIR}/Data/${DISK_UUID}.qcow2" 2>/dev/null \
        || cp "${VM_DIR}/disk.qcow2" "${UTM_DIR}/Data/${DISK_UUID}.qcow2"
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
# Step 9: Upload to R2 CDN
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
log "VM provisioning complete!"
log "================================================"
log ""
log "Disk image: ${VM_DIR}/disk.qcow2"
log ""
if [ "$UPLOAD" = true ]; then
    log "VM image uploaded to R2. Manifest updated at for-mac/vm-manifest.json."
    log "Commit and push to deploy."
else
    log "To upload to R2: ./provision-vm.sh --resume --upload"
fi
log ""
log "To test the first-launch flow:"
log "  cd for-mac && wails dev"
log ""
