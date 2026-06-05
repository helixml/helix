# Refresh all Dockerfile base image SHA digest pins

## Summary

Refresh every `@sha256:...` pin on `FROM` lines (and the `CUDA_BASE_IMAGE`
ARG default) across all Dockerfiles in the repo, so container builds pick
up the latest upstream security fixes for both `linux/amd64` and
`linux/arm64`. Image tags / version numbers are unchanged — this is a
digest refresh only, not a version upgrade.

Each new digest is the **multi-arch manifest-list digest** (the top-level
OCI image index), verified with `docker buildx imagetools inspect`. Where
the same `image:tag` appears in multiple Dockerfiles or stages, every
occurrence shares the same updated digest.

## Inspected: 11 distinct image:tags. Updated: 4

| Image | Old digest | New digest | Files touched |
| --- | --- | --- | --- |
| `golang:1.25-bookworm` | `sha256:29e59af9...da1a6d8c` | `sha256:e3a54b77385b4f8a31c1db4d12429ffb3718ea76865731a787c497755d409547` | `Dockerfile`, `Dockerfile.sandbox`, `Dockerfile.sway-helix`, `Dockerfile.ubuntu-helix`, `operator/Dockerfile` |
| `debian:bookworm-slim` | `sha256:4724b8cc...642182655d` | `sha256:67b30a61dc87758f0caf819646104f29ecbda97d920aaf5edc834128ac8493d3` | `Dockerfile` |
| `node:20-slim` | `sha256:f93745c1...0c40313a55` | `sha256:2cf067cfed83d5ea958367df9f966191a942351a2df77d6f0193e162b5febfc0` | `Dockerfile.qwen-build`, `Dockerfile.qwen-code-build`, `scripts/sse-mcp-server/Dockerfile` |
| `golang:1.25-alpine3.22` | `sha256:2c16ac01...fad2ebfb75f884c` | `sha256:26b4d7113039cd51356bd7930ecafd1031d2975dc3b6940ec8ed09457e17cf95` | `Dockerfile.demos` |

## Already current — left unchanged

These were inspected but the upstream manifest-list digest was identical
to the existing pin: `node:23-alpine`, `golangci/golangci-lint:v1.62-alpine`,
`golang:1.23-alpine3.21`, `ubuntu:25.04`, `ubuntu:25.10`,
`gcr.io/distroless/static:nonroot`, `nvidia/cuda:12.6.3-runtime-ubuntu24.04`.

## Multi-arch verification

For every refreshed image, the new digest is the manifest-list / image-index
digest and includes both `linux/amd64` and `linux/arm64` manifests:

- `golang:1.25-bookworm` — amd64, arm64/v8 (+ 386, arm/v7, mips64le, ppc64le, s390x)
- `debian:bookworm-slim` — amd64, arm64/v8 (+ 386, arm/v5, arm/v7, mips64le, ppc64le, s390x)
- `node:20-slim` — amd64, arm64/v8 (+ arm/v7, ppc64le, s390x)
- `golang:1.25-alpine3.22` — amd64, arm64/v8 (+ 386, arm/v6, arm/v7, ppc64le, riscv64, s390x)

`nvidia/cuda:12.6.3-runtime-ubuntu24.04` — observed during this sweep to
publish **both** `linux/amd64` and `linux/arm64`. The Dockerfile keeps the
existing convention of an arm64 `--build-arg CUDA_BASE_IMAGE=ubuntu:25.10`
override (no Dockerfile structural changes in this PR). Worth a follow-up
to consider if the override is still needed.

## Reachability check

Every pin in the diff was re-inspected as `image:tag@new-digest` via
`docker buildx imagetools inspect`. All 11 returned a valid manifest.

## Open questions (please review)

These version-consistency anomalies were observed during the sweep. Per
task scope they were **not silently fixed** — flagging here for stakeholder
review:

1. **`Dockerfile.lint` is on `golang:1.23-alpine3.21`** — every other
   Go-based Dockerfile in the repo is on `golang:1.25-bookworm` (or
   `1.25-alpine3.22`). Intentional pin to an older Go for lint
   compatibility, or stale?
2. **`Dockerfile.sandbox` is on `ubuntu:25.04`** — every other
   Ubuntu-based Dockerfile is on `25.10`. Intentional, or missed in a
   prior bump?
3. **`Dockerfile.demos` is on `golang:1.25-alpine3.22`** — siblings use
   `golang:1.25-bookworm`. Intentional alpine choice (smaller demo
   image), or drift?
