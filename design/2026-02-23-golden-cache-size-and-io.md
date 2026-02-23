# Golden Cache: Corruption Fix, Size Display, Copy Speedup & Disk I/O Sparkline

**Date**: 2026-02-23
**Status**: Implemented

## Changes

### P0: Fix Golden Cache Corruption

**Root cause**: Golden Docker cache includes `/var/lib/docker/containers/` metadata from the build session. These containers have bind mounts to workspace paths (e.g., `/home/retro/work/helix/...`) on ZFS. When inner dockerd starts on a new session, it tries to restart those cached containers and auto-creates missing bind mount sources as **empty directories**, corrupting `go.mod`, `package.json`, etc.

**Fix**: `PurgeContainersFromGolden()` in `golden.go` removes `containers/`, `network/`, `containerd/`, and `buildx/` dirs from the golden cache after promotion. Called from `monitorGoldenBuild()` right after `PromoteSessionToGolden()`.

Preserves: `overlay2/` (image layers), `image/` (image metadata), `tmp/` (build cache).

### Cache Size Display

Added `CacheSizeBytes` field to `GoldenBuildResult`. After successful promotion, `GetGoldenSize()` is called and the result stored. `waitForGoldenBuildCompletion()` in `golden_build_service.go` copies the size to `SandboxCacheState.SizeBytes`. Frontend already renders this when > 0.

### Copy Speedup: `--reflink=auto`

Changed `cp -a` to `cp -a --reflink=auto` in `SetupGoldenCopy`. On COW filesystems (XFS with reflink, btrfs), this is a metadata-only copy (near-instant for multi-GB). Falls back silently to full copy on ext4.

**Note**: Current setup uses ext4 on zvol. To get the speedup, reformat to XFS (`mkfs.xfs -m reflink=1`).

### Disk I/O Sparkline

During golden builds, `ProjectSettings.tsx` polls the `disk-history` endpoint for the building sandbox and computes write rate (MB/s) from consecutive `/var` disk usage samples. Shows a SparkLineChart + "N MB/s" next to "Building..." status.

### Session Startup Progress: "Unpacking build cache (X/Y GB)"

Added real-time progress during golden cache copy on session startup:

**Hydra side** (sandbox): `SetupGoldenCopy()` accepts a progress callback. A goroutine monitors destination size via `du -sb` every 2s while `cp` runs. Progress stored in `DevContainerManager.goldenCopyProgress` map, exposed via `GET /golden-copy-progress/{project_id}`.

**API side**: `HydraExecutor.StartDesktop()` spawns a goroutine that polls the progress endpoint via a separate RevDial client every 2s. Updates `session.Metadata.StatusMessage` in the DB (e.g., "Unpacking build cache (2.1/7.0 GB)"). Cleared when `CreateDevContainer` returns.

**Frontend**: `useSandboxState` hook reads `status_message` from session config. Shows it instead of "Starting Desktop..." when present.

**CLI**: `waitForTaskSession` prints status message updates as they appear.

## Files Modified

- `api/pkg/hydra/golden.go` — `PurgeContainersFromGolden()`, `--reflink=auto`, progress callback
- `api/pkg/hydra/devcontainer.go` — `CacheSizeBytes`, `GoldenCopyProgress`, purge after promote
- `api/pkg/hydra/server.go` — `GET /golden-copy-progress/{project_id}` endpoint
- `api/pkg/hydra/client.go` — `GetGoldenCopyProgress()` RevDial client method
- `api/pkg/services/golden_build_service.go` — Set `SizeBytes` from build result
- `api/pkg/external-agent/hydra_executor.go` — Progress polling goroutine, `updateSessionStatusMessage()`
- `api/pkg/types/types.go` — `StatusMessage` field on `SessionMetadata`
- `api/pkg/cli/spectask/spectask.go` — Show status message during startup polling
- `frontend/src/pages/ProjectSettings.tsx` — Disk I/O sparkline during builds
- `frontend/src/components/external-agent/ExternalAgentDesktopViewer.tsx` — Show `statusMessage`
