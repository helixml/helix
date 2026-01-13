# CI/CD Pipeline for Sandbox Builds

**Date:** 2026-01-12
**Status:** Proposed
**Author:** Claude (with Phil)

## Overview

This document describes the automated Drone CI pipeline for building Helix sandbox images. The pipeline replaces the manual process of SSH-ing into node01 and running `./stack build-sandbox`.

## Previous Process (Manual)

1. Create git release/tag on GitHub
2. SSH into node01 (specific build machine)
3. Ensure Wolf and Moonlight Web repos are checked out at correct versions
4. Run `./stack build-sandbox`
5. Images automatically pushed to registry

**Problems:**
- Manual SSH intervention required
- Depends on external repos (Wolf, Moonlight Web) being checked out locally
- Build machine state can drift
- No visibility into build progress
- Single point of failure (node01)

## New Architecture (Luke's Branch)

Luke's `feature/sway-ubuntu-25.10` branch simplifies the architecture significantly:

| Component | Old | New |
|-----------|-----|-----|
| Streaming | Wolf + Moonlight Web | Native WebSocket |
| Container isolation | Wolf | Hydra |
| External repos | 2 (Wolf, Moonlight Web) | 0 |
| Build steps | 3 | 1 |
| Base image | GOW gstreamer | Ubuntu 25.04 |

This simplification makes automated CI/CD feasible.

## Pipeline Architecture

