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
    ├── helix-registry (shared Docker registry)
    │   └── registry_data volume (layer-level transfer cache)
    ├── agent-session-A (desktop container)
    │   └── local dockerd → builds route to shared BuildKit
    ├── agent-session-B (desktop container)
    │   └── local dockerd → builds route to shared BuildKit
    └── agent-session-C ...
```

Each desktop container runs its own Docker daemon (for isolation), but all builds route to a **shared BuildKit instance** at the sandbox level. The BuildKit cache is stored on a persistent Docker volume that survives container restarts.

The key insight: when Agent B builds the same Dockerfile that Agent A already built, BuildKit says "I already have all these layers cached" and the build completes instantly. The cache is content-addressed — identical inputs produce identical cache keys regardless of which container initiated the build.

## The `--load` Bottleneck

Shared BuildKit got us halfway there. Builds were fast (~0.5 seconds for fully cached images), but there was a catch: **the image still needed to be loaded into the local Docker daemon**.

When using a remote BuildKit builder, `docker buildx build --load` exports the built image as a tarball, streams it over gRPC to the client, and imports it into the local daemon. This happens even when every layer is cached and the image hasn't changed at all.

For a 7.73GB image (our desktop base image with GNOME, IDE, and dev tools):

| Operation | Time |
|-----------|------|
| `docker buildx build` (cached, no --load) | 0.5s |
| `docker buildx build --load` (cached) | **~10s** |

That's 10 seconds to transfer an image that didn't change. The `--load` flag serializes the entire image into a Docker-format tarball, streams it over gRPC, and the receiving daemon deserializes and imports every layer — even layers it already has. There's no layer-level deduplication in the tarball transfer path.

This adds up: building Helix involves 6+ images. Even with a hot BuildKit cache, the `--load` overhead per image turns a sub-second build into a 10-second wait, and the full stack build takes ~23 seconds of mostly `--load` transfers.

## Smart `--load`

The first optimization: **don't load the image if it hasn't changed**.

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
         Differ? → Use registry push/pull for layer-level transfer
```

A transparent wrapper at `/usr/local/bin/docker` intercepts both `docker build` and `docker buildx build`, applying this logic automatically. No code changes needed in build scripts, Makefiles, or CI pipelines.

### Three Critical Details

**1. `--iidfile` is empty without an output mode on remote builders.**

`docker buildx build --iidfile /tmp/iid -t foo .` with a remote builder produces an **empty iidfile**. BuildKit doesn't compute the image config digest unless it actually exports something. The fix: `--output type=image` tells BuildKit to create the manifest in its internal store (instant for cached builds, no data transfer) and populates the iidfile.

**2. `--provenance=false` is required.**

With default provenance, BuildKit wraps the image manifest in a **manifest list** that includes an attestation document with build timestamps. The iidfile gets the manifest list digest, which changes every build (because the timestamp changes). With `--provenance=false`, the iidfile contains the bare image config digest — deterministic and matching what `docker images --no-trunc -q` returns.

**3. The wrapper must handle both `docker build` and `docker buildx build`.**

Docker 29.x's `docker build` ignores the default buildx builder entirely — it always uses the local daemon's built-in BuildKit. Only `docker buildx build` honors the configured builder. The wrapper rewrites `docker build` to `docker buildx build` (to use the shared cache) and applies smart --load (to avoid the tarball transfer).

## Registry-Accelerated Loading

Smart --load eliminates the transfer when nothing changed. But when code *does* change, even a one-line change in the top layer of a 7.73GB image still triggers a full tarball `--load` (~10s). The tarball format doesn't support layer-level deduplication — it's all or nothing.

We solved this with a **shared Docker registry** running alongside BuildKit on the sandbox network. When the wrapper detects an image has changed, instead of `--load`:

1. **Push** to the registry — BuildKit pushes only the changed layers (~0.1s)
2. **Pull** from the registry — the local daemon checks which layers it already has, downloads only the new ones (~0.5s)

The Docker registry protocol does layer-level dedup natively. For a 7.73GB image with 95 base layers and 1 changed layer, the pull shows 95 "Already exists" and downloads only the single new layer.

### Benchmarks: 1-line change in top layer of 7.73GB image

Measured E2E inside a real desktop container, 3 runs each:

| Approach | Time | Speedup |
|----------|------|---------|
| Tarball `--load` (before) | **9,973ms** | 1x baseline |
| Registry push/pull (after) | **871ms** | **11.5x** |
| Unchanged image (smart skip) | **314ms** | **31.7x** |

The three paths compose naturally:
1. **Image unchanged** → skip load entirely (314ms)
2. **Image changed, registry available** → push/pull via registry (871ms)
3. **Image changed, no registry** → fall back to tarball `--load` (10s)

