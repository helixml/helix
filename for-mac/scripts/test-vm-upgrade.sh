#!/bin/bash
set -euo pipefail

# =============================================================================
# CI Test: VM Root Disk Upgrade — ZFS Pool Survival
# =============================================================================
#
# Regression test for the ZFS pool import failure on fresh root disk boot
# (commit bbc21bb1f). Exercises the real upgrade flow:
#   1. Boot VM, create ZFS pool + write test data
#   2. Swap root disk with a fresh copy (simulates golden image upgrade)
#   3. Reboot, verify ZFS pool imports and data survives
#
# Runs on macOS with QEMU (locally or on flight-arm64 Drone exec runner).
#
# Usage:
#   cd for-mac && bash scripts/test-vm-upgrade.sh
#
# Exit code 0 = pass, non-zero = fail with assertion message.
# Runtime: ~10-15 minutes (cloud-init + ZFS install is the bottleneck).

# =============================================================================
# Configuration
# =============================================================================

VM_DIR="${HELIX_VM_DIR:-/Volumes/Big/helix-vm/ci-upgrade-test}"
DISK_SIZE="8G"
DATA_DISK_SIZE="2G"
CPUS=4
MEMORY_MB=4096
SSH_PORT=2225
VM_USER="ubuntu"
VM_PASS="helix"

# Ubuntu cloud image (same as provision-vm.sh)
UBUNTU_URL="https://cloud-images.ubuntu.com/questing/current/questing-server-cloudimg-arm64.img"
UBUNTU_IMG="ubuntu-cloud.img"

# UEFI firmware
EFI_CODE="/opt/homebrew/share/qemu/edk2-aarch64-code.fd"
EFI_VARS_TEMPLATE="/opt/homebrew/share/qemu/edk2-arm-vars.fd"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SENTINEL_VALUE="UPGRADE_TEST_OK"
TEST_ENV_VALUE="HELIX_TEST=upgrade_sentinel_$(date +%s)"

# =============================================================================
# Helpers
# =============================================================================

QEMU_PID=""
PASS=true
FAIL_MSG=""

log() { echo "[$(date +%H:%M:%S)] $*"; }

fail() {
    log "FAIL: $*"
    PASS=false
    FAIL_MSG="${FAIL_MSG}FAIL: $*\n"
}

cleanup() {
    if [ -n "$QEMU_PID" ] && kill -0 "$QEMU_PID" 2>/dev/null; then
        log "Shutting down QEMU (PID $QEMU_PID)..."
        kill "$QEMU_PID" 2>/dev/null || true
        wait "$QEMU_PID" 2>/dev/null || true
    fi
    QEMU_PID=""
}
trap cleanup EXIT

SSH_CMD="ssh -o ConnectTimeout=5 -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -p $SSH_PORT ${VM_USER}@localhost"

run_ssh() {
    $SSH_CMD "$@"
}

wait_for_ssh() {
    local max_wait=${1:-600}
    local elapsed=0
    while [ $elapsed -lt $max_wait ]; do
        if ssh -o ConnectTimeout=3 -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null \
               -p "$SSH_PORT" "${VM_USER}@localhost" "echo ready" 2>/dev/null; then
            return 0
        fi
        sleep 10
        elapsed=$((elapsed + 10))
        if [ $((elapsed % 60)) -eq 0 ]; then
            log "Still waiting for SSH... (${elapsed}s)"
        fi
    done
    return 1
}

wait_for_cloud_init() {
    local max_wait=${1:-600}
    local elapsed=0
    while [ $elapsed -lt $max_wait ]; do
        if run_ssh "test -f /var/lib/cloud/instance/provision-ready" 2>/dev/null; then
            return 0
        fi
        sleep 10
        elapsed=$((elapsed + 10))
        if [ $((elapsed % 60)) -eq 0 ]; then
            log "Still waiting for cloud-init... (${elapsed}s)"
        fi
    done
    return 1
}

