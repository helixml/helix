# Requirements: Make Helix Builds and Releases Faster

## Problem

The release build (Drone CI build #720, tag 2.9.31) took **1h 54m 15s**, which is unacceptable for developer productivity. Every release blocks development and delays shipping fixes to users.

## User Stories

1. **As a release engineer**, I want tag builds to complete in under 45 minutes so that releases don't block my entire afternoon.
2. **As a developer**, I want CI feedback on main branch pushes within 20 minutes so I can fix issues while context is fresh.
3. **As the team**, we want the release pipeline to be resilient to transient failures (e.g., registry timeouts) without requiring a full re-run from scratch.

## Acceptance Criteria

- [ ] **Release builds (tag events) complete in under 45 minutes** end-to-end (measured from Drone build start to final pipeline completion)
- [ ] **Main branch builds complete in under 15 minutes** for the default + sandbox pipelines
- [ ] No correctness regressions — all existing tests continue to pass
- [ ] Build caching is preserved and effective — repeat builds with no code changes complete in under 15 minutes
- [ ] The macOS DMG pipeline does not add more than 15 minutes to the critical path

## Current Pipeline Structure (Release Builds)

A tag event triggers **15 pipelines**. The critical path (longest chain of dependencies):

```
default (build-backend + build-frontend + release-backend)
  ├── build-controlplane-amd64 ──┐
  ├── build-controlplane-arm64 ──┤── manifest-controlplane
  ├── build-runner (retag only, fast)
  ├── build-runner-small (retag only)
  ├── build-runner-large (retag only)
  ├── build-demos
  ├── build-sandbox-amd64 ──┐
  ├── build-sandbox-arm64 ──┤── manifest-sandbox
  └── build-macos-dmg (depends on: default + controlplane-arm64 + sandbox-arm64)
```

### Estimated Time Breakdown by Pipeline

| Pipeline | Estimated Duration | Notes |
|----------|-------------------|-------|
| default (build-backend + frontend + release) | 10-15 min | Sequential: Go cross-compile + yarn build + gh release upload |
| build-controlplane-amd64 | 10-15 min | Docker build of main Dockerfile (Go + Node + models) |
| build-controlplane-arm64 | 15-20 min | Same but slower on ARM runner |
| manifest-controlplane | 1-2 min | Just creates multi-arch manifest |
| build-runner/small/large | 2-3 min each | Retag only (no rebuild) |
| build-demos | 3-5 min | Small Docker build |
| **build-sandbox-amd64** | **40-60 min** | **Critical path bottleneck** — Zed (Rust), qwen-code, 2 desktop images, E2E test |
| **build-sandbox-arm64** | **50-70 min** | Same but slower on macOS Docker Desktop |
| manifest-sandbox | 1-2 min | Multi-arch manifest |
| **build-macos-dmg** | **25-40 min** | VM provisioning + app build + notarization |

### Critical Path Analysis

The likely critical path for the 1h54m build:

1. `build-sandbox-arm64` runs on macOS Docker Desktop (slower) — ~50-70 min
2. `build-macos-dmg` depends on sandbox-arm64 completing — ~25-40 min
3. Total critical path: ~75-110 min

The sandbox pipeline internal critical path:
```
clone-deps (1 min)
  → build-zed (20-40 min, parallel with build-qwen-code 2-5 min)
  → build-desktops (25-40 min — helix-sway + helix-ubuntu built SEQUENTIALLY)
  → build-sandbox (8-12 min)
  → push-sandbox (2-5 min, gated on zed-e2e-test)
```

## Key Bottlenecks Identified

1. **Desktop images built sequentially** — helix-sway and helix-ubuntu are built one after the other in the `build-desktops` step, each taking 15-25 min
2. **Zed Rust compilation** — 20-40 min on cache miss (cache keyed on commit SHA)
3. **ARM64 builds on macOS Docker Desktop** — significantly slower than native amd64
4. **macOS DMG pipeline** — VM provisioning + notarization adds 25-40 min to critical path
5. **GHCR mirroring doubles push time** — every image push also mirrors to ghcr.io
6. **Redundant apt-get updates** in desktop Dockerfiles — scattered across many RUN blocks
7. **Embedding model downloads not cached** in main Dockerfile
8. **qwen-code Dockerfile.qwen-build** copies all source before npm ci, invalidating dependency cache

## Constraints

- Multi-arch (amd64 + arm64) is required — cannot drop ARM support
- Zed must be built from source (custom fork)
- macOS DMG is required for the desktop product
- E2E tests must gate sandbox push (quality gate)
- Docker BuildKit cache mounts are available on all runners
- ARM64 runner is a macOS machine running Docker Desktop (cannot change easily)
