# Golden Cache Storage Architecture: ZFS Clone Options

**Date**: 2026-03-16
**Status**: Design exploration (no code changes yet)

## Problem Statement

The golden cache copy is a major bottleneck. When a session starts, we `cp -a --reflink=auto` the entire golden Docker data directory (~30-60GB) into a per-session directory. XFS reflinks are enabled and work on our zvol+XFS setup — the file *data* is shared copy-on-write at the XFS level. But reflink only eliminates the data copy; we still have to create every inode, directory entry, and xattr for every file in the tree. With millions of files in overlay2, that's millions of inode allocations and dentry creations, which still takes 30-60+ seconds and generates heavy metadata I/O.

The current storage architecture also has severe secondary effects:

- **ZFS dedup DDT consumes 12.1GB in-core** (44.5M entries, 15.3GB on-disk). Every write requires a DDT lookup, adding latency to an NVMe that should do <1ms.
- **XFS inode slab: 90GB** (88.5M inodes cached in kernel). Each session's overlay2 has millions of files; XFS caches all their inodes.
- **Dentry slab: 26GB** (136M entries). Same cause — millions of paths across sessions.
- **ext4 inode slab: 10.7GB** (from host Docker on the other zvol).
- **Total kernel slab: 278GB** on a 500GB machine. 113GB unreclaimable.
- **iowait currently at 44%**, swap 99% used (8.3GB/8.4GB).
- The dedup *is* useful here — we're getting 3.97x ratio because we copy entire Docker data directories and many sessions share the same base images. The problem is that dedup's memory overhead and write-path latency are killing performance.

**Goal**: Near-instant session startup from golden cache, without the dedup memory tax, while maintaining compatibility with non-ZFS deployments.

## Current Architecture

```
ZFS pool "prod" (nvme2n1, 3.6TB)
  └─ prod/container-docker (zvol, 2TB, dedup=on, compression=lz4)
       └─ /dev/zd16 → mkfs.xfs → mount /prod/container-docker
            ├─ golden/{projectID}/docker/     ← golden snapshot (full /var/lib/docker copy)
            └─ sessions/docker-data-{sesID}/docker/  ← per-session copy of golden
```

Session startup: `cp -a --reflink=auto golden/prj_xxx/docker/ → sessions/docker-data-ses_xxx/docker/`

XFS reflinks work here — the file data blocks are shared CoW at the XFS level, so we're not doing full data copies. But the copy still creates every inode and directory entry in the tree (millions of files across overlay2 layers). This metadata creation is what takes 30-60s and generates heavy I/O, compounded by the dedup write-path overhead (every new metadata block triggers a DDT hash lookup).

ZFS dedup on top of this provides additional block-level sharing (3.97x ratio) — catching cross-session and cross-project duplicates that XFS reflink can't see (reflink only shares within a single cp operation). But the DDT costs 12GB of pinned RAM and adds latency to every write.

## Constraints

1. **Live migration** — must work on this running machine without downtime
2. **Non-ZFS fallback** — cloud VMs without ZFS, bare-metal Linux with ext4, etc. must still work. (The Mac app VM does have ZFS — `init-zfs-pool.sh` creates a pool with zvols for container-docker storage.)
3. **Docker overlay2 needs a real filesystem** — Docker's overlay2 driver cannot run directly on a ZFS dataset (it needs ext4 or XFS as backing). So we can't just use ZFS datasets.
4. **Dedup stays on** — the user wants to keep dedup because without it, N copies of Docker data dirs would use N * 30GB of actual disk. The problem is the copy *operation*, not the dedup *storage*.
5. **Hydra runs inside sandbox container** — it doesn't have direct ZFS access. ZFS commands must run on the host (or the sandbox needs ZFS privileges).

## Options

### Option A: ZFS zvol clone (snapshot golden zvol, clone per-session)

Instead of one big zvol with XFS holding everything, use **per-project golden zvols** and **clone them per-session**.

```
prod/container-docker/golden/prj_xxx     (zvol, 100GB sparse, ext4)
  └─ snapshot: @gen42
       └─ clone: prod/container-docker/sessions/ses_yyy  (zvol)
            └─ /dev/zvol/prod/container-docker/sessions/ses_yyy → mount → bind into container
```