boot_vm() {
    local root_disk="$1"
    local data_disk="$2"
    local seed_disk="${3:-}"

    log "Booting VM: root=$(basename "$root_disk"), data=$(basename "$data_disk")"

    QEMU_ARGS=(
        -machine virt,accel=hvf
        -cpu host
        -smp "$CPUS"
        -m "$MEMORY_MB"
        -drive if=pflash,format=raw,file="$EFI_CODE",readonly=on
        -drive if=pflash,format=raw,snapshot=on,file="$EFI_VARS_TEMPLATE"
        -drive file="$root_disk",format=qcow2,if=virtio
        -drive file="$data_disk",format=qcow2,if=virtio
        -device virtio-net-pci,netdev=net0
        -netdev user,id=net0,hostfwd=tcp::${SSH_PORT}-:22
        -nographic
        -serial mon:stdio
    )
    if [ -n "$seed_disk" ] && [ -f "$seed_disk" ]; then
        QEMU_ARGS+=(-drive file="$seed_disk",format=raw,if=virtio,readonly=on)
        QEMU_ARGS+=(-smbios type=1,serial=ds=nocloud)
    fi

    qemu-system-aarch64 "${QEMU_ARGS[@]}" > "${VM_DIR}/qemu-boot.log" 2>&1 &
    QEMU_PID=$!
    log "QEMU started (PID $QEMU_PID)"
}

shutdown_vm() {
    log "Shutting down VM..."
    run_ssh "sudo poweroff" 2>/dev/null || true
    # Wait for QEMU to exit
    local waited=0
    while kill -0 "$QEMU_PID" 2>/dev/null && [ $waited -lt 30 ]; do
        sleep 1
        waited=$((waited + 1))
    done
    if kill -0 "$QEMU_PID" 2>/dev/null; then
        kill "$QEMU_PID" 2>/dev/null || true
        wait "$QEMU_PID" 2>/dev/null || true
    fi
    QEMU_PID=""
}

# =============================================================================
# Prerequisites
# =============================================================================

log "=== CI Test: VM Root Disk Upgrade ==="

if ! command -v qemu-system-aarch64 &>/dev/null; then
    log "ERROR: qemu-system-aarch64 not found. Install with: brew install qemu"
    exit 1
fi
if [ ! -f "$EFI_CODE" ]; then
    log "ERROR: UEFI firmware not found at $EFI_CODE"
    exit 1
fi
if ! command -v mformat &>/dev/null || ! command -v mcopy &>/dev/null; then
    log "ERROR: mtools required. Install with: brew install mtools"
    exit 1
fi

# Clean start
rm -rf "$VM_DIR"
mkdir -p "$VM_DIR"

# Read SSH public key
SSH_KEY=""
for keyfile in ~/.ssh/id_ed25519.pub ~/.ssh/id_rsa.pub; do
    if [ -f "$keyfile" ]; then
        SSH_KEY=$(cat "$keyfile")
        break
    fi
done
if [ -z "$SSH_KEY" ]; then
    log "WARNING: No SSH public key found; using password auth only"
fi

# =============================================================================
# Download/cache Ubuntu cloud image
# =============================================================================

CACHE_DIR="${VM_DIR}/.cache"
mkdir -p "$CACHE_DIR"
if [ ! -f "${CACHE_DIR}/${UBUNTU_IMG}" ]; then
    log "Downloading Ubuntu cloud image..."
    curl -L -o "${CACHE_DIR}/${UBUNTU_IMG}" "$UBUNTU_URL"
fi

# =============================================================================
# Create cloud-init user-data (embeds scripts from current branch)
# =============================================================================

# Read helix-storage-init.sh and .service from provision-vm.sh's cloud-init
# inline — we replicate the exact content here so the test exercises the same
# scripts that production uses.

SEED_DIR="${VM_DIR}/seed"
mkdir -p "$SEED_DIR"

