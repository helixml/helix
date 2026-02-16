# Helix Frame Export Integration Status

## ✅ Completed Work

### 1. Custom QEMU Build
- Built QEMU 10.0.2-utm with helix-frame-export module
- Module source at: `~/pm/qemu-utm/hw/display/helix/`
- Binary includes helix symbols:
  ```bash
  $ nm libqemu-aarch64-softmmu.dylib | grep helix
  00000000003613d4 t _helix_encode_iosurface
  00000000003617a0 t _helix_frame_export_cleanup
  0000000000361820 t _helix_frame_export_init
  00000000003615a0 t _helix_frame_export_process_msg
  00000000003612d0 t _helix_get_iosurface_for_resource
  ```

### 2. VM Running with Custom QEMU
- VM: Linux (UUID: 01CECE09-B09D-48A4-BAB6-D046C06E3A68)
- Location: `/Volumes/Helix VM/Linux.utm`
- Running with TCG emulation (no SIP disable required)
- QEMU command line includes:
  - `-accel tcg,tb-size=16384`
  - `-device virtio-gpu-gl-pci,hostmem=256M,blob=true,venus=true`
  - `-spice ...gl=es`

### 3. Module Integration
- Module automatically initialized in `hw/display/virtio-gpu-virgl.c`:
  ```c
  helix_frame_export_init(g, 5900); /* vsock port for frame export */
  ```
- Listens on vsock port 5900 for guest frame export requests

## ⚠️ Remaining Issues

### vsock Device Not Present
**Problem:** UTM not adding vsock device from `AdditionalArguments` in config

**Attempted:**
- Added to config.plist: `["-device", "vhost-vsock-pci,guest-cid=3"]`
- Restarted VM multiple times
- UTM ignores AdditionalArguments (unknown why)

**Impact:**
- Guest cannot communicate with helix-frame-export module over vsock
- Module likely fails to bind to vsock port but continues running

**Workarounds to try:**
1. Manually patch QEMU to always create vsock device
2. Use UTM GUI to add vsock (if supported)
3. Build custom UTM that respects AdditionalArguments
4. Test with different UTM version

## 🧪 Testing Plan

### Phase 1: Verify Module Initialization
- [ ] Add debug logging to helix-frame-export.m
- [ ] Rebuild QEMU with logging
- [ ] Check if module initializes without vsock device
- [ ] Monitor for vsock bind failures

### Phase 2: Add vsock Device
- [ ] Find why UTM ignores AdditionalArguments
- [ ] Try patching QEMU to auto-create vsock
- [ ] Or use UTM GUI to configure vsock
- [ ] Verify vsock appears in `ps aux` output

### Phase 3: Guest-Side Integration
- [ ] Boot VM to Linux
- [ ] Install vsockenc GStreamer element
- [ ] Test frame export from guest
- [ ] Verify H.264 encoding on host

### Phase 4: End-to-End Test
- [ ] Run desktop-bridge in VM
- [ ] Stream video to host
- [ ] Measure frame rate and latency
- [ ] Compare with software encoding

## 📁 Key Files

- **Custom QEMU:** `~/pm/UTM/sysroot-macOS-arm64/lib/libqemu-aarch64-softmmu.dylib`
- **Installed in:** `/Applications/UTM.app/Contents/Frameworks/qemu-aarch64-softmmu.framework/Versions/A/qemu-aarch64-softmmu`
- **VM Config:** `/Volumes/Helix VM/Linux.utm/config.plist`
- **Module Source:** `~/pm/qemu-utm/hw/display/helix/`
- **Build Scripts:** `~/pm/helix/for-mac/qemu-helix/`

## 🔧 Useful Commands

### Start/Stop VM
```bash
/Applications/UTM.app/Contents/MacOS/utmctl start Linux
/Applications/UTM.app/Contents/MacOS/utmctl stop Linux
/Applications/UTM.app/Contents/MacOS/utmctl status Linux
```

### Check QEMU Process
```bash
ps aux | grep qemu-aarch64-softmmu | grep -v grep
```

### Verify Helix Symbols
```bash
nm /Applications/UTM.app/Contents/Frameworks/qemu-aarch64-softmmu.framework/Versions/A/qemu-aarch64-softmmu | grep helix
```

### Rebuild QEMU (after code changes)
```bash
cd ~/pm/helix
./for-mac/qemu-helix/build-qemu-standalone.sh
sudo cp ~/pm/UTM/sysroot-macOS-arm64/lib/libqemu-aarch64-softmmu.dylib \
     /Applications/UTM.app/Contents/Frameworks/qemu-aarch64-softmmu.framework/Versions/A/qemu-aarch64-softmmu
sudo ./for-mac/qemu-helix/fix-qemu-paths.sh
sudo codesign --force --sign - \
     /Applications/UTM.app/Contents/Frameworks/qemu-aarch64-softmmu.framework
```

## 📊 Progress Summary

| Component | Status | Notes |
|-----------|--------|-------|
| QEMU Build | ✅ 100% | Module compiled and linked |
| QEMU Installation | ✅ 100% | Running in UTM.app |
| VM Running | ✅ 100% | TCG emulation working |
| Module Init | ✅ 90% | Code called, vsock binding status unknown |
| vsock Device | ❌ 0% | UTM not adding device |
| Guest Integration | ⏳ 0% | Waiting for vsock |
| End-to-End Test | ⏳ 0% | Waiting for guest |

**Overall: ~70% Complete**

## 🎯 Next Steps

1. **Debug AdditionalArguments** - Find out why UTM ignores them
2. **Add vsock manually** - Patch QEMU or use different method
3. **Test without vsock** - See if module works with alternative IPC
4. **Guest-side setup** - Install vsockenc and test

## 📝 Notes

- TCG emulation is slow but functional for development
- HVF would require SIP disable or developer certificate
- Module architecture supports alternative IPC mechanisms (could use SPICE messages instead of vsock)
- Full performance testing will require HVF acceleration
