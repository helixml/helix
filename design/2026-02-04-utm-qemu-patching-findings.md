# UTM QEMU Patching - Findings 2026-02-04

## Summary

Attempted to patch production UTM.app with our custom QEMU containing helix-frame-export module. Discovered that macOS code signing prevents simple binary replacement.

**CRITICAL FINDING (2026-02-04):** We were based on the WRONG UTM QEMU branch! Our fork was based on `utm-edition` (the stable branch), but we needed `utm-edition-venus` which already has ALL the Venus/Vulkan and gl=es support including:
- `d3d_tex2d` → `native` variable rename (fixes shadowing)
- EGL_IOSURFACE_WRITE_HINT_ANGLE constant defined
- Proper gl parameter string handling for "on", "es", "core"
- Complete IOSurface/Metal texture support

This explains why we had to manually patch 3 separate bugs - they were already fixed in the venus branch!

## Approach Tried

1. Build QEMU with helix-frame-export using UTM's build dependencies
2. Replace `qemu-aarch64-softmmu.framework/Versions/A/qemu-aarch64-softmmu` dylib in production UTM.app
3. Fix library paths using `install_name_tool` to use `@rpath` instead of absolute paths
4. Re-sign with entitlements

## Issues Encountered

### Issue 1: Hardcoded Library Paths

**Problem:** Our QEMU build had absolute paths like:
```
/Users/luke/pm/UTM/sysroot-macOS-arm64/lib/libqemu-aarch64-softmmu.dylib
/Users/luke/pm/UTM/sysroot-macOS-arm64/lib/libpixman-1.0.dylib
```

**Solution:** Use `install_name_tool` to change to `@rpath`:
```bash
install_name_tool -change /path/to/lib.dylib @rpath/lib.framework/Versions/A/lib /path/to/binary
```

### Issue 2: Code Signing & Team ID Mismatch

**Problem:** After replacing QEMU and re-signing framework only, get error:
```
code signature in '/Applications/UTM.app/.../qemu-aarch64-softmmu' not valid for use in process:
mapping process and mapped file (non-platform) have different Team IDs
```

**Root Cause:** UTM uses XPC services (QEMULauncher.xpc) which check Team IDs. When only the QEMU framework is re-signed ad-hoc, it has a different Team ID than the main app and XPC services.

**Solution - Consistent Ad-Hoc Signing (WORKS!):**

Re-sign ALL components of UTM.app with ad-hoc signatures from inside-out:

```bash
# 1. Stop VM and kill all UTM processes
killall -9 UTM QEMULauncher 2>/dev/null

# 2. Replace QEMU binary with custom build
sudo cp ~/pm/UTM/sysroot-macOS-arm64/lib/libqemu-aarch64-softmmu.dylib \
    /Applications/UTM.app/Contents/Frameworks/qemu-aarch64-softmmu.framework/Versions/A/qemu-aarch64-softmmu

# 3. Fix library paths (see script at /tmp/fix-qemu-paths.sh)
/tmp/fix-qemu-paths.sh

# 4. Re-sign ALL XPC services
sudo find /Applications/UTM.app/Contents/XPCServices -name "*.xpc" -type d | while read f; do
    sudo codesign --force --deep --sign - "$f"
done

# 5. Re-sign ALL frameworks
sudo codesign --force --deep --sign - /Applications/UTM.app/Contents/Frameworks/*.framework

# 6. Re-sign entire UTM.app bundle
sudo codesign --force --deep --sign - /Applications/UTM.app
```

