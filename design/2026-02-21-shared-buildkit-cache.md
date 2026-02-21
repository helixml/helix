# Shared BuildKit Cache Across Spectask Sessions

**Date**: 2026-02-21
**PRs**: #1705 (env var fix), #1706 (wrapper script), #1708 (timing instrumentation), #1709 (core buildx fix), #1711 (smart --load + sway→experimental)

## Architecture: Docker Nesting in Helix-in-Helix

```
Level 0: Host Machine
├── docker-compose.dev.yaml
│   ├── helix-api-1
│   ├── helix-postgres-1
│   ├── helix-registry-1 (localhost:5000)
│   └── helix-sandbox-nvidia-1 (Level 1)
│       ├── /var/lib/docker → sandbox-docker-storage volume (HOST volume, persistent)
│       ├── Hydra (manages desktop containers)
│       ├── helix-buildkit container
│       │   └── /var/lib/buildkit → buildkit_state volume (INSIDE sandbox docker, persistent)
│       └── Desktop containers (Level 2)
│           ├── ubuntu-external-{session_id}
│           │   ├── /var/lib/docker → docker-storage-{session_id} volume (INSIDE sandbox docker)
│           │   ├── dockerd (inner Docker daemon)
│           │   ├── GNOME/Mutter (desktop environment)
│           │   ├── Zed IDE + Qwen Code
│           │   └── [helix-in-helix only] Inner Helix stack (Level 3)
│           │       ├── Inner helix-api, helix-postgres, etc.
│           │       └── Inner helix-sandbox (Level 3)
│           │           └── Inner desktop containers (Level 4)
│           └── sway-external-{session_id} (same structure)
```

### Key Volume Persistence Chain

```
Host → sandbox-docker-storage (named volume, survives sandbox restarts)
     → contains buildkit_state volume (BuildKit cache)
     → contains docker-storage-{session} volumes (per-desktop Docker state)
```

- **buildkit_state persists** because it's a Docker volume inside the sandbox's dockerd,
  which itself uses the persistent `sandbox-docker-storage` volume on the host.
- **Desktop Docker state** (docker-storage-{session}) also persists similarly, so
  built images survive desktop container restarts.
- When the **sandbox is rebuilt** (`docker compose rm -f sandbox-nvidia`), the
  `sandbox-docker-storage` volume is NOT deleted (Docker preserves named volumes).
  BuildKit cache survives.

### BuildKit Cache Flow

```
Desktop container (Level 2)
  ├── docker build -t foo .           ← user/script runs this
  │   └── /usr/local/bin/docker       ← wrapper intercepts
  │       └── docker buildx build     ← rewrites to buildx (honors default builder)
  │           └── helix-shared        ← default buildx builder (remote driver)
  │               └── tcp://10.213.0.2:1234  ← BuildKit in sandbox (Level 1)
  │                   └── buildkit_state volume  ← SHARED cache across all sessions
  └── docker images foo:latest        ← --load ensures image is in local daemon
```

### Why Plain `docker build` Doesn't Work

Docker 29.x has two build backends:
1. **Built-in BuildKit** (`docker build`): Uses local daemon's BuildKit. Per-container, not shared.
2. **Buildx BuildKit** (`docker buildx build`): Honors `docker buildx use --default`. Can use remote builders.

`docker buildx use helix-shared --default` sets the default for `docker buildx build`,
but `docker build` ignores it entirely. The `BUILDX_BUILDER` env var forces both commands
to use a specific builder, but this env var isn't available in non-login shells (like the
startup script's init system).

**Solution**: The wrapper at `/usr/local/bin/docker` rewrites `docker build` to
`docker buildx build`, and `docker_build_load()` in `stack` uses `docker buildx build`
directly.

## Problem

Docker build cache was not shared between spectask sessions. Each desktop container ran its own local BuildKit instance, so the cache was lost when the container was destroyed. Building helix-in-helix (which compiles Rust/Zed from source) took ~43 minutes every time.

## Root Cause (Original)

