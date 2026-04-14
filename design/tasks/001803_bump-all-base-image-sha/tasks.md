# Implementation Tasks

## Phase 1: Resolve All Multi-Arch Manifest Digests

- [~] Run `docker buildx imagetools inspect` for each of the 17 unique image tags listed in design.md and record the multi-arch manifest digest for each
- [ ] For `nvidia/cuda:12.6.3-runtime-ubuntu24.04`: confirm whether it publishes a multi-arch manifest list (amd64 + arm64) or is amd64-only; if amd64-only, document with a comment on the ARG line

## Phase 2: Update Group A — Already Pinned, Refresh Digest

- [ ] `Dockerfile.demos:1` — update `golang:1.25-alpine3.22` digest
- [ ] `Dockerfile.lint:3` — update `golangci/golangci-lint:v1.62-alpine` digest
- [ ] `Dockerfile.lint:5` — update `golang:1.23-alpine3.21` digest
- [ ] `Dockerfile.qwen-build:12`, `Dockerfile.qwen-code-build:12`, `scripts/sse-mcp-server/Dockerfile:1` — update `node:20-slim` digest (all three must match)
- [ ] `Dockerfile.runner:22` — update `ghcr.io/astral-sh/uv:0.5.4` active FROM digest
- [ ] `Dockerfile.runner:10` — update `ghcr.io/astral-sh/uv:0.5.4` digest in commented-out FROM
- [ ] `Dockerfile.runner:28` — update `golang:1.25-bookworm` digest
- [ ] `Dockerfile.runner:61` — update `alpine:3.21` digest
- [ ] `Dockerfile.typesense:1` — update `typesense/typesense:27.1` digest
- [ ] `haystack_service/Dockerfile:1` — update `ghcr.io/astral-sh/uv:0.10.2` digest
- [ ] `haystack_service/Dockerfile:2` — update `ghcr.io/astral-sh/uv:0.10.2-debian-slim` digest
- [ ] `haystack_service/Dockerfile:30` — update `python:3.11-slim` digest
- [ ] `operator/Dockerfile:2` — update `golang:1.25-bookworm` digest (must match other files)
- [ ] `operator/Dockerfile:28` — update `gcr.io/distroless/static:nonroot` digest

## Phase 3: Update Group B — Not Currently Pinned, Add SHA Digest

- [ ] `Dockerfile:6` — add digest to `golang:1.25-bookworm` (must match Dockerfile.runner, operator, etc.)
- [ ] `Dockerfile:88` — add digest to `node:23-alpine`
- [ ] `Dockerfile:118` — add digest to `debian:bookworm-slim`
- [ ] `Dockerfile.sandbox:14` — add digest to `golang:1.25-bookworm` (must match)
- [ ] `Dockerfile.sandbox:34` — add digest to `ubuntu:25.04`
- [ ] `Dockerfile.sway-helix:38` — add digest to `golang:1.25-bookworm` (must match)
- [ ] `Dockerfile.sway-helix:19,76,138,193,265` — add the same `ubuntu:25.10` digest to all 5 FROM lines
- [ ] `Dockerfile.ubuntu-helix:27` — add digest to `golang:1.25-bookworm` (must match)
- [ ] `Dockerfile.ubuntu-helix:77,184` — add the same `ubuntu:25.10` digest to both FROM lines (must match sway-helix and zed-build)
- [ ] `Dockerfile.ubuntu-helix:18` — add digest to `nvidia/cuda:12.6.3-runtime-ubuntu24.04` ARG default
- [ ] `Dockerfile.zed-build:15` — add digest to `ubuntu:25.10` (must match sway-helix and ubuntu-helix)

## Phase 4: Update Header Comments

- [ ] `Dockerfile.sway-helix` lines 9-12 — update date to `2026-04-13` and both digest values (`ubuntu:25.10` and `golang:1.25-bookworm`)
- [ ] `Dockerfile.ubuntu-helix` lines 10-13 — update date to `2026-04-13` and both digest values (`ubuntu:25.10` and `golang:1.25-bookworm`)
- [ ] Scan all Dockerfiles for any other inline comments referencing specific digest values and update them

## Phase 5: Cross-File Consistency Verification

- [ ] Verify `golang:1.25-bookworm` digest is identical across all 6 files: `Dockerfile`, `Dockerfile.runner`, `Dockerfile.sandbox`, `Dockerfile.sway-helix`, `Dockerfile.ubuntu-helix`, `operator/Dockerfile`
- [ ] Verify `ubuntu:25.10` digest is identical across all 8 FROM lines in `Dockerfile.sway-helix`, `Dockerfile.ubuntu-helix`, `Dockerfile.zed-build`
- [ ] Verify `node:20-slim` digest is identical across `Dockerfile.qwen-build`, `Dockerfile.qwen-code-build`, `scripts/sse-mcp-server/Dockerfile`
- [ ] Verify within each multi-stage Dockerfile that every FROM for the same image uses the same digest

## Phase 6: Verification

- [ ] Run `grep -rn '@sha256:' Dockerfile*` and `grep -rn '@sha256:' haystack_service/Dockerfile operator/Dockerfile scripts/sse-mcp-server/Dockerfile` to confirm every FROM line is pinned
- [ ] Run `grep -rn '^FROM\|^ARG.*=.*/' Dockerfile* haystack_service/Dockerfile operator/Dockerfile scripts/sse-mcp-server/Dockerfile` and verify no external base image reference lacks a digest (except `FROM scratch` and `FROM ... AS` referencing earlier stages)
- [ ] Verify each new digest with `docker buildx imagetools inspect <image:tag@sha256:digest>` to catch typos
