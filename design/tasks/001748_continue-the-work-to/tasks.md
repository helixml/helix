# Implementation Tasks

## Phase 1: Merge & Verify (DONE)

- [x] Merge main into `feature/001386-oh-heres-a-killer`
- [x] Resolve conflict in `desktop.go` — keep recording endpoints + MCP handler mount
- [x] Resolve conflict in `mcp_server.go` — keep recording tools + use StreamableHTTPServer
- [x] Verify `go build ./api/pkg/desktop/...` passes

## Phase 2: Code Quality Fixes

- [x] Fix race condition in `recording_handlers.go` lazy init — use `sync.Once` for RecordingManager initialization
- [x] Fix non-deferred mutex in `recording.go` `AddSubtitle()` (line 214-220) — use defer
- [x] Fix non-deferred mutex in `recording.go` `SetSubtitles()` (line 234-236) — use defer
- [x] Check `file.Close()` error in `StopRecording()` (line 167)
- [x] Replace `fmt.Printf("[RECORDING]...")` with structured logger (slog) — 4 occurrences in recording.go, match existing codebase patterns
- [x] Verify compilation still passes after fixes (both CGO and non-CGO builds pass)

## Phase 3: E2E Testing

- [ ] Rebuild desktop-bridge: `./stack build-ubuntu`
- [ ] Start a new desktop session
- [ ] Test `start_recording` MCP tool — verify returns recording ID
- [ ] Test `add_subtitle` MCP tool — verify accepts subtitle entries
- [ ] Test `get_recording_status` — verify reports active recording
- [ ] Test `stop_recording` — verify produces MP4 file in `/tmp/helix-recordings/`
- [ ] Verify MP4 is playable (check with `ffprobe`)
- [ ] Test `set_subtitles` — verify VTT file generated alongside MP4
- [ ] Test error cases: stop without start, double start

## Phase 4: Push & PR

- [ ] Push feature branch to origin
- [ ] Create PR against main
- [ ] Verify CI passes (CGO tests will only run in CI)
