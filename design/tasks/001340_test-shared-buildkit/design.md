# Design: Test Shared BuildKit Cache Performance

## Architecture Context

The shared BuildKit setup consists of:

1. **`helix-buildkit` container** — runs `moby/buildkit:latest` on the sandbox's main dockerd
2. **`helix-shared` buildx builder** — remote driver pointing to `tcp://<buildkit-ip>:1234`
3. **Docker wrapper** (`/usr/local/bin/docker`) — rewrites `docker build` → `docker buildx build` for transparent routing
4. **`BUILDX_BUILDER=helix-shared`** env var — set globally via `/etc/environment`

Cache is stored in:
- `/hydra-data/buildkit-cache` (bind-mounted to `/buildkit-cache` in buildkit container)
- `/var/lib/buildkit` inside the container (content-addressed blob storage)

## Test Approach

### Manual Test Script

Create a shell script that:
1. Starts a spectask session
2. Runs a `docker build` with a known Dockerfile
3. Records build time
4. Stops the session
5. Starts a NEW session
6. Runs the same `docker build`
7. Compares times

### Key Verification Points

| Check | Command | Expected |
|-------|---------|----------|
| Builder is active | `docker buildx ls` | `helix-shared * remote` |
| BUILDX_BUILDER set | `echo $BUILDX_BUILDER` | `helix-shared` |
| Cache populated | `ls -la /buildkit-cache/` | Non-empty after build |

### Test Dockerfile

Use a simple Dockerfile that has cacheable layers:

```dockerfile
FROM ubuntu:22.04
RUN apt-get update && apt-get install -y curl wget git
RUN echo "cache-test-marker-$(date +%s)" > /marker.txt
```

The `apt-get` layer should be cached; the `echo` with timestamp forces a final layer rebuild.

## Decision: Manual vs Automated

**Decision:** Manual test script (not CI integration)

**Rationale:**
- BuildKit cache testing requires actual spectask sessions with desktop containers
- CI doesn't have GPU/desktop infrastructure
- One-time validation is sufficient; this isn't a regression-prone area
- Future: Could add to smoke tests if needed

## Test Location

Script goes in `helix/scripts/test-buildkit-cache.sh` — follows existing pattern for utility scripts.