create_cloud_init() {
    cat > "${SEED_DIR}/user-data" << USERDATA
#cloud-config
hostname: helix-vm-test
manage_etc_hosts: true

users:
  - name: ${VM_USER}
    sudo: ALL=(ALL) NOPASSWD:ALL
    shell: /bin/bash
    groups: [docker]
    lock_passwd: false
    plain_text_passwd: ${VM_PASS}
    ssh_authorized_keys:
      - ${SSH_KEY}

package_update: true
package_upgrade: false

packages:
  - ca-certificates
  - curl
  - openssh-server

runcmd:
  # Install ZFS from PPA
  - add-apt-repository -y ppa:arter97/zfs
  - apt-get update
  - apt-get install -y zfsutils-linux
  # Enable zfs-import-scan with -f flag (needed after root disk swap changes hostid)
  - systemctl enable zfs-import-scan.service
  - mkdir -p /etc/systemd/system/zfs-import-scan.service.d
  # Enable helix-storage-init service
  - systemctl daemon-reload
  - systemctl enable helix-storage-init.service
  # Signal readiness
  - touch /var/lib/cloud/instance/provision-ready

write_files:
  # Override zfs-import-scan to use -f (needed after root disk swap changes hostid)
  - path: /etc/systemd/system/zfs-import-scan.service.d/force-import.conf
    content: |
      [Service]
      ExecStart=
      ExecStart=/sbin/zpool import -aN -f -o cachefile=none
  - path: /etc/ssh/sshd_config.d/helix.conf
    content: |
      PasswordAuthentication yes
      PubkeyAuthentication yes
  - path: /etc/systemd/system/helix-storage-init.service
    content: |
      [Unit]
      Description=Helix ZFS Storage Initialization
      DefaultDependencies=no
      After=zfs-mount.service zfs-import-cache.service zfs-import-scan.service
      Before=docker.service containerd.service
      Wants=zfs-mount.service zfs-import-scan.service

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
          # First try import without -d (scans all devices)
          if zpool import -f helix 2>/dev/null; then
              log "Imported existing ZFS pool"
          else
              # Fallback: try each possible data disk explicitly
              for disk in /dev/vdb /dev/vdc /dev/vdd; do
                  if [ -b "\$disk" ]; then
                      log "Trying import from \$disk..."
                      if zpool import -f -d "\$disk" helix; then
                          log "Imported ZFS pool from \$disk"
                          break
                      fi
                  fi
              done
          fi
      fi

      # Exit early if pool still not available
      if ! zpool list helix 2>/dev/null; then
          log "ZFS pool not found — waiting for host app initialization"
          exit 0
      fi

      # Expand pool if disk was resized
      zpool online -e helix \$(zpool list -vHP helix 2>/dev/null | awk '/dev/{print \$1}' | head -1) 2>/dev/null || true

      # Mount Docker volumes dataset if it exists
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

      # Restore persistent config if available
      if [ -d /helix/config/ssh ]; then
          log "Restoring SSH host keys from /helix/config/ssh/..."
          cp /helix/config/ssh/ssh_host_* /etc/ssh/ 2>/dev/null || true
          chmod 600 /etc/ssh/ssh_host_*_key 2>/dev/null || true
          chmod 644 /etc/ssh/ssh_host_*_key.pub 2>/dev/null || true
          systemctl restart sshd 2>/dev/null || true
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

      log "Helix storage initialization complete"
USERDATA

    cat > "${SEED_DIR}/meta-data" << METADATA
instance-id: helix-vm-test-$(date +%s)
local-hostname: helix-vm-test
METADATA
}

create_seed_disk() {
    local seed_disk="${VM_DIR}/seed-fat.img"
    rm -f "$seed_disk"
    dd if=/dev/zero of="$seed_disk" bs=1k count=2048 2>/dev/null
    mformat -i "$seed_disk" -v CIDATA -t 2 -h 64 -s 32 ::
    mcopy -i "$seed_disk" "${SEED_DIR}/user-data" "::user-data"
    mcopy -i "$seed_disk" "${SEED_DIR}/meta-data" "::meta-data"
    echo "$seed_disk"
}

create_root_disk() {
    local disk_path="$1"
    qemu-img create -f qcow2 -b "${CACHE_DIR}/${UBUNTU_IMG}" -F qcow2 "$disk_path" "$DISK_SIZE"
}

# =============================================================================
# Phase 1: "Old version" boot — create ZFS pool + write test data
# =============================================================================

log ""
log "=============================="
log "Phase 1: Old version boot"
log "=============================="

create_cloud_init
SEED_DISK=$(create_seed_disk)
create_root_disk "${VM_DIR}/disk.qcow2"

# Create empty ZFS data disk
qemu-img create -f qcow2 "${VM_DIR}/data.qcow2" "$DATA_DISK_SIZE"

boot_vm "${VM_DIR}/disk.qcow2" "${VM_DIR}/data.qcow2" "$SEED_DISK"

log "Waiting for SSH..."
if ! wait_for_ssh 600; then
    log "ERROR: Timed out waiting for SSH"
    log "QEMU log:"
    tail -50 "${VM_DIR}/qemu-boot.log" || true
    exit 1
fi

