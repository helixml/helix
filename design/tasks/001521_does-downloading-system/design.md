# Design: Use Parallel Downloader for All Update Downloads

## Summary

Reuse the existing `VMDownloader.downloadFileParallel` for all update downloads (VM images and DMGs) instead of the single-connection `Updater.downloadFile`, then delete `Updater.downloadFile` entirely. This is a wiring change, not new download logic.

## Architecture

### Current Flow (slow)

```
Updater.StartCombinedUpdate
  → Updater.DownloadVMUpdate
    → for each manifest file:
        Updater.downloadFile          ← single HTTP GET, 256KB buffer, ~30 MB/s
  → DMG download:
        Updater.downloadFile          ← same slow path
```

### Proposed Flow (fast)

```
Updater.StartCombinedUpdate
  → Updater.DownloadVMUpdate
    → for each manifest file:
        VMDownloader.downloadFileParallel  ← 16 parallel Range requests, 1MB buffer, ~110 MB/s
  → DMG download:
        VMDownloader.downloadFileParallel  ← same fast path (falls back to single conn for small files)
```

## Key Decisions

### Reuse `downloadFileParallel` for everything, delete `Updater.downloadFile`

The parallel downloader in `download.go` already handles:
- 16 concurrent HTTP Range requests (`downloadConcurrency = 16`)
- Chunk-based resume (`.tmp` + `.chunks` progress files)
- Fallback to single connection for small files (< 10 MB) or servers without Range support
- SHA256 verification
- Cancellation via `d.cancel` channel
- Progress reporting via the `EventsEmit` interface

There's no reason to maintain a separate single-connection downloader. The parallel downloader's built-in fallback handles small files like DMGs (~100 MB) naturally — they'll use single or few connections based on chunk size. If DMGs ever grow larger, parallel downloads kick in automatically.

After migration, `Updater.downloadFile` is dead code and should be deleted.

### Progress adapter

`DownloadVMUpdate` currently emits `UpdateProgress` structs via `u.emitVMProgress`. The parallel downloader emits `DownloadProgress` structs via the `EventsEmit` interface. A thin adapter is needed to bridge these:

- `downloadFileParallel` calls `ctx.EventsEmit("vm:download-progress", DownloadProgress{...})`
- The adapter translates `DownloadProgress` → `UpdateProgress` and calls `u.emitVMProgress`

The simplest approach: pass an adapter struct that implements `EventsEmit(string, ...interface{})` and intercepts the progress events to call `u.emitVMProgress`. This is exactly what the existing `updateEmitter` struct in `updater.go` already does for decompression — extend or reuse that pattern. The same adapter approach works for DMG progress (translating to `u.emitAppProgress`).

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

For DMG downloads, the file isn't described by a `VMManifestFile`. Options:
1. Construct a synthetic `VMManifestFile` with the DMG URL, size, and empty SHA256 (skip verification if no hash available)
2. Add a standalone download function that takes a URL and destination path directly, backed by the same parallel logic

Option 2 is cleaner — extract the core parallel download logic into a function like `DownloadURL(ctx, url, destPath, emitter)` that both `downloadFileParallel` and the DMG path can call.

## Files Changed

| File | Change |
|------|--------|
| `for-mac/download.go` | Add `context.Context` parameter to `downloadFileParallel` (and `downloadChunk`/`downloadChunkOnce`). Replace `d.cancel` channel checks with context cancellation. |
| `for-mac/download.go` | Update `DownloadAll` to create a context from `d.cancel` and pass it through. |
| `for-mac/download.go` | Add a `DownloadURL` or similar method for downloading arbitrary URLs (used by DMG path). |
| `for-mac/updater.go` | In `DownloadVMUpdate`, set the manifest on `VMDownloader`, then call `downloadFileParallel` per file instead of `u.downloadFile`. Use a progress adapter. |
| `for-mac/updater.go` | In `StartCombinedUpdate` and `ApplyAppUpdate`, replace `u.downloadFile` for DMG downloads with the parallel downloader. |
| `for-mac/updater.go` | Delete `Updater.downloadFile` entirely. |

## Risks

- **CDN Range support**: The parallel downloader already handles servers that don't support Range (falls back to single connection), so this is a non-issue.
- **Staged path difference**: `DownloadVMUpdate` writes to `finalName + ".staged"`, while `downloadFileParallel` writes to `f.Name`. The staging path logic needs to be preserved — either by adjusting the `vmDir` or the file name passed in.
- **Concurrency on `VMDownloader`**: If an initial download and an update download could theoretically race, the `d.running` mutex guard already prevents this.
- **DMG SHA256**: The current DMG download path doesn't verify a hash. The parallel downloader does SHA256 verification by default. Either skip verification when no hash is provided, or add DMG hashes to the update manifest (better long-term).

## Implementation Notes

### Approach taken: URL passed directly instead of manifest construction

The design originally proposed two options for manifest handling. During implementation, a third cleaner option emerged: refactor `downloadFileParallel` and `downloadFileSingle` to accept the download URL and destination path as explicit parameters instead of constructing them from `d.manifest.BaseURL`/`d.manifest.Version`. This eliminated the need for synthetic manifests or manifest swapping in `DownloadURL`. The URL construction (`fmt.Sprintf("%s/%s/%s", baseURL, version, name)`) was moved to the two callers (`DownloadAll` and `DownloadVMUpdate`).

### Parameter naming: `ctx` vs `emitter`

The original `downloadFileParallel` used `ctx` for the `EventsEmit` interface parameter — confusing since Go convention reserves `ctx` for `context.Context`. The refactor renamed the emitter parameters to `emitter` throughout and added `ctx context.Context` as a proper first parameter.

### `DownloadURL` is very simple

`DownloadURL` builds a synthetic `VMManifestFile{Name: basename, Size: 0, SHA256: ""}` and passes the URL straight to `downloadFileParallel`. Size is populated from the HEAD response inside `downloadFileParallel`. SHA256 is empty so verification is skipped.

### `updateEmitter.defaultPhase` added

The existing `updateEmitter` hardcoded `phase = "downloading_vm"`. Since we now use it for DMG downloads too, a `defaultPhase` field was added so DMG downloads correctly report `"downloading_app"`.

### `ApplyAppUpdate` signature change

`ApplyAppUpdate` gained a `downloader *VMDownloader` parameter since it now calls `downloader.DownloadURL` instead of the deleted `u.downloadFile`. Both callers in `app.go` (`ApplyAppUpdate()` and `ApplyCombinedUpdate()`) were updated to pass `a.downloader`.

### `decompressZstd` also refactored

`decompressZstd` used `d.cancel` for cancellation checks. It was also refactored to accept `ctx context.Context` and use `ctx.Done()`, keeping the cancellation chain consistent.

### Frontend is unaffected

The frontend only listens to `update:combined-progress`, `update:vm-progress`, `update:vm-ready`, and `update:combined-ready` events. It does NOT listen to `update:app-progress`. The `Phase` field inside `UpdateProgress` is not used by the frontend for routing — event names handle that. So phase field changes are safe.

### Cannot build on Linux

This is a macOS-only Wails app with darwin-specific dependencies (`systray`, `hdiutil`, Cocoa notifications). The code compiles only on macOS. IDE diagnostics confirmed no errors in `download.go` and `updater.go`. The pre-existing `app.go` errors (`initNotifications`, `sendNotification`) are from build-tagged macOS files.

### Net code delta

The change is net -55 lines (151 added, 206 removed). The deleted `Updater.downloadFile` was ~95 lines of single-connection download logic.