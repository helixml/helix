# Refresh all base image SHA digests to latest multi-arch manifests

## Summary
Updates every `FROM` line and `ARG` default across all 14 Dockerfiles to the latest multi-arch manifest index digests, resolved on 2026-04-13 via `docker buildx imagetools inspect`. This picks up OS-level and library CVE fixes while maintaining build reproducibility and layer-cache efficiency.

## Changes
- **Group A (14 already-pinned references refreshed):** golang:1.25-alpine3.22, golang:1.23-alpine3.21, golangci/golangci-lint:v1.62-alpine, node:20-slim (3 files), ghcr.io/astral-sh/uv:0.5.4 (active + commented-out), golang:1.25-bookworm, alpine:3.21, typesense/typesense:27.1, ghcr.io/astral-sh/uv:0.10.2, uv:0.10.2-debian-slim, python:3.11-slim, gcr.io/distroless/static:nonroot
- **Group B (11 previously-unpinned references now pinned):** golang:1.25-bookworm in 4 files, node:23-alpine, debian:bookworm-slim, ubuntu:25.04, ubuntu:25.10 (8 FROM lines across 3 files), nvidia/cuda:12.6.3-runtime-ubuntu24.04 (ARG default)
- **Header comments updated** in Dockerfile.sway-helix and Dockerfile.ubuntu-helix with new digests and date 2026-04-13
- **Cross-file consistency verified:** golang:1.25-bookworm identical across 6 files, ubuntu:25.10 identical across 8 FROM lines, node:20-slim identical across 3 files
- **All 17 digests verified** against registries via `docker buildx imagetools inspect` — no typos, all multi-arch manifest list SHAs

## Notes
- nvidia/cuda:12.6.3-runtime-ubuntu24.04 confirmed multi-arch (arm64 + amd64), pinned normally
- No version bumps — only digest refreshes at existing version tags
- ghcr.io/helixml/runner-base:${TAG} intentionally left unpinned (uses --build-arg selection)