The sandbox already ran a shared BuildKit container (`helix-buildkit`) with a persistent volume, and desktop containers configured a `helix-shared` remote buildx builder pointing to it. However, **Docker 29.x's `docker build` ignores the default buildx builder** and uses the local daemon's built-in BuildKit. Only `docker buildx build` or the `BUILDX_BUILDER` env var forces the remote builder.

## Root Cause (Deeper — PR #1709)

PRs #1705/#1706 set `BUILDX_BUILDER=helix-shared` in `/etc/profile.d/` and `~/.bashrc`, which works for interactive/login shells. But the helix-in-helix startup script runs via the container's init system (cont-init.d → startup-app.sh), which does NOT source `/etc/profile.d/` or `~/.bashrc`. So `BUILDX_BUILDER` was empty during startup, and both `docker_build_load()` in `stack` AND the docker wrapper fell through to plain `docker build` — bypassing the shared BuildKit entirely.

**Verification**: Exec'd into a running helix-in-helix desktop container and confirmed `BUILDX_BUILDER` was empty:
```
$ docker exec ubuntu-external-XXX bash -c 'echo BUILDX_BUILDER=$BUILDX_BUILDER'
BUILDX_BUILDER=
```

Meanwhile `docker buildx ls` showed `helix-shared*` as the default — but plain `docker build` ignored it.

**Verification of fix**: After deploying the updated wrapper to the container, `docker build` correctly routes through the shared BuildKit:
```
$ docker build -t test -f /tmp/test-dockerfile /tmp/
#0 building with "helix-shared" instance using remote driver
#5 [2/2] RUN echo hello
#5 CACHED          <-- cache hit from shared BuildKit!
#6 importing to docker    <-- --load auto-injected
```

## Fix

Four-part solution:

