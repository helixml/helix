# Shared BuildKit Cache Across Spectask Sessions

**Date**: 2026-02-21
**PR**: #1705 (core fix), follow-up commit (transparent user wrapper)

## Problem

Docker build cache was not shared between spectask sessions. Each desktop container ran its own local BuildKit instance, so the cache was lost when the container was destroyed. Building helix-in-helix (which compiles Rust/Zed from source) took ~43 minutes every time.

## Root Cause

The sandbox already ran a shared BuildKit container (`helix-buildkit`) with a persistent volume, and desktop containers configured a `helix-shared` remote buildx builder pointing to it. However, **Docker 29.x's `docker build` ignores the default buildx builder** and uses the local daemon's built-in BuildKit. Only `docker buildx build` or the `BUILDX_BUILDER` env var forces the remote builder.

## Fix

Two-part solution:

1. **`BUILDX_BUILDER=helix-shared` set globally** in `/etc/environment`, `/etc/profile.d/`, and `~/.bashrc` so all `docker build` commands route through the shared BuildKit. (`17-start-dockerd.sh`, `stack`)

2. **Transparent `docker` wrapper** at `/usr/local/bin/docker` that adds `--load` when a remote builder is active and `-t` is used. Without this, `docker build -t foo .` builds remotely but doesn't load the image into the local daemon. (`docker-buildx-wrapper.sh`)

## Results

| Phase | Cold Cache | Hot Cache | Speedup |
|-------|-----------|-----------|---------|
| `./stack build` | ~3 min | ~15 sec | ~12x |
| `./stack build-zed release` | ~11 min | ~45 sec | ~15x |
| `./stack build-sandbox` | ~29 min | ~20 min* | ~1.5x |
| **Total startup** | **~43 min** | **~21 min** | **~2x** |

*`build-sandbox` is mostly limited by Docker image layer push/pull, not compilation.

## Files Changed

- `stack` — `docker_build_load()` helper replaces raw `docker build` calls
- `desktop/shared/17-start-dockerd.sh` — sets `BUILDX_BUILDER` globally, installs wrapper
- `desktop/shared/docker-buildx-wrapper.sh` — transparent `--load` injection for users
- `Dockerfile.ubuntu-helix`, `Dockerfile.sway-helix` — bundle the wrapper script
