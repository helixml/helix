# Helix.app DMG Packaging

**Date:** 2026-02-08
**Status:** Implemented (with bundled VM images)
**Branch:** feature/macos-arm-desktop-port

## Overview

Package the Helix desktop app as a standalone `.dmg` that users can download and run on any Mac (Apple Silicon). The app embeds QEMU, all required open-source frameworks, EFI firmware, and pre-provisioned VM disk images so it works without installing UTM, Homebrew, or running any provisioning steps.

## Architecture

### What's Inside the App Bundle

```
Helix for Mac.app/
├── Contents/
│   ├── MacOS/
│   │   ├── Helix for Mac              # Wails app (Go + embedded React frontend)
│   │   └── libqemu-aarch64-softmmu.dylib  # Custom QEMU (~33MB)
│   ├── Frameworks/                     # ~27 open-source framework dylibs (~200MB)
│   │   ├── pixman-1.0.framework/       # Image compositing (MIT)
│   │   ├── glib-2.0.0.framework/       # GLib (LGPL 2.1)
│   │   ├── virglrenderer.1.framework/  # virtio-gpu 3D (MIT)
│   │   ├── spice-server.1.framework/   # SPICE protocol (LGPL 2.1)
│   │   ├── vulkan_kosmickrisp.framework/ # Mesa Vulkan (MIT)
│   │   └── ... (see full list below)
│   ├── Resources/
│   │   ├── firmware/
│   │   │   ├── edk2-aarch64-code.fd    # EFI firmware (BSD)
│   │   │   └── edk2-arm-vars.fd        # EFI vars template (BSD)
│   │   ├── vulkan/
│   │   │   └── icd.d/
│   │   │       └── kosmickrisp_mesa_icd.json  # Vulkan ICD config
│   │   ├── vm/                          # Pre-provisioned VM (compressed qcow2)
│   │   │   ├── disk.qcow2              # Root disk (~7GB compressed, 256G virtual)
│   │   │   ├── zfs-data.qcow2          # ZFS workspace disk (~11GB compressed, 128G virtual)
│   │   │   └── efi_vars.fd             # EFI variables (64MB)
│   │   └── NOTICES.md                   # Open-source license notices
│   └── Info.plist
```

### What's NOT Inside

- UTM's Swift/SwiftUI GUI code (proprietary to turing.llc)
- UTM.app binary or `utmctl` CLI
- QEMU architectures other than aarch64 (saves ~500MB)
- GStreamer plugins not needed by SPICE

### First-Launch VM Extraction

The compressed qcow2 disk images are bundled in the app bundle's `Contents/Resources/vm/` directory. On first launch, the app copies them to `~/Library/Application Support/Helix/vm/helix-desktop/` (a writable location) since QEMU needs write access to the disk images. This is done by `vm.go:ensureVMExtracted()` using streaming `io.Copy` to avoid loading multi-GB files into memory.

The qcow2 files are already compressed by `qemu-img convert -c` during the build step, so QEMU reads them directly — no decompression step needed. QEMU writes new data uncompressed, so the disk images grow over time as the VM is used.

### Size Budget

| Component | Size |
|-----------|------|
| Main executable (Wails) | ~9MB |
| QEMU dylib | ~33MB |
| 27 frameworks | ~73MB |
| EFI firmware | ~128MB |
| VM root disk (compressed qcow2) | ~7GB |
| VM ZFS data disk (compressed qcow2) | ~11GB |
| EFI vars | 64MB |
| **Total app bundle** | **~18GB** |
| **DMG (UDZO compressed)** | **~17GB** |

The DMG doesn't compress much below the app bundle size because the qcow2 files are already zlib-compressed internally.

### Framework Dependency Tree

