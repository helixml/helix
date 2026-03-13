# Requirements: VM Update Download Should Use Parallel Downloads

## Problem

The "Downloading system update (1/2)..." flow in the mac-app uses a **single-connection sequential HTTP download** (`updater.go:downloadFile`), while the initial VM download uses a **16-connection parallel downloader** (`download.go:downloadFileParallel`). This results in ~30 MB/s for updates vs ~110 MB/s for initial downloads — roughly 3.5x slower.

## Root Cause

Two completely separate download implementations exist:

| Path | Method | Connections | Buffer | Location |
|------|--------|-------------|--------|----------|
| Initial download | `VMDownloader.DownloadAll` → `downloadFileParallel` | 16 parallel Range requests | 1 MB | `download.go` |
| Update download | `Updater.DownloadVMUpdate` → `u.downloadFile` | 1 sequential GET | 256 KB | `updater.go` |

`Updater.downloadFile` was written as a simple helper for DMG downloads (small files, ~100 MB) and then reused for VM image downloads (~10+ GB) where it's inadequate.

## User Stories

1. **As a user**, when my mac-app downloads a system update, I want it to download at the same speed as the initial install so I'm not waiting 3-4x longer than necessary.

## Acceptance Criteria

- [ ] `Updater.DownloadVMUpdate` uses `VMDownloader.downloadFileParallel` (or equivalent parallel logic) for VM image files
- [ ] Update downloads achieve comparable throughput to initial downloads (~100+ MB/s on fast connections)
- [ ] Progress reporting continues to work correctly in the combined update UI (the 0-90% scaling for phase 1)
- [ ] Resume support (chunk-based `.tmp` + `.chunks` progress files) works for update downloads
- [ ] DMG download (phase 2, "Downloading app update (2/2)...") can remain single-connection — DMGs are small
- [ ] SHA256 verification still occurs after download
- [ ] Cancellation still works mid-download
- [ ] No regression in initial download path