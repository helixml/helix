# Shared BuildKit Cache Across Sandboxes

**Date:** 2026-01-25
**Status:** Idea (not implemented)

## Problem

Each Hydra-created sandbox has its own dockerd. Build cache isn't shared, so identical builds repeat work.

## Solution

Mount shared volume at `/buildkit-cache` in all sandboxes. Use BuildKit's local cache export/import:

```bash
docker buildx build \
  --cache-from type=local,src=/buildkit-cache/imagename \
  --cache-to type=local,dest=/buildkit-cache/imagename,mode=max \
  -t imagename .
```

## Implementation

1. Mount `/data/buildkit-cache:/buildkit-cache` in sandbox containers
2. Update `./stack build-*` scripts to use cache flags
3. BuildKit handles concurrent access via content-addressed storage
