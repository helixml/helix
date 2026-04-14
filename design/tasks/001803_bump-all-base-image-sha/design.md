# Design: Bump All Base Image SHA Digests

## Approach

This is a mechanical digest-refresh task — no architectural changes, no version bumps. For each unique base image tag, resolve the latest multi-arch manifest digest using `docker buildx imagetools inspect <image:tag>`, then update every `FROM` line and `ARG` default that references it.

## Digest Resolution Method

Use `docker buildx imagetools inspect <image:tag>` to get the **manifest list** (index) digest. This is the multi-arch SHA that Docker uses to select the correct platform-specific image at build time.

```bash
# Example: get the multi-arch manifest digest
docker buildx imagetools inspect golang:1.25-bookworm 2>&1 | grep "^Digest:"
# Output: Digest: sha256:<64-char-hex>
```

**Critical:** Do NOT use `docker pull` + `docker inspect` — that returns a single-platform layer digest which will break builds on the other architecture.

**Alternative if buildx is not available:** `docker manifest inspect <image:tag> | jq -r '.digest'` or check Docker Hub / GHCR web UI for the manifest list digest.

## Unique Images to Resolve (14 total)

| # | Image Tag | Current State | Files Affected |
|---|-----------|---------------|----------------|
| 1 | `golang:1.25-bookworm` | Pinned in 2 files, unpinned in 4 | `Dockerfile:6`, `Dockerfile.runner:28`, `Dockerfile.sandbox:14`, `Dockerfile.sway-helix:38`, `Dockerfile.ubuntu-helix:27`, `operator/Dockerfile:2` |
| 2 | `golang:1.25-alpine3.22` | Pinned | `Dockerfile.demos:1` |
| 3 | `golang:1.23-alpine3.21` | Pinned | `Dockerfile.lint:5` |
| 4 | `golangci/golangci-lint:v1.62-alpine` | Pinned | `Dockerfile.lint:3` |
| 5 | `node:20-slim` | Pinned, same digest in all 3 | `Dockerfile.qwen-build:12`, `Dockerfile.qwen-code-build:12`, `scripts/sse-mcp-server/Dockerfile:1` |
| 6 | `node:23-alpine` | Not pinned | `Dockerfile:88` |
| 7 | `debian:bookworm-slim` | Not pinned | `Dockerfile:118` |
| 8 | `ubuntu:25.04` | Not pinned | `Dockerfile.sandbox:34` |
| 9 | `ubuntu:25.10` | Not pinned (8 FROM lines total) | `Dockerfile.sway-helix:19,76,138,193,265`, `Dockerfile.ubuntu-helix:77,184`, `Dockerfile.zed-build:15` |
| 10 | `ghcr.io/astral-sh/uv:0.5.4` | Pinned (active + commented-out) | `Dockerfile.runner:10,22` |
| 11 | `alpine:3.21` | Pinned | `Dockerfile.runner:61` |
| 12 | `typesense/typesense:27.1` | Pinned | `Dockerfile.typesense:1` |
| 13 | `ghcr.io/astral-sh/uv:0.10.2` | Pinned | `haystack_service/Dockerfile:1` |
| 14 | `ghcr.io/astral-sh/uv:0.10.2-debian-slim` | Pinned | `haystack_service/Dockerfile:2` |
| 15 | `python:3.11-slim` | Pinned | `haystack_service/Dockerfile:30` |
| 16 | `gcr.io/distroless/static:nonroot` | Pinned | `operator/Dockerfile:28` |
| 17 | `nvidia/cuda:12.6.3-runtime-ubuntu24.04` | Not pinned (ARG default) | `Dockerfile.ubuntu-helix:18` |

## Key Decisions

1. **Multi-arch only:** All digests must be manifest list / index SHAs. The `docker buildx imagetools inspect` command returns this by default. Single-platform digests (from `docker pull`) will break cross-architecture builds.

2. **nvidia/cuda exception:** If `nvidia/cuda:12.6.3-runtime-ubuntu24.04` does not publish a multi-arch manifest (amd64-only), pin the single-arch digest on the `ARG` default and add a comment explaining why. The arm64 path overrides this ARG with `ubuntu:25.10`.

3. **Commented-out code in Dockerfile.runner:** Line 10 has a commented-out `FROM` for `ghcr.io/astral-sh/uv:0.5.4` with the old digest. Update the digest in the comment so it stays accurate if someone uncomments it.

4. **Header comment blocks:** `Dockerfile.sway-helix` (lines 9-12) and `Dockerfile.ubuntu-helix` (lines 10-13) have header blocks listing digests and a date. Update both the digest values and the date to `2026-04-13`.

5. **FROM syntax:** Use `image:tag@sha256:...` format — keep the tag for readability, append the digest for pinning.

## Codebase Patterns Discovered

- **Header comment convention:** `Dockerfile.sway-helix` and `Dockerfile.ubuntu-helix` both use a header block pattern:
  ```
  # BASE IMAGE DIGESTS: Pinned for stable layer caching (YYYY-MM-DD).
  # Update digests when intentionally upgrading base images.
  # - image:tag -> sha256:...
  ```
- **Inline comments:** Several Dockerfiles have `# Pin to specific digest for stable layer caching.` comments above unpinned FROM lines — the intent was always to pin, but the digest was never applied.
- **ARG-based image selection:** `Dockerfile.ubuntu-helix` uses `ARG CUDA_BASE_IMAGE=nvidia/cuda:12.6.3-runtime-ubuntu24.04` as an ARG default, then `FROM ${CUDA_BASE_IMAGE} AS cuda-libs`. The digest goes on the ARG default value.

## Risk: Typos in SHA-256 Digests

A single-character typo in a SHA-256 digest produces an invalid image reference that silently fails at build time. After updating all digests, the implementation agent should double-check each digest string character-by-character or verify by running `docker buildx imagetools inspect <image:tag@sha256:...>` for each new digest.
