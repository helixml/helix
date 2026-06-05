# Requirements: Refresh All Pinned Base Image SHA Digests in Dockerfiles

## Background

The helix repo pins every base image by `@sha256:…` for build reproducibility and layer-cache stability. Pinning is good for cache hit rates and supply-chain integrity, but stale digests accumulate unpatched CVEs (OS packages, language runtimes, system libraries). This task refreshes every pinned digest to its current upstream value without changing any image tag, version, or application behaviour.

## User Stories

### Story 1: Security-current base images
**As a** platform engineer responsible for the helix container images
**I want** every pinned base image digest refreshed to its latest upstream multi-arch manifest
**So that** images ship with the most recent OS / runtime / library security patches available for both `linux/amd64` and `linux/arm64`.

### Story 2: Cross-architecture safety
**As a** release engineer
**I want** every refreshed digest to be the digest of a multi-arch index manifest (not a platform-specific layer)
**So that** `docker buildx build --platform linux/amd64,linux/arm64` continues to succeed silently — a single-arch pin would crash one architecture's build without an obvious error.

### Story 3: Consistent digests across files
**As a** maintainer
**I want** every occurrence of the same `image:tag` across all Dockerfiles to resolve to the same SHA
**So that** layer caches are shared between builds and there is no divergence between, say, two builders both based on `golang:1.25-bookworm`.

### Story 4: Truthful pinning comments
**As a** future reader of these Dockerfiles
**I want** the `Pinned for stable layer caching (YYYY-MM-DD)` comments updated to today's date (2026-05-25)
**So that** the comment matches when the pin was last refreshed.

## Acceptance Criteria

### AC1: All pinned `FROM` lines refreshed
- Every `FROM <image>:<tag>@sha256:<digest>` line in every Dockerfile in `/home/retro/work/helix/` has the `<digest>` portion replaced with the current upstream digest for that `<image>:<tag>`.
- No tag or version is changed (e.g. `golang:1.25-bookworm` stays `golang:1.25-bookworm`; `node:20-slim` stays `node:20-slim`).
- No application code, `ARG`, `ENV`, `RUN`, `COPY`, `CMD`, `ENTRYPOINT`, or build argument is modified.

### AC2: Multi-arch digests only
- Each new digest is the digest of the multi-architecture **index manifest** (top-level manifest list), not a platform-specific child manifest.
- Verification command (per image):
  ```
  docker buildx imagetools inspect <image>:<tag>@sha256:<new-digest>
  ```
  must list both `linux/amd64` and `linux/arm64` platforms.
- If an image does not publish a multi-arch index (NVIDIA CUDA is the prime suspect — see Open Question 1), the exception is documented inline in the Dockerfile and called out in the PR description.

### AC3: Identical digests across files for the same image:tag
- Where the same `image:tag` appears in multiple Dockerfiles (e.g. `golang:1.25-bookworm` is used in 5 Dockerfiles, `ubuntu:25.10` in 8 locations, `node:20-slim` in 3 files), all occurrences carry the **identical** new digest.
- A simple `grep` of any updated digest should show a consistent SHA wherever the image:tag is referenced.

### AC4: No typos in updated lines
- Every new SHA is a full 64-character lowercase hex string.
- Image references (registry/path/name/tag) are syntactically valid and unchanged.
- The `@sha256:` separator and surrounding whitespace are preserved exactly.

### AC5: Pin-date comments refreshed
- Inline comments referring to the pin date are updated to `2026-05-25`:
  - `Dockerfile.ubuntu-helix`: comments on lines 10 and 25 (currently `2026-04-13`).
  - `Dockerfile.sway-helix`: comments on lines 9 and 37 (currently `2026-04-13`).
- The standalone `# - ubuntu:25.10 -> sha256:…` and `# - golang:1.25-bookworm -> sha256:…` summary lines in the header comments of both files are updated to the new digests as well.
- No other comments are touched.

### AC6: Multi-arch build succeeds
- After the changes, a sanity build with `docker buildx build --platform linux/amd64,linux/arm64 …` succeeds for the primary `Dockerfile` (the helix main image). Other Dockerfiles in the repo follow the same patterns and inherit confidence from that build.

### AC7: Digests come from live registries
- Every new SHA is obtained from a live `docker buildx imagetools inspect …` (or equivalent `docker manifest inspect`) call against the upstream registry. No fabricated, guessed, or copy-pasted-from-elsewhere digests.

## Out of Scope

- Upgrading image tags / versions (e.g. `golang:1.23` → `golang:1.25`, `node:20` → `node:23`, `ubuntu:25.04` → `ubuntu:25.10`). Tag bumps are a separate, riskier change.
- Adding or removing `FROM` lines.
- Refactoring multi-stage builds.
- Touching `.dockerignore` files (`Dockerfile.ubuntu-helix.dockerignore`, `Dockerfile.sway-helix.dockerignore`).
- The `FROM scratch` lines in `Dockerfile.qwen-code-build`, `Dockerfile.qwen-build`, and `Dockerfile.zed-build` — `scratch` is unpinnable.
- The `FROM api-base AS …`, `FROM ui-base AS …`, `FROM builder-env AS builder` lines — these refer to local stages, not external images.

## Open Questions

1. **NVIDIA CUDA multi-arch:** `nvidia/cuda:12.6.3-runtime-ubuntu24.04` is referenced from `Dockerfile.ubuntu-helix` via `ARG CUDA_BASE_IMAGE`. The header comment already explains the build splits: amd64 uses the CUDA image, arm64 overrides to `ubuntu:25.10` via `--build-arg`. Verify whether the CUDA tag publishes a multi-arch index or is amd64-only; if amd64-only, the existing comment block already documents the split and only needs a note clarifying the single-arch nature of the pin.
2. **Other single-arch outliers:** Flag any other image whose `imagetools inspect` does not list both `linux/amd64` and `linux/arm64`. None are expected from the current list (Docker official images, gcr.io/distroless, golangci-lint all publish multi-arch indexes), but verify and flag if found.
