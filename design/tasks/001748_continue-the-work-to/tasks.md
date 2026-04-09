# Implementation Tasks

## Phase 1: Merge & Verify (DONE)

- [x] Merge main into `feature/001386-oh-heres-a-killer`
- [x] Resolve conflict in `desktop.go` — keep recording endpoints + MCP handler mount
- [x] Resolve conflict in `mcp_server.go` — keep recording tools + use StreamableHTTPServer
- [x] Verify `go build ./api/pkg/desktop/...` passes

## Phase 2: Code Quality Fixes (DONE)

- [x] Fix race condition in `recording_handlers.go` lazy init — use `sync.Once` for RecordingManager initialization
- [x] Fix non-deferred mutex in `recording.go` `AddSubtitle()` (line 214-220) — use defer
- [x] Fix non-deferred mutex in `recording.go` `SetSubtitles()` (line 234-236) — use defer
- [x] Check `file.Close()` error in `StopRecording()` (line 167)
- [x] Replace `fmt.Printf("[RECORDING]...")` with structured logger (slog) — 4 occurrences in recording.go, match existing codebase patterns
- [x] Verify compilation still passes after fixes (both CGO and non-CGO builds pass)

## Phase 3: E2E Testing (DONE)

- [x] Rebuild desktop-bridge: `./stack build-ubuntu` (3 rebuilds total for iterative fixes)
- [x] Start a new desktop session
- [x] Test `start_recording` — returns recording ID, title, start_time
- [x] Test `add_subtitle` — accepts subtitle entries, returns `{"status":"ok"}`
- [x] Test `get_recording_status` — reports active recording with frame count, duration, subtitle count
- [x] Test `stop_recording` — produces MP4 file in `/tmp/helix-recordings/` with correct duration
- [x] Verify MP4 is playable (ffprobe: 5.53s, 59.68 fps, 1920x1080, H.264 Constrained Baseline)
- [x] Test `set_subtitles` — replaces subtitle track, VTT file generated alongside MP4
- [x] Test error cases: stop without start ("no active recording"), double start ("recording already in progress")
- [x] **Bug found and fixed**: Raw H.264 has no timestamps — ffmpeg produced 0.04s MP4 instead of correct duration. Fixed with `-fflags +genpts -r <fps>` approach.

## Phase 4: Push & PR (DONE)

- [x] Push feature branch to origin
- [x] Create PR description
