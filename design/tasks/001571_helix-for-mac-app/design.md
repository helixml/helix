# Design: Incremental VM Disk Updates for Helix for Mac

## Current Architecture (Context)

```
CDN: dl.helix.ml/vm/{VERSION}/disk.qcow2.zst   (~7.8 GB)
                    ↓
            download.go (16 parallel range requests, resume support)
                    ↓
            disk.qcow2.staged  →  disk.qcow2
```

Version = git short hash (e.g. `fa55c96c1`), stored in `vm-manifest.json`.
The disk embeds Ubuntu 25.10 base OS + Docker daemon + all Helix Docker images.

Key files:
- `for-mac/updater.go` — update orchestration (606 lines)
- `for-mac/download.go` — parallel download + zstd decompression (1047 lines)
- `for-mac/scripts/upload-vm-images.sh` — CDN upload + manifest generation
- `for-mac/vm-manifest.json` — version, URL, size, SHA256

---

## Proposed Architecture: Two-Tier Updates

### Tier 1 — Base OS disk (infrequent, ~5–8 GB compressed)

The Ubuntu base + Docker daemon changes rarely (new kernel, Docker version, etc.).
When it does change, we ship a **binary delta patch** from the previous base version to the new one.

### Tier 2 — Helix app layer (frequent, ~500 MB–1.5 GB)

The Helix services (API, Postgres, Redis, streaming, sandbox) run as Docker containers inside the VM. When only these change, we **pull updated Docker images from the Helix container registry** (`registry.helixml.tech`) inside the VM instead of replacing the whole disk.

This pattern already exists: `provision-vm-light.sh` does exactly this. The Mac app just doesn't expose it as the upgrade path yet.

---

## Delta Patching Strategy (Tier 1)

**Tool: `xdelta3`** (or `casync`/`desync` as alternative)

- `xdelta3` is a mature binary diff tool used in OS updates (e.g., ChromeOS, Android OTA)
- Operates on raw/decompressed data; decompress old + new before diff, compress patch with zstd
- Patch size for typical OS updates on a stable base: 200 MB – 2 GB
- Worst case (large OS update): fall back to full download

**Patch naming convention:**
```
vm/{FROM_VERSION}_to_{TO_VERSION}/patch.xdelta3.zst
```

**Extended manifest format:**
```json
{
  "version": "abc1234",
  "base_url": "https://dl.helix.ml/vm",
  "files": [
    {
      "name": "disk.qcow2.zst",
      "size": 7815773469,
      "sha256": "...",
      "compression": "zstd",
      "decompressed_name": "disk.qcow2",
      "decompressed_size": 19857211392
    }
  ],
  "patches": [
    {
      "from_version": "prev1234",
      "name": "patch.xdelta3.zst",
      "size": 450000000,
      "sha256": "...",
      "applies_to_sha256": "...",
      "result_sha256": "..."
    }
  ],
  "docker_only_update": false
}
```

---

## Docker-Only Update Path (Tier 2)

When `docker_only_update: true` in the manifest:

1. Mac app starts the VM (or boots it in a maintenance mode without starting Helix services)
2. Mac app sends a command to run `docker-compose pull && docker-compose up -d` inside the VM via the existing VM exec interface
3. Helix services restart with new images
4. No disk replacement needed; version recorded in settings

This makes service-only upgrades essentially a `docker pull` — much smaller, resumable, and incremental by nature (Docker layer deduplication).

The VM needs a small "update agent" script that the Mac app can invoke. The existing VM exec mechanism (used for other management tasks) can serve this purpose.

---

## Update Decision Logic (Client Side)

```
CheckForUpdate()
  → fetch manifest from CDN
  → if docker_only_update:
       → DockerOnlyUpdate() [Tier 2 path]
  → elif patch available for InstalledVMVersion:
       → verify local disk SHA256 matches patch.applies_to_sha256
       → if match: DownloadAndApplyPatch() [Tier 1 delta path]
       → else: DownloadFullDisk() [fallback]
  → else:
       → DownloadFullDisk() [new install or no patch available]
```

---

## CI / Release Pipeline Changes

In `upload-vm-images.sh`:

1. After uploading new `disk.qcow2.zst`, check if previous version exists in CDN
2. If yes: decompress both, generate xdelta3 patch, compress with zstd, upload to `vm/{FROM}_{TO}/patch.xdelta3.zst`
3. Update manifest with patch metadata
4. Keep patches for last N versions (suggest N=3), prune older ones

The patch generation step takes ~10–30 min extra in CI but is a one-time cost per release.

---

## Alternative Considered: casync/desync

casync (by Lennart Poettering) chunks files by content and stores chunks in a content-addressed store. Clients download only missing chunks. This is more elegant and supports arbitrary version jumps but:
- More complex CDN structure (chunk store vs. simple patch files)
- Requires `casync` or `desync` binary on client
- Less tooling support on macOS

