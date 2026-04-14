# Implementation Tasks

## Phase 1: Resolve All Multi-Arch Manifest Digests

- [x] Run `docker buildx imagetools inspect` for each of the 17 unique image tags listed in design.md and record the multi-arch manifest digest for each
- [x] For `nvidia/cuda:12.6.3-runtime-ubuntu24.04`: confirm whether it publishes a multi-arch manifest list (amd64 + arm64) or is amd64-only; if amd64-only, document with a comment on the ARG line
  - Result: IS multi-arch (both arm64 and amd64), gets normal digest pin

## Phase 2: Update Group A — Already Pinned, Refresh Digest

- [x] `Dockerfile.demos:1` — update `golang:1.25-alpine3.22` digest
- [x] `Dockerfile.lint:3` — update `golangci/golangci-lint:v1.62-alpine` digest
- [x] `Dockerfile.lint:5` — update `golang:1.23-alpine3.21` digest
- [x] `Dockerfile.qwen-build:12`, `Dockerfile.qwen-code-build:12`, `scripts/sse-mcp-server/Dockerfile:1` — update `node:20-slim` digest (all three must match)
- [x] `Dockerfile.runner:22` — update `ghcr.io/astral-sh/uv:0.5.4` active FROM digest
- [x] `Dockerfile.runner:10` — update `ghcr.io/astral-sh/uv:0.5.4` digest in commented-out FROM
- [x] `Dockerfile.runner:28` — update `golang:1.25-bookworm` digest
- [x] `Dockerfile.runner:61` — update `alpine:3.21` digest
- [x] `Dockerfile.typesense:1` — update `typesense/typesense:27.1` digest
- [x] `haystack_service/Dockerfile:1` — update `ghcr.io/astral-sh/uv:0.10.2` digest
- [x] `haystack_service/Dockerfile:2` — update `ghcr.io/astral-sh/uv:0.10.2-debian-slim` digest
- [x] `haystack_service/Dockerfile:30` — update `python:3.11-slim` digest
- [x] `operator/Dockerfile:2` — update `golang:1.25-bookworm` digest (must match other files)
- [x] `operator/Dockerfile:28` — update `gcr.io/distroless/static:nonroot` digest

## Phase 3: Update Group B — Not Currently Pinned, Add SHA Digest

- [x] `Dockerfile:6` — add digest to `golang:1.25-bookworm` (must match Dockerfile.runner, operator, etc.)
- [x] `Dockerfile:88` — add digest to `node:23-alpine`
- [x] `Dockerfile:118` — add digest to `debian:bookworm-slim`
- [x] `Dockerfile.sandbox:14` — add digest to `golang:1.25-bookworm` (must match)
- [x] `Dockerfile.sandbox:34` — add digest to `ubuntu:25.04`
- [x] `Dockerfile.sway-helix:38` — add digest to `golang:1.25-bookworm` (must match)
- [x] `Dockerfile.sway-helix:19,76,138,193,265` — add the same `ubuntu:25.10` digest to all 5 FROM lines
- [x] `Dockerfile.ubuntu-helix:27` — add digest to `golang:1.25-bookworm` (must match)
- [x] `Dockerfile.ubuntu-helix:77,184` — add the same `ubuntu:25.10` digest to both FROM lines (must match sway-helix and zed-build)
- [x] `Dockerfile.ubuntu-helix:18` — add digest to `nvidia/cuda:12.6.3-runtime-ubuntu24.04` ARG default
- [x] `Dockerfile.zed-build:15` — add digest to `ubuntu:25.10` (must match sway-helix and ubuntu-helix)

## Phase 4: Update Header Comments

- [x] `Dockerfile.sway-helix` lines 9-12 — update date to `2026-04-13` and both digest values (`ubuntu:25.10` and `golang:1.25-bookworm`)
- [x] `Dockerfile.ubuntu-helix` lines 10-13 — update date to `2026-04-13` and both digest values (`ubuntu:25.10` and `golang:1.25-bookworm`)
- [x] Scan all Dockerfiles for any other inline comments referencing specific digest values and update them
  - Found and updated 2 additional inline comments with date `2026-03-30` in sway-helix and ubuntu-helix

## Phase 5: Cross-File Consistency Verification

- [x] Verify `golang:1.25-bookworm` digest is identical across all 6 files: `Dockerfile`, `Dockerfile.runner`, `Dockerfile.sandbox`, `Dockerfile.sway-helix`, `Dockerfile.ubuntu-helix`, `operator/Dockerfile`
- [x] Verify `ubuntu:25.10` digest is identical across all 8 FROM lines in `Dockerfile.sway-helix`, `Dockerfile.ubuntu-helix`, `Dockerfile.zed-build`
- [x] Verify `node:20-slim` digest is identical across `Dockerfile.qwen-build`, `Dockerfile.qwen-code-build`, `scripts/sse-mcp-server/Dockerfile`
- [x] Verify within each multi-stage Dockerfile that every FROM for the same image uses the same digest

## Phase 6: Verification

- [x] Run `grep -rn '@sha256:' Dockerfile*` and `grep -rn '@sha256:' haystack_service/Dockerfile operator/Dockerfile scripts/sse-mcp-server/Dockerfile` to confirm every FROM line is pinned
- [x] Run `grep -rn '^FROM\|^ARG.*=.*/' Dockerfile* haystack_service/Dockerfile operator/Dockerfile scripts/sse-mcp-server/Dockerfile` and verify no external base image reference lacks a digest (except `FROM scratch` and `FROM ... AS` referencing earlier stages)
  - All unpinned FROM lines are internal stage references (api-base, ui-base, etc.), the runner-base ARG image (intentionally unpinned), or ${CUDA_BASE_IMAGE} which uses the pinned ARG default
- [x] Verify each new digest with `docker buildx imagetools inspect <image:tag@sha256:digest>` to catch typos
