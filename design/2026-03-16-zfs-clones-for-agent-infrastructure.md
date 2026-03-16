# I mass-copied Docker data directories on ZFS with dedup on. Here's what happened to my NVMe.

I build [Helix](https://github.com/helixml/helix), a source-available platform for AI coding agents. Each agent gets its own container with a full Linux desktop, Docker-in-Docker, Zed IDE, the lot. Think cloud dev environments but the primary user is an LLM with tool access, not a human with a keyboard.

It works well. We're shipping it. The bit I'm writing about today nearly brought the whole thing to its knees and it's entirely my fault.

## what we do with docker data directories

Every agent session needs its own `/var/lib/docker`. These get big — 60-100GB each, overlay2 layers from whatever the agent built. We pre-warm them from a "golden cache": the first successful build gets snapshotted, and every new session starts from a copy. Cold start becomes warm start. Lovely.

The copy uses `cp -a --reflink=auto` on XFS. Reflinks share file data blocks copy-on-write, so we're not actually copying 60GB. We're "just" creating new inodes and directory entries that point to the shared data.

"Just" 1.3 million inodes. 198,000 directory entries. 48,000 symlinks.

The machine: single 3.6TB NVMe, ZFS pool, one 2TB zvol with XFS on it, `dedup=on`.

If you just winced, yeah. Keep reading.

## i measured it and it was worse than i thought

I'd been assuming the golden copy was "mostly instant" because reflinks. I finally sat down with `iostat` and timed it properly.

| what | how bad |
|------|---------|
| golden cache | 59GB, 1.29 million files |
| `cp -a --reflink=auto` (warm cache) | **5 minutes 48 seconds** |
| sys time | 2m53s |
| single-file reflink (413MB) | 3ms |

3ms for a single 413MB reflink. 5 minutes 48 seconds for a directory tree. Reflinks work perfectly for data — the problem is metadata. You still allocate 1.5 million inodes regardless of how the data blocks are shared.

That's bad enough. But here's the `iostat` output during a golden copy:

```
14:40:20  nvme2n1: w=150 MB/s    zd16: w=143 MB/s   (copying)
14:40:30  nvme2n1: w=140 MB/s    zd16: w=148 MB/s
14:40:40  nvme2n1: w=150 MB/s    zd16: w= 69 MB/s
14:40:51  nvme2n1: w=400 MB/s    zd16: w=  0 MB/s   ← copy DONE
14:41:01  nvme2n1: w=230 MB/s    zd16: w=  0 MB/s   ← ...still writing?
14:41:12  nvme2n1: w=250 MB/s    zd16: w=  0 MB/s
...
14:43:00  nvme2n1: w=  3 MB/s, util=56%              ← THREE MINUTES later
14:44:00  nvme2n1: w=  4 MB/s, util=81%
14:45:00  nvme2n1: w=  2 MB/s, util=66%              ← still not done
```

Copy finishes in 40 seconds. NVMe keeps hammering at 200-400 MB/s for three more minutes. `pidstat` shows nothing — no userspace process is doing I/O. It's all in the kernel. ZFS dedup table (DDT) housekeeping.

With `dedup=on`, every block goes through: hash it → look up the hash in a 15.5GB on-disk table (12GB pinned in RAM) → if new, write the block plus a new DDT entry → every 5 seconds, flush accumulated DDT changes.

The XFS metadata blocks from the reflink copy are all unique — session-specific inode numbers, timestamps, UUIDs. None of them deduplicate. Every single one creates a new DDT entry. Zero benefit, 100% overhead.

I measured 4-6x write amplification. ~20GB of metadata writes to the zvol = ~100GB of actual I/O to the NVMe.

And during that 3-minute DDT flush, zvol write latency goes from ~1ms to **76ms**. So your `docker build` inside the container is doing 76ms writes while ZFS catches up on bookkeeping from a copy that already finished.

## but you need dedup, right?

Yeah. That's the trap.

Dedup ratio on this pool: 3.92x. 112 sessions, each with a copy of the docker data dir. Without dedup: 112 × 80GB = ~9TB on a 3.6TB NVMe. Doesn't fit. With dedup: ~2.3TB. Fits. Dedup is literally the reason this works at all.

The DDT keeping track of all that: 45.8 million entries. 15.5GB on disk. 12GB permanently pinned in RAM on a 500GB machine.

And the kernel slab caches:

```
xfs_inode:   143 million objects   → 136 GB
dentry:      147 million objects   →  27 GB
slab total:                          209 GB / 500 GB RAM
```

One XFS filesystem caching every inode across all 112 sessions. 143 million inodes. Plus the DDT. Plus the ARC. 209GB of kernel slab. Swap 75% used.

Not great.

## the bloody obvious thing i should have done from the start

I was sat there staring at `iostat`, genuinely considering buying another NVMe, when it clicked.

ZFS has a mechanism for instant copy-on-write clones that share blocks without a dedup table. It's called `zfs clone`. It's been there since 2005. It's literally the thing ZFS is famous for. And I was using dedup instead, like an idiot.

The dedup was papering over a bad architecture. I was copying millions of files on XFS and relying on ZFS block-level dedup to mop up the duplicate blocks. But ZFS clones share blocks through snapshot references. No hash lookup. No DDT. One metadata operation. Done.

Problem is, Docker overlay2 doesn't work on ZFS datasets — it needs a "real" filesystem, ext4 or XFS. Can't just clone a dataset.

So: zvols. One zvol per session, XFS on each, cloned from a golden snapshot.

```
prod/golden-prj_xxx          ← zvol, 500G sparse, dedup=OFF, XFS
  └── @gen42                 ← snapshot after golden build
       └── clone: prod/ses-ses_yyy   ← instant, O(1)
            └── mount, bind into container as /var/lib/docker
```

Clone time: sub-second. Not 5 minutes 48 seconds. Sub-second. No DDT entries. No tail writes. No 76ms latency spikes. No 100GB of write amplification.

And each session gets its own XFS instance, so when you `zfs destroy` a session (instant — no `rm -rf` traversing a million inodes), its slab allocation vanishes with it. No more 143 million inodes in one shared slab.

The zvols get `dedup=off`. Clones share blocks through the snapshot. The DDT has nothing to contribute here.

## migrating a live system

Can't exactly stop the machine with 112 active sessions on it. So the migration is online:

When ZFS is available (not all our deployments have it — the SaaS runs on plain Linux VMs with no ZFS, and the code just keeps doing file copies there):

1. First session after deploy: no golden zvol exists yet, but old golden dir does. Create golden zvol, copy old golden dir into it (~5 min, one-time), snapshot, clone for session. Concurrent sessions block on a mutex, then get instant clones once the first one's done.
2. Everything after that: instant clones.
3. GC cleans up the old file-based golden dir once the zvol is ready.

If Hydra (our container orchestrator) restarts mid-migration, it detects the partial copy (no completion marker), wipes, starts again. No manual intervention.

## "openzfs dedup is good. don't use it."

There's a [Despair Labs post](https://despairlabs.com/blog/posts/2024-10-27-openzfs-dedup-is-good-dont-use-it/) from 2024 with that title. I read it at the time, nodded, and then used dedup anyway because I thought my workload was different.

My workload was not different.

I'm running ZFS 2.4.0 from the arter97 PPA with `feature@fast_dedup` active, because apparently I'm the kind of person who puts bleeding-edge ZFS kernel modules on a production box. The new BRT-based fast dedup is genuinely better than the old ZAP-based DDT. "Better than terrible" is still "measurably bad" though. You can see the write amplification clear as day in `iostat`. 4-6x on every write. 12GB of pinned RAM you can never get back. Minutes of tail writes after operations finish.

The Despair Labs title was right. I had to learn it myself at 3am watching 400 MB/s of DDT flushes scroll by while a user's build was stuck at 76ms per write. Oh well.

## where dedup actually earns its keep (for now)

I'm not turning dedup off everywhere though. Docker data zvols: `dedup=off`, clones handle it. But workspace directories still have `dedup=on`.

Why? Each agent session does a fresh `git clone` of the user's repo. Not a fetch into a shared bare repo, not a reference clone — a full independent clone. We do it this way because each agent might be on a different branch, the upstream might have force-pushed, and we can't trust a shared object store with agents that are actively committing and pushing to different refs concurrently.

So we've got hundreds of sessions, each with a full clone of the same repos. 500MB-2GB each. That's genuine content duplication across unrelated filesystem paths. No snapshot relationship. The blocks just happen to be identical. This is the one case where DDT actually earns its RAM: catching block-level duplicates across unrelated trees. The workspace volumes are also much smaller and lower-churn than docker data, so the write amplification hurts less.

| dataset | dedup | why |
|---------|-------|-----|
| golden/session zvols (docker data) | `off` | clones share blocks via snapshot. DDT adds only overhead |
| workspace volumes (git repos) | `on` | genuine cross-session duplication. no structural sharing possible |

## ...actually wait, what about golden workspaces?

I was wrapping this post up when the obvious thing hit me.

I just spent three paragraphs explaining that dedup stays on for workspaces because "each session does a fresh git clone, the blocks just happen to be identical, there's no structural relationship."

But why am I doing fresh `git clone` every time?

What if we had a golden workspace too? Pre-clone the repo into a ZFS dataset — not a zvol, just a regular dataset, git doesn't need XFS or ext4 the way Docker does. Snapshot it. Clone per session. Then `git pull` instead of `git clone`.

`git pull` on a repo that's a few commits behind: seconds, not minutes. The ZFS clone shares the entire `.git/objects` directory copy-on-write until the agent writes something new. No DDT needed. Same trick as the docker data, except simpler because regular ZFS datasets don't need the zvol→mkfs→mount dance.

This would let us turn off dedup *entirely*. All 12GB of pinned DDT RAM: gone. All write amplification: gone. The entire pool at native NVMe speed.

Haven't built it yet. But I'm going to.

## anyway

I spent months running a system where every component was doing its job perfectly and the interaction between them was making everything 10x slower than it needed to be. XFS reflinks shared data blocks but still created a million metadata objects. ZFS dedup faithfully hashed and indexed every one of those metadata blocks, burning 100GB of I/O per session and spiking latency 76x.

The fix was a ZFS feature from 2005. Bit embarrassing, honestly.

---

*the implementation is at [github.com/helixml/helix](https://github.com/helixml/helix) on the `feature/zfs-clone-golden-cache` branch. 43 unit tests with mocked ZFS commands and real `cp` operations. design doc with measured I/O profiles and flow diagrams in `design/2026-03-16-zfs-clone-golden-cache.md`.*

*i'm Luke, i build [Helix](https://helix.ml). source-available AI coding agents that run in isolated desktop environments. if your agents need somewhere to live, come talk to us.*
