# Sandbox Docker Storage Split

**Date:** 2026-02-14
**Status:** Implementing

## Problem

The sandbox's entire Docker storage (`/var/lib/docker`) is mounted from a ZFS zvol
(`helix/sandbox-docker`). On first user boot, the zvol is freshly formatted (empty ext4),
so the sandbox's nested dockerd has no images. Desktop images (helix-ubuntu) built during
provisioning exist in the host Docker but not inside the sandbox, causing:

```
No such image: helix-ubuntu:08bbcd
```

## Solution

Split sandbox Docker storage into two tiers:

| Component | Storage | Mount | Why |
|-----------|---------|-------|-----|
| Sandbox's dockerd (images, overlay2) | Root disk | Named Docker volume (`sandbox-docker-storage`) | Desktop images persist from provisioning |
| Desktop inner dockerd (`/var/lib/docker` per session) | ZFS zvol | Bind mount from `/helix/container-docker/{sessionID}/` | Dedup across sessions, compression |
| BuildKit container state | ZFS zvol | Bind mount from `/helix/container-docker/buildkit/` | Content-addressed, dedup-friendly |
| BuildKit shared cache | ZFS dataset | `/hydra-data/buildkit-cache` (existing) | Already on hydra-data volume |
| Workspace data | ZFS dataset | `/data/workspaces/` (existing) | User files |

### Helix-in-Helix

In H-in-H, the inner sandbox runs inside a desktop container. The desktop container's
inner dockerd is on the ZFS zvol, so the inner sandbox's Docker storage is also on ZFS.
This is correct — the outer sandbox (images on root disk) is the only one that needs
provisioned images; inner sandboxes pull from the registry.

## Changes

1. **`vm.go` `injectDesktopSecret()`**: Remove `SANDBOX_DOCKER_STORAGE=/helix/sandbox-docker`
   (revert to default named volume on root disk)

2. **`vm.go` `initZFSPool()`**: Rename zvol from `helix/sandbox-docker` to
   `helix/container-docker`. Same 200GB thin-provisioned, dedup+compression.
   Mount at `/helix/container-docker`.

3. **`hydra_executor.go`**: Change per-session inner dockerd from named Docker volume
   to bind mount from `/helix/container-docker/sessions/{sessionID}/docker/`

4. **`manager.go`**: Change BuildKit container's state volume from named Docker volume
   to bind mount from `/helix/container-docker/buildkit/`

5. **Provisioning**: Transfer desktop images during provisioning, keep named volumes
   (no `-v` on `docker compose down`)
