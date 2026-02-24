# ZFS zvol Clones for Instant Golden Cache

**Date:** 2026-02-24
**Status:** Proposed
**Author:** Luke + Claude

## Problem

Golden cache copy takes 60-70s for 33GB (772K files) even with XFS reflinks,
because each reflink still requires per-file inode allocation and metadata
journaling. Parallel copy cuts this to ~20-30s, but this is still the single
largest bottleneck in spectask startup.

## Proposal

When a ZFS pool is available, use ZFS snapshots and clones instead of file-level
copies. A `zfs clone` of a snapshot is near-instant (metadata-only, <100ms for
any size) and the clone only consumes additional space as it diverges from the
snapshot (CoW at the block level).

## Key constraint: ZFS is optional

ZFS must NOT be a hard dependency. Many operators run on ext4, older RHEL, or
cloud instances without ZFS. The system must:

1. **Auto-detect** ZFS availability at runtime
2. **Fall back gracefully** to the current parallel `cp --reflink=auto` approach
3. **Not require ZFS in the Dockerfile** or any mandatory configuration

## Architecture

### Storage layout

```
# Current (file-level copy on shared XFS filesystem):
/container-docker/                  # single XFS on ZFS zvol
├── golden/{projectID}/docker/      # golden cache (full copy of /var/lib/docker)
└── sessions/{volumeName}/docker/   # per-session copy (cp -a --reflink=auto)

# Proposed (ZFS zvol clones when available):
prod/golden-{projectID}             # ZFS filesystem (not zvol — see below)
  @latest                           # snapshot after golden build completes
prod/session-{sessionID}            # clone of golden-{projectID}@latest
  (auto-mounted at /container-docker/sessions/{volumeName}/docker)
```

### Why ZFS filesystems, not zvols?

ZFS zvols are block devices that need a filesystem (XFS/ext4) on top. Cloning a
zvol creates a new block device that also needs mounting. This works but adds
complexity (mkfs on first golden, mount/umount lifecycle).

ZFS native filesystems are simpler: `zfs clone` produces a mountable filesystem
directly, with automatic mountpoint management. Docker's overlay2 works fine on
ZFS filesystems (it just uses the underlying filesystem as a regular directory).

**However**, Docker's overlay2 storage driver on ZFS has a known issue: ZFS
doesn't support `d_type` on all configurations, and overlay2 requires it.
Modern ZFS (2.0+) supports `d_type` by default, but this needs verification.

Alternative: keep the zvol approach with XFS on top (current architecture),
but create one zvol per golden project:

```
prod/container-docker              # existing zvol (XFS, shared sessions)
prod/golden-{projectID}            # per-project zvol (XFS)
  @latest                          # snapshot
prod/session-{sessionID}           # clone of golden zvol
```

This is safer — XFS on zvol is already proven in our stack.

### Detection

```go
// DetectZFSPool checks if a ZFS pool is available for golden cache management.
// Returns the pool name and true if ZFS is available, empty string and false otherwise.
func DetectZFSPool() (string, bool) {
    // Check if zfs command exists
    if _, err := exec.LookPath("zfs"); err != nil {
        return "", false
    }

    // Check if CONTAINER_DOCKER_PATH is on a ZFS zvol
    // by looking at the mount source
    containerDockerPath := os.Getenv("CONTAINER_DOCKER_PATH")
    if containerDockerPath == "" {
        return "", false
    }

    // Parse /proc/mounts to find the device backing containerDockerPath
    // If it's /dev/zvol/POOL/NAME, extract POOL
    // ...

    return poolName, true
}
```

### Golden cache lifecycle with ZFS

#### Creating a golden (after successful golden build)

```
Current:
  os.Rename(session/docker → golden/{projectID}/docker)

With ZFS:
  1. zfs snapshot prod/session-{sessionID}@golden
  2. zfs clone prod/session-{sessionID}@golden prod/golden-{projectID}-new
  3. zfs rename prod/golden-{projectID} prod/golden-{projectID}-old  (atomic)
  4. zfs rename prod/golden-{projectID}-new prod/golden-{projectID}
  5. zfs destroy prod/golden-{projectID}-old                         (background)
  6. zfs destroy prod/session-{sessionID}
```

