# Design: Make Helix Builds and Releases Faster

## Goal

Reduce release build time from ~1h54m to under 45 minutes by attacking the critical path.

## Current Critical Path

```
build-sandbox-arm64 (~50-70 min)
  → build-macos-dmg (~25-40 min)
Total: ~75-110 min (critical path)
```

The sandbox pipeline is the bottleneck. Its internal critical path:

```
clone-deps (1 min)
  → build-zed (20-40 min)
  → build-desktops (25-40 min, sequential: sway then ubuntu)
  → build-sandbox (8-12 min)
  → push-sandbox (2-5 min)
```

## Optimization Strategy

### Phase 1: Pipeline Restructuring (Biggest Impact — saves ~25-35 min)

#### 1A. Parallelize Desktop Image Builds

**Current**: `build-desktops` step builds helix-sway AND helix-ubuntu sequentially in one step.

**Proposed**: Split into two separate Drone steps that run in parallel:
- `build-desktop-sway` — depends on build-zed + build-qwen-code
- `build-desktop-ubuntu` — depends on build-zed + build-qwen-code

**Savings**: ~15-25 minutes (currently sequential, each takes 15-25 min)

This is the single biggest win. The desktop images are independent — they don't depend on each other.

#### 1B. Decouple macOS DMG from Sandbox Pipeline

**Current**: `build-macos-dmg` depends on `build-sandbox-arm64` completing. But the DMG only needs the controlplane image and the VM disk — it doesn't use the sandbox image directly.

**Proposed**: Remove `build-sandbox-arm64` from `build-macos-dmg`'s `depends_on`. The DMG's `provision-vm` step uses the controlplane image (already built by `build-controlplane-arm64`). Let the sandbox pipeline finish independently.

**Savings**: ~25-40 minutes off the critical path (DMG starts earlier, overlaps with sandbox).

**Risk**: Need to verify that `provision-vm-light.sh` doesn't pull the sandbox image. If it does, keep the dependency but investigate whether the VM can use a pre-built sandbox.

#### 1C. Run Zed E2E Test in Parallel with Desktop Builds

**Current**: `zed-e2e-test` depends on `build-zed` and blocks `push-sandbox` (which also depends on `build-sandbox`).

**Proposed**: No change needed — the E2E test already runs in parallel with `build-desktops`. But verify that on the critical path, E2E finishes before `build-sandbox` (otherwise it adds wait time).

### Phase 2: Docker Build Optimization (Saves ~10-15 min)

#### 2A. Cache Embedding Model Downloads (Dockerfile)

**Current**: The `embedding-model` stage in the main Dockerfile runs `go run` to download/convert ONNX models without cache mounts. This re-downloads on every build.

**Proposed**: Add `--mount=type=cache,target=/tmp/helix-models` (or wherever the models are stored) to persist downloads across builds.

**Savings**: ~3-5 minutes per controlplane build.

#### 2B. Consolidate apt-get Calls in Desktop Dockerfiles

**Current**: Dockerfile.sway-helix and Dockerfile.ubuntu-helix have 5-6 separate `apt-get update` calls scattered through different RUN blocks.

**Proposed**: Combine package installations into fewer RUN blocks, or use BuildKit cache for apt lists:
```dockerfile
RUN --mount=type=cache,target=/var/cache/apt \
    --mount=type=cache,target=/var/lib/apt \
    apt-get update && apt-get install -y ...
```

**Savings**: ~2-3 minutes per desktop build.

#### 2C. Fix qwen-build Layer Caching

**Current**: `Dockerfile.qwen-build` copies all source before `npm ci`, invalidating the npm install cache on any source change.

**Proposed**: Copy `package.json` and `package-lock.json` first, run `npm ci`, then copy remaining source:
```dockerfile
COPY package*.json ./
RUN npm ci
COPY . .
RUN npm run build
```

**Savings**: ~1-2 minutes when only source (not dependencies) changed.

### Phase 3: Registry Push Optimization (Saves ~5-10 min)

#### 3A. Async GHCR Mirroring

**Current**: Every image push to `registry.helixml.tech` is immediately followed by a `ghcr-push.sh` call that pulls the image and re-pushes to GHCR. This doubles push time.

**Proposed**: Move GHCR mirroring to a separate, non-blocking pipeline that runs after the main build completes. The internal registry is what matters for the release — GHCR is for public distribution and can lag a few minutes.

**Implementation**: Add a new `mirror-to-ghcr` pipeline with `depends_on` on all build pipelines, triggered on tags. This removes GHCR pushes from the critical path entirely.

**Savings**: ~5-10 minutes across all image pushes.

#### 3B. Push Desktop Images in Parallel with Sandbox Build

**Current**: Desktop images are pushed to registry during `build-desktops`, then sandbox pulls them. The sandbox build writes version refs (`sandbox-images/`) that reference the registry images.

**Proposed**: Since sandbox uses registry refs (not local images), the push can happen asynchronously. But the sandbox build needs the refs — this is already handled correctly.

### Phase 4: Conditional Builds (Saves variable time)

#### 4A. Skip Unchanged Components

**Current**: Every release builds everything from scratch (Zed, qwen-code, desktops, sandbox).

**Proposed**: Extend the existing Zed commit-based caching to all components:
- Cache desktop images by content hash of their Dockerfiles + dependencies
- Cache qwen-code build output by commit hash
- If all inputs are unchanged, skip the build entirely

**Implementation**: Before each build step, check if the output already exists in the registry with the expected tag. If so, skip building and just retag.

**Savings**: Variable — 0 min (all changed) to 30+ min (no sandbox changes).

## Architecture Decision: What NOT to Change

1. **Don't drop ARM64** — required for macOS Desktop product
2. **Don't remove E2E tests** — quality gate is important
3. **Don't switch CI systems** — Drone is working, the issues are pipeline structure
4. **Don't pre-build desktop images** — they contain per-release components (Zed binary, qwen-code)
5. **Don't merge all images into one** — multi-image design is correct for the architecture

## Expected Results

| Optimization | Estimated Savings | Effort |
|---|---|---|
| Parallelize desktop builds | 15-25 min | Low (split one step into two) |
| Decouple DMG from sandbox | 25-40 min | Low (remove depends_on, verify) |
| Cache embedding models | 3-5 min | Low (add cache mount) |
| Async GHCR mirroring | 5-10 min | Medium (new pipeline) |
| Consolidate apt-get | 2-3 min | Low (edit Dockerfiles) |
| Fix qwen-build caching | 1-2 min | Low (reorder COPY) |
| Conditional builds | 0-30 min | Medium (cache checking logic) |

**Phase 1 alone** should reduce the critical path from ~110 min to ~45-60 min by parallelizing desktops and decoupling the DMG pipeline.

## Codebase Patterns Discovered

- **Drone CI**: Uses `depends_on` for step-level dependencies within a pipeline, and top-level `depends_on` for pipeline-level ordering
- **Registry mirroring**: `scripts/ghcr-push.sh` and `scripts/ghcr-manifest.sh` handle GHCR mirroring inline
- **Zed build caching**: Already implemented — caches binary by commit SHA in `/var/cache/drone/sandbox-build/zed-bin-{SHA}`
- **BuildKit cache mounts**: Used extensively for Go, Cargo, and npm caches
- **Desktop Dockerfiles**: Very large (900+ lines for sway, 1000+ for ubuntu) — build from source for sway, wlroots, grim; install GNOME for ubuntu
- **ARM64 runner**: macOS Docker Desktop, cache path `/Volumes/Big/drone-cache/`
