# Implementation Tasks

## Refactor `downloadFileParallel` to accept `context.Context`

- [x] Add `context.Context` parameter to `downloadFileParallel` in `for-mac/download.go`
- [x] Replace `d.cancel` channel checks inside `downloadFileParallel` with `ctx.Done()`
- [x] Propagate context to `downloadChunk` and `downloadChunkOnce` (replace cancel channel with context)
- [x] Update `DownloadAll` to create a `context.Context` from the existing `d.cancel` channel and pass it to `downloadFileParallel`
- [ ] Verify initial download path still works (parallel, resume, cancellation)

## Add a standalone `DownloadURL` method for arbitrary URLs

- [x] Add a method like `VMDownloader.DownloadURL(ctx, url, destPath, emitter)` that wraps the parallel download logic for a single URL (HEAD to get size + Range support, then parallel or single-connection download)
- [x] DMG downloads and other non-manifest files use this method
- [x] Skip SHA256 verification when no hash is provided (or make it optional)

## Wire `DownloadVMUpdate` to use parallel downloads

- [x] In `Updater.DownloadVMUpdate`, set the CDN manifest on the `VMDownloader` instance (so `downloadFileParallel` can build URLs from `d.manifest.BaseURL`/`d.manifest.Version`)
- [x] Create a progress adapter (extend or reuse the `updateEmitter` pattern) that translates `DownloadProgress` events from `downloadFileParallel` into `UpdateProgress` calls to `u.emitVMProgress`
- [x] Replace `u.downloadFile(ctx, downloadURL, ...)` calls for VM files with `downloader.downloadFileParallel(ctx, emitter, file, vmDir)`
- [x] Handle the `.staged` destination path — adjust either the file name or `vmDir` passed to `downloadFileParallel` so output lands at `finalName + ".staged"` instead of `f.Name`
- [x] Preserve zstd decompression step after parallel download (currently works, just verify path wiring)
- [x] Verify cancellation propagates from `u.vmCancelFunc` through the context to the parallel downloader

## Wire DMG downloads to use parallel downloader

- [x] In `StartCombinedUpdate`, replace `u.downloadFile(ctx, info.DMGURL, dmgPath, ...)` with `downloader.DownloadURL(ctx, info.DMGURL, dmgPath, emitter)`
- [x] In `ApplyAppUpdate`, replace `u.downloadFile(ctx, info.DMGURL, dmgPath, ...)` with the same
- [x] Create a progress adapter for DMG downloads that translates `DownloadProgress` → `UpdateProgress` and calls `u.emitAppProgress`
- [x] Verify combined update progress scaling still works (90-100% range for phase 2)

## Delete `Updater.downloadFile`

- [x] Remove `Updater.downloadFile` method from `for-mac/updater.go` — no callers should remain
- [x] Verify no other code references it

## Testing

- [ ] Build the mac-app (`cd for-mac && wails build`) and verify it compiles
- [ ] Test initial VM download still works at full speed (~110 MB/s)
- [ ] Test "Downloading system update (1/2)..." now uses parallel connections (check log output for "N parallel connections")
- [ ] Test "Downloading app update (2/2)..." uses the parallel downloader
- [ ] Test combined update progress bar (0-90% for VM, 90-100% for DMG) still reports correctly
- [ ] Test cancelling a system update mid-download
- [ ] Test resume: kill the app during system update download, relaunch, confirm it resumes from chunks