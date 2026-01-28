# Implementation Tasks

## Phase 1: Hydra Cache Directory Setup

- [ ] Create `/hydra-data/buildkit-cache/` directory in Hydra's `NewManager` or `Start` function
- [ ] Ensure directory has correct permissions (0755) for all dockerd instances to access

## Phase 2: Mount Cache in Dev Containers

- [ ] Update `devcontainer.go` `buildMounts()` to add `/hydra-data/buildkit-cache` → `/buildkit-cache` bind mount
- [ ] Test that dev containers can read/write to `/buildkit-cache`
- [ ] Verify concurrent access from multiple sessions doesn't cause errors

## Phase 3: Deduplicate and Extend Docker Wrapper

- [ ] Move `desktop/sway-config/docker-wrapper.sh` to `desktop/shared/docker-wrapper.sh`
- [ ] Move `desktop/sway-config/docker-compose-wrapper.sh` to `desktop/shared/docker-compose-wrapper.sh`
- [ ] Update helix-sway and helix-ubuntu Dockerfiles to copy from `shared/` instead
- [ ] Delete the duplicate copies in `desktop/ubuntu-config/`
- [ ] Add cache flag injection: detect `build` and `buildx build` commands
- [ ] Extract image name from `-t` flag to use as cache key subdirectory
- [ ] Inject `--cache-from` and `--cache-to` flags when `/buildkit-cache` directory exists

## Phase 4: Testing

- [ ] Test: Build image in session A, verify cache hit in session B
- [ ] Test: Concurrent builds from 3+ sessions simultaneously
- [ ] Test: Cache survives session termination
- [ ] Measure disk space savings with shared vs isolated cache

## Phase 5: Documentation

- [ ] Update CLAUDE.md build section if needed
- [ ] Document cache flags for users who want explicit control
- [ ] Add troubleshooting notes for cache corruption recovery (`docker buildx prune`)