1. **`BUILDX_BUILDER=helix-shared` set globally** in `/etc/environment`, `/etc/profile.d/`, and `~/.bashrc` for interactive sessions. (PR #1705, `17-start-dockerd.sh`)

2. **`docker_build_load()` uses `docker buildx build`** instead of `docker build`. `docker buildx build` honors the default builder (set via `docker buildx use --default`), while `docker build` does not. Also auto-detects the builder driver via `docker buildx inspect` without requiring `BUILDX_BUILDER`. (PR #1709, `stack`)

3. **Docker wrapper rewrites `docker build` → `docker buildx build`** and adds `--load` for remote builders. No longer requires `BUILDX_BUILDER` — auto-detects the default builder. (PR #1709, `docker-buildx-wrapper.sh`)

4. **Smart `--load`: skip image export when unchanged** (PR #1711). Both `docker_build_load()` and the wrapper now build WITHOUT `--load` first to get the image digest via `--iidfile`, then compare with the local daemon's image ID. If they match, the image hasn't changed and `--load` is skipped entirely. See [Smart --load Optimization](#smart---load-optimization) below.

## Smart `--load` Optimization

### The `--load` Bottleneck

With remote BuildKit builders, `docker buildx build --load` exports the built image as a tarball from the remote builder, streams it to the local daemon, and imports it. This happens even when ALL build steps are cached and the image hasn't changed.

Measured inside a spectask (helix-in-helix):

| Image | Size | `--load` time (cached) | Without `--load` (cached) |
|-------|------|----------------------|--------------------------|
| helix-ubuntu | 7.24 GB | ~655s (11 min) | ~5s |
| helix-api | 619 MB | ~40s | ~3s |
| helix-frontend | 1.67 GB | ~120s | ~4s |

### How Smart `--load` Works

```
docker build -t foo:bar .     ← user/script runs this
  └── wrapper/docker_build_load()
      1. Is builder remote?  → NO: just run docker buildx build (no --load needed)
      2. Does local daemon have foo:bar? → NO: run with --load (first build)
      3. Quick build with --output type=image --provenance=false --iidfile  (~0.5s for cached)
      4. Compare iidfile digest with local image ID:
         - MATCH: skip --load ("Image unchanged, skipping load")
         - DIFFER: run with --load ("Image changed, loading into daemon...")
```

### Why This Works

- With remote BuildKit, `--iidfile` is EMPTY without an output mode — the builder doesn't compute the image digest unless it exports something
- `--output type=image` exports the manifest to BuildKit's internal image store (fast, ~0s, no tarball transfer to client)
- `--provenance=false` prevents BuildKit from wrapping the manifest in a manifest list — so `--iidfile` returns the **config digest** (sha256 of image config), not a manifest list digest
- `docker images --no-trunc -q` returns the same config digest for loaded images
- These digests are deterministic for identical build inputs (layers + config)
- Verified empirically: `--output type=image --provenance=false --iidfile` matches `docker images --no-trunc -q` for the same image

### Overhead

- **Image unchanged (common case)**: ~0.5s instead of ~655s. Savings: **~654s per 7.7GB image**.
- **Image changed**: ~5s extra for the check build, then full --load. Context is sent twice (~6MB for helix, negligible).
- **First build ever**: no overhead (directly uses --load since no local image exists).

### Correctness

- If `--iidfile` fails or is empty: falls back to --load (no optimization, but no regression)
- If digest comparison fails (different formats): `!=` is true, falls back to --load (no regression)
- Works for all users' docker usage — no helix-specific behavior in the wrapper

## Guidance for Users / Agent Authors

To ensure good build caching across spectask sessions:

1. **Always use `docker build` or `docker buildx build`** — both now route through the shared BuildKit via the wrapper. Don't bypass the wrapper by calling `/usr/bin/docker build` directly.

2. **Pin base images with sha256 digests** in Dockerfiles (`FROM ubuntu:25.10@sha256:...`). Without a digest, BuildKit may re-resolve the `latest` tag, which can change and invalidate the cache.

3. **Order Dockerfile layers by change frequency** — put rarely-changing layers (apt-get install, system deps) before frequently-changing layers (COPY source code). BuildKit caches layers sequentially.

4. **Use `.dockerignore`** files to exclude large/volatile directories (`.git/`, `node_modules/`, build artifacts). Smaller build contexts mean faster transfers and better cache hit rates.

5. **Don't `docker builder prune` or `docker system prune`** — this destroys the shared BuildKit cache, affecting ALL sessions. Use targeted cleanup (`docker rmi old-image:tag`) instead.

6. **BuildKit cache mounts** (`--mount=type=cache,target=/root/.cargo`) persist across builds on the shared BuildKit. Use them for package manager caches (cargo, go mod, apt, npm).

7. **Be aware of nesting levels**: The shared BuildKit runs at the sandbox level (Level 1). All desktop containers (Level 2) share this cache. If you start a helix-in-helix stack inside a desktop container, that inner stack's builds also go through the same shared BuildKit.

8. **`docker compose build`** natively reads `BUILDX_BUILDER` and also honors the default buildx builder. It does NOT go through the wrapper (wrapper intercepts `docker build`, not `docker compose build`). Both `BUILDX_BUILDER=helix-shared` and `docker buildx use helix-shared --default` make compose builds use the shared cache.

## Results

### With Smart `--load` (PR #1712) — Measured

Measured inside spectask `ubuntu-external-01khzm570xsv3ac2yr51vvhfrq` with all caches hot:

| Phase | Cold Cache | Hot Cache (old) | Hot Cache (smart --load) | Improvement |
|-------|-----------|-----------------|--------------------------|-------------|
| `./stack build` (compose) | ~3 min | ~200s | **8s** | 25x |
| `./stack build-zed release` | ~11 min | ~459s | **1s** | 459x |
| `./stack build-sandbox` | ~29 min | ~2075s | **13s** | 160x |
| **Total startup** | **~43 min** | **~2734s (45 min)** | **22s** | **124x** |

Key: `./stack build` (compose) also benefits because shared BuildKit cache is hot — compose only needs to --load tiny changed layers.

### Previous Results (Before Smart `--load`)

| Phase | Cold Cache | Hot Cache | Speedup |
|-------|-----------|-----------|---------|
| `./stack build` | ~3 min | ~15 sec | ~12x |
| `./stack build-zed release` | ~11 min | ~45 sec | ~15x |
| `./stack build-sandbox` | ~29 min | ~20 min* | ~1.5x |
| **Total startup** | **~43 min** | **~21 min** | **~2x** |

*`build-sandbox` was limited by `--load` exporting full images even when cached.

## Investigation: build-sandbox Bottlenecks

### Local Timing (host Docker, partially cached)

Run on 2026-02-21 with timing instrumentation in `stack` (PR #1708):

| Phase | Duration | Notes |
|-------|----------|-------|
| Zed check | 0s | Binary existed |
| Build sway (docker build) | 206s | grim-build + rust-build-env cache miss |
| Build ubuntu (docker build) | 55s | Most layers shared with sway, cached |
| Build sandbox Dockerfile | 8s | Small, mostly cached |
| Sandbox restart + dockerd | 15s | |
| Push sway to registry | 65s | ~7.7GB image via localhost:5000 |
| Pull sway into sandbox | 41s | Via sandbox's dockerd |
| Push ubuntu to registry | 32s | |
| Pull ubuntu into sandbox | 35s | |
| Cleanup/GC | ~30s | |
| **Total** | **462s (7.7 min)** | Partially cached |

Key findings:
- Images are ~7.7GB uncompressed, not ~3GB as originally estimated
- Per-Dockerfile `.dockerignore` files already exclude `.git` (7GB) — context is only 6MB
- Base images already pinned with sha256 digests — not the cache invalidation source
- Push/pull of two ~7.7GB images = 173s (~3 min) even on localhost
- Sequential transfers: sway finishes entirely before ubuntu starts

### In-Spectask Timing (before smart --load)

Measured inside spectask `ubuntu-external-01khzm570xsv3ac2yr51vvhfrq` with shared BuildKit hot cache:

| Phase | Duration | Notes |
|-------|----------|-------|
| `./stack build` (compose) | 200s | All cached but compose still does --load internally |
| `./stack build-zed release` | 459s | Cached build + --load export of Zed binary image |
| `./stack build-sandbox` | ~2075s | Desktop builds + --load + registry transfers |
| **Total** | **~2734s (45 min)** | Dominated by --load exports |

The `--load` export was the clear bottleneck:
- `docker buildx build --load` for cached helix-ubuntu: **655s** (175s exporting + 68s loading + overhead)
- `docker buildx build` (no --load) for same cached image: **5s**
- Savings from smart --load when image unchanged: **~650s per build**

### Image sizes (inside spectask)

| Image | Size |
|-------|------|
| helix-ubuntu | 7.24 GB |
| helix-sway | 5.2 GB |
| helix-haystack | 5.14 GB |
| helix-frontend | 1.67 GB |
| helix-typesense | 996 MB |
| helix-api | 619 MB |
| helix-sandbox | 616 MB |

### Optimizations Applied

1. **Smart `--load`** (PR #1711): Skip --load when image unchanged. Saves ~650s per 7.7GB image.
2. **Sway moved to experimental** (PR #1711): `PRODUCTION_DESKTOPS=(ubuntu)` instead of `(sway ubuntu)`. Saves building+transferring the 5.2GB sway image. Spectasks only use ubuntu.
3. **Shared BuildKit cache** (PRs #1705-#1709): All builds use the shared BuildKit at sandbox level. Cache persists across sessions.

### Remaining Bottlenecks

1. **`docker compose build` (./stack build)**: Compose handles --load internally and doesn't support smart --load. Still takes ~200s in spectask even when fully cached. Potential fix: compose might support `--iidfile` or we could wrap compose too.
2. **Registry push/pull for desktop transfer**: When image DOES change, push/pull of 7.7GB image takes ~100s on localhost. Not optimizable without smaller images.
3. **Build-zed --load**: Zed binary image is small (~85MB) so --load is fast. With smart --load, this should be ~5s.

## Files Changed

- `stack` — `docker_build_load()` with smart --load (build without --load, compare iidfile digest with local image ID, skip --load if unchanged). `PRODUCTION_DESKTOPS=(ubuntu)` (sway moved to experimental).
- `desktop/shared/17-start-dockerd.sh` — sets `BUILDX_BUILDER` globally, installs wrapper
- `desktop/shared/docker-buildx-wrapper.sh` — rewrites `docker build` → `docker buildx build`, smart --load for remote builders
- `Dockerfile.ubuntu-helix`, `Dockerfile.sway-helix` — bundle the wrapper script
