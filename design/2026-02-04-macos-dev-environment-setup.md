# macOS ARM64 Development Environment Setup - 2026-02-04

## Overview

This document describes the reproducible development environment for macOS ARM64 Helix desktop port with custom QEMU and zero-copy video streaming.

## Repository Structure

The development setup requires 3 repositories under `~/pm/`:

```
~/pm/
├── helix/           # Main Helix repository
├── qemu-utm/        # Our QEMU fork with helix-frame-export patches
├── UTM/             # UTM app repository (auto-cloned, build scripts only)
├── zed/             # Zed IDE fork (for custom builds)
└── qwen-code/       # Qwen Code agent
```

## Versions

**Current tested configuration (2026-02-06):**
- **UTM**: v5.0.1+ (main branch, commit `8d34e35b`)
- **QEMU (UTM's version)**: v10.0.2-utm (Venus branch with Vulkan support)
- **QEMU (our fork)**: utm-edition-venus + helix-frame-export patches
- **Video transport**: TCP direct on port 15937 (no socat needed)

When setting up the environment, clone the exact UTM commit to ensure build compatibility:
```bash
cd ~/pm/UTM
git checkout 8d34e35b  # Or use latest main if compatible
```

## Repository Setup

### 1. Helix (Main)

```bash
cd ~/pm
git clone git@github.com:helixml/helix.git
cd helix
# Follow standard Helix setup in README
```

### 2. QEMU-UTM (Our Fork with helix-frame-export)

This is our fork of UTM's QEMU fork, with custom patches for zero-copy GPU frame export.

```bash
cd ~/pm
git clone git@github.com:helixml/qemu-utm.git
cd qemu-utm
git checkout utm-edition-venus-helix  # Our branch (upstream venus + helix patches)
```

**Remotes:**
- `origin`: https://github.com/utmapp/qemu (upstream UTM QEMU fork)
- `helixml`: git@github.com:helixml/qemu-utm.git (our fork)

**Key patches:**
- `hw/display/helix/helix-frame-export.m` - DisplaySurface -> IOSurface -> VideoToolbox H.264 encoding
- `hw/display/helix/helix-frame-export.h` - Protocol definitions and safe helper declarations
- `hw/display/virtio-gpu-virgl.c` - DisplaySurface creation hooks + safe accessor function
- TCP server on port 15937 for streaming encoded frames (guest connects via 10.0.2.2:15937)

### 3. UTM (Build Scripts - Auto-Cloned)

The official UTM app repository provides build scripts for QEMU with all dependencies.

We need UTM's sysroot (pre-built dependencies like SPICE, virglrenderer, etc.) for building QEMU.

```bash
cd ~/pm
git clone https://github.com/utmapp/UTM.git
cd UTM
git checkout 8d34e35b  # v5.0.1+ - tested working version
```

**Why needed:**
- Provides `sysroot-macOS-arm64/` with pre-built SPICE, virglrenderer, GStreamer, etc.
- Contains `pkg-config` and library paths that QEMU's configure needs
- We do NOT modify UTM itself

**Important:**
- This is **NOT** a fork - it's the official utmapp/UTM repository
- The sysroot must be built first (one-time): `cd ~/pm/UTM && Scripts/build_dependencies.sh -p macos -a arm64`
- Version pinned to `8d34e35b` (v5.0.1+) for reproducibility

### 4. Zed (Optional - for custom IDE builds)

```bash
cd ~/pm
git clone https://github.com/zed-industries/zed.git
cd zed
# Build with: cargo build --release
```

### 5. Qwen Code (Optional - for agent features)

```bash
cd ~/pm
git clone git@github.com:helixml/qwen-code.git
```

## Build Dependencies

**Required for `./stack build-utm` (building custom QEMU):**

### Homebrew Packages
Source: UTM CI `.github/workflows/build.yml` line 89

```bash
brew install bison pkg-config gettext glib-utils libgpg-error nasm make meson cmake libclc
```

**Key packages:**
- `bison` (>= 3.0) - Parser generator (macOS ships with old 2.3)
- `nasm` - x86 assembler
- `meson` + `cmake` - Build systems
- `libclc` - Mesa/Vulkan OpenCL support
- `glib-utils` - GLib development utilities
- `libgpg-error` - Error codes for GnuPG components

### Python Packages
Source: UTM CI `.github/workflows/build.yml` line 90

```bash
pip3 install --break-system-packages --user six pyparsing pyyaml setuptools distlib mako
```

**Validation:**
`./stack build-utm` automatically checks all dependencies and errors if any are missing.

## Build Process

### Build Custom QEMU with helix-frame-export

```bash
cd ~/pm/helix
./for-mac/qemu-helix/build-qemu-standalone.sh
```

This script does EVERYTHING automatically:
1. Builds QEMU from `~/pm/qemu-utm` using UTM's sysroot at `~/pm/UTM/sysroot-macOS-arm64/`
2. Installs to the **correct UTM framework location** (see below)
3. Fixes library paths with `install_name_tool`
4. Clears UTM caches
5. Restarts UTM if running

**CRITICAL: UTM QEMU Install Path**

UTM loads QEMU from a **framework bundle**, NOT a loose dylib:
- ✅ **CORRECT:** `/Applications/UTM.app/Contents/Frameworks/qemu-aarch64-softmmu.framework/Versions/A/qemu-aarch64-softmmu`
- ❌ **WRONG:** `/Applications/UTM.app/Contents/Frameworks/libqemu-aarch64-softmmu.dylib`

Installing to the wrong location means your code changes won't run. The build script handles this automatically, but if you ever need to install manually:

```bash
# Build output goes here:
~/pm/UTM/sysroot-macOS-arm64/lib/libqemu-aarch64-softmmu.dylib

# Install to UTM framework:
sudo cp ~/pm/UTM/sysroot-macOS-arm64/lib/libqemu-aarch64-softmmu.dylib \
     /Applications/UTM.app/Contents/Frameworks/qemu-aarch64-softmmu.framework/Versions/A/qemu-aarch64-softmmu

# Fix library paths (changes absolute sysroot paths to @rpath):
sudo ~/pm/helix/scripts/fix-qemu-paths.sh

# Restart UTM:
killall UTM && sleep 2 && open /Applications/UTM.app
```

**Build cache:** Stored in `~/pm/qemu-utm/build/` (ninja incremental)
**Build time:** ~5 minutes full rebuild, seconds for incremental changes

### Verify QEMU Install

```bash
# Check version string in installed binary
strings /Applications/UTM.app/Contents/Frameworks/qemu-aarch64-softmmu.framework/Versions/A/qemu-aarch64-softmmu | grep "HELIX.*VERSION"

# Check library paths are using @rpath (not absolute sysroot paths)
otool -L /Applications/UTM.app/Contents/Frameworks/qemu-aarch64-softmmu.framework/Versions/A/qemu-aarch64-softmmu | head -10
```

### Build Desktop Images

After QEMU is ready, build the Helix desktop containers:

```bash
cd ~/pm/helix
./stack build-ubuntu   # GNOME desktop image
./stack build-sway     # Sway desktop image (lighter weight)
```

## Repository Relationship

**Important:** We use TWO separate repositories, but only ONE is a QEMU fork:

**qemu-utm** (~/pm/qemu-utm) - Our QEMU fork:
- Fork of: https://github.com/utmapp/qemu
- Our fork: https://github.com/helixml/qemu-utm
- Branch: `utm-edition-venus-helix` (based on upstream `origin/utm-edition-venus` which adds Vulkan/Venus support)
- Contains helix-frame-export module with DisplaySurface + VideoToolbox encoding
- ~21 commits on top of upstream Venus branch
- This is the actual QEMU source code we modify

**UTM** (~/pm/UTM) - Official UTM virtualization app:
- Repository: https://github.com/utmapp/UTM
- **This is NOT a QEMU repository** - it's a macOS/iOS virtualization app
- Provides build scripts (`Scripts/build_dependencies.sh`) that build QEMU with all dependencies
- **We do NOT fork this** - we clone and use the official version
- We modify the build to use our qemu-utm fork instead of downloading upstream

This separation allows:
- Clean tracking of our QEMU patches in helixml/qemu-utm
- Using UTM's comprehensive build infrastructure (SPICE, GStreamer, virglrenderer, etc.)
- Easy rebasing on both UTM and QEMU updates without conflicts

## Virtual Machine Setup

### VM Disk Image Locations

**CRITICAL: Use the external SSD VM for active development. Always maintain backups before testing.**

**⚠️ Incident 2026-02-04 15:33**: Original backup at `~/Library/Containers/com.utmapp.UTM/Data/Documents/Linux.utm.backup` was accidentally deleted with `rm -rf` during UTM patching attempt. New backup created from external SSD to `Linux.utm.backup.safe`. This incident led to adding explicit rules in CLAUDE.md forbidding `rm -rf` without user consent.

**Current VM Images (2026-02-09):**

1. **Primary (ACTIVE)**: "new-dev-vm" in UTM
   - UTM bundle: `/Volumes/Big/Linux-broken.utm` (repurposed registration)
   - Root disk: `/Volumes/Big/helix-desktop-vm/helix-desktop.utm/Data/D999E038-76E9-4C81-BCF1-80A6F2E157CF.qcow2` (256GB, 47GB used)
   - ZFS disk: `/Volumes/Big/helix-desktop-vm/helix-desktop.utm/Data/0DADCFD5-C147-4870-9C0F-925F33B5D405.qcow2` (128GB virtual, thin-provisioned)
   - CPUs: 20 cores, RAM: 60GB (61440 MB)
   - **UTM UUID**: `17DC4F96-F1A9-4B51-962B-03D85998E0E7`
   - Ubuntu 25.10 (kernel 6.17+), Docker CE 29.2.1, Go 1.25, ZFS 2.4.0
   - Desktop image: helix-ubuntu:latest (5.82GB, hash 410bc0)
   - Provisioned fresh on 2026-02-09 with `provision-vm.sh`
   - **This is the one to use for all work**

2. **Old broken VM disk** (preserved for analysis):
   - `/Volumes/Big/Linux-broken.utm/Data/780188AB-old-broken.qcow2` (745GB)
   - Broke on 2026-02-09: GNOME Shell freezes in `dma_fence_default_wait`
   - Keep for future debugging of the freeze issue

3. **Backup**: `~/Library/Containers/com.utmapp.UTM/Data/Documents/Linux.utm.backup.safe`
   - **Do not delete - this is the safety backup of the original VM**

**How to control the VM:**
```bash
# List VMs
/Applications/UTM.app/Contents/MacOS/utmctl list

# Start VM
/Applications/UTM.app/Contents/MacOS/utmctl start 17DC4F96-F1A9-4B51-962B-03D85998E0E7

# Stop VM
/Applications/UTM.app/Contents/MacOS/utmctl stop 17DC4F96-F1A9-4B51-962B-03D85998E0E7

# Check status
/Applications/UTM.app/Contents/MacOS/utmctl status 17DC4F96-F1A9-4B51-962B-03D85998E0E7
```

**SSH into the VM:**
```bash
ssh -p 2222 luke@localhost
```

### Create Ubuntu VM with provision-vm.sh

The automated provisioning script creates a fully configured VM:

```bash
# Provision on external SSD (recommended - root disk is usually too small)
./for-mac/scripts/provision-vm.sh --vm-dir /Volumes/Big/helix-desktop-vm \
    --cpus 20 --memory 61440 --disk-size 256G

# Resume if interrupted
./for-mac/scripts/provision-vm.sh --vm-dir /Volumes/Big/helix-desktop-vm --resume
```

**What it creates:**
- Ubuntu 25.10 ARM64 cloud image (kernel 6.17+ for multi-scanout)
- 256GB root disk (ext4, Docker, builds) + 128GB ZFS disk (workspaces with dedup)
- Docker CE, Go 1.25, ZFS 2.4.0, helix-drm-manager
- Helix repo cloned, desktop image built (helix-ubuntu)
- UTM .utm bundle ready to import

**The script uses Homebrew QEMU** (`brew install qemu`) for headless provisioning, then creates a UTM bundle for running with our custom QEMU (with helix-frame-export).

### Registering the VM in UTM 5.0

**UTM 5.0 does NOT auto-discover VMs from the Documents directory.**

UTM uses an internal registry in `~/Library/Containers/com.utmapp.UTM/Data/Library/Preferences/com.utmapp.UTM.plist` with security-scoped macOS bookmarks. VMs can only be registered through UTM's own UI (File > Open or drag-and-drop). Bookmarks created externally (via Swift/Python) lack proper sandbox scope and are silently ignored.

**To register a provisioned VM:**

1. **Best approach**: Open UTM GUI, File > Open, navigate to the `.utm` bundle
2. **Workaround if UTM won't open it** (e.g., bundle on external drive with no space on root): Repurpose an existing registered VM:
   ```bash
   # In the existing VM's Data/ directory, rename old disks and symlink new ones
   cd /path/to/existing.utm/Data/
   mv OLD_UUID.qcow2 OLD_UUID-backup.qcow2
   ln -s /Volumes/Big/new-vm/disk.qcow2 OLD_UUID.qcow2
   # Update config.plist Name, add QEMU arguments, etc.
   ```

### UTM QEMU Arguments (config.plist)

The following QEMU AdditionalArguments are needed for the multi-scanout pipeline:

```xml
<key>AdditionalArguments</key>
<array>
    <string>-global</string>
    <string>virtio-gpu-gl-pci.edid=on</string>
    <string>-global</string>
    <string>virtio-gpu-gl-pci.xres=5120</string>
    <string>-global</string>
    <string>virtio-gpu-gl-pci.yres=2880</string>
    <string>-global</string>
    <string>virtio-gpu-gl-pci.max_outputs=16</string>
    <string>-serial</string>
    <string>file:/tmp/qemu-serial.log</string>
</array>
```

Also set `DebugLog` to `true` and `Hypervisor` to `true` under the `QEMU` dict.

### Cloud-init Netplan MAC Address Gotcha

**Problem:** Cloud-init generates `/etc/netplan/50-cloud-init.yaml` with `match: macaddress` locked to the provisioning QEMU's MAC (`52:54:00:12:34:56` for Homebrew QEMU). When the VM runs under UTM, which assigns a different MAC (`52:42:xx:xx:xx:xx`), netplan doesn't configure the interface and the VM has no network. SSH connects (TCP handshake succeeds via QEMU port forwarding) but hangs during banner exchange because the guest can't complete the connection.

**Fix (already in provision-vm.sh):** A cloud-init `write_files` entry creates `/etc/netplan/99-helix-override.yaml` that matches by driver instead of MAC:
```yaml
network:
  version: 2
  ethernets:
    id0:
      match:
        driver: virtio_net
      dhcp4: true
      dhcp6: true
```

The `99-` prefix ensures it takes priority over cloud-init's `50-cloud-init.yaml`.

**If you're fixing an existing VM** that has this problem, boot it with Homebrew QEMU (which uses the original MAC), SSH in, and create the override file:
```bash
# Boot with Homebrew QEMU (uses the MAC cloud-init expects)
qemu-system-aarch64 -machine virt,accel=hvf -cpu host -smp 8 -m 32768 \
    -drive if=pflash,format=raw,file=/opt/homebrew/share/qemu/edk2-aarch64-code.fd,readonly=on \
    -drive if=pflash,format=raw,file=efi_vars.fd \
    -drive file=disk.qcow2,format=qcow2,if=virtio \
    -device virtio-net-pci,netdev=net0 \
    -netdev user,id=net0,hostfwd=tcp::2223-:22 \
    -nographic

# SSH in and fix
ssh -p 2223 luke@localhost
sudo tee /etc/netplan/99-helix-override.yaml << 'EOF'
network:
  version: 2
  ethernets:
    id0:
      match:
        driver: virtio_net
      dhcp4: true
      dhcp6: true
EOF
sudo netplan apply
```

### Docker on ext4, NOT ZFS

**Docker's overlay2 storage driver has compatibility issues with ZFS.** Keep Docker data on the ext4 root disk (`/var/lib/docker`). Only workspaces go on ZFS for dedup benefits.

The provisioning script creates two disks:
- **vda** (ext4, 256GB): OS, Docker images, build cache
- **vdc** (ZFS, 128GB): `/helix/workspaces` with `dedup=on` and `compression=lz4`

### Expand VM Disk

```bash
# Stop the VM
utmctl stop <UUID>

# Expand qcow2 image
qemu-img resize /path/to/disk.qcow2 1T

# Start VM and expand partition
utmctl start <UUID>

# Inside VM:
sudo growpart /dev/vda 2
sudo resize2fs /dev/vda2
df -h  # Verify
```

### Control VMs

```bash
# utmctl is at /Applications/UTM.app/Contents/MacOS/utmctl
utmctl list                    # List all VMs
utmctl start <UUID>            # Start a VM
utmctl stop <UUID>             # Stop a VM
utmctl status <UUID>           # Check status
```

## Testing Zero-Copy Video Pipeline

### Prerequisites

1. Custom QEMU with helix-frame-export built and installed
2. Ubuntu VM running with expanded disk
3. Helix desktop images built and pushed to sandbox

### Test Flow

```bash
# Build helix CLI
cd ~/pm/helix/api
CGO_ENABLED=0 go build -o /tmp/helix .

# Set up credentials
export HELIX_API_KEY=`grep HELIX_API_KEY ~/pm/helix/.env.usercreds | cut -d= -f2-`
export HELIX_URL=`grep HELIX_URL ~/pm/helix/.env.usercreds | cut -d= -f2-`
export HELIX_PROJECT=`grep HELIX_PROJECT ~/pm/helix/.env.usercreds | cut -d= -f2-`

# Start a new session
/tmp/helix spectask start --project $HELIX_PROJECT -n "video test"

# Wait for GNOME to initialize (~15 seconds)
sleep 15

# Test video streaming with benchmark
/tmp/helix spectask benchmark ses_xxx --duration 30
```

## Troubleshooting

### QEMU Build Fails

**Error:** `-spice: invalid option`
- QEMU was built without SPICE support
- Solution: Use `./stack build-utm` instead of custom QEMU configure

**Error:** `EGL_IOSURFACE_WRITE_HINT_ANGLE` undeclared
- ANGLE constant not defined in headers
- Fixed in: commit (TBD) - adds #define to ui/spice-display.c

**Error:** `d3d_tex2d` redefinition
- Variable declared twice in virtio-gpu-virgl.c
- Fixed in: commit (TBD) - removes duplicate declaration

### VM Won't Start

Check utmctl logs:
```bash
utmctl status <UUID>
/Applications/UTM.app/Contents/MacOS/utmctl list --verbose
```

### Build Cache Issues

If rebuilds take too long, check cache:
```bash
ls -lh ~/pm/UTM/build-macOS-arm64/
# Should contain built dependencies (glib, spice, etc.)
```

To clean cache (rarely needed):
```bash
rm -rf ~/pm/UTM/build-macOS-arm64/
```

## References

- UTM build documentation: https://github.com/utmapp/UTM/blob/main/Documentation/MacDevelopment.md
- UTM repository: https://github.com/utmapp/UTM
- Our QEMU fork: https://github.com/helixml/qemu-utm
- Upstream QEMU (UTM fork): https://github.com/utmapp/qemu

## Design Docs

Related design documents:
- `2026-02-04-utm-build-integration.md` - ./stack build-utm implementation
- `2026-02-03-macos-arm-port-strategy.md` - Overall porting strategy
- `2026-01-XX-zero-copy-video.md` - Zero-copy video pipeline design
