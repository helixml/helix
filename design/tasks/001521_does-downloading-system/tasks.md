# Implementation Tasks

## Refactor `downloadFileParallel` to accept `context.Context`

- [ ] Add `context.Context` parameter to `downloadFileParallel` in `for-mac/download.go`
- [ ] Replace `d.cancel` channel checks inside `downloadFileParallel` with `ctx.Done()`
- [ ] Propagate context to `downloadChunk` and `downloadChunkOnce` (replace cancel channel with context)
- [ ] Update `DownloadAll` to create a `context.Context` from the existing `d.cancel` channel and pass it to `downloadFileParallel`
- [ ] Verify initial download path still works (parallel, resume, cancellation)

## Wire `DownloadVMUpdate` to use parallel downloads

- [ ] In `Updater.DownloadVMUpdate`, set the CDN manifest on the `VMDownloader` instance (so `downloadFileParallel` can build URLs from `d.manifest.BaseURL`/`d.manifest.Version`)
- [ ] Create a progress adapter (extend or reuse the `updateEmitter` pattern) that translates `DownloadProgress` events from `downloadFileParallel` into `UpdateProgress` calls to `u.emitVMProgress`
- [ ] Replace `u.downloadFile(ctx, downloadURL, ...)` calls for VM files with `downloader.downloadFileParallel(ctx, emitter, file, vmDir)`
- [ ] Handle the `.staged` destination path — adjust either the file name or `vmDir` passed to `downloadFileParallel` so output lands at `finalName + ".staged"` instead of `f.Name`
- [ ] Preserve zstd decompression step after parallel download (currently works, just verify path wiring)
- [ ] Verify cancellation propagates from `u.vmCancelFunc` through the context to the parallel downloader

## Keep DMG download as-is

- [ ] Confirm `Updater.downloadFile` (single-connection) is only used for DMG downloads in `StartCombinedUpdate` and `ApplyAppUpdate` — no VM files go through it

## Testing

- [ ] Build the mac-app (`cd for-mac && wails build`) and verify it compiles
- [ ] Test initial VM download still works at full speed (~110 MB/s)
- [ ] Test "Downloading system update (1/2)..." now uses parallel connections (check log output for "N parallel connections")
- [ ] Test combined update progress bar (0-90% scaling) still reports correctly
- [ ] Test cancelling a system update mid-download
- [ ] Test resume: kill the app during system update download, relaunch, confirm it resumes from chunks

## Cleanup (optional)

- [ ] Consider removing `Updater.downloadFile` entirely if DMG download can use `downloadFileSingle` from `download.go` instead (reduces code duplication)