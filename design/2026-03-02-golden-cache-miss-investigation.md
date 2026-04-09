# Golden Build Cache Miss Investigation

**Date:** 2026-03-02
**Status:** Investigation — root cause identified
**Context:** Golden build cache (49.8 GB, completed 13:10) was rebuilt, but subsequent
session `spt_01kjpxx7teg0xyafa7r332z7ep` (started 13:24, same code, same commit) still
shows BuildKit cache misses during startup builds.

## Key Finding (TL;DR)

The shared BuildKit cache is NOT preserving layers between the golden build and
subsequent sessions. Evidence: the session's **first** probe build has cache misses,
but its **second** build (same BuildKit, same files, seconds later) gets 100% cache
hits. This proves the files are identical — BuildKit just doesn't have the entries
from the golden build.

## Actual Build Log Analysis

Source: `/tmp/-AHFBU-iDcMlZ_5EAgiT9w/screen.txt` inside container
`ubuntu-external-01kjqbermx8saym6848dgsk8dc`.

### Summary: What the docker wrapper does for each service

| Service | First Pass (Probe) | Second Pass (Load) | Time |
|---------|-------------------|-------------------|------|
| **helix-api** | Cache misses (api-dev-env, tokenizers-lib, embedding-model) | ALL CACHED → registry load | ~75s |
| **helix-haystack** | diffusers-build-env entirely rebuilt | ALL CACHED → registry load | ~45s |
| **helix-frontend** | ALL CACHED | Skipped ("Image unchanged") | ~0.5s |
| **helix-typesense** | ALL CACHED | Skipped ("Image unchanged") | ~0.5s |
| **Zed IDE** | COPY . /zed not cached → 550s cargo build | (no second pass — uses --output type=local) | **554s** |
| **helix-ubuntu** | 82/96 steps cached, ~14 rebuilt | ALL CACHED → registry load | ~24s |
| **qwen-code** | Base image not cached, builder rebuilt | (uses --output type=local) | ~44s |

**Total rebuild time: ~740s (~12 min), dominated by Zed at 554s (75%).**

### Detailed Trace: helix-api

**First pass (probe build):**
```
[docker-wrapper] compose build: 4 service(s) via smart --load
[docker-wrapper]   api → helix-api

=> [api-base 1/5] FROM golang:1.25-bookworm@sha256:5...   0.0s   ← CACHED
=> CACHED [api-base 2/5] WORKDIR /app                      0.0s   ← CACHED
=> CACHED [api-base 3/5] RUN apt-get update ...             0.0s   ← CACHED
=> CACHED [api-base 4/5] COPY go.mod go.sum ./              0.0s   ← CACHED
=> CACHED [api-base 5/5] RUN --mount=type=cache...          0.0s   ← CACHED
=> CACHED [api-dev-env 1/7] RUN go install air...           0.0s   ← CACHED
=> [api-dev-env 2/7] RUN apt-get update ...                 3.2s   ← MISS ❌
=> [tokenizers-lib 1/1] RUN --mount=type=cache...           1.3s   ← MISS ❌
=> [embedding-model 1/2] COPY --from=uv:debian-slim...      0.3s   ← MISS ❌
=> [embedding-model 2/2] RUN --mount=type=cache...         64.4s   ← MISS ❌ (big!)
=> [api-dev-env 3-7] ...                                   ~10s    ← cascade

[docker-wrapper] Image changed (new: sha256:a37365ce09d7), loading into daemon...
```

**Second pass (registry push/pull, seconds later, same BuildKit):**
```
=> CACHED [api-base 1-5]                                    0.0s   ← CACHED
=> CACHED [api-dev-env 1-7]                                 0.0s   ← ALL CACHED ✅
=> CACHED [tokenizers-lib 1/1]                              0.0s   ← CACHED ✅
=> CACHED [embedding-model 1-2]                             0.0s   ← CACHED ✅

[docker-wrapper] Loaded via registry (layer-level dedup)
```

**The smoking gun:** `api-dev-env 2/7`, `tokenizers-lib 1/1`, `embedding-model 2/2`
are all NOT cached in the first pass but CACHED in the second pass. The second pass
runs on the same BuildKit seconds later with the same files. This proves:
1. The files ARE identical (same cache key → hit on second pass)
2. BuildKit DID NOT have these entries when the session first asked

### Detailed Trace: helix-haystack (diffusers)

**First pass:**
```
=> [uv 1/1] FROM ghcr.io/astral-sh/uv:0.10.2@sha256:...            0.0s   ← CACHED
=> [diffusers-build-env 1/9] FROM uv:0.10.2-debian-slim@sha256:...  1.1s   ← MISS ❌
=> [stage-2 1/9] FROM python:3.11-slim@sha256:...                   0.0s   ← CACHED
=> CACHED [stage-2 2-4]                                              0.0s   ← CACHED
=> [diffusers-build-env 2-9] ...                                    ~24s   ← ALL MISS ❌
=> [stage-2 5/9] CACHED (COPY from diffusers uses old cached ver)    0.0s
=> [stage-2 6-9] ... (cascade from diffusers rebuild)               ~27s

[docker-wrapper] Image changed → registry push/pull
```

