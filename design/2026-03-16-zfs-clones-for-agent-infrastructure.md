# ZFS Clones Saved My Agent Infrastructure (And Dedup Nearly Killed It)

*Or: how I accidentally built a system that writes 100GB to an NVMe every time a user clicks "Start Session"*

---

I've been building [Helix](https://github.com/helixml/helix), an open-source platform for running AI coding agents in isolated desktop environments. Each agent gets its own VM-like container with a full Linux desktop, Docker-in-Docker, Zed IDE, the works. Think cloud dev environments, except the primary user is an AI with tool access, not a human.

It works. Users love it. The architecture is solid.

The storage, however, has been quietly trying to kill me.

## The Setup

Each agent session needs its own Docker data directory (`/var/lib/docker`). These are big — 60-100GB each, stuffed with overlay2 layers from whatever the agent built. We have a "golden cache" system: when a project's first build completes, we snapshot that Docker data directory. Every subsequent session starts from a copy of the golden cache, pre-loaded with all the base images and build cache. Cold start → warm start.

The copy uses `cp -a --reflink=auto` on XFS. XFS reflinks are great — they share the file *data* blocks copy-on-write, so we're not actually copying 60GB of data. We're just creating new inodes and directory entries that point to the shared data blocks.

"Just" creating new inodes. 1.3 million of them. Plus 198,000 directory entries. Plus 48,000 symlinks.

Here is where I tell you about the machine this runs on. Single NVMe, 3.6TB. ZFS pool. One big 2TB zvol with XFS on it. `dedup=on`.

If you winced at that last part, you know where this is going.

## The Numbers That Made Me Sad

I finally sat down and measured what actually happens when a session starts. I had been assuming the golden copy was "mostly instant" because reflinks. I was wrong.

| What | How bad |
|------|---------|
| Golden cache size | 59GB, 1.29 million files |
| `cp -a --reflink=auto` (warm cache) | **5 minutes 48 seconds** |
| sys time during copy | 2 min 53 sec |
| Single-file reflink (413MB) | 3ms |

Three milliseconds to reflink a 413MB file. Five minutes and forty-eight seconds to reflink a directory tree with 1.3 million files. The reflinks work perfectly — the *data* is shared. The problem is the *metadata*. Every file in that tree needs a new inode allocated in XFS, a new directory entry created, xattrs copied. It doesn't matter that the data blocks are shared. You still have to create 1.5 million filesystem objects.

But that's not even the bad part.

## The DDT Tax

Let me tell you about the ZFS Dedup Table. Better yet, let me show you what my NVMe does during a golden copy:

```
14:40:20  nvme2n1: w=150 MB/s    zd16: w=143 MB/s   (copying)
14:40:30  nvme2n1: w=140 MB/s    zd16: w=148 MB/s
14:40:40  nvme2n1: w=150 MB/s    zd16: w= 69 MB/s
14:40:51  nvme2n1: w=400 MB/s    zd16: w=  0 MB/s   ← copy DONE
14:41:01  nvme2n1: w=230 MB/s    zd16: w=  0 MB/s   ← still writing?
14:41:12  nvme2n1: w=250 MB/s    zd16: w=  0 MB/s   ← STILL writing??
...
14:43:00  nvme2n1: w=  3 MB/s, util=56%              ← 3 MINUTES later
14:44:00  nvme2n1: w=  4 MB/s, util=81%
14:45:00  nvme2n1: w=  2 MB/s, util=66%              ← STILL not done
```

The copy finishes in 40 seconds. Then the NVMe keeps writing at 200-400 MB/s for *three more minutes*. Nothing in userspace is doing I/O — `pidstat` shows zero. It's all kernel-level ZFS.

This is the DDT. With `dedup=on`, every block written to the zvol goes through this pipeline:

1. Compute a cryptographic hash of the block
2. Look up the hash in the dedup table (15.5GB on disk, 12GB pinned in RAM)
3. If it matches an existing block: increment a reference counter (small write)
4. If it's new: write the block + insert a new DDT entry (two writes)
5. Every 5 seconds, flush the accumulated DDT changes to disk

