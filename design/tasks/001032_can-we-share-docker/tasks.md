# Implementation Tasks

## Phase 1: Volume Setup

- [ ] Add `buildkit-cache` volume to `docker-compose.dev.yaml` and `docker-compose.yaml`
- [ ] Mount `/buildkit-cache` in sandbox-nvidia, sandbox-amd-intel, and sandbox-software services
- [ ] Verify volume persists across sandbox restarts

## Phase 2: Hydra Integration

- [ ] Update `devcontainer.go` to add `/buildkit-cache` bind mount to all dev containers
- [ ] Test that dev containers can read/write to `/buildkit-cache`
- [ ] Verify concurrent access from multiple sessions doesn't cause errors

## Phase 3: Build Wrapper (Optional Convenience)

- [ ] Create `/usr/local/bin/docker-build-cached` wrapper script in desktop images
- [ ] Script extracts image name and uses it as cache key subdirectory
- [ ] Add wrapper to helix-ubuntu and helix-sway Dockerfiles
- [ ] Document usage in desktop container README

## Phase 4: Testing

- [ ] Test: Build image in session A, verify cache hit in session B
- [ ] Test: Concurrent builds from 3+ sessions simultaneously
- [ ] Test: Cache survives session termination
- [ ] Measure disk space savings with shared vs isolated cache

## Phase 5: Documentation

- [ ] Update CLAUDE.md build section if needed
- [ ] Document cache flags for users who want explicit control
- [ ] Add troubleshooting notes for cache corruption recovery (`docker buildx prune`)