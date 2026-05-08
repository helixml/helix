# Design: Make Helix Builds and Releases Faster

## Goal

- Main branch builds: under **15 minutes**
- Release builds (tags): under **45 minutes**

## Current Timing Analysis

### Default Pipeline (push events) — Critical Path: ~10-14 min

```
build-api-binary (3-5 min: apt-get + ORT download + Go compile)
  → run-migrations (1 min)
  → unit-test (5-8 min, parallel with api-integration-test)
  → api-integration-test (5-8 min)
```

**Waste**: Both `unit-test` and `api-integration-test` redundantly install `build-essential` and download ORT (~2-3 min each), even though `build-api-binary` already did this. That's ~4-6 min wasted.

### Sandbox Pipeline (main branch) — Critical Path: ~40-70 min

```
clone-deps (1 min)
  → build-zed (0-40 min, cached by commit SHA — usually ~0 on main)
  → build-desktops (25-40 min: sway + ubuntu built SEQUENTIALLY)
    → build-sandbox (8-12 min)
      → push-sandbox (2-5 min)
```

**Root cause**: Desktop Dockerfiles are 970-1168 lines each. The first ~900 lines install OS packages, Chrome, OnlyOffice, Sway/GNOME from source — this rarely changes but rebuilds every time. Only the last ~70 lines (Zed binary, qwen-code, scripts) actually change per release.

### Release Pipeline — Critical Path: ~75-110 min

```
build-sandbox-arm64 (50-70 min, on slow macOS Docker Desktop)
  → build-macos-dmg (25-40 min: VM provision + notarize)
```

## Strategy: Pre-built Base Images (The Key to 15 min)

### Core Insight

The desktop Dockerfiles have two distinct halves:

| Section | Lines | Changes | Build Time |
|---|---|---|---|
| **Base** (OS, packages, Chrome, compositor) | ~900 lines | Monthly | 30-60 min |
| **App layer** (Zed, qwen-code, scripts, Go binaries) | ~70 lines | Every release | 1-3 min |

Building both halves every time is the main bottleneck. Split them.

### Implementation: Two-Tier Desktop Images

**Tier 1: Base images** (built separately, cached in registry)

Create `Dockerfile.sway-base` and `Dockerfile.ubuntu-base` containing everything up to the "FREQUENTLY CHANGING CODE" comment:
- OS packages and apt installs
- Chrome / OnlyOffice / Ghostty
- Sway/wlroots from source (sway) or GNOME packages (ubuntu)
- Fonts, themes, cursor generation
- Rust/Go build stages for zerocopy plugin and desktop-bridge dependencies

Push as `registry.helixml.tech/helix/helix-sway-base:latest` and `helix-ubuntu-base:latest`.

**When to rebuild base images**: A separate Drone pipeline triggered by:
- Changes to `Dockerfile.sway-helix` or `Dockerfile.ubuntu-helix` base sections
- A weekly cron job for security updates
- Manual trigger

**Tier 2: App layer** (built on every main merge / release)

Modify `Dockerfile.sway-helix` and `Dockerfile.ubuntu-helix` to start from the base:
```dockerfile
FROM registry.helixml.tech/helix/helix-sway-base:latest

# ---- FREQUENTLY CHANGING CODE (same as current lines 912-974) ----
COPY qwen-code-build/ /opt/qwen-code/
COPY zed-build/zed /zed-build/zed
ADD desktop/sway-config/ ...
COPY --from=go-build-env /desktop-bridge /usr/local/bin/desktop-bridge
...
```

The Go build stage still runs (for desktop-bridge, settings-sync-daemon) but uses the pre-cached go mod download. This layer takes **1-3 minutes**.

### New Sandbox Pipeline Timing (Main Branch)

```
clone-deps (1 min)
  → build-zed (cached, <1 min) ──┐
  → build-qwen-code (cached, <1 min) ──┤
  → build-desktop-sway-app (2-3 min) ──┤── parallel
  → build-desktop-ubuntu-app (2-3 min) ┘
    → build-sandbox (3-5 min, smaller context)
      → push-sandbox (2-3 min)
```

