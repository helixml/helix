# Requirements: Shared Docker BuildKit Cache

## Problem Statement

Each Hydra-created session gets its own isolated dockerd instance with separate `--data-root`. This means:
- Build cache isn't shared between sessions
- Identical `docker build` commands repeat all work
- Disk space explodes as each session stores duplicate layers
- Builds are slow for users who frequently create new sessions

## Primary Use Case: Helix-in-Helix Development

When developing Helix inside a Helix desktop session ("Helix-in-Helix"), running `./stack start` triggers `docker compose up -d` which builds multiple services (api, frontend, haystack, etc.). 

**Current pain**: Starting a new Helix-in-Helix dev session means waiting 10+ minutes for builds that should be cached from previous sessions. This destroys developer productivity.

**Expected behavior**: Second `./stack start` in a new session should reuse cached layers from previous sessions and complete in <1 minute.

## User Stories

1. **As a Helix developer** working in Helix-in-Helix mode, I want `./stack start` to reuse build cache from my previous sessions, so I don't waste 10+ minutes waiting for identical builds.

2. **As a developer**, I want my second `docker build` to be fast even if I start a new session, so I don't waste time waiting for cached layers to rebuild.

3. **As a platform operator**, I want build cache shared across sessions to reduce disk space usage.

## Acceptance Criteria

- [ ] `docker build` in one session can use cache from builds in other sessions
- [ ] `docker compose build` in one session can use cache from builds in other sessions
- [ ] `docker compose up --build` uses shared cache
- [ ] Concurrent builds from multiple sessions don't corrupt the cache
- [ ] Cache sharing works for both `docker buildx build` and legacy `docker build`
- [ ] No security isolation regression (users can't access each other's actual build artifacts, just cache layers)
- [ ] Disk space usage for N identical builds is O(1) not O(N)
- [ ] **Helix-in-Helix: `./stack start` in a new session completes builds in <1 minute when cache is warm**

## Technical Constraints

- Docker Compose v2 does NOT shell out to `docker build` - it uses BuildKit API directly via gRPC
- Existing bash wrappers (`docker-wrapper.sh`, `docker-compose-wrapper.sh`) must be rewritten in Go before adding cache logic
- Solution must work with Compose v2.24+ (`--set` flag) and fall back gracefully for older versions

## Out of Scope

- Cross-sandbox cache sharing: This design shares cache between sessions **within** a single sandbox. If multiple sandboxes exist (e.g., in Kubernetes), each has its own isolated cache volume - no sharing between sandboxes.
- Remote cache backends (registry-based caching)
- Cache eviction policies (rely on Docker's built-in LRU)