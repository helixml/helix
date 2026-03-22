# Requirements: Use Parallel Downloader for All Update Downloads

## Problem

The "Downloading system update (1/2)..." flow in the mac-app uses a **single-connection sequential HTTP download** (`updater.go:downloadFile`), while the initial VM download uses a **16-connection parallel downloader** (`download.go:downloadFileParallel`). This results in ~30 MB/s for updates vs ~110 MB/s for initial downloads — roughly 3.5x slower.

## Root Cause

Two completely separate download implementations exist:

| Path | Method | Connections | Buffer | Location |
|------|--------|-------------|--------|----------|
| Initial download | `VMDownloader.DownloadAll` → `downloadFileParallel` | 16 parallel Range requests | 1 MB | `download.go` |
| Update download | `Updater.DownloadVMUpdate` → `u.downloadFile` | 1 sequential GET | 256 KB | `updater.go` |
| DMG download | `Updater.StartCombinedUpdate` / `ApplyAppUpdate` → `u.downloadFile` | 1 sequential GET | 256 KB | `updater.go` |

`Updater.downloadFile` was written as a simple helper and then reused for all update downloads — both multi-GB VM images and DMGs. The parallel downloader already handles small files gracefully (falls back to single connection for files < 10 MB or servers without Range support), so there's no reason to maintain a separate single-connection implementation.

## User Stories

1. **As a user**, when my mac-app downloads a system update, I want it to download at the same speed as the initial install so I'm not waiting 3-4x longer than necessary.

## Acceptance Criteria

- [ ] All update downloads (VM images and DMGs) use `VMDownloader.downloadFileParallel` (or equivalent parallel logic)
- [ ] `Updater.downloadFile` is deleted — one download path for everything
- [ ] Update downloads achieve comparable throughput to initial downloads (~100+ MB/s on fast connections)
- [ ] Progress reporting continues to work correctly in the combined update UI (the 0-90% scaling for phase 1, 90-100% for phase 2)
- [ ] Auto-resume works for all update downloads — if the app is killed or crashes mid-download, clicking "Start update" again picks up where it left off (the parallel downloader detects existing `.tmp` + `.chunks` files on disk and skips completed chunks; no separate "resume" button needed)
- [ ] SHA256 verification still occurs after download
- [ ] Cancellation still works mid-download
- [ ] No regression in initial download path