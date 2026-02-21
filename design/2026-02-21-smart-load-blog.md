# How We Made Docker Builds 124x Faster Across Agent Sessions

**Date**: 2026-02-21

## The Problem

Helix runs AI coding agents inside isolated desktop containers — each agent gets its own GNOME desktop with a full IDE, Docker daemon, and development environment. When an agent needs to build a project, it runs `docker build` inside its container.

The problem: **every new agent session started with a cold Docker build cache**. The containers are ephemeral — when a session ends, the container is destroyed along with its Docker state. For a project like Helix itself (which compiles a Rust IDE, Go APIs, Python services, and a Node.js frontend), a cold build takes **43 minutes**. That's 43 minutes of an agent sitting there waiting for builds before it can start working.

This matters because multiple agents regularly clone the exact same source code. Ten agents working on ten different tasks in the same repo all need to build the same base images. Without shared caching, that's 10 * 43 minutes = 7 hours of redundant compilation.

## The Architecture

The container nesting looks like this:

```
Host Machine
└── sandbox-nvidia (Docker-in-Docker host)
    ├── helix-buildkit (shared BuildKit instance)
    │   └── buildkit_state volume (persistent cache)
    ├── agent-session-A (desktop container)
    │   └── local dockerd → builds route to shared BuildKit
    ├── agent-session-B (desktop container)
    │   └── local dockerd → builds route to shared BuildKit
    └── agent-session-C ...
```

Each desktop container runs its own Docker daemon (for isolation), but all builds route to a **shared BuildKit instance** at the sandbox level. The BuildKit cache is stored on a persistent Docker volume that survives container restarts.

The key insight: when Agent B builds the same Dockerfile that Agent A already built, BuildKit says "I already have all these layers cached" and the build completes instantly. The cache is content-addressed — identical inputs produce identical cache keys regardless of which container initiated the build.

## The `--load` Bottleneck

Shared BuildKit got us halfway there. Builds were fast (5 seconds for fully cached images), but there was a catch: **the image still needed to be loaded into the local Docker daemon**.

When using a remote BuildKit builder, `docker buildx build --load` exports the built image as a tarball, streams it over gRPC to the client, and imports it into the local daemon. This happens even when every layer is cached and the image hasn't changed at all.

For a 7.7GB image (our desktop base image with GNOME, IDE, and dev tools):

| Operation | Time |
|-----------|------|
| `docker buildx build` (cached, no --load) | 5s |
| `docker buildx build --load` (cached) | **655s** |

That's 11 minutes to transfer an image that didn't change. The `--load` flag serializes the entire image into a Docker-format tarball, streams it over gRPC, and the receiving daemon deserializes and imports every layer — even layers it already has. There's no layer-level deduplication in the tarball transfer path. It's essentially `docker save | docker load` over a network connection.

Why is it so slow? The tarball format is sequential and uncompressed-then-recompressed. BuildKit exports each layer as a tar archive, concatenates them into the image tarball, streams it over gRPC (with flow control overhead), and the daemon re-extracts each layer to its content store. At ~12 MB/s effective throughput for 7.7GB, that's the 655 seconds.

## Smart `--load`

The fix is conceptually simple: **don't load the image if it hasn't changed**.

```
docker build -t myapp:latest .
  └── wrapper intercepts
      1. Build with --output type=image --provenance=false --iidfile /tmp/iid
         → BuildKit resolves all layers (cached: ~0.5s)
         → Writes image config digest to iidfile
         → No tarball transfer (--output type=image stores in BuildKit only)
      2. Compare iidfile digest with local daemon's image ID
         → docker images --no-trunc -q myapp:latest
      3. Match? → Skip --load. "Image unchanged, skipping load"
         Differ? → Run full --load. "Image changed, loading..."
```

A transparent wrapper at `/usr/local/bin/docker` intercepts both `docker build` and `docker buildx build`, applying this logic automatically. No code changes needed in build scripts, Makefiles, or CI pipelines.

### Three Critical Details

**1. `--iidfile` is empty without an output mode on remote builders.**

`docker buildx build --iidfile /tmp/iid -t foo .` with a remote builder produces an **empty iidfile**. BuildKit doesn't compute the image config digest unless it actually exports something. The fix: `--output type=image` tells BuildKit to create the manifest in its internal store (instant for cached builds, no data transfer) and populates the iidfile.

**2. `--provenance=false` is required.**

With default provenance, BuildKit wraps the image manifest in a **manifest list** that includes an attestation document with build timestamps. The iidfile gets the manifest list digest, which changes every build (because the timestamp changes). With `--provenance=false`, the iidfile contains the bare image config digest — deterministic and matching what `docker images --no-trunc -q` returns.

