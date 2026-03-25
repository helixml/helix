# Add incremental VM update support (docker-only + xdelta3 patches)

## Summary

- Users no longer need to download the full 18 GB VM disk on every upgrade
- **Tier 2 (quick win):** Service-only releases set `docker_only_update: true` in the manifest; the Mac app SSHes into the running VM and runs `docker compose pull && up -d`, cutting upgrade downloads from ~7.8 GB to ~200–500 MB
- **Tier 1 (binary delta):** OS-level releases include xdelta3 binary patches in the manifest; the client downloads and applies the patch (typically 500 MB–2 GB) instead of the full disk, with automatic fallback to full download on any failure

## Changes

### `for-mac/download.go`
- Added `VMManifestPatch` struct (from_version, name, size, sha256, applies_to_sha256, result_sha256)
- Extended `VMManifest` with `DockerOnlyUpdate bool` and `Patches []VMManifestPatch`

### `for-mac/updater.go`
- Added `vm *VMManager` field and `SetVMManager()` method (mirrors existing `SetAppContext` pattern)
- Added `dockerOnlyUpdate()`: SSHes into running VM, runs `docker compose pull && up -d`, records new version
- Added `downloadAndApplyPatch()`: downloads + verifies + decompresses + applies xdelta3 patch, falls back to full download on any step failure
- Added `findXdelta3()`, `verifyFileSHA256()`, `decompressZstdFile()` utilities
- Updated `DownloadVMUpdate()` decision tree: docker-only → patch → full disk fallback
- `force=true` skips incremental paths (used by RedownloadVMImage)

### `for-mac/app.go`
- Wire `SetVMManager(a.vm)` at startup so docker-only updates can SSH into the VM

### `for-mac/vm-manifest.json`
- Added `docker_only_update: false` and `patches: []` to the bundled manifest schema

### `for-mac/scripts/upload-vm-images.sh`
- New `DOCKER_ONLY_UPDATE=1` mode: skips disk upload, generates `docker_only_update: true` manifest
- New patch generation loop: downloads previous disk, generates xdelta3 patch, compresses, uploads to `vm/{FROM}_to_{TO}/patch.xdelta3.zst`, writes metadata to manifest
- New patch pruning: deletes stale patch directories older than `PATCH_VERSIONS` (default 3) releases
- New env vars: `SKIP_PATCH=1`, `PATCH_VERSIONS=N`

### `for-mac/updater_test.go`
- `TestVMManifestNewFields`: JSON round-trip for DockerOnlyUpdate + Patches
- `TestVerifyFileSHA256`: correct/incorrect/empty hash handling
- `TestDecompressZstdFile`: round-trip compress → decompress
- `TestDecompressZstdFileCancelled`: context cancellation
