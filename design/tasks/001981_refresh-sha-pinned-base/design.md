# Design

## Approach

Mechanical, file-by-file digest refresh. No new tooling — `docker buildx imagetools inspect` is the source of truth for multi-arch manifest digests, and `sed`/`Edit` does the rewrites.

## Resolving multi-arch digests

For each unique `image:tag` referenced across the in-scope Dockerfiles:

```bash
docker buildx imagetools inspect <image>:<tag> --format '{{.Manifest.Digest}}'
```

This returns the **manifest list** digest (the one that gets resolved per-platform at pull time). It is the only correct digest for a multi-arch repo. The platform-specific digests appear under `Manifest.Manifests[].Digest` and **must not** be used.

Verification step (run for each result):

```bash
docker buildx imagetools inspect <image>:<tag>@sha256:<digest> --raw \
  | jq -r '.mediaType, (.manifests[].platform | "\(.os)/\(.architecture)")'
```

The `mediaType` should be `application/vnd.oci.image.index.v1+json` or `application/vnd.docker.distribution.manifest.list.v2+json` (manifest list / index — confirms multi-arch). The platform list must include both `linux/amd64` and `linux/arm64`.

Exception: `nvidia/cuda:12.6.3-runtime-ubuntu24.04` is published as a single-platform (amd64) image. The arm64 build path swaps to `ubuntu:25.10` via `--build-arg CUDA_BASE_IMAGE=...` in `.drone.yml`. Pin the Dockerfile ARG default to the nvidia/cuda manifest digest as-is; do not require arm64 for this one image.

## Unique base images to resolve (10 distinct refs)

| Image:tag                                          | Used in                                                                                                              |
| -------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------- |
| `golang:1.25-bookworm`                             | `Dockerfile`, `Dockerfile.sandbox`, `Dockerfile.sway-helix`, `Dockerfile.ubuntu-helix`, `operator/Dockerfile`        |
| `golang:1.25-alpine3.22`                           | `Dockerfile.demos`                                                                                                   |
| `golang:1.23-alpine3.21`                           | `Dockerfile.lint`                                                                                                    |
| `golangci/golangci-lint:v1.62-alpine`              | `Dockerfile.lint`                                                                                                    |
| `node:23-alpine`                                   | `Dockerfile`                                                                                                         |
| `node:20-slim`                                     | `Dockerfile.qwen-build`, `Dockerfile.qwen-code-build`, `scripts/sse-mcp-server/Dockerfile`                            |
| `debian:bookworm-slim`                             | `Dockerfile`                                                                                                         |
| `ubuntu:25.04`                                     | `Dockerfile.sandbox`                                                                                                 |
| `ubuntu:25.10`                                     | `Dockerfile.sway-helix` (×5 stages), `Dockerfile.ubuntu-helix`, `Dockerfile.zed-build`                                |
| `gcr.io/distroless/static:nonroot`                 | `operator/Dockerfile`                                                                                                |
| `nvidia/cuda:12.6.3-runtime-ubuntu24.04` (amd64-only) | `Dockerfile.ubuntu-helix` (ARG default)                                                                          |

Resolve each once; reuse the digest everywhere the image appears.

## Rewrite rules

- `FROM golang:1.25-bookworm AS api-base` → `FROM golang:1.25-bookworm@sha256:<digest> AS api-base`
- `FROM ubuntu:25.10` → `FROM ubuntu:25.10@sha256:<digest>` (preserve any `AS <stage>` suffix)
- `FROM api-base AS embedding-model` → unchanged (internal stage reference)
- `FROM scratch` → unchanged
- `FROM ${CUDA_BASE_IMAGE} AS cuda-libs` → unchanged (the ARG carries the digest)
- `ARG CUDA_BASE_IMAGE=nvidia/cuda:12.6.3-runtime-ubuntu24.04` → `ARG CUDA_BASE_IMAGE=nvidia/cuda:12.6.3-runtime-ubuntu24.04@sha256:<digest>`

## Header comment refresh

`Dockerfile.sway-helix` (lines 9–12) and `Dockerfile.ubuntu-helix` (lines 10–13) carry a documentation block listing the active digests and a "(2026-03-30)" date. Update both:
- date → `2026-05-04`
- digest values → newly resolved digests

`Dockerfile.sway-helix` line 37 and `Dockerfile.ubuntu-helix` line 25 each have a "(golang:1.25-bookworm as of 2026-03-30)" inline comment — update the date.

## Verification before commit

1. `grep -nE '@sha256:[^a-f0-9]|@sha256:[a-f0-9]{0,63}\b|@sha256:[a-f0-9]{65,}\b' Dockerfile*` returns no matches (catches malformed digests).
2. `grep -nE '^FROM ' Dockerfile* operator/Dockerfile scripts/sse-mcp-server/Dockerfile` — every line either references a registry image with `@sha256:`, references a previous stage by name, references `scratch`, or references `${CUDA_BASE_IMAGE}`.
3. For each Dockerfile with multi-stage reuse of one image (notably `Dockerfile.sway-helix` with 5×`ubuntu:25.10` and `Dockerfile.ubuntu-helix` with 2× internal `golang:1.25-bookworm`/`ubuntu:25.10`), confirm the digest is identical across stages with `awk '/^FROM/ {print}' <file> | sort -u`.
4. For each unique image, the documented header digest equals the digest applied in the active `FROM` line.
5. `docker buildx build --platform linux/amd64,linux/arm64 -f <Dockerfile> .` succeeds for at least the leaf desktop images (`Dockerfile.sway-helix`, `Dockerfile.ubuntu-helix` both arch paths via the existing build-arg trick) and `operator/Dockerfile`. The other Dockerfiles can be smoke-tested per their drone steps.

## Cross-file consistency note (out of scope, flag only)

`.drone.yml` lines ~1890 and ~2182 hardcode `--build-arg CUDA_BASE_IMAGE=...@sha256:...` for amd64 (`nvidia/cuda`) and arm64 (`ubuntu:25.10`). The arm64 `ubuntu:25.10` digest there must match the new `ubuntu:25.10` digest applied in the Dockerfiles, otherwise cache layers diverge. The implementer should update those two `.drone.yml` strings in the same PR even though `.drone.yml` is technically outside the "Dockerfile" scope — leaving them stale defeats the purpose of the refresh.

## Why no automation script

This is a one-off, ~25-line edit across 11 files. A script (Python/bash wrapper around `imagetools inspect`) would take longer to write, test, and review than just doing the edits. If this becomes recurring (quarterly), revisit by adding a small helper to `scripts/`. For now: manual + careful.
