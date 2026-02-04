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

### Issue 2: Code Signing & Hypervisor Access

**Problem:** After replacing QEMU and re-signing, get error:
```
qemu-aarch64-softmmu: -accel hvf: Error: ret = HV_DENIED (0xfae94007)
```

**Root Cause:** macOS Hypervisor framework requires:
- Proper code signature with `com.apple.security.virtualization` entitlement
- Ad-hoc signing (`codesign --sign -`) is NOT sufficient for Hypervisor access
- Even with entitlements file, ad-hoc signatures are rejected by the kernel

**Why this happens:**
1. Hypervisor.framework uses kernel-level access checks
2. Kernel validates code signatures against Apple's trust chain
3. Ad-hoc signatures (self-signed) don't meet kernel's trust requirements
4. Entitlements are only honored for properly signed binaries

## Options Going Forward

### Option 1: Build Complete UTM.app (RECOMMENDED)

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