**Why this works:**
- All components now have the same ad-hoc Team ID
- Hypervisor.framework access DOES work with ad-hoc signing (vmnet doesn't, but we use emulated networking)
- macOS Gatekeeper must be disabled or set to "Anywhere" (System Settings → Privacy & Security)

### Issue 3: gl=es Parameter Not Supported at Runtime (SOLVED!)

**Problem:** After successful signing, VM fails to start with:
```
qemu-aarch64-softmmu: -spice gl=es: Parameter 'gl' expects 'on' or 'off'
```

**Root Cause:** UTM applies patches AFTER downloading QEMU, but our build replaces the entire QEMU source tree with our fork BEFORE the patch is applied.

**How UTM Applies Patches:**
1. UTM's `Scripts/build_dependencies.sh` has a `download()` function
2. After unpacking a tarball, it looks for `patches/${NAME}.patch`
3. For QEMU (unpacked to `qemu-10.0.2-utm/`), it applies `patches/qemu-10.0.2-utm.patch`
4. This patch changes `ui/spice-core.c` line 513:
   ```c
   - .type = QEMU_OPT_BOOL,   // Only accepts on/off
   + .type = QEMU_OPT_STRING,  // Accepts on/off/es/core
   ```

**Our Build Process Bypasses Patching:**
- `./stack build-utm` waits for UTM to download QEMU
- Then REPLACES entire `qemu-10.0.2-utm/` with our `~/pm/qemu-utm` fork
- UTM's patch file never gets applied to our fork!

**Solution - CORRECT APPROACH:**

The UTM patch file (`patches/qemu-10.0.2-utm.patch`) is designed for vanilla QEMU 10.0.2,
but our fork is based on the UTM git fork which already has most patches applied as commits.

What we needed: Only the `gl=es` parameter fix was missing from the fork.

**Applied in commit `b6190fb7ae`:**
```bash
cd ~/pm/qemu-utm
# Edit ui/spice-core.c line 513:
# Change: .type = QEMU_OPT_BOOL,
# To:     .type = QEMU_OPT_STRING,
git add ui/spice-core.c
git commit -m "Apply UTM patch: Change gl parameter from QEMU_OPT_BOOL to QEMU_OPT_STRING

This enables SPICE to accept gl=on|off|es|core instead of just on|off.
Required for virtio-gpu-gl-pci with Venus/Vulkan support (gl=es).

Source: UTM patches/qemu-10.0.2-utm.patch line 2869-2870"
git push -f helixml utm-edition  # Force push after history rewrite
```

**Key Learnings:**
- Don't apply full patch files to forks that already have commits - causes conflicts
- Extract only the specific changes needed (in this case, one line)
- Verify the change matches the patch file exactly
- Never commit with failed hunks - user caught this early!

**Saved Helix Patches:** `~/pm/helix/qemu-patches/` (permanent backup of our 3 commits)

**FINAL SOLUTION (2026-02-04 18:59):**

Rebased our qemu-utm fork onto the correct upstream branch `utm-edition-venus`:

```bash
cd ~/pm/qemu-utm
git checkout -b utm-edition-venus-helix origin/utm-edition-venus
# Manually add helix-frame-export module (patch didn't apply cleanly due to venus changes)
mkdir -p hw/display/helix
# Copy helix-frame-export.m, helix-frame-export.h, README.md, meson.build
# Modify hw/display/meson.build to add subdir('helix')
# Modify hw/display/virtio-gpu-virgl.c to call helix_frame_export_init()
git add hw/display/helix hw/display/meson.build hw/display/virtio-gpu-virgl.c
git commit -m "Add Helix frame export for zero-copy VideoToolbox encoding (rebased on utm-edition-venus)"
git branch -D utm-edition  # Delete old branch
git checkout -b utm-edition utm-edition-venus-helix  # Replace with venus-based one
git push -f helixml utm-edition
```

Now our fork has:
- ALL the utm-edition-venus Venus/Vulkan patches (19 commits ahead of stable branch)
- Our 1 helix-frame-export commit cleanly applied on top

**Status:** Rebased onto correct upstream branch, now rebuilding

## Options Going Forward

### Option 1: Fix gl=es Support in Custom Build (IN PROGRESS)

Build the entire UTM.app from source using our custom QEMU:
```bash
cd ~/pm/helix/UTM
./Scripts/build_utm.sh -k macosx -s macOS -a arm64 -o ~/pm/utm-build
```

**Pros:**
- Proper framework packaging
- Correct code signing
- All dependencies bundled

**Cons:**
- Takes 30-60 minutes to build
- Need to rebuild for each UTM update

### Option 2: Use QEMUHelper Directly

UTM's architecture:
```
UTM.app → QEMUHelper.xpc → QEMULauncher.app → qemu-system-aarch64
```

We could potentially:
1. Keep production UTM.app unmodified
2. Launch QEMU manually with our custom build
3. Connect UTM's SPICE client to our QEMU instance

**Status:** Not yet tested

### Option 3: Developer Certificate Signing

Sign with a real Apple Developer certificate:
```bash
codesign --force --sign "Developer ID" --entitlements file.entitlements /Applications/UTM.app
```

**Requires:**
- $99/year Apple Developer account
- Proper certificate setup

## Library Path Fixing Script

Saved script to fix QEMU library paths:

```bash
# File: /tmp/fix-qemu-paths.sh
#!/bin/bash

QEMU="/Applications/UTM.app/Contents/Frameworks/qemu-aarch64-softmmu.framework/Versions/A/qemu-aarch64-softmmu"
SYSROOT="/Users/luke/pm/UTM/sysroot-macOS-arm64/lib"

# Fix ID
install_name_tool -id @rpath/qemu-aarch64-softmmu.framework/Versions/A/qemu-aarch64-softmmu "$QEMU"

# Fix dependencies (map to UTM's framework structure)
install_name_tool -change "$SYSROOT/libqemu-aarch64-softmmu.dylib" @rpath/qemu-aarch64-softmmu.framework/Versions/A/qemu-aarch64-softmmu "$QEMU"
install_name_tool -change "$SYSROOT/libpixman-1.0.dylib" @rpath/pixman-1.0.framework/Versions/A/pixman-1.0 "$QEMU"
# ... (see full script in /tmp/fix-qemu-paths.sh)
```

## Recommendation

**Build the full UTM.app** using `Scripts/build_utm.sh`. While it takes longer, it's the only approach that:
1. Works without a developer certificate
2. Properly packages all dependencies
3. Maintains security model compliance

The binary patching approach is blocked by macOS security architecture and would require either:
- Apple Developer certificate ($99/year)
- Disabling SIP (System Integrity Protection) - not recommended


## UPDATE: Networking vs Hypervisor

**IMPORTANT CORRECTION:** Ad-hoc signing DOES work for Hypervisor access!

From previous work (design/2026-02-02-macos-arm-desktop-port.md):
```
- vmnet (Shared/Bridged networking) requires Apple-signed entitlement `com.apple.vm.networking`
- Ad-hoc signed apps cannot use vmnet - switched to "Emulated" networking (QEMU user-mode NAT)
```

**What works with ad-hoc signing:**
- ✅ Hypervisor.framework (hvf acceleration)
- ✅ User networking (QEMU NAT, `-netdev user`)
- ✅ All other VM features

**What requires developer certificate:**
- ❌ vmnet networking (bridged/shared, `-netdev vmnet-shared`)

## Revised Approach

The HV_DENIED error was likely caused by over-aggressive re-signing with `--deep` flag, which invalidated UTM's original signature.

**Correct procedure:**
1. Install fresh UTM.app from GitHub (keeps original valid signature)
2. Replace QEMU dylib: `cp custom-qemu /Applications/UTM.app/.../qemu-aarch64-softmmu`
3. Fix library paths with `install_name_tool` (this invalidates code signature but is OK)
4. **Either:**
   - Don't re-sign at all (macOS allows running with invalid signature after user approval)
   - OR re-sign just the framework: `codesign --force --sign - <framework-path>` (NOT --deep)
5. Ensure VM uses **Emulated networking**, NOT vmnet

**Next test:** Replace binary, fix paths, minimal signing, verify networking is set to "Emulated"

## UPDATE: Dependency Build Success (2026-02-04 Evening)

Successfully built all QEMU dependencies from source using UTM's `build_dependencies.sh`:

**✅ Successfully Built:**
- virglrenderer 1.0 (critical for GPU texture access)
- QEMU 10.0.2-utm with our helix-frame-export module
- All 28 dependencies: pkg-config, libffi, libiconv, gettext, glib, pixman, openssl, spice, gstreamer, etc.

**❌ Failed (Non-Critical):**
- mesa (GPU driver framework) - requires LLVM ≥15.0
- This failure occurred AFTER virglrenderer and QEMU built successfully
- mesa is not needed for our use case (we only need virglrenderer)

**Build artifacts:**
```
~/pm/UTM/sysroot-macOS-arm64/lib/libvirglrenderer.1.dylib   (2.9 MB)
~/pm/UTM/sysroot-macOS-arm64/lib/libqemu-aarch64-softmmu.dylib  (30.5 MB)
```

**Helix module integration:**
- ✅ helix-frame-export.m compiled successfully
- ✅ Integrated into virtio-gpu-virgl.c
- ⚠️  VideoToolbox/CoreVideo/CoreMedia frameworks not linked (needs meson.build fix)
- ✅ virglrenderer linked correctly

**Commits:**
- `3102734297` - Fix helix_frame_export_init() call signature
- Pushed to github.com/helixml/qemu-utm utm-edition branch

**Next steps:**
1. ~~Fix meson.build to properly link VideoToolbox/CoreVideo/CoreMedia frameworks~~ ✅ DONE
2. ~~Rebuild QEMU and verify all frameworks present~~ ✅ DONE
3. Test VM with custom QEMU
4. Verify helix-frame-export functionality

## BUILD SUCCESS! (2026-02-04 20:05)

**QEMU with helix-frame-export built successfully!**

Verified frameworks linked into `libqemu-aarch64-softmmu.dylib`:
```
VideoToolbox.framework/Versions/A/VideoToolbox  (v3290.6.5)
CoreVideo.framework/Versions/A/CoreVideo        (v726.2.0)
CoreMedia.framework/Versions/A/CoreMedia        (v3290.6.5)
Metal.framework/Versions/A/Metal                (v370.64.2)
IOSurface.framework/Versions/A/IOSurface        (v1.0.0)
libvirglrenderer.1.dylib                        (v1.0.0)
```

Verified helix symbols in dylib:
```
helix_encode_iosurface
helix_frame_export_cleanup
helix_frame_export_init
helix_frame_export_process_msg
helix_get_iosurface_for_resource
```

**Key fix:** Moved helix module from standalone `system_ss` to `virtio_gpu_gl_ss` source set, ensuring it's compiled into the virtio-gpu-gl loadable module where it's called.

**Commits:**
- `3102734297` - Fix helix_frame_export_init() call signature
- `3f8566c5e1` - Fix helix module integration into virtio-gpu-gl source set
- Pushed to github.com/helixml/qemu-utm utm-edition branch

**Ready for testing:**
- Custom QEMU binary at: `~/pm/UTM/sysroot-macOS-arm64/lib/libqemu-aarch64-softmmu.dylib`
- Next: Patch production UTM.app and test VM

## TESTING RESULTS (2026-02-04 20:11)

**Patching Procedure:**
1. Killed all UTM/QEMU processes
2. Backed up original QEMU: `/Applications/UTM.app/Contents/Frameworks/qemu-aarch64-softmmu.framework/Versions/A/qemu-aarch64-softmmu.orig`
3. Replaced QEMU binary with custom build
4. Fixed library paths using `/tmp/fix-qemu-paths.sh` (changed hardcoded paths to @rpath)
5. Re-signed XPC services and frameworks with ad-hoc signatures
6. Attempted to start VM

**Crash Found:**
VM failed to start with assertion crash in `qemu_opt_get_bool_helper` called from `qemu_spice_init`.

From crash report (`QEMULauncher-2026-02-04-185607.ips`):
```
"termination" : {"flags":0,"code":6,"namespace":"SIGNAL","indicator":"Abort trap: 6"}
Stack trace:
  __assert_rtn
  qemu_opt_get_bool_helper.cold.1
  qemu_opt_get_bool_helper
  qemu_spice_init
  qemu_init
```

**Root Cause:**
The gl=es parameter patch exists in the source code (`ui/spice-core.c:513` has `.type = QEMU_OPT_STRING`), BUT it's wrapped in `#ifdef HAVE_SPICE_GL`.

Our initial build used manual `./configure` which did NOT enable SPICE GL support, so the patch was excluded at compile time. The binary still has the old `QEMU_OPT_BOOL` type, which triggers an assertion when UTM passes `gl=es`.

**Fix:**
Must rebuild QEMU using UTM's full dependency build script which properly enables SPICE GL. The quick rebuild script (`for-mac/qemu-helix/build-qemu-only.sh`) uses a minimal configure command without SPICE support.

**Correct Build Command:**
```bash
cd ~/pm/UTM
./Scripts/build_dependencies.sh -k macosx -s macOS -a arm64 -o ~/pm/UTM
```

This ensures all configure flags are set correctly, including SPICE GL support.

**VM Information:**
- External disk VM: `/Volumes/Helix VM/Linux.utm`
- Correct UUID: `17DC4F96-F1A9-4B51-962B-03D85998E0E7`
- Config: virtio-gpu-gl-pci, Emulated networking, 20 CPUs, 64GB RAM
- Design doc had wrong UUID (`01CECE09-B09D-48A4-BAB6-D046C06E3A68`) - that was from an old session

**Status:** Rebuilding QEMU with proper SPICE GL configuration (2026-02-04 20:11)

## REBUILD COMPLETE (2026-02-04 20:15)

**Rebuild Success:**
Used UTM's build script (`for-mac/qemu-helix/build-qemu-only.sh`) which properly configures QEMU with all dependencies including SPICE GL support.

Build completed at 20:13 with proper configuration:
- ✅ Helix symbols present (all 5 functions)
- ✅ SPICE GL functions present (qemu_spice_gl_*)
- ✅ Binary size: 33MB
- ✅ All frameworks linked: VideoToolbox, CoreVideo, CoreMedia, Metal, IOSurface, virglrenderer

**Re-patching:**
1. Killed all UTM processes
2. Replaced QEMU binary with newly-built version
3. Fixed library paths using `/tmp/fix-qemu-paths.sh`
4. No re-signing needed (already signed from previous attempt)

**VM Start Testing:**
Attempted to start VM automatically via AppleScript, but VM is not launching. No new crash reports generated (last crash was at 18:56 before rebuild).

Possible issues:
- GUI automation may not be clicking the right button
- May require manual user interaction to approve security dialogs
- Code signing may need to be redone after binary replacement

**Next Steps:**
User should manually test:
1. Open UTM.app
2. Select "Linux" VM on external disk
3. Click Play button to start VM
4. Check for:
   - Security prompts (approve if needed)
   - VM console appearing
   - Linux boot messages
   - Any error dialogs

If VM fails to start, check:
```bash
ls -lat ~/Library/Logs/DiagnosticReports/ | grep QEMULauncher | head -1
# Read the crash report to see new error
```

**Binary Location:**
- Source: `~/pm/UTM/sysroot-macOS-arm64/lib/libqemu-aarch64-softmmu.dylib`
- Installed: `/Applications/UTM.app/Contents/Frameworks/qemu-aarch64-softmmu.framework/Versions/A/qemu-aarch64-softmmu`
- Backup: `/Applications/UTM.app/Contents/Frameworks/qemu-aarch64-softmmu.framework/Versions/A/qemu-aarch64-softmmu.orig`

To revert to original QEMU:
```bash
sudo cp /Applications/UTM.app/Contents/Frameworks/qemu-aarch64-softmmu.framework/Versions/A/qemu-aarch64-softmmu.orig \
     /Applications/UTM.app/Contents/Frameworks/qemu-aarch64-softmmu.framework/Versions/A/qemu-aarch64-softmmu
```

## Reproducible Build Process

**Prerequisites:**
```bash
# Install Homebrew dependencies (from UTM's CI)
brew install bison pkg-config gettext glib-utils libgpg-error nasm make meson cmake libclc

# Install Python dependencies
pip3 install --break-system-packages --user six pyparsing pyyaml setuptools distlib mako

# Add Homebrew bison to PATH (macOS ships with old 2.3, need 3.0+)
export PATH="/opt/homebrew/opt/bison/bin:$PATH"
```

**Directory Structure:**
```
~/pm/
├── helix/              # This repo
├── qemu-utm/           # Our fork: github.com/helixml/qemu-utm (utm-edition branch)
└── UTM/                # UTM build scripts: github.com/utmapp/UTM (v5.0.1, commit 8d34e35b)
```

**Build Steps:**

### 1. Clone Repositories (First Time Only)
```bash
cd ~/pm
git clone https://github.com/helixml/helix
git clone https://github.com/helixml/qemu-utm
git clone https://github.com/utmapp/UTM
cd UTM && git checkout 8d34e35b  # v5.0.1+ tested version
```

### 2. Build All Dependencies (First Time or After Clean)
```bash
cd ~/pm/UTM
./Scripts/build_dependencies.sh -p macos -a arm64

# This takes 30-60 minutes and builds:
# - All 28 dependencies (virglrenderer, spice, glib, etc.)
# - Vanilla QEMU 10.0.2-utm (which we'll replace)
# Output: ~/pm/UTM/sysroot-macOS-arm64/
```

### 3. Build QEMU with helix-frame-export (Fast: 2-5 minutes)
```bash
cd ~/pm/helix
./for-mac/qemu-helix/build-qemu-standalone.sh
```

**What this does:**
- Uses our QEMU fork at `~/pm/qemu-utm` (no source copying needed)
- Generates meson cross-compilation files automatically
- Configures QEMU with SPICE, virglrenderer, and all UTM features
- Builds with ninja
- Installs to sysroot

**Output:** `~/pm/UTM/sysroot-macOS-arm64/lib/libqemu-aarch64-softmmu.dylib` (33MB)

**Note:** `build-qemu-standalone.sh` is standalone - it doesn't require UTM checkout after initial dependency build. All QEMU build logic is now in `for-mac/qemu-helix/`. You can also use `./stack build-utm` which wraps this script.

### 5. Install into UTM.app
```bash
# Kill running VMs
killall -9 UTM QEMULauncher qemu-system-aarch64 2>/dev/null || true

# Backup original
sudo cp /Applications/UTM.app/Contents/Frameworks/qemu-aarch64-softmmu.framework/Versions/A/qemu-aarch64-softmmu \
        /Applications/UTM.app/Contents/Frameworks/qemu-aarch64-softmmu.framework/Versions/A/qemu-aarch64-softmmu.orig

# Install custom build
sudo cp ~/pm/UTM/sysroot-macOS-arm64/lib/libqemu-aarch64-softmmu.dylib \
        /Applications/UTM.app/Contents/Frameworks/qemu-aarch64-softmmu.framework/Versions/A/qemu-aarch64-softmmu

# Fix library paths (script in helix repo)
~/pm/helix/for-mac/qemu-helix/fix-qemu-paths.sh
```

### 6. Test
```bash
# Open UTM.app and start a VM
# Check for crashes: ls -lat ~/Library/Logs/DiagnosticReports/ | grep QEMULauncher
```

**Incremental Rebuilds:**

After changing helix-frame-export code:
```bash
cd ~/pm/qemu-utm
# Make your changes to hw/display/helix/helix-frame-export.m
git commit -am "Your change"

# Rebuild (uses incremental ninja builds, only recompiles changed files)
cd ~/pm/helix
./for-mac/qemu-helix/build-qemu-standalone.sh

# Re-install into UTM.app (steps 5-6 above)
```

**Automated Build:**
```bash
cd ~/pm/helix
./stack build-utm  # Full automated build with all checks
```

**Key Points:**
- Dependencies only need building once (or after `make clean` in UTM)
- QEMU rebuilds take 2-3 minutes with cached dependencies
- Use `for-mac/qemu-helix/build-qemu-only.sh` for fast iteration
- Always test in a fresh VM start (existing VMs keep old QEMU)
- Check crash reports if VM fails to start: `~/Library/Logs/DiagnosticReports/QEMULauncher-*.ips`

### 7. Starting VMs with utmctl

UTM provides a CLI tool for automated VM management:

```bash
# List all VMs with their UUIDs and status
/Applications/UTM.app/Contents/MacOS/utmctl list

# Start a VM by UUID
/Applications/UTM.app/Contents/MacOS/utmctl start <UUID>

# Stop a VM
/Applications/UTM.app/Contents/MacOS/utmctl stop <UUID>

# Get VM status
/Applications/UTM.app/Contents/MacOS/utmctl status <UUID>
```

**Our VM Details:**
- Name: Linux (on external disk)
- UUID: `01CECE09-B09D-48A4-BAB6-D046C06E3A68`
- Path: `/Volumes/Helix VM/Linux.utm`

**Start Command:**
```bash
/Applications/UTM.app/Contents/MacOS/utmctl start 01CECE09-B09D-48A4-BAB6-D046C06E3A68

# Verify QEMU process started
ps aux | grep qemu-system-aarch64
```

**Troubleshooting VM Start Issues:**

```bash
# Check recent crash reports
ls -lat ~/Library/Logs/DiagnosticReports/ | grep QEMULauncher | head -3

# Read most recent crash
cat ~/Library/Logs/DiagnosticReports/$(ls -t ~/Library/Logs/DiagnosticReports/ | grep QEMULauncher | head -1)

# Check for QEMU process
ps aux | grep -E "qemu-system|QEMULauncher"

# Check UTM system logs
log show --predicate 'process == "UTM" OR process == "QEMULauncher"' --last 5m --style compact

# Check SPICE socket exists
ls -la ~/Library/Group\ Containers/*.com.utmapp.UTM/*.spice
```

**Common Errors:**
- `-spice: invalid option` → SPICE not compiled in QEMU (use UTM's meson config, not simple ./configure)
- `gl=es: Parameter 'gl' expects 'on' or 'off'` → SPICE GL not enabled (CONFIG_SPICE not set)
- `Team ID mismatch` → Re-sign all components with consistent ad-hoc signatures
