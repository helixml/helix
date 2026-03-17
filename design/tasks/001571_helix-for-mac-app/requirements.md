# Requirements: Incremental VM Disk Updates for Helix for Mac

## Problem Statement

Every upgrade to Helix for Mac requires downloading the full VM disk: ~7.8 GB compressed / ~19.8 GB decompressed (a single `disk.qcow2.zst`). Users on high-latency, low-bandwidth connections face a painful experience — even users on decent connections wait a long time.

The root cause: the disk image bundles the Ubuntu 25.10 base OS and all Helix Docker images into a single monolithic file. Every release refreshes the whole file even when only the Helix app code changed.

---

## User Stories

**As a user,**
I want upgrades to download only what changed since my last version,
so that updates are fast and don't waste bandwidth regardless of my connection speed.

**As a user who just upgraded last week,**
I want to receive a small patch rather than a full 18 GB disk,
so that I don't have to wait for a download proportional to a large portion of the internet.

**As a user on metered bandwidth,**
I want to understand how large an update is before I commit to downloading it,
so that I can plan around my data limits.

**As a developer shipping a release,**
I want the CI pipeline to produce both a full image (for new installs) and incremental patches (for upgraders),
so that existing users get a much better experience without extra manual steps.

---

## Acceptance Criteria

### Incremental updates
- [ ] A typical Helix service-only update (new Docker images, no OS change) downloads ≤ 500 MB instead of ~7.8 GB
- [ ] An OS-level base update downloads ≤ 2 GB in most cases
- [ ] Full-disk download is still available as a fallback for new installs or broken state

### User experience
- [ ] Update UI shows the actual download size before starting
- [ ] Progress reporting works correctly for incremental patches (bytes, speed, ETA)
- [ ] If a patch fails or the local disk is corrupted, the app automatically falls back to full download
- [ ] No increase in time-to-boot after a successful incremental update

### Reliability
- [ ] SHA256 verification applies to each patch/chunk, not just the final assembled disk
- [ ] Resume support works for incremental downloads (same as current full-disk resume)
- [ ] Rollback to `disk.qcow2.old` still works after a failed patch apply

### CI / release pipeline
- [ ] `upload-vm-images.sh` generates patch files between consecutive versions
- [ ] Manifest (`vm-manifest.json`) extended to list available patches and their source/target versions
- [ ] Old patch files are pruned after N releases to bound CDN storage growth

---

## Out of Scope

- Per-file delta sync inside the VM filesystem (too complex, fragile)
- Streaming updates to a running VM (requires live migration complexity)
- Reducing the base image size (separate effort)