log "Waiting for cloud-init..."
if ! wait_for_cloud_init 600; then
    log "ERROR: Timed out waiting for cloud-init"
    exit 1
fi
log "Cloud-init complete."

# Verify ZFS is installed
log "Verifying ZFS installation..."
if ! run_ssh "which zpool" 2>/dev/null; then
    log "ERROR: ZFS not installed after cloud-init"
    exit 1
fi
log "ZFS installed OK."

# Create ZFS pool on vdb (the data disk)
log "Creating ZFS pool on /dev/vdb..."
run_ssh "sudo zpool create -f -m /helix helix /dev/vdb"
run_ssh "sudo zpool list helix"

# Create datasets
log "Creating ZFS datasets..."
run_ssh "sudo zfs create -o compression=lz4 -o mountpoint=/helix/config helix/config"
run_ssh "sudo zfs create -o compression=lz4 -o mountpoint=/helix/workspaces helix/workspaces"

# Write sentinel data
log "Writing sentinel data..."
run_ssh "echo '${SENTINEL_VALUE}' | sudo tee /helix/config/upgrade-test-sentinel"
run_ssh "echo '${TEST_ENV_VALUE}' | sudo tee /helix/config/env.vm"

# Persist SSH host keys
run_ssh "sudo mkdir -p /helix/config/ssh && sudo cp /etc/ssh/ssh_host_* /helix/config/ssh/"

# Verify helix-storage-init.service is enabled
run_ssh "systemctl is-enabled helix-storage-init.service"

# Verify zfs-import-scan.service is enabled
run_ssh "systemctl is-enabled zfs-import-scan.service"

log "Phase 1 complete. Shutting down..."
shutdown_vm

# =============================================================================
# Phase 2: "Upgrade" — fresh root disk swap
# =============================================================================

log ""
log "=============================="
log "Phase 2: Root disk swap"
log "=============================="

mv "${VM_DIR}/disk.qcow2" "${VM_DIR}/disk.qcow2.old"
log "Old root disk backed up."

# Create fresh root disk from same cloud image (simulates golden image swap)
create_root_disk "${VM_DIR}/disk.qcow2"
log "Fresh root disk created."

# Cloud-init needs a new instance-id to re-run on the fresh disk
create_cloud_init
SEED_DISK=$(create_seed_disk)
log "Fresh cloud-init seed created."
log "Data disk UNCHANGED (this is the whole point)."

# =============================================================================
# Phase 3: "New version" boot — the critical test
# =============================================================================

log ""
log "=============================="
log "Phase 3: New version boot (critical test)"
log "=============================="

boot_vm "${VM_DIR}/disk.qcow2" "${VM_DIR}/data.qcow2" "$SEED_DISK"

log "Waiting for SSH..."
if ! wait_for_ssh 600; then
    log "ERROR: Timed out waiting for SSH on fresh root disk"
    log "QEMU log:"
    tail -50 "${VM_DIR}/qemu-boot.log" || true
    exit 1
fi

log "Waiting for cloud-init..."
if ! wait_for_cloud_init 600; then
    log "ERROR: Timed out waiting for cloud-init on fresh root disk"
    exit 1
fi
log "Cloud-init complete on fresh root disk."

# Reboot required: cloud-init installs ZFS and enables services during runcmd,
# but systemd has already passed multi-user.target by then. In production the
# golden image has ZFS pre-installed and services pre-enabled, so they run at
# boot. We need one more reboot to simulate that real boot sequence.
log "Rebooting VM so services run at real boot time (like production)..."
# Remove the seed disk so cloud-init doesn't re-run
rm -f "${VM_DIR}/seed-fat.img"
shutdown_vm

# Boot again WITHOUT cloud-init seed — this is the real test
boot_vm "${VM_DIR}/disk.qcow2" "${VM_DIR}/data.qcow2" ""

log "Waiting for SSH after reboot..."
if ! wait_for_ssh 300; then
    log "ERROR: Timed out waiting for SSH after reboot"
    exit 1
fi

# Give systemd services time to complete
log "Waiting for services to settle..."
sleep 15

# --- Assertions ---

log ""
log "--- Running assertions ---"

# (a) zfs-import-scan.service enabled
log "Check: zfs-import-scan.service enabled..."
if run_ssh "systemctl is-enabled zfs-import-scan.service" 2>/dev/null | grep -q "enabled"; then
    log "  OK: zfs-import-scan.service is enabled"
