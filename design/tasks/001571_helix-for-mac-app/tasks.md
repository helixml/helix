# Implementation Tasks

## Phase 1: Docker-Only Update Path (Tier 2 — Quick Win)

- [x] Add `docker_only_update` boolean field to `vm-manifest.json` schema and manifest generation in `upload-vm-images.sh`
- [x] Write a VM-side update agent script (inlined as SSH commands — no separate agent needed): runs `docker compose pull && up -d` via `vm.runSSH()`
- [x] Add `dockerOnlyUpdate()` method to `updater.go` that SSHes into the VM and pulls/restarts containers
- [x] Update `DownloadVMUpdate()` in `updater.go` to branch into `dockerOnlyUpdate()` when `docker_only_update: true`
- [x] Update the update progress UI to show "Pulling updated services…" via `pulling_vm` phase events
- [ ] Test: trigger a docker-only update from a running VM and verify services restart with new images

## Phase 2: Delta Patch Infrastructure (Tier 1)

- [x] Extend `VMManifest` in `download.go` with `Patches []VMManifestPatch` (from_version, name, size, sha256, applies_to_sha256, result_sha256)
- [x] Add patch generation step to `upload-vm-images.sh`:
  - Downloads previous version's `disk.qcow2.zst` from CDN
  - Decompresses both old and new disks
  - Generates `xdelta3 -e -s old.qcow2 new.qcow2 patch.xdelta3`
  - Compresses patch with zstd
  - Uploads to `vm/{FROM}_to_{TO}/patch.xdelta3.zst`
  - Writes patch metadata into manifest
- [x] Add patch pruning logic to `upload-vm-images.sh` (keep patches for last N versions, configurable via `PATCH_VERSIONS`)
- [ ] Add xdelta3 to CI build environment and document dependency

## Phase 3: Client-Side Patch Download & Apply

- [x] Add `downloadAndApplyPatch()` method to `updater.go`:
  - Downloads `patch.xdelta3.zst` via existing `downloadFile()` + progress events
  - Verifies SHA256 of downloaded patch
  - Decompresses patch with `decompressZstdFile()` (new ctx-aware helper)
  - Verifies local `disk.qcow2` SHA256 matches `applies_to_sha256` before applying
  - Applies patch: `xdelta3 -d -s disk.qcow2 patch.xdelta3 disk.qcow2.staged`
  - Verifies SHA256 of `disk.qcow2.staged` matches `result_sha256`
  - On any failure: deletes staged files, returns error → caller falls back to full disk download
- [x] Update `DownloadVMUpdate()` to implement the full decision tree (docker-only → patch → full fallback)
- [x] Update disk space preflight check to account for temporary space needed during patch apply (patch file + output disk + 2 GB headroom)
- [x] Patch download size is reported in `BytesTotal` of `downloading_vm` progress events (same channel as full download)
- [x] Reuses existing `update:vm-progress` Wails event with new `applying_patch` phase value

## Phase 4: Testing & Hardening

- [ ] Integration test: fresh install → upgrade via patch → verify disk SHA256 and VM boots correctly
- [ ] Integration test: corrupt local disk (SHA256 mismatch) → verify fallback to full download
- [ ] Integration test: interrupted patch download → resume works
- [ ] Integration test: no patch available for installed version → full download
- [ ] Manual test on a throttled connection (e.g., 5 Mbps) to verify user experience improvement
- [ ] Update `vm-manifest.json` documentation / README with new schema

## Phase 5: Release Pipeline Validation

- [ ] Verify CI produces correct patch files and manifest for a real release
- [ ] Verify CDN storage growth is bounded (patch pruning works)
- [ ] Verify new installs still use the full disk download path (no regression)
- [ ] Monitor CDN bandwidth metrics after first release with incremental updates