```
QEMU (libqemu-aarch64-softmmu.dylib)
├── pixman-1.0        (direct, MIT)
├── jpeg.62           (direct, IJG/BSD)
├── epoxy.0           (direct, MIT)
├── zstd.1            (direct, BSD)
├── slirp.0           (direct, BSD)
│   └── glib-2.0.0
├── usbredirparser.1  (direct, LGPL 2.1)
├── usb-1.0.0         (direct, LGPL 2.1)
├── spice-server.1    (direct, LGPL 2.1)
│   ├── ssl.1.1 → crypto.1.1
│   ├── opus.0
│   ├── gstreamer-1.0.0 → gstapp-1.0.0 → gstbase-1.0.0
│   ├── pixman-1.0 (shared)
│   └── jpeg.62 (shared)
├── virglrenderer.1   (direct, MIT)
│   ├── epoxy.0 (shared)
│   └── vulkan.1 → vulkan_kosmickrisp
├── gio-2.0.0         (direct, LGPL 2.1)
│   └── intl.8 → iconv.2
├── gobject-2.0.0     (direct, LGPL 2.1)
│   ├── glib-2.0.0 (shared)
│   └── ffi.8
├── glib-2.0.0        (direct, LGPL 2.1)
│   ├── iconv.2
│   └── intl.8
└── gmodule-2.0.0     (direct, LGPL 2.1)
```

Total: ~27 frameworks, all open-source (MIT, BSD, LGPL, IJG).

System frameworks used (not bundled — provided by macOS):
- Hypervisor.framework, ParavirtualizedGraphics.framework
- Metal.framework, CoreAudio.framework, CoreVideo.framework
- CoreMedia.framework, VideoToolbox.framework
- IOSurface.framework, OpenGL.framework, IOKit.framework
- CoreFoundation.framework, vmnet.framework
- libz, libsasl2, libpam, libbz2, libSystem, libobjc

## Code Signing

### Current State (No Developer ID)

Ad-hoc signing (`codesign -s -`):
- Works on the build machine
- Other Macs reject it — users must go to System Settings > Privacy & Security > "Open Anyway"
- macOS Sequoia (15.0+) removed the right-click → Open bypass, making this more friction

### With Developer ID ($99/year Apple Developer Account)

1. **Sign inside-out:** frameworks → QEMU dylib → main app
2. **Hardened runtime:** `--options runtime`
3. **Entitlements:** `com.apple.security.hypervisor` (HVF), `com.apple.vm.networking` (vmnet), JIT, unsigned memory
4. **Notarize:** `xcrun notarytool submit` → wait → `xcrun stapler staple`
5. **Result:** Gatekeeper accepts on any Mac, no user intervention needed

### Entitlements Required

| Entitlement | Why |
|-------------|-----|
| `com.apple.security.hypervisor` | QEMU HVF acceleration |
| `com.apple.vm.networking` | QEMU vmnet.framework networking |
| `com.apple.security.cs.allow-jit` | QEMU TCG fallback JIT |
| `com.apple.security.cs.allow-unsigned-executable-memory` | QEMU code generation |
| `com.apple.security.cs.allow-dyld-environment-variables` | Loading bundled frameworks |
| `com.apple.security.cs.disable-library-validation` | Loading non-Apple-signed frameworks |

## Licensing Obligations

All bundled libraries are open-source. Requirements:

- **GPL v2 (QEMU):** Source code must be available. Our fork is at `github.com/helixml/qemu-utm`.
- **LGPL 2.1 (GLib, GIO, SPICE, etc.):** Dynamically linked as frameworks, satisfying LGPL. Users can replace these dylibs.
- **MIT/BSD (pixman, virglrenderer, epoxy, etc.):** Include copyright notices.

`NOTICES.md` is bundled in `Contents/Resources/` with all attributions and source URLs. The About dialog also references the GPL obligations.

## Build Instructions

### Quick Build

```bash
cd for-mac
./scripts/build-helix-app.sh          # Builds app + bundles VM images (~18GB total)
./scripts/create-dmg.sh               # Creates DMG (~17GB)
```

**Disk space:** The app bundle with VM images is ~18GB. If your boot volume is low on space, use an external volume:

```bash
./scripts/create-dmg.sh --build-dir /Volumes/Big/helix-build
```

### Prerequisites

1. **Wails CLI:**
   ```bash
   go install github.com/wailsapp/wails/v2/cmd/wails@latest
   ```

2. **Custom QEMU (already built for this project):**
   ```bash
   ./qemu-helix/build-qemu-standalone.sh
   ```
   Output: `~/pm/UTM/sysroot-macOS-arm64/lib/libqemu-aarch64-softmmu.dylib`

