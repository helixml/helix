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

**Current tested configuration:**
- **UTM**: v5.0.1+ (main branch, commit `8d34e35b`)
- **QEMU (UTM's version)**: v10.0.2-utm
- **QEMU (our fork)**: v10.0.2-utm + 3 helix-frame-export patches (commit `886c4e4797`)

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
git checkout utm-edition  # Our branch with helix-frame-export
```

**Remotes:**
- `origin`: https://github.com/utmapp/qemu (upstream UTM QEMU fork)
- `helixml`: git@github.com:helixml/qemu-utm.git (our fork)

**Key patches:**
- `hw/display/helix/helix-frame-export.m` - IOSurface extraction and VideoToolbox H.264 encoding
- Custom virtio-gpu-gl-pci integration for Metal texture sharing
- vsock server for streaming encoded frames to host

### 3. UTM (Build Scripts - Auto-Cloned)

The official UTM app repository provides build scripts for QEMU with all dependencies.

**You do NOT need to clone this manually** - `./stack build-utm` will auto-clone it for you.

If you want to clone it manually:
```bash
cd ~/pm
git clone https://github.com/utmapp/UTM.git
cd UTM
git checkout 8d34e35b  # v5.0.1+ - tested working version
```

**Why needed:**
- `Scripts/build_dependencies.sh` - Builds QEMU with SPICE, GStreamer, virglrenderer, etc.
- Provides proper build configuration that matches UTM requirements
- Includes all patches and build flags needed for macOS ARM64

**Important:**
- This is **NOT** a fork - it's the official utmapp/UTM repository
- We never modify UTM - it's just a build tool dependency
- Version pinned to `8d34e35b` (v5.0.1+) for reproducibility
- Auto-managed by `./stack build-utm`

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
./stack build-utm
```

This command:
1. Validates all Homebrew and Python dependencies (errors if missing)
2. Uses `UTM/Scripts/build_dependencies.sh` to build all dependencies including:
   - ANGLE (from WebKit) for EGL/OpenGL ES support
   - SPICE server with OpenGL
   - virglrenderer with Metal backend
   - GStreamer
3. Points the script to our `qemu-utm` fork (not upstream)
4. Builds QEMU with:
   - SPICE support with `gl=es` option
   - All helix-frame-export patches
   - OpenGL ES / Vulkan acceleration
5. Outputs to `~/pm/UTM/sysroot-macOS-arm64/lib/libqemu-aarch64-softmmu.dylib`

**Build cache:** Stored in `~/pm/UTM/build-macOS-arm64/` for faster rebuilds
**Build time:** 30-60 minutes on first build, ~5 minutes on incremental

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
- Branch: `utm-edition`
- Contains our helix-frame-export module (3 commits on top of v10.0.2-utm)
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

**Current VM Images:**

1. **Primary (ACTIVE)**: `/Volumes/Helix VM/Linux.utm`
   - Location: External NVMe SSD
   - Disk: 1TB capacity, 506GB used, 1007GB partition
   - CPUs: 20 cores, RAM: 64GB
   - **UTM UUID**: `01CECE09-B09D-48A4-BAB6-D046C06E3A68` (use this for utmctl commands)
   - Config.plist UUID: `17DC4F96-F1A9-4B51-962B-03D85998E0E7` (different from UTM's registration)
   - Status: Expanded and ready for development, running custom QEMU with helix-frame-export
   - **This is the one to use for all work**

2. **Backup (large)**: `~/Library/Containers/com.utmapp.UTM/Data/Documents/Linux.utm.backup.safe`
   - Location: Internal disk (UTM container)
   - Disk: 506GB (rsync copy from external SSD, created 2026-02-04 15:35, completed 16:24)
   - Status: Fresh backup created after accidental deletion
   - **Do not delete - this is the safety backup**

3. **Original (small)**: `~/Documents/UTM/Linux.utm.small-backup`
   - Location: Internal disk
   - Disk: 11GB (original small image before expansion)
   - Status: Original VM, moved out of the way
   - **Keep as reference, do not use for development**

**How to control the external SSD VM:**
```bash
# List VMs
/Applications/UTM.app/Contents/MacOS/utmctl list

# Start the external SSD VM
/Applications/UTM.app/Contents/MacOS/utmctl start 01CECE09-B09D-48A4-BAB6-D046C06E3A68

# Stop the VM
/Applications/UTM.app/Contents/MacOS/utmctl stop 01CECE09-B09D-48A4-BAB6-D046C06E3A68

# Check status
/Applications/UTM.app/Contents/MacOS/utmctl status 01CECE09-B09D-48A4-BAB6-D046C06E3A68
```

**Note:** To add the external VM to UTM, use: `open -a UTM "/Volumes/Helix VM/Linux.utm"`

### Create Ubuntu VM

1. Download Ubuntu ARM64 server ISO
2. Create VM in UTM GUI or via config file
3. Install Ubuntu
4. Configure for development (SSH, rsync, etc.)

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