**How it works**:
1. Golden build completes → unmount golden zvol → `zfs snapshot prod/container-docker/golden/prj_xxx@gen42`
2. New session → `zfs clone prod/container-docker/golden/prj_xxx@gen42 prod/container-docker/sessions/ses_yyy` → mount the clone's block device → bind mount into container
3. Clone is instant (metadata-only). Writes go to the clone; reads fall through to snapshot.
4. Session ends → unmount → `zfs destroy prod/container-docker/sessions/ses_yyy`

**Pros**:
- Clone is O(1) — instant regardless of golden size
- CoW at ZFS level, no double filesystem overhead
- Dedup still works if desired (blocks written by sessions that match golden blocks get deduped), but we get the space savings from cloning *without* needing dedup for the common case
- Could eventually turn off dedup — clones already share blocks with the snapshot, dedup is only needed for cross-project or cross-session sharing
- Each session has its own zvol/filesystem — no slab explosion from millions of overlay2 inodes sharing one XFS instance
- Clean lifecycle: `zfs destroy` is instant and thorough

**Cons**:
- Zvol device management — need to create/mount/unmount/destroy zvol devices
- **Privilege escalation**: `zfs clone`, `zfs destroy`, `mount`, `umount` all require root. Hydra runs inside the sandbox container. Options:
  - Give sandbox `--privileged` (already the case for DinD)
  - Use a host-side helper (systemd unit, socket-activated service, or sidecar)
  - ZFS delegation (`zfs allow`) — can delegate `clone`, `create`, `destroy`, `mount`, `snapshot` to a user. But zvol mount still needs root.
- Snapshot dependency chain: can't destroy `@gen42` while clones exist. Need to track and destroy all session clones before promoting a new golden. Or use `zfs promote` to break the dependency.
- More zvol devices = more `/dev/zd*` entries. Linux default is 256 zvols max (`volmode=default` uses 16 minor numbers per zvol). With 100+ sessions this could hit limits. Fix: increase `zvol_max_disks` or use `volmode=dev`.
- Need to format each new golden zvol with a filesystem — adds ~1s per golden build.
- Golden promotion is more complex: current code does `os.Rename()`, but now we need `zfs rename` or `zfs promote`.

**Filesystem choice for zvols**: ext4 is simpler (no reflink confusion). XFS works too. Since the CoW happens at the ZFS layer, the filesystem choice doesn't matter for clone performance.

### Option B: ZFS dataset clone with Docker ZFS storage driver

Switch inner dockerd from overlay2 to Docker's native ZFS storage driver. The golden and session data live on ZFS datasets (not zvols), and Docker manages its own clones/snapshots.

```
prod/container-docker/golden/prj_xxx/    (ZFS dataset, mounted)
  └─ Docker uses zfs storage driver internally
       └─ Each layer = ZFS dataset, images = snapshots+clones
```

**How it works**:
1. Golden build uses inner dockerd with `--storage-driver=zfs` on a ZFS dataset
2. Golden promotion = `zfs snapshot` + metadata
3. New session = `zfs clone` of the golden dataset
4. Inner dockerd on session starts with the cloned dataset, all Docker images/layers already present as ZFS clones

**Pros**:
- Most "pure" approach — Docker manages its own ZFS clones per-layer
- No ext4/XFS layer at all
- Best space efficiency — sharing at the Docker layer level

**Cons**:
- **Requires ZFS inside the container** — the sandbox container needs access to ZFS commands *and* the ZFS kernel module. This is a significant privilege escalation.
- Docker's ZFS storage driver is less tested/optimized than overlay2 in production. Known issues with performance on high-churn workloads.
- Breaking change for non-ZFS deployments — would need to maintain two completely different Docker storage configurations.
- ZFS datasets can't be easily "exported" or moved between pools.
- The golden cache is no longer a portable directory — it's a ZFS dataset hierarchy, making backup/restore harder.
- **Nested ZFS**: The sandbox already runs DinD. Adding ZFS into the mix means the inner dockerd needs ZFS access, which means the sandbox needs to pass through `/dev/zfs` and ZFS dataset delegation to the inner container. This is fragile.

**Verdict**: Too invasive. The overlay2 driver works well; the problem is the *copy*, not the storage driver.

### Option C: ZFS zvol clone with host-side helper daemon

Same as Option A, but solves the privilege problem with a small host-side daemon.

