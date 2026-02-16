# Helix.app Size Reduction

**Date:** 2026-02-10
**Status:** Proposal

## Problem

The current Helix.app bundle is ~17GB with VM images, which creates:
- Very long DMG creation time (compression)
- Very long Gatekeeper verification on first launch after install
- Large download size for distribution

### Current Size Breakdown

| Component | Size | Compressible? |
|-----------|------|---------------|
| VM root disk (qcow2) | ~7GB compressed | Already compressed |
| VM ZFS data disk (qcow2) | ~11GB compressed | Already compressed |
| EFI firmware | 128MB | ~50% |
| 27 open-source frameworks | 73MB | ~60% |
| QEMU dylib | 33MB | ~60% |
| Wails app binary | 9MB | ~60% |
| **Total** | **~17.2GB** | |

The VM disk images are 95% of the bundle size.

## Option 1: Download VM Images on First Launch (Recommended)

Ship the app without VM images (~300MB). On first launch, download them from a CDN.

**Pros:**
- App is 300MB instead of 17GB
- Fast install, fast Gatekeeper verification
- Can update VM images independently of app updates
- Users who already have a VM from a previous install can skip download

**Cons:**
- Requires internet on first launch
- Need to host ~17GB on a CDN (S3/CloudFront, ~$0.02/GB transfer)
- Need a progress UI for the download
- Air-gapped installs need a separate mechanism

**Implementation:**
1. Host compressed VM images on S3/CloudFront with a version manifest
2. On first launch, check `~/Library/Application Support/Helix/vm/` for existing images
3. If missing, show download progress in the Wails UI
4. Download + decompress in background
5. Store a version file alongside the images for upgrade detection

**CDN cost estimate:** At $0.02/GB (CloudFront), 100 downloads = $34. Acceptable for beta.

## Option 2: Smaller VM Images via Minimal Install

Reduce what's pre-installed in the VM to shrink the disk images.

**Current VM contents (~16GB raw before compression):**
- Ubuntu 25.10 base: ~2GB
- Docker + images (helix-ubuntu desktop): ~8GB
- Go 1.25: ~500MB
- ZFS 2.4.0: ~200MB
- Build tools, dev packages: ~1GB
- Docker layer cache: ~4GB

**Reduction strategies:**
- Don't pre-pull Docker images — let them pull on first session start (+30s first session)
- Don't pre-install Go — install on demand (+60s)
- Prune Docker build cache before bundling
- Use `fstrim` / `qemu-img convert` to reclaim sparse space

**Expected reduction:** 16GB raw → 4-5GB raw → 2-3GB compressed qcow2.

**Cons:**
- First session start is slower (pulling Docker images)
- More complex first-run experience

## Option 3: Streaming VM Provisioning

Don't bundle a VM at all. On first launch, run `provision-vm.sh` logic from the app.

**Pros:**
- App is tiny (300MB)
- Always gets latest Ubuntu + packages
- No stale VM images

**Cons:**
- First launch takes 30-60 minutes (full provisioning)
- Requires fast internet (downloads ~5GB of packages)
- More failure modes (apt mirrors, network issues)
- Terrible first-run experience

## Recommendation

**Option 1 (CDN download) + Option 2 (smaller images)** combined:

1. Shrink VM images to ~3GB by not pre-pulling Docker images
2. Host on S3/CloudFront
3. Download on first launch with progress bar
4. App ships at 300MB, downloads 3GB on first run

This gives fast install, reasonable first-run experience, and manageable CDN costs.

## Alternative: Differential Updates

For subsequent VM updates, use `qemu-img rebase` or rsync-style block diffs to only download changed blocks. This is a v2 optimization — not needed for initial release.
