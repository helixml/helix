# Requirements: Bump All Base Image SHA Digests

## User Stories

**As a platform engineer**, I want every external base image in our Dockerfiles pinned to the latest multi-arch manifest digest so that builds pick up OS-level and library CVE fixes while remaining reproducible and cache-efficient.

**As a CI operator**, I want all `FROM` lines referencing the same image tag to carry identical digests across files so that builds are deterministic and consistent across all pipelines.

## Acceptance Criteria

1. Every `FROM` line and every `ARG` default referencing an external base image carries a valid `@sha256:…` digest.
2. All digests are multi-arch manifest list SHAs (the index-level SHA returned by `docker buildx imagetools inspect`), not single-platform layer SHAs.
3. The same image tag used across multiple files carries an identical digest in all of them (see cross-file consistency rules below).
4. Within each multi-stage Dockerfile, every `FROM` for the same image uses the same digest.
5. Header comments in `Dockerfile.sway-helix` and `Dockerfile.ubuntu-helix` are updated with new digests and the date `2026-04-13`.
6. `docker buildx build --platform linux/amd64,linux/arm64` succeeds for every Dockerfile (verified by CI).

## Cross-File Consistency Rules

| Image | Files | Rule |
|-------|-------|------|
| `golang:1.25-bookworm` | `Dockerfile`, `Dockerfile.runner`, `Dockerfile.sandbox`, `Dockerfile.sway-helix`, `Dockerfile.ubuntu-helix`, `operator/Dockerfile` | All 6+ occurrences must use the **identical** digest |
| `ubuntu:25.10` | `Dockerfile.sway-helix` (5 FROM lines), `Dockerfile.ubuntu-helix` (2 FROM lines), `Dockerfile.zed-build` (1 FROM line) | All occurrences must use the **identical** digest |
| `node:20-slim` | `Dockerfile.qwen-build`, `Dockerfile.qwen-code-build`, `scripts/sse-mcp-server/Dockerfile` | All 3 must use the **identical** digest |

## Open Questions for Implementation Agent

1. **`uv` version mismatch:** `Dockerfile.runner` uses `uv:0.5.4` while `haystack_service/Dockerfile` uses `uv:0.10.2`. This task refreshes each at its current version tag — do NOT align versions unless explicitly told to.
2. **`nvidia/cuda` multi-arch availability:** `nvidia/cuda:12.6.3-runtime-ubuntu24.04` may be amd64-only. If so, the SHA pin on the `ARG` default is single-arch by design — document this in a comment. The arm64 build path passes `--build-arg CUDA_BASE_IMAGE=ubuntu:25.10` and is unaffected.
3. **`ubuntu:25.04` vs `ubuntu:25.10`:** `Dockerfile.sandbox` uses `ubuntu:25.04` while desktop Dockerfiles use `ubuntu:25.10`. Treat this as intentional — pin `25.04` at its own digest.