3. **UTM sysroot (already built):**
   Located at `~/pm/UTM/sysroot-macOS-arm64/` with all framework dylibs.

4. **Homebrew QEMU (for EFI firmware):**
   ```bash
   brew install qemu
   ```

### Build Steps Detail

1. **`build-helix-app.sh`** runs `wails build`, then:
   - Copies QEMU dylib into `Contents/MacOS/`
   - Copies 27 frameworks from UTM's framework directory
   - Copies EFI firmware into `Contents/Resources/firmware/`
   - Creates Vulkan ICD config pointing to bundled KosmicKrisp
   - Compresses and bundles VM disk images from `~/Library/Application Support/Helix/vm/helix-desktop/` into `Contents/Resources/vm/`
   - Copies `NOTICES.md` into `Contents/Resources/`
   - Fixes dylib paths with `install_name_tool`
   - Ad-hoc signs everything

2. **`create-dmg.sh`** creates a `.dmg` with:
   - The signed .app
   - /Applications symlink (drag-and-drop install)
   - HFS+ filesystem, UDZO compression

3. **`sign-app.sh`** (when Developer ID is available):
   ```bash
   ./scripts/sign-app.sh --identity "Developer ID Application: ..." --notarize \
     --apple-id you@email.com --team-id XXXXX
   ```

## VM Image Builder

The `for-mac/scripts/provision-vm.sh` creates a fresh VM from scratch. This must be run before `build-helix-app.sh` to produce the VM images that get bundled into the app.

### What It Does

1. Downloads Ubuntu 25.10 ARM64 cloud image
2. Creates 256G root disk + 128G ZFS data disk (thin-provisioned)
3. Generates cloud-init seed (FAT12) with user config + Docker
4. Boots headless QEMU on port 2223 (avoids dev VM on 2222)
5. Sets up 16GB swap (needed for Zed release builds with LTO)
6. SSHs in to install ZFS 2.4.0, Go 1.25, helix-drm-manager
7. Clones helix repo, builds Zed from source (release mode)
8. Builds desktop Docker image (helix-ubuntu)
9. Shuts down, creates UTM bundle

**Memory:** Uses 32GB RAM (`MEMORY_MB=32768`). Zed release builds with LTO require >16GB to avoid OOM.

### Usage

```bash
# Full provisioning from scratch
./for-mac/scripts/provision-vm.sh

# Resume interrupted run
./for-mac/scripts/provision-vm.sh --resume

# Custom sizing
./for-mac/scripts/provision-vm.sh --disk-size 256G --cpus 8 --memory 16384
```

### Output

- VM directory: `~/Library/Application Support/Helix/vm/helix-desktop/`
- UTM bundle: `~/Library/Application Support/Helix/vm/helix-desktop/helix-desktop.utm`
- Auto-linked into UTM's documents directory

## How QEMU is Loaded

The Go app (`vm.go`) finds QEMU using a search order:

1. **Bundled:** `Contents/MacOS/libqemu-aarch64-softmmu.dylib` (standalone distribution)
2. **System PATH:** `qemu-system-aarch64` (development with `brew install qemu`)

Similarly for firmware and Vulkan ICD. This means the app works both:
- As a standalone .app (production/distribution)
- When run via `wails dev` or `go run` (development)

## File Inventory

| File | Purpose |
|------|---------|
| `for-mac/scripts/build-helix-app.sh` | Build .app with embedded QEMU + frameworks |
| `for-mac/scripts/create-dmg.sh` | Package .app into .dmg |
| `for-mac/scripts/sign-app.sh` | Code signing + notarization |
| `for-mac/build/darwin/entitlements.plist` | macOS entitlements for QEMU |
| `for-mac/build/darwin/Info.plist` | App bundle metadata (com.helixml.Helix) |
| `for-mac/vm.go` | VM manager with bundled QEMU discovery + first-launch VM extraction |
| `for-mac/NOTICES.md` | Open-source license notices (bundled in app) |
| `for-mac/README.md` | Build-from-scratch instructions |
| `for-mac/scripts/provision-vm.sh` | Automated VM provisioning from scratch |
