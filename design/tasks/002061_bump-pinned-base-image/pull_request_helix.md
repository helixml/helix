# chore(docker): bump pinned base image SHA digests (2026-06-01)

## Summary

Refresh every `@sha256:` digest in every Dockerfile to the latest currently-published multi-arch (manifest list / OCI image index) digest for the same `image:tag`. Picks up upstream OS and runtime security fixes released since the last pin date (2026-04-13) without altering any toolchain version, tag name, or build structure.

## Changes

- **`Dockerfile`** — `golang:1.25-bookworm` and `debian:bookworm-slim` digests refreshed.
- **`Dockerfile.demos`** — `golang:1.25-alpine3.22` digest refreshed.
- **`Dockerfile.sandbox`** — `golang:1.25-bookworm` digest refreshed.
- **`Dockerfile.sway-helix`** — `golang:1.25-bookworm` digest refreshed in `FROM`, in the head-of-file documentary digest list, and the `2026-04-13` → `2026-06-01` pin-date stamps.
- **`Dockerfile.ubuntu-helix`** — `golang:1.25-bookworm` digest refreshed in `FROM`, in the head-of-file documentary digest list, and the `2026-04-13` → `2026-06-01` pin-date stamps.
- **`operator/Dockerfile`** — `golang:1.25-bookworm` and `gcr.io/distroless/static:nonroot` digests refreshed.
- 7 of 11 unique `image:tag` references (`node:23-alpine`, `ubuntu:25.04`, `golangci/golangci-lint:v1.62-alpine`, `golang:1.23-alpine3.21`, `node:20-slim`, `ubuntu:25.10`, `nvidia/cuda:12.6.3-runtime-ubuntu24.04`) already pinned the latest published digest and need no edit.

## Digests bumped

| image:tag | old digest | new digest |
|-----------|------------|------------|
| `golang:1.25-bookworm` | `sha256:e3a54b77…d409547` | `sha256:bbb255b0e131db500cf0520adc97441d2260cf629c7fa7e39e025ddf53995a24` |
| `debian:bookworm-slim` | `sha256:67b30a61…ac8493d3` | `sha256:96e378d7e6531ac9a15ad505478fcc2e69f371b10f5cdf87857c4b8188404716` |
| `golang:1.25-alpine3.22` | `sha256:26b4d711…e17cf95` | `sha256:65b4400aee0927412e9ed791a11893273a49d55df24841f7599660fb80dae464` |
| `gcr.io/distroless/static:nonroot` | `sha256:e3f94564…2a80a39` | `sha256:963fa6c544fe5ce420f1f54fb88b6fb01479f054c8056d0f74cc2c6000df5240` |

## Verification

- Each new digest was resolved via `docker buildx imagetools inspect <image>:<tag>` and confirmed to be the top-level manifest list (`application/vnd.oci.image.index.v1+json` or `application/vnd.docker.distribution.manifest.list.v2+json`) supporting both `linux/amd64` and `linux/arm64`.
- Internal consistency: every `image:tag` resolves to exactly one digest string across the repo (verified by `grep | sort -u`).
- `docker buildx build --platform linux/amd64,linux/arm64 --check -f Dockerfile .` completed with **"Check complete, no warnings found."** — the three refreshed digests in the main `Dockerfile` resolved cleanly against the registry.

## Out of scope (intentionally untouched)

- No toolchain version bumps (Go, Node, Rust, Debian, Ubuntu, CUDA, Alpine, golangci-lint).
- No package-version ARGs (`GOSU_VERSION`, `DOCKER_VERSION`, `CONTAINERD_VERSION`, `GHOSTTY_TAG`, `GOOSE_COMMIT`, MCP pkg pins).
- No `.dockerignore`, CI, or build-script changes.
- No multi-stage restructuring or tag-name edits.
