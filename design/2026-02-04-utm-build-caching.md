# UTM Build Caching - Performance Improvement

**Date:** 2026-02-04
**Status:** Proposed

## Problem

UTM's `build_dependencies.sh` script rebuilds all 28 dependencies from scratch every time it's run, even when the sysroot already contains built libraries. This makes every QEMU build take 30-60 minutes instead of 2-3 minutes.

### Current Behavior

```bash
./stack build-utm
# ❌ Rebuilds all 28 dependencies (30-60 minutes)
# ❌ Even when sysroot-macOS-arm64 already exists
# ❌ Dependencies haven't changed since last build
```

### Why It's Slow

From `build_dependencies.sh`:
```bash
echo "${GREEN}Deleting existing build directory ${DIR}...${NC}"
rm -rf "$DIR"
```

The script unconditionally deletes and rebuilds:
- pkg-config, libffi, libiconv, gettext
- libpng, libjpeg, glib, libgpg-error, libgcrypt
- pixman, openssl, libtpms, swtpm
- opus, spice-protocol, spice
- json-glib, gstreamer, gst-plugins-base/good
- libxml2, libsoup, phodav, spice-gtk
- virglrenderer, QEMU

## How GitHub Actions Does It

From `.github/workflows/build.yml`:

```yaml
- name: Cache Sysroot
  id: cache-sysroot
  uses: actions/cache/restore@v4
  with:
    path: ./sysroot-${{ matrix.platform }}-${{ matrix.arch }}
    key: ${{ matrix.platform }}-${{ matrix.arch }}-${{ hashFiles('scripts/build_dependencies.sh') }}-${{ hashFiles('patches/**') }}

- name: Build Sysroot
  if: steps.cache-sysroot.outputs.cache-matched-key == ''
  run: ./scripts/build_dependencies.sh -p ${{ matrix.platform }} -a ${{ matrix.arch }}
```

**Cache key = hash of build script + hash of patches**

If nothing changed, reuse cached sysroot. Only rebuild if:
- build_dependencies.sh changed
- Any patch file changed

## Proposed Solution

### Option 1: Smart Build Script (Quick Win)

Update `./stack build-utm` to check if sysroot exists and is up-to-date:

```bash
function build-utm() {
    SYSROOT="$PROJECTS_ROOT/UTM/sysroot-macOS-arm64"
    SYSROOT_MARKER="$SYSROOT/.build-complete"
    BUILD_SCRIPT_HASH=$(sha256sum "$PROJECTS_ROOT/UTM/Scripts/build_dependencies.sh" | cut -d' ' -f1)

    # Check if sysroot is up-to-date
    if [ -f "$SYSROOT_MARKER" ]; then
        CACHED_HASH=$(cat "$SYSROOT_MARKER")
        if [ "$CACHED_HASH" = "$BUILD_SCRIPT_HASH" ]; then
            echo "✅ Sysroot up-to-date, skipping dependency build"
            echo "🏗️  Building QEMU only..."
            ./scripts/build-qemu-only.sh
            return 0
        fi
    fi

    # Full build needed
    echo "🔨 Building dependencies + QEMU (first time or dependencies changed)..."
    cd "$PROJECTS_ROOT/UTM"
    ./Scripts/build_dependencies.sh -p macos -a arm64

    # Mark sysroot as complete
    echo "$BUILD_SCRIPT_HASH" > "$SYSROOT_MARKER"
}
```

**Benefits:**
- Fast incremental builds (2-3 minutes vs 30-60 minutes)
- Only rebuild dependencies when actually needed
- Matches GitHub Actions caching behavior

### Option 2: Pre-built Sysroot Tarball (Advanced)

Download pre-built sysroot from GitHub Actions artifacts:

```bash
# Download cached sysroot from last successful CI run
curl -L https://github.com/helixml/UTM/actions/artifacts/latest/sysroot-macOS-arm64.tgz \
    -H "Authorization: token $GITHUB_TOKEN" \
    -o sysroot.tgz
tar -xzf sysroot.tgz
```

**Benefits:**
- Zero dependency build time for CI builds
- Consistent environment with CI

**Drawbacks:**
- Requires GitHub token
- Depends on CI artifacts

## Immediate Action

1. Let current full build complete once to populate sysroot
2. Create `scripts/build-qemu-only.sh` (✅ Done)
3. Implement Option 1 in next `./stack build-utm` update
4. Document caching behavior

## Files to Update

- `stack` - Add smart caching to build-utm function
- `scripts/build-qemu-only.sh` - Fast QEMU-only rebuild (✅ Created)
- `design/2026-02-04-utm-build-caching.md` - This document

## Expected Performance

| Scenario | Current | With Caching |
|----------|---------|--------------|
| First build (clean sysroot) | 30-60 min | 30-60 min (same) |
| Incremental (QEMU changes only) | 30-60 min | **2-3 min** |
| Incremental (dependency changes) | 30-60 min | 30-60 min (needed) |

## Related Issues

- User complaint: "Is there a reason this is painfully slow every time you run it? I thought we were going to have a nice build cache."
- Root cause: No caching implementation yet
- Fix: Implement sysroot caching like GitHub Actions
