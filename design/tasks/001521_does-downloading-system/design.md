# Design: VM Update Download Should Use Parallel Downloads

## Summary

Reuse the existing `VMDownloader.downloadFileParallel` for VM update downloads instead of the single-connection `Updater.downloadFile`. This is a wiring change, not new download logic.

## Architecture

### Current Flow (slow)

```
Updater.StartCombinedUpdate
  → Updater.DownloadVMUpdate
    → for each manifest file:
        Updater.downloadFile          ← single HTTP GET, 256KB buffer, ~30 MB/s
```

### Proposed Flow (fast)

```
Updater.StartCombinedUpdate
  → Updater.DownloadVMUpdate
    → for each manifest file:
        VMDownloader.downloadFileParallel  ← 16 parallel Range requests, 1MB buffer, ~110 MB/s
```

## Key Decisions

### Reuse `downloadFileParallel`, don't duplicate it

The parallel downloader in `download.go` already handles:
- 16 concurrent HTTP Range requests (`downloadConcurrency = 16`)
- Chunk-based resume (`.tmp` + `.chunks` progress files)
- Fallback to single connection for small files or servers without Range support
- SHA256 verification
- Cancellation via `d.cancel` channel
- Progress reporting via the `EventsEmit` interface

There's no reason to write new download logic. The updater just needs to call the existing method.

### Keep `Updater.downloadFile` for DMGs

The DMG (phase 2 of combined update) is ~100 MB. Single-connection is fine for that size and keeps the code simple. Only the VM image files (multi-GB) need parallel downloads.

### Progress adapter

`DownloadVMUpdate` currently emits `UpdateProgress` structs via `u.emitVMProgress`. The parallel downloader emits `DownloadProgress` structs via the `EventsEmit` interface. A thin adapter is needed to bridge these:

- `downloadFileParallel` calls `ctx.EventsEmit("vm:download-progress", DownloadProgress{...})`
- The adapter translates `DownloadProgress` → `UpdateProgress` and calls `u.emitVMProgress`

The simplest approach: pass an adapter struct that implements `EventsEmit(string, ...interface{})` and intercepts the progress events to call `u.emitVMProgress`. This is exactly what the existing `updateEmitter` struct in `updater.go` already does for decompression — extend or reuse that pattern.

### Cancellation bridging

`DownloadVMUpdate` uses `context.Context` for cancellation. `downloadFileParallel` uses a `d.cancel` channel. Two options:

1. **Preferred**: Refactor `downloadFileParallel` to accept a `context.Context` instead of using `d.cancel`. This is cleaner and aligns with Go conventions. The `DownloadAll` path would create its own context from `d.cancel`.
2. **Alternative**: Spawn a goroutine that closes `d.cancel` when the context is cancelled. Simpler change but messier.

Option 1 is better since it's a small change and makes the API more standard.

### Manifest handling

`downloadFileParallel` currently takes a `VMManifestFile` and `vmDir` and constructs the URL from `d.manifest.BaseURL` and `d.manifest.Version`. In `DownloadVMUpdate`, the manifest is loaded into a local variable, not stored on the `VMDownloader`. Two options:

1. Temporarily set the manifest on the downloader before calling `downloadFileParallel`
2. Add a method variant that accepts a URL directly

Option 1 is simpler — `VMDownloader` already has a `manifest` field and `LoadManifest` sets it. The updater can set it to the CDN manifest before downloading.

## Files Changed

| File | Change |
|------|--------|
| `for-mac/download.go` | Add `context.Context` parameter to `downloadFileParallel` (and `downloadChunk`/`downloadChunkOnce`). Replace `d.cancel` channel checks with context cancellation. |
| `for-mac/download.go` | Update `DownloadAll` to create a context from `d.cancel` and pass it through. |
| `for-mac/updater.go` | In `DownloadVMUpdate`, set the manifest on `VMDownloader`, then call `downloadFileParallel` per file instead of `u.downloadFile`. Use a progress adapter. |
| `for-mac/updater.go` | Optionally remove `Updater.downloadFile` if DMG download is also moved to use `downloadFileSingle` from `download.go` (cleanup, not required). |

## Risks

- **CDN Range support**: The parallel downloader already handles servers that don't support Range (falls back to single connection), so this is a non-issue.
- **Staged path difference**: `DownloadVMUpdate` writes to `finalName + ".staged"`, while `downloadFileParallel` writes to `f.Name`. The staging path logic needs to be preserved — either by adjusting the `vmDir` or the file name passed in.
- **Concurrency on `VMDownloader`**: If an initial download and an update download could theoretically race, the `d.running` mutex guard already prevents this.