# Building Custom QEMU for UTM

This directory contains scripts to build QEMU with the helix-frame-export module for use with UTM.

## Quick Start

```bash
# 1. Build dependencies (one-time, 30-60 minutes)
cd ~/pm/helix
./scripts/build-dependencies.sh

# 2. Build QEMU (2-5 minutes, repeat as needed)
./scripts/build-qemu-standalone.sh

# 3. Install into UTM.app
./scripts/install-qemu-to-utm.sh
```

## Architecture

### Two-Phase Build

**Phase 1: Dependencies (sysroot)**
- Build 28 packages: virglrenderer, SPICE, GLib, GStreamer, etc.
- Output: `~/pm/UTM/sysroot-macOS-arm64/` (the "sysroot")
- Only needs to run once (or when dependencies change)
- Takes 30-60 minutes

**Phase 2: QEMU**
- Builds QEMU using dependencies from sysroot
- Output: `~/pm/UTM/sysroot-macOS-arm64/lib/libqemu-aarch64-softmmu.dylib`
- Runs quickly (2-5 minutes)
- Repeat whenever you change helix-frame-export code

### What is the Sysroot?

The **sysroot** (`~/pm/UTM/sysroot-macOS-arm64/`) is a directory containing:

```
sysroot-macOS-arm64/
├── lib/                    # Built libraries
│   ├── libvirglrenderer.1.dylib
│   ├── libspice-server.so
│   ├── libglib-2.0.dylib
│   └── pkgconfig/          # .pc files for pkg-config
├── include/                # Header files
├── host/
│   └── bin/
│       └── pkg-config      # Custom pkg-config that searches sysroot
└── Frameworks/             # macOS frameworks
```

This is like a mini `/usr/local` that contains everything QEMU needs to build.

### What is Meson?

**Meson** is the build system QEMU 10.x uses (replaced autotools `./configure`).

Key differences from autotools:
- Configuration: `meson setup build/` (creates build directory)
- Building: `ninja` (runs in build directory)
- Installing: `ninja install`
- Cross-compilation: Uses cross-files to specify compilers, flags, etc.

## Scripts

### build-dependencies.sh

Builds all QEMU dependencies using UTM's build scripts. Creates the sysroot.

**When to run:**
- First time setup
- When you need to update a dependency (rare)

**What it does:**
1. Clones UTM repo (for build scripts only)
2. Runs `UTM/Scripts/build_dependencies.sh`
3. Replaces vanilla QEMU source with our helix fork
4. Builds everything → sysroot

**Time:** 30-60 minutes

### build-qemu-standalone.sh

Builds QEMU using existing sysroot. No UTM checkout needed after initial dependency build.

**When to run:**
- Every time you change helix-frame-export code
- When testing QEMU changes

**What it does:**
1. Generates meson cross-compilation files
2. Configures QEMU with meson (auto-detects SPICE, virglrenderer, etc.)
3. Builds with ninja
4. Installs to sysroot

**Time:** 2-5 minutes

### fix-qemu-paths.sh

Fixes dynamic library paths in the QEMU binary so UTM can find its frameworks.

**What it does:**
- Changes absolute paths (e.g., `/Users/luke/pm/UTM/sysroot-macOS-arm64/lib/libpixman.dylib`)
- To relative paths (e.g., `@rpath/pixman-1.0.framework/Versions/A/pixman-1.0`)
- Leaves Homebrew paths alone (capstone, gnutls)

## How SPICE Detection Works

QEMU has 100+ optional features that are auto-detected by meson.

**For SPICE:**
1. Meson looks for `spice-protocol.pc` and `spice-server.pc`
2. It uses `pkg-config` to find them
3. UTM's custom pkg-config searches the sysroot
4. If found → SPICE enabled, if not → SPICE disabled

**This is why the sysroot is critical:**
- It contains the SPICE libraries
- It contains a custom pkg-config that knows where to look
- Without it, meson can't find SPICE → QEMU built without SPICE support

## Troubleshooting

### Error: `-spice: invalid option`

QEMU was built without SPICE support. This means meson didn't find SPICE libraries.

**Fix:**
1. Check sysroot exists: `ls ~/pm/UTM/sysroot-macOS-arm64/lib/pkgconfig/spice-*.pc`
2. Rebuild with: `./scripts/build-qemu-standalone.sh`
3. Check meson log: `grep -i spice ~/pm/qemu-utm/build/meson-logs/meson-log.txt`

### Error: `spice protocol support: NO`

Meson configured without SPICE. Check:
```bash
# Verify SPICE libraries are in sysroot
ls ~/pm/UTM/sysroot-macOS-arm64/lib/pkgconfig/spice-*.pc

# Verify custom pkg-config exists
ls ~/pm/UTM/sysroot-macOS-arm64/host/bin/pkg-config

# Test pkg-config can find SPICE
~/pm/UTM/sysroot-macOS-arm64/host/bin/pkg-config --modversion spice-protocol
```

### Build fails with missing dependencies

You need to build the sysroot first:
```bash
./scripts/build-dependencies.sh
```

## Environment Variables

Optional overrides:

```bash
# Use different QEMU source location
export QEMU_SRC=~/my-qemu-fork
./scripts/build-qemu-standalone.sh

# Use different sysroot location
export SYSROOT=~/my-sysroot
./scripts/build-qemu-standalone.sh
```
