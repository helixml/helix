# Golden snapshots lose the BuildKit cache (2026-10-01)

## Symptom (meta/node01)

A spec task cloned from `golden-prj_01kg02vqqyg178c1n2ydscn5fb@gen111` 10 minutes
after promotion re-ran `./stack build-zed release` from scratch: the first
stage's `apt-get install` re-ran, cargo re-fetched 1.8 GB of git deps, and every
`exec.cachemount` record in `docker buildx du` was 0B.

## Root causes (each reproduced in isolation)

Reproduced with an isolated dockerd on a copy of a golden-like data root:
multi-stage Dockerfile, `RUN --mount=type=cache` (84 MB), 125.8 MB of build cache.

| Step | Effect on a clone | Verdict |
|---|---|---|
| Plain copy of the data root (control) | first stage `CACHED`, cache mount has 1 entry after a source change | copy is fine |
| In-container golden cleanup: `docker builder prune -f --filter unused-for=0s` | reclaimed all 125.8 MB; after a one-line source change the first stage `RUN` re-ran and the cache mount was empty | **cause 1** |
| Hydra `pruneUnreferencedOverlay2Layers` | deleted 8 overlay2 dirs = 125.8 MB = every BuildKit snapshot; `buildkit/*.db` still listed the records; build failed `stat …/overlay2/<id>: no such file or directory` | **cause 2** |
| dockerd default builder GC (no `builder.gc` in daemon.json) | rule #0 evicts `exec.cachemount`/`source.local`/git checkouts unused for 48h once over 6.4 GiB; GC runs ~1s after dockerd start. With the rule scaled to 30s/1MB, a 15-minute-old golden copy lost its cache mount within 5s of boot | **cause 3** |

Unchanged rebuilds can still look fully `CACHED` after cause 1: when the final
image survives, BuildKit satisfies the last vertex from it and never loads the
intermediate snapshots. Any source change exposes the loss. A single-stage build
also hides it: its intermediate layers are the image's layers.

`containerd/` (24K runtime state) and `buildx/` (not a dockerd directory) contain
no BuildKit state. `layerdb/mounts` entries are created only by containers (zero
after BuildKit builds, one per `docker run`); this desktop, itself a clone of the
outer golden, carried 1090 of them for 0 containers — RW-layer records leaked by
the old prune.

## Fix

- `helix-workspace-setup.sh` (golden mode): no wholesale build-cache prune. Apply
  dockerd's own GC thresholds synchronously (`docker builder prune
  --reserved-space/--max-used-space/--min-free-space` from daemon.json): LRU, so
  what the golden build used survives and the golden is bounded.
- `17-start-dockerd.sh`: explicit size-only `builder.gc` policy as fractions of
  the `/var/lib/docker` filesystem (the zvol): reserved 10%, max used 25%, min
  free 10%; no `keepDuration` rules, because a snapshot freezes "last used". The
  golden applies the same thresholds, so it never carries cache a session's GC
  would evict.
- Hydra `purgeContainerDirs`: the layerdb-allowlist overlay2 prune is gone.
  Instead it deletes exactly the purged containers' RW/init layers listed in
  `layerdb/mounts/<id>/{mount-id,init-id}` plus those records. Teaching the old
  prune BuildKit's references would mean parsing moby's internal bbolt schema;
  deleting data behind a live database is what caused the corruption above.
- Instrumentation: `helix-buildkit-cache-stats` (record count and total from
  the Engine API `/system/df`). Cache mount bytes are NOT taken from
  `/system/df`: BuildKit measures a mutable snapshot once and caches the size
  (`size` in `buildkit/snapshots.db`) forever, so later builds writing into the
  mount never change it (2026-10-09: the Zed target mount reported
  8,942,972,009 bytes in every golden from gen112 to gen129). Each cache mount
  is resolved record ID → `metadata_v2.db` `_main/<id>/cache.snapshot` →
  `snapshots.db` `<key>/committed` (else the key) → `overlay2/<id>/diff` and
  measured on disk (`bytes`, as `du -sb`; `last_written`, newest mtime). The golden build writes `build_end` and `pre_snapshot` to
  `/var/lib/docker/.golden-buildkit-stats.json`; Hydra adds them to
  `GOLDEN_BUILD_SUMMARY` (`buildkit_build_end_bytes`, `buildkit_golden_bytes`,
  `buildkit_golden_cache_mount_bytes`, …); every session logs
  `[buildkit-cache] session:` vs `golden:` ~10s after dockerd starts.

## COPY cache keys and mtime

Two `git clone`s of the same commit (all mtimes different, one file touched to
2001): `COPY . .` and the following `RUN` were `CACHED`. Changing a file mode
invalidated them. BuildKit's content hash ignores mtime. But if `.git` is in the
build context, `COPY . .` misses on every fresh clone because `.git/` differs —
projects should exclude `.git` via `.dockerignore`. No mtime handling is needed
for a full cache hit.

### Partial changes: clone mtimes (2026-10-09)

When any file differs, the `RUN` re-executes against the cache mount, and
mtime-based tools (cargo, make) compare source mtimes with the cached outputs.
Fresh clones gave every file the clone time, so one changed file rebuilt every
workspace crate. `helix-workspace-setup.sh` now runs `helix-git-mtime restore`
on repos it cloned, after the branch checkout and before Zed starts: each tracked
file gets the committer time of the last commit touching it (one `git log` walk
per repo, repos in parallel).

Commit times alone are unsafe: a PR merged after the golden build keeps its
older committer time, so its files look older than the golden's outputs and
cargo reuses stale output (reproduced: app printed the pre-change value). So the
golden build `record`s every repo's HEAD, its dirty files and `built_at` in
`/var/lib/docker/.golden-git-checkout.json`; sessions raise every file that
differs from that checkout to `built_at + 1`. A golden without the record (older
image) leaves clone mtimes in place.

Not solved: BuildKit reuses a `COPY` layer by content, with the mtimes of
whichever build first created it. A session whose tree matches an older,
still-cached layer gets that layer's mtimes even if newer outputs are in the
cache mount. This predates the mtime change and applies equally to clone times.

Measured (inner Helix, real desktops): clones 16–44s; mtime step 2–3s
(zed 39.5k commits/4.3k files 2.3s, helix 30k commits 1.5s).

## Live acceptance (inner Helix, file-copy golden path)

The inner sandbox can see the outer node's `prod` pool via `/dev/zfs`, so
`CONTAINER_DOCKER_PATH` was backed by a loop-mounted ext4 image to force the
file-copy golden path rather than create zvols in the production pool. Both
paths share `purgeContainerDirs` and the in-container cleanup; a ZFS clone is a
block-exact copy.

| | P1 multi-stage + 2 cache mounts | P2 plain |
|---|---|---|
| golden build_end BuildKit | 396.6 MB, mounts 90.6 MB | 24.9 MB |
| golden pre_snapshot | 396.6 MB, mounts 90.6 MB | 24.9 MB |
| session at boot | 396.6 MB, mounts 90.6 MB | 24.9 MB |
| unchanged rebuild in session | every step `CACHED` | every step `CACHED` |
| one-line change | `COPY . .` + `go build` re-run, 860 go-build entries reused; toolchain + `go mod download` `CACHED` | `COPY . .` + last `RUN` re-run; apt + pip `CACHED` |
| data root, new pipeline | 628.8 MB | 146.6 MB |
| data root, old pipeline replayed | 12.7 MB (cache mounts 0) | 146.5 MB |