```
Host: helix-zvol-manager (systemd service, runs as root)
  ├─ listens on unix socket at /run/helix-zvol-manager.sock
  ├─ API: clone(golden_zvol, session_name) → mount_path
  ├─ API: destroy(session_name)
  ├─ API: snapshot(golden_zvol, generation)
  └─ API: promote(session_zvol, golden_zvol)

Sandbox container:
  ├─ bind mount: /run/helix-zvol-manager.sock
  └─ Hydra calls helper via HTTP-over-unix-socket
```

**How it works**:
1. `helix-zvol-manager` is a minimal Go binary that accepts clone/destroy/snapshot commands
2. It validates requests (only operates on `prod/container-docker/*` paths) to prevent abuse
3. Returns the mount path for the cloned zvol
4. Hydra code in `golden.go` calls the helper instead of `cp -a`

**Pros**:
- Clean privilege separation — sandbox doesn't need ZFS access
- Helper is simple (~200 lines of Go), auditable, and only exposes limited operations
- Can be deployed alongside the stack via docker-compose or systemd
- Same instant-clone benefits as Option A
- Easy to make optional: if socket doesn't exist, fall back to cp

**Cons**:
- Another moving part to deploy and manage
- Mac app VM deployments would need this daemon installed (though the VM already has ZFS, so Option D is simpler there)
- Slightly more complex than just giving the sandbox ZFS access

### Option D: Hybrid — privileged sandbox with ZFS delegation

The sandbox container already runs `--privileged` for DinD. ZFS commands work in privileged containers if `/dev/zfs` is accessible and the ZFS kernel module is loaded.

```
Sandbox container (--privileged):
  ├─ /dev/zfs accessible (already the case with --privileged)
  ├─ zfs, zpool binaries installed
  └─ Hydra calls zfs directly: zfs clone, zfs destroy, mount, umount
```

**How it works**:
1. Install `zfsutils-linux` in the sandbox image
2. Hydra's `golden.go` detects ZFS availability at startup: try `zfs list` — if it works, use ZFS clone path
3. `SetupGoldenCopy` becomes `SetupGoldenClone`: `zfs clone @snapshot → session zvol`, `mount`, return path
4. `CleanupGoldenSession` becomes: `umount`, `zfs destroy`

**Pros**:
- No extra daemon — Hydra does it all
- Sandbox is already `--privileged`, so `/dev/zfs` is already accessible
- Simplest implementation
- Detection at startup means non-ZFS environments automatically fall back

**Cons**:
- Tight coupling — Hydra directly manages ZFS, which is a host-level concern
- If sandbox isn't privileged (future hardening), this breaks
- ZFS commands from inside a container can be surprising (pool is the host's pool)
- Need to install ~20MB of ZFS userspace tools in the sandbox image

### Option E: Loopback files with reflink (no ZFS changes)

Instead of zvol clones, keep the single XFS filesystem but use per-session loopback files.

```
/prod/container-docker/
  ├─ golden/prj_xxx.img     ← sparse file, ext4 formatted, contains golden /var/lib/docker
  └─ sessions/ses_yyy.img   ← cp --reflink=auto of golden .img file
       └─ loop mount → bind into container
```

**How it works**:
1. Golden = a sparse ext4/XFS image file on the XFS filesystem
2. New session = `cp --reflink=auto golden.img sessions/ses_yyy.img` — if XFS reflink works, this is instant
3. `losetup` + `mount` the copy, bind into container

XFS reflinks already work on our zvol — `cp --reflink=auto` shares file data blocks via XFS CoW. But the bottleneck isn't data copying, it's *metadata* creation: each file in the tree needs a new inode, directory entry, and xattr allocation. With millions of overlay2 files, that's still 30-60s of metadata I/O.

Using loopback image files would reduce the problem to a single-file reflink (one `.img` file), making the metadata cost O(1) instead of O(millions). But it adds another layer to an already deep stack.

**Pros**:
- Works today — no ZFS changes needed
- Single-file reflink is genuinely instant (one inode, one reflink op)
- No zvol management complexity

**Cons**:
- Doesn't solve the inode/dentry slab explosion (one XFS instance still caches all sessions' inodes once mounted)
- Doesn't solve the dedup DDT overhead
- Adds a layer: block device → XFS → sparse file → losetup → ext4 → overlay2
- Fixed-size image files (need to pre-allocate or resize)
- Loop device management (losetup/cleanup)

