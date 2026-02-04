# Workspace Storage Optimization with ZFS Deduplication

**Date:** 2026-02-02
**Status:** Deployed
**Author:** Claude (with Luke)

## Update: 2026-02-02 - Migration Complete

### Production Migration Results

| Metric | Value |
|--------|-------|
| Original ext4 size | 427 GB |
| Logical data migrated | 402 GB |
| **Actual disk growth** | **35 GB** |
| Compression ratio | 2.14x |
| **Effective dedup+compression** | **11.5x** |
| **Disk saved** | **367 GB (91%)** |
| Files migrated | 19,211,290 |
| DDT (dedup table) size | 344 MB |

Configuration: `HELIX_SANDBOX_DATA=/prod/helix-workspaces` in `.env`

### ZFS 2.4.0 Required

**Critical finding:** ZFS 2.3.x has a dedup regression bug (PR #17120) that prevents deduplication from working correctly. After upgrading to ZFS 2.4.0 via the arter97 PPA, dedup works as expected.

### Initial Test Results (20 workspaces)

| Metric | ext4 (baseline) | ZFS 2.4.0 (dedup + lz4) |
|--------|-----------------|-------------------------|
| 20 workspaces size | 22 GB | ~1 GB actual disk |
| Compression ratio | - | 2.28x |
| **Dedup ratio** | - | **17.89x** |
| **Total savings** | - | **95%** |

The DDT shows blocks with refcounts of 16-32x, meaning node_modules and .zed-state files are heavily deduplicated across workspaces as expected.

## Problem Statement

The Docker storage volume is consuming 1.2TB, with 410GB in spec-task workspaces alone. Investigation reveals:

| Category | Size | Count | Notes |
|----------|------|-------|-------|
| **node_modules** | 244GB | 191 dirs | Highly duplicated (many identical 66MB directories) |
| .git directories | 20GB | 461 repos | helix-specs clones |
| Other workspace files | ~146GB | - | Project files, Zed state, etc. |
| **Total spec-tasks** | **410GB** | 463 workspaces | |

The key observation: **60% of workspace storage is node_modules**, and many of these are identical or near-identical copies of the same dependencies.

## Current Architecture

```
/dev/zd0 (ZFS zvol) -> ext4 -> /var/lib/docker
                                ├── overlay2/     (576GB - images, build cache)
                                └── volumes/
                                    └── helix_sandbox-data/
                                        └── workspaces/spec-tasks/  (410GB)
```

Docker's overlay2 storage driver requires ext4/xfs, so the zvol approach is necessary. However, this means we can't use ZFS features (dedup, compression) on Docker's internal storage.

## ZFS Dedup: Current State of the Art

### Memory Requirements

ZFS dedup requires keeping the Deduplication Table (DDT) in memory for acceptable performance:

- **320 bytes per unique block** (typically 128KB blocks)
- **~5GB RAM per 1TB of unique data** (with 3x dedup ratio)
- **25% of ARC** - ZFS limits metadata to 25% of available memory

For our 410GB workspaces with estimated 3x dedup potential:
- Unique data after dedup: ~137GB
- DDT size: ~1-2GB RAM required

### Known Problems (2026)

Based on production experience reports:

1. **Performance cliff**: "Initial performance was tolerable but at one point it dropped off a cliff" - even with 256GB RAM for 120TB
2. **Snapshot deletion**: Extremely slow, can "kill the ZFS machine for hours"
3. **Memory pressure**: When DDT spills to disk, performance degrades severely
4. **No partial dedup**: Can't easily limit dedup to specific directories

Sources:
- [Oracle ZFS Dedup Sizing](https://www.oracle.com/technical-resources/articles/it-infrastructure/admin-o11-113-size-zfs-dedup.html)
- [TrueNAS ZFS Deduplication](https://www.truenas.com/docs/references/zfsdeduplication/)
- [Why ZFS dedup is not something we can use](https://utcc.utoronto.ca/~cks/space/blog/solaris/ZFSDedupMemoryProblem)
- [OpenZFS Issue #6116 - Less RAM hungry dedup](https://github.com/openzfs/zfs/issues/6116)

## Proposed Solution: Separate ZFS Dataset for Workspaces

Instead of enabling dedup on Docker's storage, move the workspace data to a dedicated ZFS dataset with dedup enabled.

### Architecture

```
prod/docker (zvol, ext4)
├── overlay2/     (images, build cache - NO dedup, ext4 required)
└── volumes/
    └── helix_sandbox-data/
        ├── sessions/          (33MB - keep on ext4)
        └── workspaces/ -> /workspaces (bind mount to ZFS)

prod/workspaces (ZFS dataset, dedup=on, compression=lz4)
├── sessions/
└── spec-tasks/     (410GB -> estimated 150GB with dedup)
```

### Prerequisites

**ZFS 2.4.0 or later is required.** ZFS 2.3.x has a dedup regression bug (PR #17120 - "Fix deduplication of overridden blocks") that causes dedup to silently fail on many block writes.

```bash
# Check current version
zfs --version

# If on Ubuntu 25.10 (Questing) with ZFS 2.3.x, add the arter97 PPA:
# This PPA provides ZFS 2.4.0 packages: https://launchpad.net/~arter97/+archive/ubuntu/zfs
sudo add-apt-repository ppa:arter97/zfs
sudo apt update
sudo apt install zfsutils-linux zfs-dkms
# Version installed: 2.4.0-0arter97~ubuntu25.10.1

# Reboot to load new kernel module
sudo reboot

# After reboot, verify:
zfs --version  # Should show zfs-2.4.0

# Upgrade pool features after ZFS upgrade
sudo zpool upgrade <poolname>
```

### Implementation Steps

1. **Create dedicated ZFS dataset**
   ```bash
   zfs create -o dedup=on -o compression=lz4 prod/workspaces
   ```

2. **Migrate data** (one-time, during maintenance window)
   ```bash
   # Stop sandbox
   docker compose -f docker-compose.dev.yaml stop sandbox-nvidia

   # Copy data
   rsync -av /var/lib/docker/volumes/helix_sandbox-data/_data/workspaces/ /prod/workspaces/

   # Verify
   diff -r /var/lib/docker/volumes/helix_sandbox-data/_data/workspaces/ /prod/workspaces/

   # Remove old data and create bind mount
   rm -rf /var/lib/docker/volumes/helix_sandbox-data/_data/workspaces
   mkdir /var/lib/docker/volumes/helix_sandbox-data/_data/workspaces
   mount --bind /prod/workspaces /var/lib/docker/volumes/helix_sandbox-data/_data/workspaces

   # Add to /etc/fstab for persistence
   echo "/prod/workspaces /var/lib/docker/volumes/helix_sandbox-data/_data/workspaces none bind 0 0" >> /etc/fstab
   ```

3. **Monitor dedup ratio**
   ```bash
   zfs get dedupratio prod/workspaces
   ```

### Expected Results (Updated with Actual Test Data)

With 191 node_modules directories (many identical 66MB), and 461 .git directories (clones of same repos):

| Metric | Before | After (actual from test) |
|--------|--------|--------------------------|
| Workspace storage | 410GB | **~23GB** (extrapolated) |
| Dedup ratio | 1.0x | **17.89x** |
| Compression ratio | 1.0x | 2.28x |
| RAM for DDT | 0 | ~180MB (48MB in-core) |

**Note:** Original estimates were conservative. Actual dedup is 6x better than predicted due to high duplication of .zed-state and node_modules across workspaces.

### Memory Considerations

**Current system RAM: 503GB** (323GB available)

For 150GB unique data with dedup:
- DDT size: ~1-2GB
- ARC recommendation: 8GB minimum for this workload
- **Conclusion: RAM is NOT a concern** - we have 160x the required memory

### Risks and Mitigations

| Risk | Mitigation |
|------|------------|
| DDT memory pressure | Small dataset (150GB unique), minimal DDT |
| Slow snapshot deletion | Don't use snapshots on this dataset initially |
| Performance degradation | Monitor `zpool iostat`, can disable dedup if needed |
| Bind mount complexity | Document in runbook, add to fstab |

## Alternatives Considered

### 1. ZFS Block Cloning (OpenZFS 2.2+)
- Newer feature, explicit clone-on-copy semantics
- Requires application changes to use `cp --reflink`
- Not automatic like dedup

### 2. Hardlinks for node_modules
- pnpm-style content-addressable storage
- Would require modifying how workspaces are created
- More invasive change

### 3. Shared node_modules volume
- Mount common dependencies read-only
- Breaks workspace isolation
- Complex to maintain

### 4. Just delete old workspaces
- 43 "done" task workspaces = 64GB immediate savings
- Doesn't address ongoing growth
- Should do this anyway

## Recommended Approach

1. **Immediate**: Delete 43 "done" spec-task workspaces (64GB savings)
2. **Short-term**: Create prod/workspaces ZFS dataset with dedup for new workspaces
3. **Migrate**: Move existing workspaces to new dataset
4. **Monitor**: Track dedup ratio and performance for 2 weeks
5. **Evaluate**: Disable dedup if performance issues arise

## Open Questions

1. ~~How much RAM is available on this system?~~ **Answered: 503GB - not a concern**
2. Should we also enable dedup on helix_sandbox-docker-storage (91GB)?
3. What's the workspace retention policy? (Currently keeping all workspaces indefinitely)
4. Should spec-task workspace cleanup be automated based on task status?

## Appendix: Disk Usage Breakdown

```
/var/lib/docker (1.2TB total)
├── overlay2/                    576GB  (images, layers, build cache)
├── volumes/                     599GB
│   ├── helix_sandbox-data/      427GB
│   │   └── workspaces/
│   │       ├── spec-tasks/      410GB  <- TARGET FOR DEDUP
│   │       └── sessions/         18GB
│   ├── helix_sandbox-docker-storage/  91GB
│   ├── helix_wolf-docker-storage/     24GB
│   ├── helix_go-build-cache/          16GB  <- DO NOT TOUCH
│   └── other volumes/                 ~41GB
└── buildkit/                     1.2GB
```
