# Design: Shared Docker BuildKit Cache

## Current Architecture

```
helix-sandbox container
├── Sandbox dockerd (/var/lib/docker)
│   └── Desktop images (helix-ubuntu, helix-sway)
│
└── Hydra manager
    └── /hydra-data/ (hydra-storage volume - persists across sandbox restarts)
        ├── sessions/ses_001/docker (Session 1 dockerd --data-root)
        │   └── User builds cached here (isolated)
        ├── sessions/ses_002/docker (Session 2 dockerd --data-root)
        │   └── User builds cached here (isolated, no sharing!)
        └── sessions/ses_N/...
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

## Recommended Approach: Option A with Existing Wrapper

1. **Hydra creates shared cache directory** at `/hydra-data/buildkit-cache/`
2. **Hydra mounts this directory** into all dev containers it creates
3. **Extend existing `docker-wrapper.sh`** to inject cache flags for `buildx build` commands
4. **BuildKit handles concurrency** via content-addressed storage (safe for concurrent access)

### Implementation Details

**Hydra creates shared cache directory** (manager.go):
```go
// In NewManager or Start, create the shared cache directory
cacheDir := filepath.Join(m.dataDir, "buildkit-cache")
os.MkdirAll(cacheDir, 0755)
```

**Hydra passes mount to dev containers** (devcontainer.go):
```go
// Add shared buildkit cache to all dev containers
mounts = append(mounts, mount.Mount{
    Type:   mount.TypeBind,
    Source: filepath.Join(dm.manager.dataDir, "buildkit-cache"),
    Target: "/buildkit-cache",
})
```

This is entirely internal to Hydra - no docker-compose changes needed. The `/hydra-data` volume already exists and persists across sandbox restarts.

**Extend existing docker-wrapper.sh** (`desktop/sway-config/docker-wrapper.sh` and `desktop/ubuntu-config/docker-wrapper.sh`):

There's already a Docker wrapper that intercepts all `docker` commands to translate paths for Hydra. We add cache flag injection there:

```bash
# In docker-wrapper.sh, detect "buildx build" or "build" commands and inject cache flags
# Add this logic before the final exec:

if [[ "${args[0]}" == "buildx" && "${args[1]}" == "build" ]] || [[ "${args[0]}" == "build" ]]; then
    # Extract image name from -t flag for cache key
    IMAGE_NAME=""
    for ((i=0; i<${#args[@]}; i++)); do
        if [[ "${args[$i]}" == "-t" && $((i+1)) -lt ${#args[@]} ]]; then
            IMAGE_NAME="${args[$((i+1))]}"
            break
        fi
    done
    
    # Sanitize image name for directory path
    CACHE_KEY=$(echo "${IMAGE_NAME:-default}" | tr '/:' '_')
    CACHE_DIR="/buildkit-cache/${CACHE_KEY}"
    
    # Inject cache flags (only if /buildkit-cache exists)
    if [[ -d "/buildkit-cache" ]]; then
        args+=("--cache-from" "type=local,src=$CACHE_DIR")
        args+=("--cache-to" "type=local,dest=$CACHE_DIR,mode=max")
    fi
fi
```

This reuses the existing "evil shit" wrapper instead of creating a new one.

## Concurrency Safety

BuildKit's local cache exporter uses content-addressed storage (blobs identified by SHA256). This is safe for concurrent access:

- **Reads**: Multiple readers can access the same blob simultaneously
- **Writes**: Each writer creates new blobs atomically (write to temp, rename)
- **No corruption**: Same content = same hash = same file (idempotent)

The BuildKit team confirms this works for concurrent builds: https://github.com/moby/buildkit/issues/1512

## Disk Space Considerations

- **Cache location**: `/hydra-data/buildkit-cache/` (inside existing hydra-storage volume)
- **Deduplication**: Content-addressed storage means identical layers stored once
- **Pruning**: Use `docker buildx prune` periodically, or let Docker's LRU handle it
- **Estimated savings**: 10 identical builds go from ~50GB to ~5GB

## Key Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Cache method | Local directory | Simplest, no registry dependency |
| Cache location | `/hydra-data/buildkit-cache/` | Uses existing volume, Hydra implementation detail |
| Mount type | Bind mount | Share across Hydra dockerd instances |
| Concurrency | Trust BuildKit | Content-addressed, proven safe |
| Wrapper | Extend existing `docker-wrapper.sh` | Reuse existing path-translation wrapper, no new scripts |