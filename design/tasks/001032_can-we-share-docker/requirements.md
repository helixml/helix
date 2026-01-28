# Requirements: Shared Docker BuildKit Cache

## Problem Statement

Each Hydra-created session gets its own isolated dockerd instance with separate `--data-root`. This means:
- Build cache isn't shared between sessions
- Identical `docker build` commands repeat all work
- Disk space explodes as each session stores duplicate layers
- Builds are slow for users who frequently create new sessions

## User Stories

1. **As a developer**, I want my second build to be fast even if I start a new session, so I don't waste time waiting for cached layers to rebuild.

2. **As a platform operator**, I want build cache shared across sessions to reduce disk space usage.

## Acceptance Criteria

- [ ] Docker builds in one session can use cache from builds in other sessions
- [ ] Concurrent builds from multiple sessions don't corrupt the cache
- [ ] Cache sharing works for both buildx and legacy `docker build`
- [ ] No security isolation regression (users can't access each other's actual build artifacts, just cache layers)
- [ ] Disk space usage for N identical builds is O(1) not O(N)

## Out of Scope

- Cross-sandbox cache sharing (each sandbox has its own cache volume)
- Remote cache backends (registry-based caching)
- Cache eviction policies (rely on Docker's built-in LRU)