**Second pass:** ALL CACHED ✅

Note: `uv:0.10.2` is cached but `uv:0.10.2-debian-slim` is NOT. Both are pinned with
`@sha256:...` digests. The golden build should have pulled and cached both. Yet one is
missing from BuildKit's cache.

### Detailed Trace: Zed IDE (the big one)

```
=> CACHED [builder-env 2/3] RUN apt-get update ...            0.0s   ← CACHED
=> CACHED [builder-env 3/3] RUN curl ... rustup ...           0.0s   ← CACHED
=> [builder 1/3] COPY . /zed                                  0.6s   ← MISS ❌
=> [builder 2/3] WORKDIR /zed                                 0.1s   ← cascade
=> [builder 3/3] RUN --mount=type=cache ... cargo build ...  550.4s  ← MISS ❌ (!!!)
```

`COPY . /zed` transfers 69.78MB build context and is NOT cached. The user confirms
nothing changed in the repos between the golden build and the session. Both cloned
from the same git server at the same commit. The `builder-env` stage (apt-get + rustup)
IS cached, so BuildKit has SOME cache from the golden build — just not the `COPY` layer.

There is no second pass for Zed (uses `--output type=local`, bypasses smart --load).
So this is a 554s penalty every session.

### helix-ubuntu (96-step build)

First pass: steps 2-82 ALL CACHED ✅. Then:
```
=> [stage-3 83/96] ADD dconf-settings.ini ...                 0.7s   ← MISS ❌
=> [go-build-env 4/8] COPY go.mod go.sum ./                   1.1s   ← MISS ❌
```

Step 83 (`ADD dconf-settings.ini`) broke the cache chain, causing steps 83-96 to
rebuild. `go-build-env 4/8` (`COPY go.mod go.sum`) also wasn't cached, triggering
`go-build-env 5-8` rebuilds.

Second pass (registry push): ALL CACHED ✅.

Same pattern — files are identical (second pass proves it), but BuildKit doesn't have
the cache from the golden build.

### qwen-code

```
=> [builder 1/8] FROM node:20-slim@sha256:d8a35d...            0.4s   ← MISS ❌
=> [builder 2/8] RUN apt-get update ...                         7.4s   ← cascade
```

Base image `node:20-slim` not cached (0.4s instead of 0.0s). Everything cascades.

## Architecture: Two Separate Caching Layers

The system has two independent caches that work together:

### Cache A: Docker Daemon Data (`/var/lib/docker/`)
- Contains final Docker images (overlay2 layers, image metadata)
- Copied from golden → session via `cp -a --reflink=auto` (49.8 GB, ~55s)
- Used by `docker images -q TAG` to check if image exists locally
- **Works correctly.** Sessions start with all images pre-loaded.

