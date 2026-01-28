# Implementation Tasks

## Phase 1: Hydra Cache Directory Setup

- [x] Create `/hydra-data/buildkit-cache/` directory in Hydra's `NewManager` or `Start` function
- [x] Ensure directory has correct permissions (0755) for all dockerd instances to access

## Phase 2: Mount Cache in Dev Containers

- [x] Update `devcontainer.go` `buildMounts()` to add `/hydra-data/buildkit-cache` → `/buildkit-cache` bind mount
- [ ] Test that dev containers can read/write to `/buildkit-cache`
- [ ] Verify concurrent access from multiple sessions doesn't cause errors

## Phase 3: Rewrite Bash Wrappers in Go

**Prerequisite**: The existing bash wrappers are unmaintainable garbage. Rewrite before adding cache logic.

- [x] Create new `docker-shim` Go package (probably in `api/pkg/docker-shim/` or standalone `cmd/docker-shim/`)
- [x] Implement core functionality from `docker-wrapper.sh`:
  - [x] Path translation for Hydra bind mounts
  - [x] Docker socket routing
  - [x] Argument parsing and passthrough
- [x] Implement core functionality from `docker-compose-wrapper.sh`:
  - [x] Compose file path translation
  - [x] Project name handling
  - [x] Argument parsing and passthrough
- [x] Add `argv[0]` detection to act as both `docker` and `docker-compose` shim
- [x] Build static binary for inclusion in desktop images
- [~] Update `desktop/sway-config/` and `desktop/ubuntu-config/` Dockerfiles to use Go shim
- [ ] Delete the bash wrapper scripts
- [x] Write unit tests for argument parsing and path translation

## Phase 4: Add Cache Injection to Go Shim

### For `docker build` / `docker buildx build`:

- [x] Detect `build` and `buildx build` commands
- [x] Extract image name from `-t` flag to use as cache key subdirectory
- [x] Inject `--cache-from type=local,src=/buildkit-cache/{key}` flag
- [x] Inject `--cache-to type=local,dest=/buildkit-cache/{key},mode=max` flag
- [x] Only inject if `/buildkit-cache` directory exists

### For `docker compose build` / `docker compose up --build`:

**Note**: Docker Compose v2 does NOT shell out to `docker build`. It uses the BuildKit API directly via gRPC, so the docker shim won't intercept these builds. Must handle at compose wrapper level.

- [x] Detect `compose build` and `compose up` (with `--build`) commands
- [x] Get Compose version: `docker compose version --short`
- [x] For Compose v2.24+: Inject `--set` flags:
  - `--set "*.build.cache_from=[\"type=local,src=/buildkit-cache\"]"`
  - `--set "*.build.cache_to=[\"type=local,dest=/buildkit-cache,mode=max\"]"`
- [x] For older Compose: Implement compose file preprocessing fallback
  - [x] Parse compose file with `gopkg.in/yaml.v3`
  - [x] Inject `cache_from` and `cache_to` into all services with `build:` sections
  - [x] Write to temp file, pass with `-f` flag
  - [x] Clean up temp file after compose exits

## Phase 5: Testing

- [ ] Test: `docker build` in session A, verify cache hit in session B
- [ ] Test: `docker compose build` in session A, verify cache hit in session B
- [ ] Test: `docker compose up --build` uses cache from previous builds
- [ ] Test: Concurrent builds from 3+ sessions simultaneously
- [ ] Test: Cache survives session termination
- [ ] **Test: Helix-in-Helix `./stack start` uses shared cache across sessions** (primary use case!)
- [ ] Measure disk space savings with shared vs isolated cache
- [ ] Verify no regression in path translation functionality

## Phase 6: Documentation

- [ ] Update CLAUDE.md build section if needed
- [ ] Document cache flags for users who want explicit control
- [ ] Add troubleshooting notes for cache corruption recovery (`docker buildx prune`)
- [ ] Document the Go shim for future maintainers