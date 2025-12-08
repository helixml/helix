# Production vs Experimental Desktop Builds

**Date:** 2025-12-08
**Status:** Implementation Plan
**Branch:** `feature/ubuntu-desktop`

## Goal

Separate desktop images into:
- **Production desktops** (Sway, Ubuntu): Always built during `./stack build-sandbox`
- **Experimental desktops** (XFCE, Zorin): Only built when explicitly requested via environment variable

## Key Insight

The system already handles missing desktop tarballs gracefully:
- `04-start-dockerd.sh` checks if tarball exists, logs "not available" and continues if missing
- Heartbeat only discovers `.version` files that exist
- No skeleton images needed - simply don't create the tarball

## Implementation

### 1. Modify `stack` script

**Define desktop categories (hard-coded defaults):**
```bash
# Production desktops - always built
PRODUCTION_DESKTOPS="sway ubuntu"

# Experimental desktops available for opt-in
AVAILABLE_EXPERIMENTAL_DESKTOPS="zorin xfce"
```

**Add environment variable for opt-in:**
```bash
# User can set: EXPERIMENTAL_DESKTOPS="zorin" or EXPERIMENTAL_DESKTOPS="zorin xfce"
# Default: empty (don't build experimental desktops)
EXPERIMENTAL_DESKTOPS="${EXPERIMENTAL_DESKTOPS:-}"
```

**Modify `build-sandbox` function:**

Replace the current sequential build of all 4 desktops with:

```bash
# Build production desktops (always)
for desktop in $PRODUCTION_DESKTOPS; do
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "📦 Building production desktop: helix-${desktop}..."
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    SKIP_DESKTOP_TRANSFER=1 build-desktop "$desktop"
    verify_tarball "$desktop"
done

# Build experimental desktops (only if requested)
if [ -n "$EXPERIMENTAL_DESKTOPS" ]; then
    for desktop in $EXPERIMENTAL_DESKTOPS; do
        echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
        echo "🧪 Building experimental desktop: helix-${desktop}..."
        echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
        SKIP_DESKTOP_TRANSFER=1 build-desktop "$desktop"
        verify_tarball "$desktop"
    done
else
    echo "ℹ️  Experimental desktops skipped (set EXPERIMENTAL_DESKTOPS=\"zorin xfce\" to build)"
fi

# Note: We preserve existing experimental tarballs even when not building them
# This allows quick switching back without rebuilding
```

**Add helper function for tarball verification:**
```bash
verify_tarball() {
    local desktop="$1"
    if [ ! -f "sandbox-images/helix-${desktop}.tar" ]; then
        echo "❌ sandbox-images/helix-${desktop}.tar not found after build"
        rm -f sandbox-images/helix-*.tar
        exit 1
    fi
    echo "✅ helix-${desktop}.tar ($(du -h sandbox-images/helix-${desktop}.tar | cut -f1)) version=$(cat sandbox-images/helix-${desktop}.version)"
}
```

### 2. Update build-sandbox summary output

At the start of build-sandbox, show what will be built:

```bash
echo "📋 Build configuration:"
echo "   Production desktops: $PRODUCTION_DESKTOPS"
if [ -n "$EXPERIMENTAL_DESKTOPS" ]; then
    echo "   Experimental desktops: $EXPERIMENTAL_DESKTOPS"
else
    echo "   Experimental desktops: (none - set EXPERIMENTAL_DESKTOPS to enable)"
fi
```

### 3. No changes needed to:

- `Dockerfile.sandbox` - already copies whatever's in `sandbox-images/`
- `sandbox/04-start-dockerd.sh` - already handles missing tarballs gracefully
- `api/cmd/sandbox-heartbeat/main.go` - already dynamically discovers available desktops

## Files to Modify

| File | Change |
|------|--------|
| `stack` | Modify `build-sandbox` function to conditionally build desktops |

## Usage Examples

```bash
# Default: build only production desktops (sway, ubuntu)
./stack build-sandbox

# Include zorin experimental desktop
EXPERIMENTAL_DESKTOPS="zorin" ./stack build-sandbox

# Include both experimental desktops
EXPERIMENTAL_DESKTOPS="zorin xfce" ./stack build-sandbox

# Include all desktops (equivalent to current behavior)
EXPERIMENTAL_DESKTOPS="zorin xfce" ./stack build-sandbox
```

## Benefits

1. **Faster default builds**: Skip ~10GB of experimental desktop builds
2. **Easy to switch back**: Existing experimental tarballs are preserved (no rebuild needed)
3. **Easy to test experimental**: Just set one environment variable
4. **No skeleton images needed**: System already handles missing desktops gracefully
5. **Backward compatible**: `EXPERIMENTAL_DESKTOPS="zorin xfce"` gives current behavior

## Verification

After implementation:

1. Run `./stack build-sandbox` - should only build sway and ubuntu
2. Verify sandbox-images/ only has helix-sway.* and helix-ubuntu.*
3. Start sandbox and verify heartbeat only reports sway and ubuntu
4. Run `EXPERIMENTAL_DESKTOPS="zorin" ./stack build-sandbox` - should also build zorin
5. Verify sandbox-images/ now also has helix-zorin.*