The XFS metadata blocks created by the reflink copy are all unique — they contain session-specific inode numbers, timestamps, UUIDs. None of them deduplicate. Every single one creates a new DDT entry. Zero dedup benefit, 100% overhead.

I measured 4-6x write amplification. A golden copy that writes ~20GB of metadata to the XFS zvol causes ~100GB of actual I/O to the NVMe.

And here's the kicker: during that 3-minute DDT flush, the zvol's write latency spikes from ~1ms to **76ms**. The DDT I/O and the container's I/O are on the same physical NVMe. Your `docker build` is doing 76ms writes while ZFS catches up on bookkeeping from a copy that already finished.

## But You Need Dedup, Right?

This is the trap. The dedup ratio on this pool is 3.92x. That's genuinely good! We have 112 sessions, each with a copy of the Docker data directory. Without dedup, that's 112 × 80GB = ~9TB of data on a 3.6TB NVMe. With dedup, it's ~2.3TB. Dedup is literally the only reason this fits on disk.

The DDT keeping track of all those shared blocks: 45.8 million entries. 15.5GB on disk. 12GB pinned in RAM that can never be swapped. On a 500GB machine.

And the slab caches. Oh, the slab caches.

```
xfs_inode:   143 million objects   → 136 GB
dentry:      147 million objects   →  27 GB
Slab total:                          209 GB / 500 GB RAM
```

One XFS filesystem instance is caching the inodes for every file across all 112 sessions. 143 million inodes sitting in kernel memory. Plus the DDT. Plus the ARC.

That's 209GB of kernel slab on a 500GB machine. Swap is 75% used. The machine is not happy.

## The Realization

I was looking at this, wondering if I should just throw more NVMe at it, when it hit me:

ZFS already has a mechanism for instant copy-on-write clones that share blocks without a dedup table. It's called... clones. `zfs snapshot` + `zfs clone`. That's literally what ZFS is famous for. And I was using dedup instead.

The dedup was compensating for a bad architecture. I was copying Docker data directories — millions of files on XFS — and relying on ZFS block-level dedup to share the duplicate blocks. But ZFS clones share blocks through *snapshot references*. No DDT lookup. No hash computation. No write amplification. The clone is a single metadata operation regardless of how many files are in the tree.

The problem: Docker overlay2 doesn't run on ZFS datasets. It needs a "real" filesystem — ext4 or XFS. So I can't just use ZFS datasets for Docker data.

The solution: zvols. One zvol per session, XFS formatted, cloned from a golden snapshot.

```
prod/golden-prj_xxx          ← zvol, 500G thin-provisioned, dedup=OFF, XFS
  └── @gen42                 ← snapshot after golden build
       └── clone: prod/ses-ses_yyy   ← instant, O(1)
            └── mount, bind into container as /var/lib/docker
```

Clone time: **sub-second**. Not 5 minutes 48 seconds. Not even 48 seconds. Sub-second.

No DDT entries created. No DDT tail writes. No 76ms latency spikes. No 100GB of write amplification. No 143 million cached inodes in one slab (each session gets its own XFS instance now).

And the zvols are created with `dedup=off`. Because clones share blocks through the snapshot, not through the DDT. You get the same space savings — actually better — with none of the overhead.

## Wait, Doesn't This Mean Multiple XFS Instances?

Yes. And that's actually *better*. Right now, one XFS filesystem caches 143 million inodes across all sessions. With per-session zvols, each XFS instance only caches its own inodes. When a session is destroyed (`zfs destroy` — instant, no `rm -rf` traversing millions of files), its entire slab allocation vanishes with it.

## The Migration

The tricky part is migrating a running system. I can't just delete the 2TB zvol with 112 active sessions on it. The approach:

**When ZFS is available** (not all deployments have ZFS — our SaaS runs on plain Linux VMs):