### Cache B: BuildKit Data (`/var/lib/buildkit/`)
- Contains intermediate build layers, build contexts, cache mounts
- Shared across ALL sessions via the `helix-buildkit` container (tcp://10.213.0.3:1234)
- **NOT copied** by the golden cache mechanism — it's shared, not per-session
- Used by the docker wrapper's probe build to check if images changed

### The intended flow

1. Golden build runs `./stack build` → docker wrapper → probe build via BuildKit
2. BuildKit has cache entries → probe is instant (0.5s) → digest matches local → skip
3. Golden's Docker data is promoted to golden cache
4. Normal session starts → gets golden's Docker data → all images pre-loaded
5. Normal session runs `./stack build` → probe build via BuildKit
6. BuildKit has same cache entries → probe is instant → digest matches → skip
7. **Result: session startup builds take ~5 seconds total**

### What actually happens

Steps 1-4 work correctly. But at step 6, BuildKit does NOT have the cache entries.
The probe build encounters cache misses, triggering full rebuilds (~740s total).

## Root Cause: BuildKit Cache Hits Don't Refresh Timestamps → GC Evicts "Stale" Entries

### The key discovery

The golden build (12:56-13:10) got **100% cache hits** — it created zero new layer
entries (only 2 mandatory "local source" context entries). But it left **zero
fingerprint** on cache entry `LastUsedAt` timestamps:

```
Distribution of LastUsedAt by hour:
110 entries: March 2, 14:xx  (session 2 — rebuilds)
 79 entries: March 2, 09:xx  (morning build — original creation)
  9 entries: March 2, 13:xx  (2 golden context + 7 session 1 frontend)
  0 entries: March 2, 12:xx  ← golden build was 12:56-13:10. ZERO entries.
 79 entries: Feb 21           ← 9 days old, still alive!
 60 entries: Feb 25           ← 5 days old, still alive!
```

**BuildKit does NOT update `LastUsedAt` on cache hits** (at least in practice —
the source code has an `updateLastUsed()` function, but the build solver code path
apparently doesn't exercise it for cache hits during builds).

### The mechanism

1. Morning build (09:xx) creates 84 layer entries with `LastUsedAt ≈ 09:50`
2. Golden build (12:56) gets 100% cache hits — entries keep `LastUsedAt ≈ 09:50`
3. Session 1 starts at 13:24, runs probe builds
4. Some probes get cache misses (for reasons below), triggering rebuilds
5. New entries push total cache from ~94 GB to ~115 GB
6. GC evicts entries to get back under the 100 GB limit (93.13 GiB)
7. GC correctly evicts by `LastUsedAt` — the 09:50 entries look ~4 hours old
8. Small orphan entries from Feb 21 (4 KB each) survive because GC targets large entries first
9. Session 2 finds even more entries missing → more rebuilds → more eviction

### Why the golden build's cache hits don't help

The golden build reuses entries from the 09:xx build. Those entries retain their
**original** `LastUsedAt` from 09:xx. When GC runs 4+ hours later, those entries
are among the oldest and get evicted — even though the golden build "used" them.

### The numbers

```
$ du -sb /var/lib/buildkit/
94,391,078,569 bytes = 94.4 GB = 87.9 GiB

$ docker buildx inspect helix-shared
GC Policy rule#3:   All: true   Max Used Space: 93.13 GiB    (= ~100 GB)
```

**At rest, the cache (94 GB) is under the limit (100 GB).** Only 6 GB of headroom.
Each session's builds add ~20 GB, temporarily pushing to ~115 GB, triggering GC.

### Why the second pass always succeeds

The second build runs immediately after the first on the same BuildKit. The entries
from the first build are freshly created (not cache hits), so they have recent
`LastUsedAt` timestamps. GC hasn't run yet to evict them.

## Architecture Review: The Implementation Is Correct But Fragile

### All three layers work correctly in isolation

1. **Golden cache → Docker daemon:** ✅ Working. `cp --reflink=auto` copies images.
2. **Docker wrapper → smart --load:** ✅ Working. Probe build, digest compare,
   registry push/pull all function correctly.
3. **Shared BuildKit → build cache:** ✅ Working within a single build session.
   The second pass always gets 100% hits.

### The gap: BuildKit cache isn't persisting between sessions

The golden build populates BuildKit cache. But 14 minutes later, the session can't
find some of those entries. The two builds use the same shared BuildKit endpoint
(`tcp://10.213.0.3:1234`), so the cache should be shared.

### The wrapper's two-pass pattern works but doubles build cost on cache miss

When the probe build has cache misses, the wrapper:
1. Builds once (probe, `--output type=image`) — full rebuild cost
2. Detects image changed
3. Builds again (`--output type=image,push=true`) — now cached (instant)
4. Pulls from registry

This means a cache miss costs one full build + one instant cached build + one registry
pull. The full build dominates. The second build is cheap, but it wouldn't be needed
if the first one could also push to registry.

**Optimization:** Merge the probe and push into a single build. Instead of:
1. Build with `--output type=image --iidfile` → check digest → if changed → build
   again with `--output type=image,push=true`

Do:
1. Build with `--output type=image,name=<registry>/<tag>,push=true --iidfile` → check
   digest → if changed → pull from registry

This eliminates the second build entirely.

## Fix: Raise BuildKit GC Limit to 300 GiB

**Commit:** `api/pkg/hydra/manager.go` — `configureBuildKitRegistry()` now writes
`[worker.oci]` GC policies with `keepBytes = 322122547200` (300 GiB) to
`buildkitd.toml`, up from the default 93 GiB.

The fix mirrors BuildKit's default 4-rule GC structure but raises the caps:
- Rule 0: source.local/exec.cachemount/source.git.checkout → 48h, 10 GiB (was 488 MB)
- Rule 1-3: referenced/all blobs → 300 GiB (was 93 GiB)

The "already configured" check now also looks for `worker.oci` in the existing config,
so running sandboxes will pick up the GC policy on next restart.

**How to apply to running sandbox:** Restart the API (which restarts hydra manager →
`configureBuildKitRegistry()` detects missing GC config → rewrites TOML → restarts
BuildKit container).

## Future Optimizations

### Short-term: Eliminate the two-pass penalty

1. **Merge probe + push into one build.** Change the wrapper to always push to registry
   on the first build, then compare iidfile with local daemon. Saves 100% of the second
   build time.

### Medium-term: Skip redundant builds

2. **Store golden build commit SHA.** When promoting golden, record the git commit SHA
   of each repo. When a session starts with the same SHA, skip builds entirely —
   the golden cache already has the images.

### The Zed build (554s) needs special attention

3. The Zed build is 75% of the total time. `COPY . /zed` (69.78MB) hashes the entire
   source tree. Options:
   - Pin to a specific commit hash in the Dockerfile ARG so the cache key is stable
   - Use a two-stage COPY: first copy `Cargo.toml`/`Cargo.lock` (stable → cached dep
     build), then copy source (volatile → only recompiles changed crates)
   - Skip the Zed build entirely if the binary already exists at the correct version
     (check version string or git SHA)
