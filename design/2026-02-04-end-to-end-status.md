# macOS ARM Desktop Port - End-to-End Status

## ✅ Completed

### 1. QEMU Build (100%)
- Custom QEMU 10.0.2-utm with helix-frame-export module built successfully
- SPICE support properly detected and compiled
- Binary location: `~/pm/UTM/sysroot-macOS-arm64/lib/libqemu-aarch64-softmmu.dylib`
- Helix module integrated at `hw/display/helix/`
- Build scripts organized in `for-mac/qemu-helix/`

### 2. Build System (100%)
- `build-qemu-standalone.sh` - Standalone QEMU build (2-5 min, no UTM checkout needed)
- `build-qemu-only.sh` - Fast incremental rebuild  
- `fix-qemu-paths.sh` - Fixes @rpath library references
- All scripts properly configure PKG_CONFIG_PATH for SPICE detection
- Documentation in `README-QEMU-BUILD.md`

### 3. VM Setup (100%)
- 506GB Linux VM copied to Big NVMe (`/Volumes/Big/Linux.utm`)
- Big NVMe performance: 5.8 GB/s write, 4.6 GB/s sustained (1m50s copy time)
- VM UUID: `01CECE09-B09D-48A4-BAB6-D046C06E3A68`
- VM configured for testing (20 CPUs, 64GB RAM, virtio-gpu-gl-pci)

### 4. Code Organization (100%)
- QEMU build scripts moved to `for-mac/qemu-helix/`
- Design docs updated with new VM location and build process
- `./stack build-utm` updated to use standalone build approach
- All changes committed to git

## ⚠️ Remaining Issue

### Hypervisor Access with Ad-Hoc Signing

**Problem:** 
- Custom QEMU binary needs `com.apple.security.hypervisor` entitlement to use HVF (Hypervisor.framework)
- Ad-hoc signing (`codesign --sign -`) cannot grant hypervisor access when SIP is enabled
- Error: `HV_DENIED (0xfae94007)`

**Current Status:**
- QEMU binary installed: ✅
- Library paths fixed: ✅  
- Frameworks signed: ✅
- Hypervisor access: ❌ (blocked by SIP + ad-hoc signing)

**Solutions:**

#### Option 1: Disable SIP (Fastest)
```bash
# Reboot into Recovery Mode (hold Cmd+R during boot)
# In Recovery, open Terminal:
csrutil disable
reboot

# After testing, re-enable:
csrutil enable
```

Then start VM with:
```bash
/Applications/UTM.app/Contents/MacOS/utmctl start 01CECE09-B09D-48A4-BAB6-D046C06E3A68
```

#### Option 2: Use Developer Certificate
- Requires Apple Developer account ($99/year)
- Sign with proper certificate that can grant hypervisor entitlement
- More permanent solution

#### Option 3: Build UTM from Source
- Embeds custom QEMU into UTM.app
- Proper signing during build process
- Complex: requires fixing framework dependencies (attempted, linker issues)

#### Option 4: Use TCG Emulation (No HVF)
- Extremely slow (no hardware acceleration)
- Not practical for testing

## Next Steps

1. **Disable SIP** (requires user reboot into Recovery Mode)
2. **Start VM** with utmctl
3. **Test helix-frame-export** functionality:
   - Verify QEMU module loads
   - Test GPU frame export via IOSurface
   - Validate zero-copy texture sharing
   - Test VideoToolbox H.264 encoding

## Files Ready for Testing

- **Custom QEMU:** `~/pm/UTM/sysroot-macOS-arm64/lib/libqemu-aarch64-softmmu.dylib`
- **VM:** `/Volumes/Big/Linux.utm`
- **UTM App:** `/Applications/UTM.app` (with custom QEMU installed)
- **Test Scripts:** `for-mac/qemu-helix/*.sh`

## Build Reproducibility

Full rebuild from scratch:
```bash
cd ~/pm/helix

# 1. Build dependencies (one-time, 30-60 min)
./stack build-utm  # Builds sysroot if needed

# 2. Build QEMU (2-5 min, repeatable)
./for-mac/qemu-helix/build-qemu-standalone.sh

# 3. Install into UTM.app
sudo cp ~/pm/UTM/sysroot-macOS-arm64/lib/libqemu-aarch64-softmmu.dylib \
     /Applications/UTM.app/Contents/Frameworks/qemu-aarch64-softmmu.framework/Versions/A/qemu-aarch64-softmmu
sudo ./for-mac/qemu-helix/fix-qemu-paths.sh

# 4. Sign (with or without entitlements)
sudo codesign --force --sign - \
     /Applications/UTM.app/Contents/Frameworks/qemu-aarch64-softmmu.framework
```

## Summary

**95% Complete** - All code built and ready. Only blocker is macOS security policy preventing ad-hoc signed binaries from accessing Hypervisor.framework. Requires either:
- SIP disabled (temporary, for testing)
- Developer certificate (permanent solution)
- UTM rebuild (complex, attempted but has dependency issues)