1. First session after deploy: if no golden zvol exists but old golden dir does, migrate inline. Create the golden zvol, copy old golden dir into it (~5 min, one-time), snapshot, then clone for the session. Concurrent sessions block on a mutex and get instant clones once the first one finishes.

2. All subsequent sessions: instant clone from the golden zvol snapshot.

3. GC cleans up old file-based golden dirs after migration.

**When ZFS is not available**: existing file-copy path, unchanged. Zero behavior change for deployments without ZFS.

The dedup stays `on` for workspace data (git repos etc.) where genuine cross-session duplication exists. It goes `off` only on the Docker data zvols where clones already provide block sharing.

## The Dedup Table Is Still Garbage Though

I want to be clear about something. ZFS dedup — even the "fast dedup" (BRT-based, `feature@fast_dedup`) in 2.4.0 — is still, in my considered opinion after running it in production for months, a loaded footgun.

The theory is beautiful. Automatic block-level deduplication! Space savings! The practice:

- **12GB of permanently pinned RAM** for the DDT. Not pageable. Not shrinkable. Just... there. On a 500GB machine.
- **4-6x write amplification** on every write, because every block needs a DDT hash lookup and potentially a DDT update.
- **Minutes of tail writes** after operations complete, because the TXG sync flushes accumulated DDT changes.
- **76ms zvol latency** during DDT flush, destroying the performance of everything else on the same pool.

The fast dedup is faster than the old dedup. It's still slow. The BRT (Block Reference Table) replaced the old ZAP-based DDT, and it's genuinely better — but "better than terrible" isn't "good". Every write still goes through a hash-lookup-and-maybe-insert pipeline that adds latency you can measure with `iostat`.

For our use case — N copies of essentially the same Docker data directory — dedup works brilliantly *for storage efficiency* and terribly *for I/O performance*. Clones give you the same storage efficiency with none of the I/O overhead, because the sharing is structural (snapshot references) rather than content-addressed (hash table lookups).

## Why This Matters For Agent Infrastructure

AI coding agents are different from human developers in one critical way: they spin up sessions constantly. A human might start a dev environment once in the morning and use it all day. An agent might start 10 sessions in an hour, each running a different experiment or debugging approach.

Every session start was a 6-minute I/O storm. 5 minutes of golden copy plus 3 minutes of DDT tail writes (overlapping). During that time, every other session on the machine gets 76ms writes instead of 1ms writes. Scale this to 10 concurrent sessions and the machine is spending more time on golden copies and DDT flushes than on actual work.

With zvol clones: session start is sub-second. No DDT involvement. No write amplification. The NVMe is available for actual container workloads instead of shuffling metadata.

## The Uncomfortable Truth About "Simple" Storage

I spent months running a system where the storage architecture was quietly making everything 10x slower than it needed to be. The reflinks were "working" — no errors, data was shared, everything was correct. The dedup was "working" — 3.92x ratio, exactly as advertised. Every individual component was doing its job.

The interaction between them was a disaster. XFS reflinks eliminated data copies but still created millions of metadata objects. ZFS dedup faithfully hashed and indexed every single metadata block, creating 45 million DDT entries that consumed 12GB of RAM and caused 4-6x write amplification. Each layer was locally optimal and globally catastrophic.

The fix isn't a new technology. ZFS clones have existed since 2005. The fix is using the right mechanism for the job: structural sharing (clones) instead of content-addressed sharing (dedup) for data that you *know* is a copy.

Sometimes the obvious thing is obvious for a reason.

---

*The implementation is at [github.com/helixml/helix](https://github.com/helixml/helix) on the `feature/zfs-clone-golden-cache` branch. 31 unit tests with mocked ZFS commands, because I'm not deploying this to production until I've convinced myself the promotion and crash-recovery paths actually work. The design doc with measured I/O profiles and flow diagrams is in `design/2026-03-16-zfs-clone-golden-cache.md`.*

*I'm Luke, I build [Helix](https://helix.ml). We make open-source AI coding agents that run in isolated desktop environments. If your agents need somewhere to live, come talk to us.*