## Recommendation

**Option D (privileged sandbox with direct ZFS) for the fast path, with the existing cp fallback for non-ZFS environments.**

Rationale:
- The sandbox is already `--privileged` and this is unlikely to change (DinD requires it)
- No extra daemon to deploy or manage
- Detection-based fallback means zero changes needed for non-ZFS environments
- Clean mapping to the existing code structure: `SetupGoldenCopy` gets a sibling `SetupGoldenClone`

If we later want to remove `--privileged` from the sandbox, we can migrate to Option C (helper daemon) at that point. The Hydra-side interface would be the same either way.

## Detailed Design for Option D

### ZFS Naming Convention

```
prod/container-docker                        ← parent dataset (not a zvol anymore)
  ├─ prod/container-docker/golden-prj_xxx    ← zvol per project (golden)
  │     └─ @gen42                            ← snapshot after golden build
  └─ prod/container-docker/ses-ses_yyy       ← zvol clone per session
```

Zvol properties:
- `volsize=500G` (sparse/thin-provisioned via `-s` — just a ceiling, actual allocation is thin)
- `dedup=off` — **explicitly disabled on golden/session zvols**. Block sharing comes from ZFS clones (free, no DDT involvement). Dedup adds nothing here because cloned blocks already share via snapshot reference. Disabling dedup on these zvols eliminates DDT hash lookups and write amplification on the docker data path, which is where most of the I/O pain comes from.
- `compression=lz4` (inherited)
- `volblocksize=64K` (larger than default 16K — better throughput for Docker's large sequential overlay2 writes)

**Dedup strategy per dataset**:
| Dataset | dedup | Rationale |
|---------|-------|-----------|
| `golden-prj_*` zvols | `off` | Clones share blocks via snapshot; DDT adds only overhead |
| `ses-*` clone zvols | `off` | Inherited from golden; writes are session-unique anyway |
| Workspace/git data | `on` | Genuine cross-session duplication (same repos cloned N times) |
| Legacy `container-docker` zvol | `on` | Existing; will be drained and decommissioned |

### Lifecycle

**Golden build completes**:
```
1. Inner dockerd stops (existing behavior)
2. umount /dev/zvol/.../golden-prj_xxx (if mounted)
3. zfs destroy prod/container-docker/golden-prj_xxx@current (if exists, after destroying all clones)
   — OR use a rolling scheme: @gen41, @gen42
4. zfs snapshot prod/container-docker/golden-prj_xxx@gen42
5. PurgeContainersFromGolden still runs (mount, purge, umount)
```

**Session starts**:
```
1. zfs clone prod/container-docker/golden-prj_xxx@gen42 \
       prod/container-docker/ses-ses_yyy
2. mount /dev/zvol/prod/container-docker/ses-ses_yyy /container-docker/sessions/ses_yyy/docker
3. Bind mount that path into the desktop container (existing behavior)
```

**Session stops**:
```
1. Stop inner dockerd in container (existing)
2. umount /container-docker/sessions/ses_yyy/docker
3. zfs destroy prod/container-docker/ses-ses_yyy
```

**Session restart (reuse existing clone)**:
```
1. Clone zvol still exists and is mounted → reuse as-is
   (equivalent to current "reuse existing session Docker data dir")
```

### Fallback Chain

```go
func SetupSessionDockerDir(projectID, volumeName string, onProgress ...) (string, error) {
    // Try 1: ZFS clone (instant)
    if zfsAvailable() && goldenSnapshotExists(projectID) {
        path, err := setupGoldenClone(projectID, volumeName)
        if err == nil {
            return path, nil
        }
        log.Warn().Err(err).Msg("ZFS clone failed, falling back to copy")
    }

    // Try 2: File copy with reflink (instant on XFS/btrfs with reflink, slow on ext4)
    if GoldenExists(projectID) {
        return SetupGoldenCopy(projectID, volumeName, onProgress)
    }

    // Try 3: Empty directory (cold start)
    return setupEmptySessionDir(volumeName)
}
```

### Migration Plan (this machine)

The tricky part: we currently have one big zvol (`prod/container-docker`, 2TB) with XFS containing both golden and session dirs. We need to move to per-project zvols.

**Phase 1: Create new structure alongside old**
```bash
# Create parent dataset for the new structure
sudo zfs create prod/container-docker-v2

# Create golden zvol for the active project (dedup=off — clones share blocks natively)
sudo zfs create -V 500G -s -o dedup=off -o compression=lz4 \
    -o volblocksize=64k prod/container-docker-v2/golden-prj_xxx
sudo mkfs.xfs -f -q /dev/zvol/prod/container-docker-v2/golden-prj_xxx
sudo mkdir -p /prod/container-docker-v2/golden-prj_xxx
sudo mount /dev/zvol/prod/container-docker-v2/golden-prj_xxx \
    /prod/container-docker-v2/golden-prj_xxx

# Copy existing golden data
sudo cp -a /prod/container-docker/golden/prj_xxx/docker/* \
    /prod/container-docker-v2/golden-prj_xxx/

# Unmount, snapshot
sudo umount /prod/container-docker-v2/golden-prj_xxx
sudo zfs snapshot prod/container-docker-v2/golden-prj_xxx@gen1
```

**Phase 2: Deploy code that uses new structure**
- Hydra detects `prod/container-docker-v2/golden-prj_xxx@gen1` exists
- New sessions use `zfs clone` from the snapshot
- Old sessions on the old zvol continue to work (existing session dirs still exist)

**Phase 3: Drain old sessions, decommission old zvol**
- Once no sessions reference old `/prod/container-docker/sessions/*`
- `sudo zfs destroy prod/container-docker` (frees the 2TB zvol)

### Mac App / VM Provisioning Changes

`init-zfs-pool.sh` changes:
- Instead of one big `helix/container-docker` zvol, create `helix/container-docker` as a **dataset** (not zvol)
- Golden zvols created on-demand by Hydra when first golden build completes
- If ZFS not available (bare metal Linux without ZFS, cloud VMs), existing cp behavior works unchanged

### Open Questions

1. **volblocksize**: Default 16K gives fine-grained dedup but many DDT entries. 64K or 128K would reduce DDT size by 4-8x at the cost of coarser dedup. Since clones share blocks exactly, dedup is mainly for cross-project sharing. Worth benchmarking.

2. **Dedup on golden/session zvols**: DECIDED — `dedup=off`. Clones share blocks via snapshot reference (no DDT needed). This eliminates the 4-6x write amplification on docker data writes and stops the DDT tail writes that cause 76ms zvol latency spikes. Dedup remains `on` for workspace/git data where genuine cross-session duplication exists.

3. **Snapshot lifecycle**: When golden gen43 is built, we can't destroy `@gen42` until all clones from it are destroyed. Options:
   - Keep old snapshots until all their clones (sessions) are gone. GC cleans up.
   - `zfs promote` one long-running clone to break the dependency. But promote makes the clone independent, losing sharing.
   - Accept multiple snapshots coexisting. ZFS handles this fine; they just consume space for the delta.

4. **Max zvols**: Linux zvol subsystem defaults may limit total zvols. Need to check `/sys/module/zfs/parameters/zvol_max_disks` and increase if needed for many concurrent sessions.

5. **Sandbox image size**: Adding `zfsutils-linux` to the sandbox image adds ~20MB. Acceptable.

## Measured Performance (2026-03-16)

### System State

| Metric | Value | Problem? |
|--------|-------|----------|
| ZFS version | 2.4.0 (arter97 PPA, bleeding edge) | `feature@fast_dedup` active |
| Dedup ratio | 3.97x (44.5M DDT entries, 15.5GB on disk) | Good ratio, high overhead |
| Slab total | 278GB / 500GB RAM (56%) | xfs_inode 90GB, dentry 26GB, DDT+ARC ~50GB |
| SUnreclaim | 113GB | Unpageable kernel memory |
| Swap used | 6-8GB / 8.4GB | Thrashing when under I/O load |
| nvme1n1, nvme3n1 | Idle (3.6TB + 1.8TB unused) | Wasted capacity |

### Golden Cache Copy (59GB, 1.3M files + 198K dirs + 48K symlinks)

| Measurement | Value |
|-------------|-------|
| Single-file reflink (413MB) | **3ms** — reflinks work perfectly |
| Full `cp -a --reflink=auto` (warm cache) | **5 min 48 sec** |
| sys time during copy | 2 min 53 sec (almost all metadata creation) |
| zd16 (XFS zvol) writes during copy | 140-170 MB/s, 10-19K ops/sec |
| nvme2n1 (ZFS pool) writes during copy | 400-800 MB/s |
| Write amplification ratio | **4-6x** (pool writes vs zvol writes) |
| DDT tail writes after copy completes | Continues **3+ minutes** at 30-90% util |

### DDT Write Amplification Explained

The dedup table (DDT) is a hash table mapping block checksums to their locations on disk. With `dedup=on`, every block written to the zvol triggers:

1. **Hash computation** — ZFS checksums the block
2. **DDT lookup** — search the 15.5GB on-disk DDT for a matching hash
3. **If duplicate**: increment refcount in DDT entry (small write)
4. **If unique**: write the block + insert a new DDT entry (two writes)
5. **TXG sync** — every 5s, ZFS flushes accumulated DDT changes as a batch write

Even though XFS reflinks avoid copying file *data* (the data blocks are shared at the XFS level), the copy still creates ~1.5M new filesystem metadata objects (inodes, dentries, xattrs). Each metadata block is a fresh write to the zvol, which goes through the full DDT pipeline. Since these metadata blocks are unique (they contain session-specific inode numbers, timestamps, etc.), they all create new DDT entries — no dedup benefit, just overhead.

The result: a golden copy that writes ~20GB of metadata to the XFS zvol causes ~100GB of I/O to the backing NVMe due to DDT operations. The DDT writes then continue for minutes after the copy completes as ZFS flushes its in-memory DDT log to disk.

### Spectask Startup I/O Profile (measured live)

Started a spectask and traced I/O for 3 minutes:

```
14:40:20  nvme2n1: w=150 MB/s (golden copy + DDT)     zd16: w=143 MB/s
14:40:30  nvme2n1: w=140 MB/s                          zd16: w=148 MB/s
14:40:40  nvme2n1: w=150 MB/s                          zd16: w= 69 MB/s
14:40:51  nvme2n1: w=400 MB/s (DDT catching up)        zd16: w=  0 MB/s (copy done)
14:41:01  nvme2n1: w=230 MB/s (still DDT)              zd16: w=  0 MB/s
14:41:12  nvme2n1: w=250 MB/s                          zd16: w=  0 MB/s
...
14:43:00  nvme2n1: w=  3 MB/s, util=56% (DDT tail)     zd16: w=  0 MB/s
14:44:00  nvme2n1: w=  4 MB/s, util=81%                zd16: w=  0 MB/s
14:45:00  nvme2n1: w=  2 MB/s, util=66%                zd16: w=  0 MB/s
```

Key observations:
- Golden copy itself takes ~30-40s to write to zd16
- DDT writes to nvme2n1 **continue for 3+ minutes** after the copy finishes
- During DDT flush, zd16 w_await spikes to **76ms** (normally <1ms) — the contention between DDT I/O and zvol I/O on the same physical NVMe makes the container's filesystem slow
- No userspace process shows significant I/O in `pidstat` — it's all kernel-level ZFS activity

### Why This Matters for the Build Inside the Container

The build running inside the desktop container writes to zd16 (its inner Docker overlay2 on the XFS zvol). When ZFS is simultaneously flushing DDT entries to nvme2n1, the zvol's write latency spikes 76x (from ~1ms to 76ms). This makes `docker build`, `npm install`, `go build` etc. inside the container feel sluggish even though the container's own I/O is modest.

### What ZFS Clones Would Change

The ZFS clone approach addresses the root cause: we stop copying 30-60GB of metadata per session, which means:
- **Clone is O(1)** — one ZFS metadata operation instead of 1.5M inode/dentry creates
- **No DDT entries created** — cloned blocks share via snapshot reference, not dedup hash
- **No DDT tail writes** — nothing to flush
- **No zvol contention** — container I/O latency stays at ~1ms
- Per-zvol filesystem instances mean smaller inode caches (no 90GB XFS slab)
- Session cleanup is `zfs destroy` (instant) instead of `rm -rf` (traverses millions of inodes)
- Dedup explicitly `off` on golden/session zvols — clones provide the same block sharing without the DDT cost. Dedup stays `on` only for workspace/git data where cross-session duplication is genuine