**Total: ~8-12 minutes** (down from 40-70).

### Default Pipeline Optimization

#### Pre-built CI Test Image

Create `helix-ci:bookworm` with:
- `golang:1.25-bookworm` base
- `build-essential`, `git` pre-installed
- ORT library pre-downloaded to `/usr/lib`

Steps `build-api-binary`, `unit-test`, and `api-integration-test` all use this image instead of `golang:1.25-bookworm` + apt-get + ORT download.

**Savings**: ~2-3 min per step × 3 steps = ~6-9 min total, but since they're partially parallel, net saving is ~4-5 min off critical path.

#### Eliminate Redundant ORT Download

Currently, both `unit-test` and `api-integration-test` run:
```
apt-get update && apt-get install -y build-essential
go run github.com/helixml/kodit/tools/download-ort
```

With the pre-built CI image, these lines are eliminated.

### New Default Pipeline Timing

```
build-api-binary (1-2 min with pre-built image)
  → run-migrations (1 min)
  → unit-test (4-6 min) ──── parallel
  → api-integration-test (4-6 min)
```

**Total critical path: ~7-9 minutes** (down from ~10-14).

### Release Pipeline Optimization

#### Decouple DMG from Sandbox

Check `for-mac/scripts/provision-vm-light.sh` to verify whether it pulls the sandbox image. If not (likely — it only needs the controlplane), remove `build-sandbox-arm64` from `build-macos-dmg`'s `depends_on`.

#### Async GHCR Mirroring

Move all `ghcr-push.sh` calls to a separate non-blocking `mirror-to-ghcr` pipeline. Internal registry pushes are what matter for release correctness.

### New Release Pipeline Timing

```
default (7-9 min) ──────────────────────────┐
build-controlplane-amd64 (10-15 min) ───────┤
build-controlplane-arm64 (15-20 min) ───────┤── manifest-controlplane (1 min)
build-sandbox-amd64 (8-12 min with base) ───┤── manifest-sandbox (1 min)
build-sandbox-arm64 (10-15 min with base) ──┘
build-macos-dmg (25-40 min, starts after controlplane-arm64)
build-runner* (2-3 min, retag)
mirror-to-ghcr (runs async after all builds)
```

**Critical path**: `build-controlplane-arm64` (15-20 min) → `build-macos-dmg` (25-40 min) = **~40-55 min**

Or if DMG depends on sandbox-arm64: ~25-35 min → DMG = **~50-70 min** (still much better than 110 min).

## Summary of Changes

| Change | Saves | Effort | Risk |
|---|---|---|---|
| **Split desktop Dockerfiles into base + app** | 25-35 min | Medium | Low — no logic change, just splitting |
| **Parallelize desktop builds** | 15-25 min | Low | None |
| **Pre-built CI test image** | 4-5 min | Low | Low — just a Docker image |
| **Decouple DMG from sandbox** | 25-40 min | Low | Needs verification |
| **Async GHCR mirroring** | 5-10 min | Medium | None |
| **Cache embedding models** | 3-5 min | Low | None |
| **Fix qwen-build layer caching** | 1-2 min | Low | None |

## Codebase Patterns Discovered

- Desktop Dockerfiles are clearly split into "stable base" and "FREQUENTLY CHANGING CODE" sections (already marked with comments at ~line 912/1100)
- The "stable base" section includes: build stages (Go, Rust), OS package installs, Chrome/OnlyOffice, compositor from source, themes/fonts/cursors
- The "app layer" section: qwen-code, Zed binary, startup scripts, Go binaries
- Both Dockerfiles use BuildKit cache mounts for Go/Cargo builds
- Zed build already has commit-based binary caching (cache hit = instant)
- ORT library is downloaded redundantly in 3 separate steps
- GHCR mirroring happens inline in every image push step
- ARM64 runner is macOS Docker Desktop (inherently slower, volume paths under `/Volumes/Big/`)