```
┌─────────────────────────────────────────────────────────────────────────┐
│                        Drone CI Pipeline                                │
│                        (build-sandbox)                                  │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  ┌──────────────────┐                                                   │
│  │clone-dependencies│ Clone zed + qwen-code repos                       │
│  └────────┬─────────┘                                                   │
│           │                                                             │
│           ├──────────────────────┐                                      │
│           │                      │                                      │
│           ▼                      ▼                                      │
│  ┌──────────────┐      ┌─────────────────┐                              │
│  │build-qwen-   │      │   build-zed     │  Parallel builds             │
│  │code (npm)    │      │   (cargo)       │                              │
│  └──────┬───────┘      └────────┬────────┘                              │
│         │                       │                                       │
│         └───────────┬───────────┘                                       │
│                     │                                                   │
│                     ▼                                                   │
│  ┌─────────────────────────────────────┐                                │
│  │         build-desktops              │                                │
│  │  helix-sway.tar + helix-ubuntu.tar  │                                │
│  └────────────────┬────────────────────┘                                │
│                   │                                                     │
│                   ▼                                                     │
│  ┌─────────────────────────────────────┐                                │
│  │          build-sandbox              │                                │
│  │  Hydra + embedded desktop tarballs  │                                │
│  └────────────────┬────────────────────┘                                │
│                   │                                                     │
│                   ▼                                                     │
│  ┌─────────────────────────────────────┐                                │
│  │          push-sandbox               │  Push to registry              │
│  │                                     │  (on tag or main branch)       │
│  │  registry.helixml.tech/helix/       │                                │
│  │    helix-sandbox:v1.2.3             │                                │
│  │    helix-sandbox:abc123f            │                                │
│  │    helix-sandbox:latest             │                                │
│  └─────────────────────────────────────┘                                │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

## Versioning Strategy

| Trigger | Version Tag | Push to Registry |
|---------|-------------|------------------|
| Push to main | `${commit_hash}` (e.g., `abc123f`) | Yes |
| Push to feature branch | `${commit_hash}` | Yes |
| Tag `v*` | `${git_tag}` (e.g., `v1.2.3`) | Yes + `latest` |

**Release images:**
```
registry.helixml.tech/helix/helix-sandbox:v1.2.3   # git tag
registry.helixml.tech/helix/helix-sandbox:abc123f  # commit hash
registry.helixml.tech/helix/helix-sandbox:latest   # only on release tag
```

**Development images:**
```
registry.helixml.tech/helix/helix-sandbox:abc123f  # commit hash
```

## GPU Requirements

**Build phase:** No GPU required
- Docker builds run on standard Drone runners
- All compilation is CPU-based
- Desktop images are ~3GB each but don't require GPU to build

**Runtime phase:** GPU required
- Video encoding (nvh264enc, vaapih264enc) happens at runtime
- Only needed when sandbox is actually serving streams
- Supports: NVIDIA, AMD (VAAPI), Intel (VAAPI)

**Testing:** No GPU encoding tests in CI currently
- Unit tests don't require GPU
- Integration tests don't require GPU
- Future: Could add GPU smoke tests on dedicated runner

## External Dependencies

| Dependency | Source | Branch | Versioning |
|------------|--------|--------|------------|
| Zed | helixml/zed | feature/external-thread-sync | Commit hash |
| qwen-code | helixml/qwen-code | main | Commit hash |

Both repos are public and cloned without authentication.

## Caching Strategy

Drone uses host volume mounts for caching:

| Cache | Host Path | Purpose |
|-------|-----------|---------|
| External repos | `/var/cache/drone/sandbox-build` | Zed + qwen-code clones |
| Cargo registry | `/var/cache/drone/cargo` | Rust dependencies |
| npm cache | `/var/cache/drone/npm` | Node.js dependencies |
| Zed binary | `/var/cache/drone/sandbox-build/cache/zed-{commit}` | Pre-built Zed binaries |

The Zed binary is cached by commit hash - if the same Zed commit is used, the cached binary is reused instead of rebuilding (saves 15-20 minutes).

## Build Times (Estimated)

| Step | Cold (no cache) | Warm (cached) |
|------|-----------------|---------------|
| clone-dependencies | 1-2 min | 30 sec |
| build-qwen-code | 2-3 min | 1-2 min |
| build-zed | 15-20 min | < 1 min (cached binary) |
| build-desktops | 15-20 min | 5-10 min |
| build-sandbox | 5-10 min | 2-5 min |
| push-sandbox | 2-5 min | 2-5 min |
| **Total** | **40-60 min** | **15-25 min** |

## Triggers

The `build-sandbox` pipeline triggers on:
- Push to `main` branch
- Push to `feature/sway-ubuntu-25.10` branch
- Tag events (any tag)

## Secrets Required

| Secret | Purpose |
|--------|---------|
| `helix_registry_password` | Push to registry.helixml.tech |

Note: No token needed for cloning zed/qwen-code - both repos are public.

## Pipeline Steps Detail

### 1. clone-dependencies
- Uses `alpine/git:latest`
- Clones `helixml/zed` (branch: `feature/external-thread-sync`)
- Clones `helixml/qwen-code` (branch: `main`)
- Records commit hashes for build args

### 2. build-qwen-code
- Uses `node:20-slim`
- Runs `npm ci --ignore-scripts` (skip husky prepare hooks)
- Runs `npm run bundle`
- Outputs to `qwen-code-build/` directory

### 3. build-zed
- Uses `docker:cli` with Docker socket mount
- Builds `zed-builder:ubuntu25` image from `Dockerfile.zed-build`
- Runs cargo build inside container
- Caches binary by Zed commit hash
- Outputs `zed-build/zed` binary

### 4. build-desktops
- Uses `docker:cli` with Docker socket mount
- Builds `helix-sway` from `Dockerfile.sway-helix`
- Builds `helix-ubuntu` from `Dockerfile.ubuntu-helix`
- Saves as tarballs in `sandbox-images/`

### 5. build-sandbox
- Uses `docker:cli` with Docker socket mount
- Builds `helix-sandbox` from `Dockerfile.sandbox`
- Embeds desktop tarballs from `sandbox-images/`
- Tags with version and commit hash

### 6. push-sandbox
- Uses `docker:cli` with Docker socket mount
- Logs in to `registry.helixml.tech`
- Pushes commit hash tag (always)
- Pushes version tag (on release)
- Pushes `latest` tag (on release only)

## Local Development

Use the stack script for local builds:

```bash
# Build Zed binary (release mode)
./stack build-zed release

# Build qwen-code
./stack build-qwen-code

# Build desktop images
./stack build-sway
./stack build-ubuntu

# Build complete sandbox
./stack build-sandbox

# Build and push to registry (production release)
./stack build-and-push-helix-code
```

## Migration Steps

1. **Merge Luke's branch** - Required for simplified architecture
2. **Verify secrets** - Ensure `github_token` and `helix_registry_password` are configured in Drone
3. **Test pipeline** - Push to feature branch to verify builds work
4. **Update release process** - Tag creation triggers automatic build/push
5. **Deprecate node01 manual builds** - Keep as fallback initially

## Monitoring

Check build status at: `https://drone.lukemarsden.net/helixml/helix`

Failed builds will show in Drone UI with step-by-step logs.

## Future Improvements

1. **Parallel desktop builds** - Currently sequential, could use matrix
2. **GPU smoke tests** - Self-hosted runner with GPU for encoding tests
3. **Pre-built Zed releases** - Publish Zed binary to GitHub releases
4. **Nightly builds** - Cron trigger for automatic builds from main
5. **Build notifications** - Slack notification on failure
