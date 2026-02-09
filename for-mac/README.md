# Helix for Mac

A standalone macOS app that runs an Ubuntu VM with the Helix AI development platform. Embeds QEMU, open-source GPU rendering frameworks, and EFI firmware — no need to install UTM or Homebrew.

## Building from Scratch

### Prerequisites

Install on your Mac:

```bash
# Go (1.24+)
brew install go

# Wails CLI
go install github.com/wailsapp/wails/v2/cmd/wails@latest

# Node.js (for frontend build)
brew install node

# QEMU (for EFI firmware files + VM provisioning)
brew install qemu

# mtools (for cloud-init FAT12 disk in VM provisioning)
brew install mtools
```

You also need:

1. **Custom QEMU build** — Our fork with helix-frame-export patches, compiled against UTM's sysroot:
   ```bash
   # Clone our QEMU fork (if not already done)
   git clone https://github.com/helixml/qemu-utm ~/pm/qemu-utm
   cd ~/pm/qemu-utm && git checkout utm-edition-venus-helix

   # Build (requires UTM sysroot at ~/pm/UTM/sysroot-macOS-arm64/)
   cd ~/pm/helix/for-mac
   ./qemu-helix/build-qemu-standalone.sh
   ```

2. **UTM sysroot** — Pre-built open-source frameworks (virglrenderer, SPICE, glib, etc.):
   ```bash
   # Clone UTM and build dependencies
   git clone https://github.com/utmapp/UTM ~/pm/UTM
   cd ~/pm/UTM
   ./scripts/build_dependencies.sh -p macos -a arm64
   ```
   Output: `~/pm/UTM/sysroot-macOS-arm64/` with ~170 files in `lib/`.

### Step 1: Build the App

```bash
cd for-mac
./scripts/build-helix-app.sh
```

This runs `wails build`, then bundles QEMU, 27 open-source frameworks, EFI firmware, and Vulkan ICD config into the app. Output: `build/bin/helix-for-mac.app` (~243MB).

Use `--skip-wails` to re-run just the packaging steps without rebuilding the Go/frontend code.

### Step 2: Create the DMG

```bash
./scripts/create-dmg.sh
```

Output: `build/bin/Helix-for-Mac.dmg` (~32MB compressed).

### Step 3: Code Signing (Optional)

Without an Apple Developer account ($99/year), the app uses ad-hoc signing. Users on other Macs must go to System Settings > Privacy & Security > "Open Anyway".

```bash
# Ad-hoc (default, done automatically by build-helix-app.sh)
./scripts/sign-app.sh

# With Developer ID certificate
./scripts/sign-app.sh --identity "Developer ID Application: Your Name (TEAMID)"

# With notarization (Gatekeeper approved, no user intervention needed)
./scripts/sign-app.sh \
  --identity "Developer ID Application: Your Name (TEAMID)" \
  --notarize \
  --apple-id you@email.com \
  --team-id XXXXX
```

## Provisioning a VM

The app needs a VM image to run. The provisioning script creates one from scratch.

### Run Provisioning

```bash
# From the helix repo root
./for-mac/scripts/provision-vm.sh
```

This takes 30-60 minutes on first run and:

1. Downloads Ubuntu 25.10 ARM64 cloud image
2. Creates 128GB root disk + 128GB ZFS data disk (thin-provisioned)
3. Boots a headless QEMU on port **2223** (avoids conflicts with any dev VM on 2222)
4. SSHs in to install Docker, ZFS 2.4.0, Go 1.25, helix-drm-manager
5. Clones helix repo, builds the desktop Docker image
6. Shuts down and creates a UTM bundle

**Resumable:** If interrupted, run `./for-mac/scripts/provision-vm.sh --resume` to continue from the last completed step.

### Output

- VM directory: `~/Library/Application Support/Helix/vm/helix-desktop/`
- UTM bundle: `~/Library/Application Support/Helix/vm/helix-desktop/helix-desktop.utm` (auto-linked into UTM's documents)
- Disk image: `~/Library/Application Support/Helix/vm/helix-desktop/disk.qcow2`

### Custom Options

```bash
./for-mac/scripts/provision-vm.sh --disk-size 256G --cpus 8 --memory 16384
```

## What's in the App Bundle

```
helix-for-mac.app/
  Contents/
    MacOS/
      Helix for Mac                        # Wails app (9MB)
      qemu-system-aarch64                  # QEMU wrapper executable (75KB)
      libqemu-aarch64-softmmu.dylib        # Custom QEMU core (33MB)
    Frameworks/                             # 27 open-source frameworks (73MB)
      virglrenderer.1.framework/            # GPU 3D rendering
      spice-server.1.framework/             # SPICE protocol
      glib-2.0.0.framework/                 # GLib
      vulkan_kosmickrisp.framework/         # Mesa Vulkan driver
      ...
    Resources/
      firmware/                             # EFI boot firmware (128MB)
        edk2-aarch64-code.fd
        edk2-arm-vars.fd
      vulkan/icd.d/                         # Vulkan driver config
        kosmickrisp_mesa_icd.json
      vm/                                   # Pre-provisioned VM (compressed)
        disk.qcow2                          # Root disk (~7GB compressed)
        zfs-data.qcow2                      # ZFS workspace disk (~11GB compressed)
        efi_vars.fd                         # EFI variables (64MB)
      NOTICES.md                            # Open-source license notices
```

All bundled libraries are open-source (MIT, BSD, LGPL, GPL). See `design/2026-02-08-helix-app-dmg-packaging.md` for the full dependency tree and licensing details.

## Development

### Running in Dev Mode

```bash
cd for-mac
wails dev
```

In dev mode, the app finds QEMU from your system PATH (`brew install qemu`) and firmware from Homebrew (`/opt/homebrew/share/qemu/`). No bundled frameworks needed.

### Standalone Probe Tools

The files `display_capture.go`, `iosurface_probe.go`, and `virgl_probe.go` are standalone test programs excluded from the main build (`//go:build ignore`). Run them individually:

```bash
go run display_capture.go    # Test IOSurface display capture
go run iosurface_probe.go    # Probe for active IOSurfaces
go run virgl_probe.go        # Probe virglrenderer availability
```

### Project Structure

| File | Purpose |
|------|---------|
| `main.go` | Wails entry point, app menu |
| `app.go` | Application state, VM lifecycle, video |
| `vm.go` | QEMU process management, bundled binary discovery |
| `utm.go` | UTM integration (dev mode fallback) |
| `encoder.go` | Software video encoder |
| `vsock.go` | Virtio-vsock for host-guest frame transfer |
| `settings.go` | Persistent settings (~/Library/Application Support/Helix/settings.json) |
| `scripts/build-helix-app.sh` | Build .app with embedded QEMU |
| `scripts/create-dmg.sh` | Package into .dmg |
| `scripts/sign-app.sh` | Code signing + notarization |
| `scripts/provision-vm.sh` | Create VM from scratch |