**3. The wrapper must handle both `docker build` and `docker buildx build`.**

Docker 29.x's `docker build` ignores the default buildx builder entirely — it always uses the local daemon's built-in BuildKit. Only `docker buildx build` honors the configured builder. The wrapper rewrites `docker build` to `docker buildx build` (to use the shared cache) and applies smart --load (to avoid the tarball transfer).

## Results

With shared BuildKit + smart `--load`, the full build sequence for Helix-in-Helix:

| Phase | Before | After | Speedup |
|-------|--------|-------|---------|
| API + frontend (compose) | 200s | **9.6s** | 21x |
| Zed IDE (Rust, release) | 459s | **1.2s** | 383x |
| Sandbox + desktop images | 2,075s | **12s** | 173x |
| **Total** | **45 min** | **23s** | **124x** |

The first agent to build a project pays the full cold build cost. Every subsequent agent (building the same source) gets a **23-second startup** — just enough time for Docker to verify the cache hits and confirm the images haven't changed.

When code actually changes, the build is still fast: BuildKit only recompiles changed layers (incremental), and `--load` only fires when the final image digest differs. A one-line Go change might rebuild only the final compilation layer (~30s) instead of the entire 43-minute pipeline.

### What the 23 seconds actually does

The 23 seconds breaks down as:
- **9.6s** — `docker compose build` resolves all 4 service images (API, frontend, haystack, typesense) against the shared BuildKit cache. Compose still does `--load` internally, but for fully-cached images the tarballs are small delta transfers.
- **1.2s** — `./stack build-zed release` checks the Zed builder image via smart --load. Image unchanged → skip the 300MB binary export.
- **12s** — `./stack build-sandbox` builds the sandbox Dockerfile (~4s), restarts the inner sandbox, and verifies desktop images via smart --load. No registry push/pull needed when images match.

The images DO need to exist in the local daemon (for `docker run` / `docker compose up`). The first time, `--load` transfers them. After that, smart --load skips the transfer as long as the source hasn't changed.

### What about single-layer changes?

When only one layer changes in a 7.7GB image, `--load` still transfers the **entire image** as a tarball (~10s). The tarball format doesn't support layer-level deduplication — it's all or nothing.

We solved this with a **shared Docker registry** running alongside BuildKit. When the wrapper detects an image has changed, it pushes to the registry (which only transfers changed layers) and pulls locally. The Docker pull protocol does layer-level dedup — it checks which layers the local daemon already has and only downloads the new ones.

Benchmarked on a 7.73GB image with a 1-line change in the top layer:

| Approach | Time | vs tarball |
|----------|------|-----------|
| Tarball `--load` (old) | **9,973ms** | 1x baseline |
| Registry push/pull (new) | **871ms** | 11.5x faster |
| Unchanged image (smart skip) | **314ms** | 31.7x faster |

The registry push shows `pushing layers 0.1s done` (one tiny layer). The pull shows 95 layers as "Already exists" and downloads only the single changed layer. Layer-level dedup turns a 10-second full-image transfer into a sub-second delta.

The three paths compose naturally:
1. **Image unchanged** → skip load entirely (314ms)
2. **Image changed, registry available** → push/pull via registry (871ms)
3. **Image changed, no registry** → fall back to tarball `--load` (10s)

## Implementation

The solution has three components:

1. **Docker wrapper** (`desktop/shared/docker-buildx-wrapper.sh`) — installed at `/usr/local/bin/docker` in each desktop container. Intercepts `docker build` and `docker buildx build`, routes through shared BuildKit, applies smart --load with registry acceleration.

2. **Shared BuildKit + Registry** (`api/pkg/hydra/manager.go`) — the Hydra manager starts a `helix-buildkit` container (shared cache) and a `helix-registry` container (layer-level transfer) at the sandbox level. Both are on the same Docker network as desktop containers.

3. **Init script** (`desktop/shared/17-start-dockerd.sh`) — configures the desktop container's dockerd to trust the insecure registry and exports `HELIX_REGISTRY` globally so the wrapper knows where to push/pull.

The wrapper is generic — it works for any `docker build` workload, not just Helix. It auto-detects whether the active builder is remote, and only applies smart --load when it is. On a standard local Docker setup, it's a transparent passthrough.

Source: [desktop/shared/docker-buildx-wrapper.sh](../desktop/shared/docker-buildx-wrapper.sh)
Design doc: [2026-02-21-shared-buildkit-cache.md](2026-02-21-shared-buildkit-cache.md)
