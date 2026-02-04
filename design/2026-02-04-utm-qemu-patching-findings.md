# UTM QEMU Patching - Findings 2026-02-04

## Summary

Attempted to patch production UTM.app with our custom QEMU containing helix-frame-export module. Discovered that macOS code signing prevents simple binary replacement.

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

**Status:** Clean patch applied, rebuild in progress (task b7761ba)

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
