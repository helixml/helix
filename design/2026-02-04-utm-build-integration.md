# UTM Custom QEMU Build Integration - 2026-02-04

## Summary

Integrated UTM's QEMU build system with our helix-frame-export patches to create a `./stack build-utm` command that properly builds QEMU with all necessary dependencies including SPICE support.

## Problem

Initial attempt to build QEMU with helix-frame-export patches failed when used with UTM because:
- Custom QEMU build didn't include SPICE support
- UTM requires SPICE protocol for display management
- Missing dependencies caused VM startup failures with "-spice: invalid option" errors

## Solution

Created `./stack build-utm` command that:
1. Uses UTM's official `build_dependencies.sh` script
2. Points it to our `qemu-utm` fork (which has helix-frame-export patches)
3. Builds all dependencies including:
   - SPICE protocol
   - SPICE server  
   - GStreamer (for video streaming)
   - All other UTM requirements
4. Installs the resulting QEMU into UTM.app
5. Re-signs the application for macOS

## Implementation

Added to `stack` script (line ~1770):
- `build-utm()` function - Builds QEMU with UTM config + our patches
- Help text entry for the command
- Platform check (macOS only)
- Dependency verification
- Automatic installation and code signing

## Usage

```bash
# Build custom QEMU with helix-frame-export for UTM
./stack build-utm

# This will:
# 1. Build QEMU using UTM's build script (~30-60 minutes)
# 2. Install to ~/pm/UTM/build/Build/Products/Release/UTM.app
# 3. Copy to /Applications/UTM.app if it exists
# 4. Re-sign the application

# Then start VMs with:
utmctl start <UUID>
```

## Benefits

- ✅ Proper SPICE support for UTM compatibility
- ✅ All helix-frame-export patches included
- ✅ Uses UTM's tested build configuration
- ✅ Repeatable build process via stack command
- ✅ Automatic installation and signing

## Related Files

- `stack` - Added build-utm command
- `CLAUDE.md` - Updated with UTM control instructions
- `~/pm/qemu-utm` - Our QEMU fork with helix-frame-export
- `~/pm/UTM` - UTM repository with build scripts

## Next Steps

Once QEMU build completes:
1. Start VM with expanded 1TB disk
2. Expand Linux partition inside VM
3. Test zero-copy video pipeline
4. Document performance vs software encoding

## References

- UTM build script: `UTM/Scripts/build_dependencies.sh`
- QEMU configure flags in UTM: Search for `QEMU_PLATFORM_BUILD_FLAGS`
- Our QEMU fork: https://github.com/helixml/qemu-utm (utm-edition branch)
