# Requirements: Refresh All Dockerfile Base Image SHA Digest Pins

## User Story

As a Helix maintainer, I want every `FROM ... @sha256:...` digest pin in the
repository refreshed to the current upstream multi-arch manifest, so that
container builds pick up recent security fixes for both `linux/amd64` and
`linux/arm64` without changing image versions.

## In Scope

Every Dockerfile in `helix/` that contains an `@sha256:...` pin on a `FROM`
line, including:

- `Dockerfile`
- `Dockerfile.lint`
- `Dockerfile.qwen-code-build`
- `Dockerfile.qwen-build`
- `Dockerfile.sandbox`
- `Dockerfile.demos`
- `Dockerfile.sway-helix`
- `Dockerfile.ubuntu-helix` (including the `CUDA_BASE_IMAGE` ARG default)
- `Dockerfile.zed-build`
- `operator/Dockerfile`
- `scripts/sse-mcp-server/Dockerfile`

## Out of Scope

- Bumping image **tags** (e.g. `golang:1.23` → `1.25`, `ubuntu:25.04` → `25.10`).
- Switching base image (e.g. distroless → alpine).
- Refactoring Dockerfile structure or stages.
- Editing dockerignore or build scripts.

## Acceptance Criteria

1. **Every** `@sha256:...` pin on a `FROM` line (and the `CUDA_BASE_IMAGE` ARG
   default in `Dockerfile.ubuntu-helix`) reflects the current upstream digest
   for the same `image:tag`.
2. The new digest for any image that publishes a manifest list is the
   **multi-arch manifest-list digest**, not an architecture-specific child
   digest. Verified with `docker buildx imagetools inspect <image>:<tag>` —
   the reported `Digest:` of the top-level OCI image index is what gets used.
3. When the same `image:tag` appears in multiple `FROM` stages of the same
   Dockerfile, all stages share the **same** updated digest string.
4. Image tags and version numbers are unchanged. Only the 64-hex-char string
   after `@sha256:` may change.
5. Architecture-specific images that legitimately have no arm64 manifest
   (e.g. `nvidia/cuda:12.6.3-runtime-ubuntu24.04`) keep their amd64-only
   digest and are called out in the PR description.
6. Any version-consistency anomaly noticed during the sweep
   (e.g. `Dockerfile.lint` uses `golang:1.23-alpine3.21` while the rest of
   the repo is on `golang:1.25`) is **flagged in the PR description** as an
   open question — not silently changed.
7. Each new digest is verified with `docker buildx imagetools inspect`
   before commit; the verified digest string is what lands in the file.
