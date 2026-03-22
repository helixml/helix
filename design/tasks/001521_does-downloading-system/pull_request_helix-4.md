# Use parallel downloader for all update downloads

## Summary

The "Downloading system update (1/2)..." flow used a single-connection sequential HTTP download (`Updater.downloadFile`) while the initial VM download used a 16-connection parallel downloader (`VMDownloader.downloadFileParallel`). This caused update downloads to run at ~30 MB/s vs ~110 MB/s for initial installs — roughly 3.5x slower.

This PR wires all update downloads (VM images and DMGs) through the existing parallel downloader, then deletes the slow single-connection `Updater.downloadFile` entirely.

## Changes

- Refactored `downloadFileParallel`, `downloadFileSingle`, `downloadChunk`, `downloadChunkOnce`, and `decompressZstd` to accept `context.Context` instead of using the `d.cancel` channel directly
- `DownloadAll` now creates a `context.Context` bridged from `d.cancel` for backward compatibility
- `downloadFileParallel` and `downloadFileSingle` now accept URL and destination path as explicit parameters instead of constructing them from manifest fields
- Added `DownloadURL` method for downloading arbitrary URLs (used for DMGs) — builds a synthetic `VMManifestFile` with empty SHA256 to skip verification
- Added `SetManifest` method so the updater can configure the downloader with a CDN-fetched manifest
- SHA256 verification is now skipped when hash is empty (for `DownloadURL`)
- `DownloadVMUpdate` now calls `downloader.downloadFileParallel` (16 parallel connections) instead of `u.downloadFile` (1 connection)
- `StartCombinedUpdate` and `ApplyAppUpdate` DMG downloads now use `downloader.DownloadURL`
- Added `defaultPhase` field to `updateEmitter` so DMG downloads correctly report `"downloading_app"` phase
- Deleted `Updater.downloadFile` (single-connection, 256KB buffer, ~95 lines)
- Updated `ApplyAppUpdate` signature to accept `*VMDownloader`; updated both callers in `app.go`

## Files changed

- `for-mac/download.go` — context.Context refactor, URL parameter refactor, `DownloadURL`, `SetManifest`, conditional SHA256 verification
- `for-mac/updater.go` — wire parallel downloader, delete `downloadFile`, `updateEmitter.defaultPhase`
- `for-mac/app.go` — pass `a.downloader` to `ApplyAppUpdate`

## Testing

This is a macOS-only Wails app that cannot be built on Linux. IDE diagnostics confirm no errors in modified files. The following require manual testing on macOS:

- Initial VM download still works at full speed
- "Downloading system update (1/2)..." uses parallel connections (check log for "N parallel connections")
- "Downloading app update (2/2)..." uses parallel downloader
- Combined update progress bar (0-90% VM, 90-100% DMG) reports correctly
- Cancelling mid-download works
- Resume after kill works (chunk-based `.tmp` + `.chunks` files)