**Decision: Start with xdelta3 patches for simplicity; casync is a future optimization if patch sizes are unsatisfactory.**

---

## Alternative Considered: QCOW2 Backing Files

QEMU supports layered QCOW2 images (base + delta overlay). We could ship only the overlay. However:
- The overlay grows over time with writes (VM writes to the overlay layer)
- Applying a new overlay to a user's disk that has accumulated writes is complex
- Backing file paths are embedded in QCOW2 metadata, making CDN hosting awkward

**Decision: Not suitable for this use case.**

---

## Storage and Space Impact

| Scenario | Current | New |
|----------|---------|-----|
| New install | ~7.8 GB download | ~7.8 GB (unchanged) |
| Docker-only upgrade | ~7.8 GB download | ~200–500 MB (Docker layers) |
| OS upgrade (patch) | ~7.8 GB download | ~500 MB – 2 GB (patch) |
| OS upgrade (no patch) | ~7.8 GB download | ~7.8 GB (fallback) |
| Disk space during update | 2× disk (~40 GB) | 2× disk for delta apply (temporary) |

Patch apply requires enough free space to hold decompressed old disk + patch output simultaneously. The existing disk space preflight check must be updated to account for this.

---

## Key Implementation Notes

- All existing resume, retry, SHA256 verification logic in `download.go` is reusable
- `xdelta3` is invoked as a subprocess (`exec.CommandContext`); no Go binding needed. Client must have `xdelta3` in PATH (brew install xdelta) or bundled in the app. The code falls back to full download if xdelta3 is not available.
- The update UI (Wails frontend events) reuses `update:vm-progress` with a new `applying_patch` phase value — no frontend changes needed for basic support
- Rollback: keep `disk.qcow2.old` logic unchanged; if patch apply fails before `os.Rename`, the old disk is untouched

## Implementation Notes (from coding)

### Files changed

- `for-mac/download.go`: Added `VMManifestPatch` struct; extended `VMManifest` with `DockerOnlyUpdate bool` and `Patches []VMManifestPatch`
- `for-mac/updater.go`: Added `vm *VMManager` field + `SetVMManager()` (mirrors `SetAppContext` pattern); added `dockerOnlyUpdate()`, `downloadAndApplyPatch()`, `findXdelta3()`, `verifyFileSHA256()`, `decompressZstdFile()` private methods; updated `DownloadVMUpdate()` with decision tree
- `for-mac/app.go`: Added `a.updater.SetVMManager(a.vm)` in `startup()`
- `for-mac/vm-manifest.json`: Added `docker_only_update: false` and `patches: []`
- `for-mac/scripts/upload-vm-images.sh`: Added `DOCKER_ONLY_UPDATE`, `SKIP_PATCH`, `PATCH_VERSIONS` env vars; docker-only manifest path (exits early); patch generation loop; patch pruning
- `for-mac/updater_test.go`: Added `TestVMManifestNewFields`, `TestVerifyFileSHA256`, `TestDecompressZstdFile`, `TestDecompressZstdFileCancelled`

### Design decisions during implementation

- **VM-side agent script**: Decided to SSH the docker-compose commands inline rather than baking a separate agent script into the VM image. This is simpler and the VM exec mechanism (`vm.runSSH`) already handles this perfectly.
- **decompressZstdFile**: Added a standalone zstd decompressor (no VMDownloader dependency) because `decompressZstd` is tightly coupled to `VMDownloader.cancel` channel. The new function uses `context.Context` for cancellation, which is cleaner.
- **Patch URL convention**: `{BaseURL}/{FROM}_to_{TO}/{Name}` — simple, self-documenting, no extra manifest field needed for the patch CDN path.
- **Disk space check**: Added `patch.Size + disk.Size + 2GB headroom` check before downloading any patch, to fail fast before wasting time on a large download.
- **xdelta3 binary**: Uses `exec.LookPath` + app bundle check. No brew dependency added to the Go module — xdelta3 is a system tool. Future: ship it bundled in the .app.
- **`applying_patch` phase**: Added as a new `UpdateProgress.Phase` value so the frontend can display "Applying update..." during xdelta3 execution (which can take minutes for 20 GB disks).

### Gotchas

- `decompressZstd` in `download.go` uses `d.cancel` (a `chan struct{}` on VMDownloader), not `context.Context`, making it non-reusable from `updater.go`. Hence the new `decompressZstdFile(ctx, ...)` helper.
- The `GOOS=darwin` build requires macOS for systray CGo — can't test-compile on Linux. Syntax verified with `gofmt`.
- `DownloadVMUpdate` decision tree: `force=true` skips both docker-only and patch paths (used by `RedownloadVMImage` to force a full disk download).
- Patch generation in `upload-vm-images.sh` is guarded by `command -v xdelta3` — gracefully skipped if xdelta3 is not installed on the CI machine.
