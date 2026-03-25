# Implementation Tasks

## Phase 1: Docker-Only Update Path (Tier 2 — Quick Win)

- [~] Add `docker_only_update` boolean field to `vm-manifest.json` schema and manifest generation in `upload-vm-images.sh`
- [~] Write a VM-side update agent script (`/usr/local/bin/helix-update-images.sh`) that runs `docker-compose pull && docker-compose up -d` for the Helix services
- [~] Add `DockerOnlyUpdate()` method to `updater.go` that invokes the update agent via the existing VM exec mechanism
- [~] Update `CheckForUpdate()` in `updater.go` to branch into `DockerOnlyUpdate()` when `docker_only_update: true`
- [~] Update the update progress UI to show "Pulling updated services…" with Docker pull progress events
- [ ] Test: trigger a docker-only update from a running VM and verify services restart with new images

## Phase 2: Delta Patch Infrastructure (Tier 1)

- [~] Extend `vm-manifest.json` with a `patches` array field (from_version, name, size, sha256, applies_to_sha256, result_sha256)
- [~] Add patch generation step to `upload-vm-images.sh`:
  - Fetch previous version's `disk.qcow2.zst` from CDN
  - Decompress both old and new disks
  - Generate `xdelta3 -e -s old.qcow2 new.qcow2 patch.xdelta3`
  - Compress patch with zstd
  - Upload to `vm/{FROM}_{TO}/patch.xdelta3.zst`
  - Write patch metadata into manifest
- [~] Add patch pruning logic to `upload-vm-images.sh` (keep patches for last 3 versions, delete older)
- [ ] Add xdelta3 to CI build environment and document dependency

## Phase 3: Client-Side Patch Download & Apply

- [~] Add `DownloadAndApplyPatch()` method to `updater.go`:
  - Download `patch.xdelta3.zst` using existing parallel range download + resume logic from `download.go`
  - Verify SHA256 of downloaded patch
  - Decompress patch with zstd
  - Verify local `disk.qcow2` SHA256 matches `applies_to_sha256` before applying
  - Apply patch: `xdelta3 -d -s disk.qcow2 patch.xdelta3 disk.qcow2.staged`
  - Verify SHA256 of resulting `disk.qcow2.staged` matches `result_sha256`
  - If any step fails: delete staged files, fall back to full disk download
- [~] Update `CheckForUpdate()` to implement the full decision tree (docker-only → patch → full fallback)
- [~] Update disk space preflight check to account for temporary space needed during patch apply (old decompressed + patch output)
- [~] Update update progress UI to show actual patch download size before starting (e.g., "Update: 450 MB" instead of "7.8 GB")
- [~] Add `update:patch-progress` Wails event for patch-specific progress (or reuse `update:vm-progress`)

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
