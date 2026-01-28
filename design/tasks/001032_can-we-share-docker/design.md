# Design: Shared Docker BuildKit Cache

## Current Architecture

```
helix-sandbox container
├── Sandbox dockerd (/var/lib/docker)
│   └── Desktop images (helix-ubuntu, helix-sway)
│
└── Hydra manager
    ├── Session 1 dockerd (--data-root=/hydra-data/sessions/ses_001/docker)
    │   └── User builds cached here (isolated)
    ├── Session 2 dockerd (--data-root=/hydra-data/sessions/ses_002/docker)
    │   └── User builds cached here (isolated, no sharing!)
    └── Session N dockerd...
```

Each Hydra-spawned dockerd has its own `--data-root`, so BuildKit cache is NOT shared.

## Proposed Solution: Shared BuildKit Cache Directory

BuildKit stores cache in `<data-root>/buildkit/`. We can use BuildKit's local cache export/import to share cache across dockerd instances via a shared volume.

### Option A: Inline Cache (Simplest)

Use `--cache-from` and `--cache-to` with a shared directory:

```bash
docker buildx build \
  --cache-from type=local,src=/shared-cache/myimage \
  --cache-to type=local,dest=/shared-cache/myimage,mode=max \
  -t myimage .
```

**Pros**: Works with any dockerd, no daemon config changes
**Cons**: Requires user/agent to pass cache flags explicitly

### Option B: Registry Cache (Alternative)

Use the sandbox's local registry for caching:

```bash
docker buildx build \
  --cache-from type=registry,ref=registry:5000/cache/myimage \
  --cache-to type=registry,ref=registry:5000/cache/myimage,mode=max \
  -t myimage .
```

**Pros**: Standard pattern, works across sandboxes
**Cons**: More network overhead, registry must be running

## Recommended Approach: Option A with Wrapper

1. **Mount shared cache volume** in all Hydra dockerd containers
2. **Provide a build wrapper** that automatically adds cache flags
3. **BuildKit handles concurrency** via content-addressed storage (safe for concurrent access)

### Implementation Details

**Volume Mount** (in docker-compose):
```yaml
volumes:
  - buildkit-cache:/buildkit-cache
```

**Hydra passes mount to dev containers** (devcontainer.go):
```go
mounts = append(mounts, mount.Mount{
    Type:   mount.TypeBind,
    Source: "/buildkit-cache",
    Target: "/buildkit-cache",
})
```

**Build wrapper script** (`/usr/local/bin/docker-build-cached`):
```bash
#!/bin/bash
# Extract image name from -t flag, use as cache key
IMAGE_NAME=$(echo "$@" | grep -oP '(?<=-t\s)\S+' | head -1 | tr '/:' '_')
CACHE_DIR="/buildkit-cache/${IMAGE_NAME:-default}"

exec docker buildx build \
  --cache-from "type=local,src=$CACHE_DIR" \
  --cache-to "type=local,dest=$CACHE_DIR,mode=max" \
  "$@"
```

## Concurrency Safety

BuildKit's local cache exporter uses content-addressed storage (blobs identified by SHA256). This is safe for concurrent access:

- **Reads**: Multiple readers can access the same blob simultaneously
- **Writes**: Each writer creates new blobs atomically (write to temp, rename)
- **No corruption**: Same content = same hash = same file (idempotent)

The BuildKit team confirms this works for concurrent builds: https://github.com/moby/buildkit/issues/1512

## Disk Space Considerations

- **Cache location**: `/buildkit-cache/` on the `buildkit-cache` volume
- **Deduplication**: Content-addressed storage means identical layers stored once
- **Pruning**: Use `docker buildx prune` periodically, or let Docker's LRU handle it
- **Estimated savings**: 10 identical builds go from ~50GB to ~5GB

## Key Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Cache method | Local directory | Simplest, no registry dependency |
| Mount type | Bind mount | Share across Hydra dockerd instances |
| Concurrency | Trust BuildKit | Content-addressed, proven safe |
| Wrapper | Optional script | Don't force users, but provide convenience |