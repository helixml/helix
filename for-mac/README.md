# Helix for Mac

A standalone macOS app that runs an Ubuntu VM with the Helix AI development platform. Embeds QEMU, open-source GPU rendering frameworks, and EFI firmware — no need to install UTM or Homebrew.

The app bundle is ~300MB. VM disk images (~18GB) are downloaded from Cloudflare R2 on first launch with progress UI and resume support.

## Building from Scratch

There are two phases: **one-time setup** (dependencies you install once) and the **build itself** (3 commands).

### One-Time Setup

#### 1. Install Homebrew packages

```bash
brew install go node qemu mtools
```

- **go** (1.24+) — compiles the app
- **node** — builds the React frontend
- **qemu** — provides EFI firmware files (`edk2-aarch64-code.fd`)
- **mtools** — used by VM provisioning to create cloud-init FAT12 disks

#### 2. Install the Wails CLI

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@latest
```

Wails is the Go framework that produces a native `.app` bundle with an embedded WebView frontend.

#### 3. Build the UTM sysroot

The app bundles 27 open-source frameworks (virglrenderer, SPICE, glib, etc.) that QEMU links against. These come from UTM's dependency build system:

```bash
git clone https://github.com/utmapp/UTM ~/pm/UTM
cd ~/pm/UTM
./scripts/build_dependencies.sh -p macos -a arm64
```

This produces `~/pm/UTM/sysroot-macOS-arm64/` containing the framework bundles and libraries. It only needs to be run once (or when updating UTM dependencies).

#### 4. Build our custom QEMU

We maintain a QEMU fork with helix-frame-export patches for host-guest video transfer. It must be compiled against the UTM sysroot from step 3:

```bash
git clone https://github.com/helixml/qemu-utm ~/pm/qemu-utm
cd ~/pm/qemu-utm && git checkout utm-edition-venus-helix

cd ~/pm/helix/for-mac
./qemu-helix/build-qemu-standalone.sh
```

Output: `~/pm/UTM/sysroot-macOS-arm64/lib/libqemu-aarch64-softmmu.dylib` and `~/pm/UTM/sysroot-macOS-arm64/bin/qemu-system-aarch64`.

#### 5. Set up code signing (required for the app to launch)

macOS kills unsigned or ad-hoc signed apps on launch with `SIGKILL (Code Signature Invalid)`. You need an Apple Developer ID certificate ($99/year from [developer.apple.com](https://developer.apple.com)).

Create `for-mac/.env.signing`:

```
APPLE_SIGNING_IDENTITY="Developer ID Application: Your Name (TEAMID)"
APPLE_TEAM_ID="TEAMID"
APPLE_ID="you@email.com"
```

For notarization (so Gatekeeper accepts the app without "Open Anyway"), store credentials in keychain. You need an [app-specific password](https://appleid.apple.com) (under Sign-In and Security > App-Specific Passwords):

```bash
xcrun notarytool store-credentials "helix-notarize" \
  --apple-id you@email.com \
  --team-id TEAMID \
  --password "xxxx-xxxx-xxxx-xxxx"
```

**Without a Developer ID:** The app will be ad-hoc signed. It will only launch if the user goes to System Settings > Privacy & Security > "Open Anyway" after each install.

### Build

After one-time setup is done, building the app is three commands:

```bash
cd for-mac

# 1. Build the app (compiles Go + React, bundles QEMU + frameworks + firmware)
./scripts/build-helix-app.sh

# 2. Create the DMG (auto-signs with Developer ID if .env.signing exists)
./scripts/create-dmg.sh

# 3. (Optional) Notarize + upload to CDN
./scripts/create-dmg.sh --notarize --upload --version v1.0.0
```

That's it. Here's what each step does:

**`build-helix-app.sh`** runs `wails build`, then bundles QEMU, 27 open-source frameworks, EFI firmware, Vulkan ICD config, and a VM manifest into the app. Output: `build/bin/helix-for-mac.app` (~300MB). Use `--skip-wails` to re-run just the packaging steps without recompiling Go/frontend code. VM disk images are **not** bundled — they are downloaded from the CDN on first launch.

**`create-dmg.sh`** automatically calls `sign-app.sh` before packaging (if `.env.signing` exists), then creates `build/bin/Helix-for-Mac.dmg` with ULFO (lzfse) compression. With `--notarize`, it also submits the DMG to Apple's notary service and staples the ticket.

**`--upload`** pushes the DMG and VM images to Cloudflare R2:
- `s3://helix-releases/desktop/{version}/Helix-for-Mac.dmg`
- `s3://helix-releases/vm/{version}/disk.qcow2`, `zfs-data.qcow2`, `efi_vars.fd`
- `s3://helix-releases/vm/{version}/manifest.json`
- `s3://helix-releases/desktop/latest.json`

R2 upload requires a `.env.r2` file (copy from `.env.r2.example` and fill in your credentials).

### Signing Without a DMG (Direct Install)

To sign and install the `.app` directly without creating a DMG:

```bash
./scripts/sign-app.sh                   # Signs with Developer ID from .env.signing
cp -R build/bin/helix-for-mac.app /Applications/
```

To re-sign after changing `.env.signing` or the app bundle:

```bash
./scripts/sign-app.sh --notarize        # Sign + notarize the .app directly
```

## Trial and Licensing

The app includes a 24-hour free trial and offline license key validation.

**Trial flow:**
1. On first launch, the VM page shows a "Start 24-Hour Free Trial" button
2. During the trial, a countdown badge shows remaining time
3. After 24 hours, the VM refuses to start and prompts for a license key

**License validation:**
- License keys are validated offline using ECDSA P-256 signature verification
- Compatible with Launchpad license keys (trial, community, enterprise)
- License keys can be obtained from [deploy.helix.ml](https://deploy.helix.ml/licenses/new)
- Validated keys are stored in `~/Library/Application Support/Helix/settings.json`

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
      vm/                                   # VM manifest + EFI vars
        vm-manifest.json                    # CDN download manifest (SHA256, sizes, URLs)
        efi_vars.fd                         # EFI variables (64MB)
      NOTICES.md                            # Open-source license notices
```

VM disk images (~18GB) are downloaded from the CDN on first launch and stored at `~/Library/Application Support/Helix/vm/helix-desktop/`.

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
| `app.go` | Application state, VM lifecycle, video, download/license wiring |
| `vm.go` | QEMU process management, bundled binary discovery |
| `download.go` | VM image CDN downloader with HTTP Range resume + SHA256 |
| `license.go` | 24h trial + ECDSA license validation (offline) |
| `settings.go` | Persistent settings (~/Library/Application Support/Helix/settings.json) |
| `utm.go` | UTM integration (dev mode fallback) |
| `encoder.go` | Software video encoder |
| `vsock.go` | Virtio-vsock for host-guest frame transfer |
| `scripts/build-helix-app.sh` | Build .app with embedded QEMU + VM manifest |
| `scripts/create-dmg.sh` | Package into .dmg + upload to R2 |
| `scripts/sign-app.sh` | Code signing + notarization |
| `scripts/provision-vm.sh` | Create VM from scratch |
| `.env.r2.example` | Template for Cloudflare R2 credentials |