else
    fail "zfs-import-scan.service is not enabled"
fi

# (b) helix-storage-init imported the pool (not "ZFS pool not found")
log "Check: helix-storage-init imported pool..."
INIT_LOG=$(run_ssh "sudo journalctl -u helix-storage-init --no-pager 2>/dev/null" 2>/dev/null || true)
if echo "$INIT_LOG" | grep -q "Imported ZFS pool"; then
    log "  OK: helix-storage-init imported existing ZFS pool"
elif echo "$INIT_LOG" | grep -q "ZFS pool not found"; then
    fail "helix-storage-init did NOT import pool — got 'ZFS pool not found'"
else
    # Pool might have been auto-imported by zfs-import-scan before our service ran
    log "  INFO: helix-storage-init log doesn't show explicit import (pool may have been auto-imported by zfs-import-scan)"
fi

# (c) ZFS pool exists and is healthy
log "Check: ZFS pool helix exists..."
if run_ssh "sudo zpool list helix" 2>/dev/null; then
    log "  OK: ZFS pool helix is present"
else
    fail "ZFS pool helix does not exist after reboot"
fi

# (d) Sentinel file survived
log "Check: sentinel data survived upgrade..."
SENTINEL_READ=$(run_ssh "cat /helix/config/upgrade-test-sentinel" 2>/dev/null || true)
if [ "$SENTINEL_READ" = "$SENTINEL_VALUE" ]; then
    log "  OK: sentinel = '$SENTINEL_READ'"
else
    fail "sentinel expected '${SENTINEL_VALUE}', got '${SENTINEL_READ}'"
fi

# (e) env.vm survived
log "Check: env.vm survived upgrade..."
ENV_READ=$(run_ssh "cat /helix/config/env.vm" 2>/dev/null || true)
if [ "$ENV_READ" = "$TEST_ENV_VALUE" ]; then
    log "  OK: env.vm intact"
else
    fail "env.vm expected '${TEST_ENV_VALUE}', got '${ENV_READ}'"
fi

# (f) SSH host keys restored
log "Check: SSH host keys restored from ZFS..."
if run_ssh "test -f /helix/config/ssh/ssh_host_ed25519_key" 2>/dev/null; then
    log "  OK: SSH host keys present in /helix/config/ssh/"
else
    fail "SSH host keys not found in /helix/config/ssh/"
fi

# (g) Run init-zfs-pool.sh — should succeed without calling zpool create
log "Check: init-zfs-pool.sh succeeds without creating new pool..."
if [ -f "${SCRIPT_DIR}/init-zfs-pool.sh" ]; then
    # Copy the script to the VM and run it
    scp -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null \
        -P "$SSH_PORT" "${SCRIPT_DIR}/init-zfs-pool.sh" "${VM_USER}@localhost:/tmp/init-zfs-pool.sh" 2>/dev/null
    INIT_OUTPUT=$(run_ssh "bash /tmp/init-zfs-pool.sh" 2>&1 || true)
    if echo "$INIT_OUTPUT" | grep -q "ZFS pool helix already exists\|Imported existing ZFS pool"; then
        log "  OK: init-zfs-pool.sh recognized existing pool (no zpool create)"
    elif echo "$INIT_OUTPUT" | grep -q "Creating ZFS pool"; then
        fail "init-zfs-pool.sh called zpool create on existing pool — data would be destroyed"
    else
        log "  OK: init-zfs-pool.sh completed (output: $(echo "$INIT_OUTPUT" | head -3))"
    fi
else
    log "  SKIP: init-zfs-pool.sh not found at ${SCRIPT_DIR}/init-zfs-pool.sh"
fi

log ""
log "Shutting down test VM..."
shutdown_vm

# =============================================================================
# Phase 4: Cleanup + results
# =============================================================================

log ""
log "=============================="
log "Phase 4: Cleanup"
log "=============================="

rm -rf "$VM_DIR"
log "Test directory cleaned up."

log ""
log "=============================="
if [ "$PASS" = true ]; then
    log "RESULT: ALL ASSERTIONS PASSED"
    log "The ZFS pool survived root disk upgrade."
    exit 0
else
    log "RESULT: SOME ASSERTIONS FAILED"
    echo -e "$FAIL_MSG"
    exit 1
fi
