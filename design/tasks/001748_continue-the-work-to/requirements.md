# Requirements: Continue Agent Screencast Recording

## Context

Task 001386 implemented the core agent screencast recording feature. The code compiles and has basic unit tests, but needs merge with main, code quality fixes, and E2E testing before it can be merged upstream.

## What Was Already Built (Task 001386)

- `RecordingManager` in `api/pkg/desktop/recording.go` — subscribes to `SharedVideoSource`, writes raw H.264 frames, uses ffmpeg to mux to MP4
- 5 MCP tools: `start_recording`, `stop_recording`, `add_subtitle`, `set_subtitles`, `get_recording_status`
- HTTP endpoints: `/recording/{start,stop,subtitle,subtitles,status}`
- WebVTT subtitle generation
- CGO/non-CGO build stubs
- Unit tests for WebVTT formatting and struct creation
- Shutdown cleanup (auto-stops recording on exit)

## What This Task Must Do

### R1: Merge Main and Fix Conflicts
**Status: DONE** — Merged main into `feature/001386-oh-heres-a-killer`, resolved conflicts:
- `desktop.go`: Kept both recording HTTP endpoints and new MCP handler mount (`s.mcpHandler`)
- `mcp_server.go`: Kept recording tools, switched from `SSEServer` to `StreamableHTTPServer` (main changed MCP transport)

### R2: Fix Code Quality Issues Found During Review
- Race condition in lazy initialization of `RecordingManager` (two concurrent `/recording/start` calls)
- Non-deferred mutex unlocks in `AddSubtitle()` and `SetSubtitles()` inner lock (panic-unsafe)
- Missing `file.Close()` error check in `StopRecording()`
- Use `fmt.Printf` for logging instead of structured logger (should use `slog` or the existing logger pattern)

### R3: Build and Deploy for E2E Testing
- Rebuild desktop-bridge container: `./stack build-ubuntu`
- Start a new session to pick up the new image
- Test the MCP tools manually through a running agent session

### R4: Verify MCP Routing Works End-to-End
Main changed MCP routing to go through the API gateway (PR #1850). The recording tools call `desktopURL` (localhost:9876) from within the MCP handler, which is in the same process as desktop-bridge. Verify this still works with the new routing architecture.

## Acceptance Criteria

- [ ] Feature branch is up to date with main (conflicts resolved)
- [ ] Code compiles cleanly (`go build ./api/pkg/desktop/...`)
- [ ] Race condition in handler initialization fixed
- [ ] Non-idiomatic mutex patterns fixed
- [ ] E2E test: `start_recording` → `add_subtitle` → `stop_recording` produces valid MP4 and VTT
- [ ] No regressions in existing desktop/MCP functionality