## Results

There are two cases that matter: cold start (first agent to build a project) and warm start (subsequent agents building the same source).

### Cold start: ~10 minutes (down from 45 minutes)

A fresh agent session starts with an empty Docker daemon — no images, no layers. Even though every build is a cache hit in shared BuildKit (the compilation is instant), the images still need to be transferred into the local daemon. For Helix-in-Helix, this is a deeply nested pipeline:

| Phase | Before | After (cold) | Notes |
|-------|--------|-------------|-------|
| API + frontend (compose) | 200s | **~60s** | Tarball --load (compose bypasses wrapper) |
| Zed IDE + desktop image | 459s | **~132s** | 7.24GB image load into local daemon |
| Inner sandbox setup | 2,075s | **~380s** | Push/pull desktop image through inner registry |
| **Total** | **45 min** | **~10 min** | **4.5x** |

The cold start is dominated by **image transfer, not compilation**. BuildKit resolves all layers instantly (cached), but loading 7+ GB images into each nesting level takes time. The bottleneck is the `--load` tarball path: it serializes the entire image regardless of what the receiving daemon already has.

The nesting makes this worse: Helix-in-Helix has the desktop container (L2) building an inner sandbox (L3), which needs the same 7.24GB desktop image transferred again to a fresh daemon one level deeper.

### Warm start: 23 seconds (124x faster)

Once images exist in the local daemon, subsequent builds are near-instant:

| Phase | Before | After (warm) | Speedup |
|-------|--------|-------------|---------|
| API + frontend (compose) | 200s | **9.6s** | 21x |
| Zed IDE (Rust, release) | 459s | **1.2s** | 383x |
| Sandbox + desktop images | 2,075s | **12s** | 173x |
| **Total** | **45 min** | **23s** | **124x** |

Smart --load checks the image digest against the local daemon (~0.3s) and skips the transfer when nothing changed. This is the common case: agents working on the same codebase where the base images haven't been modified.

### Incremental changes: ~1 second per image

When code actually changes, the registry-accelerated load transfers only the changed layers:

| Scenario | Time |
|----------|------|
| Unchanged image (smart skip) | **314ms** |
| 1-layer change via registry | **871ms** |
| 1-layer change via tarball (fallback) | **9,973ms** |

A one-line Go change rebuilds only the final compilation layer (~30s) and transfers only that layer via the registry (~1s) instead of the entire 43-minute pipeline.

### Remaining bottleneck: cold-start image transfer

The cold start is the main remaining problem. The 10-minute first build is 4.5x better than 45 minutes, but it's still 10 minutes of an agent waiting. The bottleneck is transferring large images (~7 GB) as tarballs through nested Docker daemons.

Two things would help:
1. **Make `docker compose build` use the registry path** — compose currently bypasses the wrapper and always uses tarball `--load`. Intercepting compose (or switching to buildx bake with registry output) would cut the compose build from ~60s to ~10s on cold start.
2. **Pre-warm the inner daemon** — if the desktop container's Docker data directory were a persistent volume (instead of ephemeral), images would survive across sessions. The first session pays the transfer cost; subsequent sessions start warm.

## Implementation

The solution has three components:

1. **Docker wrapper** (`desktop/shared/docker-buildx-wrapper.sh`) — installed at `/usr/local/bin/docker` in each desktop container. Intercepts `docker build` and `docker buildx build`, routes through shared BuildKit, applies smart --load with registry acceleration. Falls back to tarball `--load` if registry is unavailable.

2. **Shared BuildKit + Registry** (`api/pkg/hydra/manager.go`) — the Hydra manager starts a `helix-buildkit` container (shared build cache) and a `helix-registry` container (layer-level transfer) at the sandbox level. Both are on the same Docker network as desktop containers. BuildKit is configured to trust the insecure registry for push operations.

3. **Init script** (`desktop/shared/17-start-dockerd.sh`) — configures the desktop container's dockerd to trust the insecure registry (`insecure-registries` in `daemon.json`) and exports `HELIX_REGISTRY` and `BUILDX_BUILDER` globally so the wrapper knows where to push/pull and which builder to use.

The wrapper is generic — it works for any `docker build` workload, not just Helix. It auto-detects whether the active builder is remote, and only applies smart --load when it is. On a standard local Docker setup, it's a transparent passthrough.

Source: [desktop/shared/docker-buildx-wrapper.sh](../desktop/shared/docker-buildx-wrapper.sh)
Design doc: [2026-02-21-shared-buildkit-cache.md](2026-02-21-shared-buildkit-cache.md)