Actually simpler:
```
  1. Stop the golden build container (dockerd stopped, data quiesced)
  2. zfs snapshot prod/session-{sessionID}@promote
  3. zfs destroy prod/golden-{projectID} (if exists, destroys old golden)
  4. zfs rename prod/session-{sessionID} prod/golden-{projectID}
     — This preserves the @promote snapshot
  5. Done. Next session clones from prod/golden-{projectID}@promote
```

#### Creating a session (on container start)

```
Current (60-70s):
  cp -a --reflink=auto golden/{projectID}/docker → sessions/{volumeName}/docker

With ZFS (<100ms):
  1. zfs clone prod/golden-{projectID}@promote prod/session-{sessionID}
  2. Mount is automatic (mountpoint=container-docker/sessions/{sessionID})
  3. Bind mount into container as /var/lib/docker
```

#### Cleaning up a session (on container stop)

```
Current:
  os.RemoveAll(sessions/{volumeName})

With ZFS (<1s):
  zfs destroy prod/session-{sessionID}
```

### Sizing

With ZFS dedup (4.27x ratio on our pool) and CoW clones:
- Golden cache: 33GB apparent, ~8GB actual (dedup)
- Each session clone: 0 bytes initially, grows only as Docker writes new layers
- Typical session divergence: 2-5GB (new build layers, container metadata)
- 10 concurrent sessions: ~50GB actual instead of 330GB (10x savings)

### Implementation plan

1. **Phase 1: Detection + fallback** (this PR)
   - Add `DetectZFSPool()` to golden.go
   - No behavior change if ZFS not detected
   - Log detection result on startup

2. **Phase 2: ZFS golden + clone** (follow-up PR)
   - Implement `SetupGoldenClone()` as alternative to `SetupGoldenCopy()`
   - Implement `PromoteSessionToGoldenZFS()` as alternative to `PromoteSessionToGolden()`
   - Implement `CleanupSessionZFS()` as alternative to file-level cleanup
   - Wire into devcontainer.go with runtime detection

3. **Phase 3: macOS VM provisioning**
   - Update `for-mac/vm.go` to create ZFS pool structure during VM setup
   - Already uses ZFS zvols, so this is extending existing infrastructure

### Risks and mitigations

| Risk | Mitigation |
|------|-----------|
| ZFS not available on operator's system | Auto-detect, fall back to parallel cp |
| ZFS version too old for d_type | Use zvol+XFS approach (proven), not native ZFS |
| Clone accumulation fills pool | Existing GC handles cleanup, ZFS `zfs list` is O(1) |
| Snapshot dependency chains | Keep it simple: one @promote snapshot per golden |
| Docker overlay2 issues on ZFS | Use zvol+XFS, not ZFS native filesystem |
| Concurrent promote during clone | ZFS operations are atomic, safe under concurrency |

### Alternatives considered

1. **Docker image save/load**: Store golden as tarball, load in new sessions.
   Rejected: slower (decompression), loses layer sharing, doesn't cache build state.

2. **Overlayfs with golden as lowerdir**: Mount golden read-only with session
   as upper. Rejected: Docker's overlay2 can't run on top of overlayfs (nested
   overlay restriction on upper dir).

3. **Btrfs subvolumes**: Similar to ZFS clones, instant snapshots. Rejected:
   less common than ZFS in our target deployments, and we already use ZFS.

4. **Block-level dedup with rsync**: Rejected: rsync doesn't support reflink,
   and even with parallelism it's slower than ZFS clones.

5. **Keep parallel cp forever**: Viable fallback (20-30s), but ZFS clones
   would make it <100ms. Worth the implementation effort for the 200x speedup.

## Expected impact

| Metric | Current (parallel cp) | With ZFS clones |
|--------|----------------------|-----------------|
| Golden copy time | 20-30s | <100ms |
| Session disk overhead | Full copy (33GB) | CoW delta only (2-5GB) |
| 10 concurrent sessions | 330GB | ~50GB |
| Cleanup time | `rm -rf` (5-10s) | `zfs destroy` (<1s) |
| Total startup improvement | ~40s saved | ~60s saved |
