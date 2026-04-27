# Implementation Tasks

## Phase 1: Branch & Merge

- [ ] Create `feature/001956-continue-the-work-in` from `feature/001748-continue-the-work-to`
- [ ] `git fetch origin && git merge origin/main`
- [ ] Resolve conflicts (most likely in `api/pkg/desktop/desktop.go` — keep recording fields + routes + cleanup, accept main's additions)
- [ ] Inspect the 7 post-001748 commits to `api/pkg/desktop/` (esp. `2084e478a` shared_video_source GPU leak fix) — confirm `Subscribe()` API and `VideoFrame` struct (`IsReplay`, `IsKeyframe`, `Data`) are unchanged
- [ ] `go build ./api/pkg/desktop/...` (CGO build) passes
- [ ] `go build -tags '!cgo' ./api/pkg/desktop/...` (non-CGO stub build) passes
- [ ] `go test ./api/pkg/desktop/...` passes

## Phase 2: E2E Re-Verification

- [ ] `./stack build-ubuntu` to rebuild desktop-bridge image
- [ ] Start a fresh session (old sessions still use the prior image)
- [ ] `start_recording` returns a recording ID
- [ ] `add_subtitle` accepts an entry, `get_recording_status` shows incrementing frame count
- [ ] `stop_recording` returns paths; MP4 exists at `/tmp/helix-recordings/<sid>/<rid>/recording.mp4`
- [ ] `ffprobe recording.mp4` shows: H.264, ~correct duration (not 0.04s — that's the SPS-VUI bug), expected resolution
- [ ] `set_subtitles` workflow: provide full track, stop, verify VTT sidecar matches
- [ ] Error case: `stop_recording` without start → clear error
- [ ] Error case: `start_recording` while one is active → "recording already in progress"

## Phase 3: Regression Check

- [ ] Non-recording MCP tools still work: `take_screenshot`, `list_windows`, `mouse_click`
- [ ] Live video stream to browser still works (recording subscribes alongside, doesn't disrupt)
- [ ] Desktop-bridge shutdown with active recording → MP4 still finalizes (cleanup hook)

## Phase 4: PR

- [ ] Push `feature/001956-continue-the-work-in` to origin
- [ ] Open PR against main referencing tasks 001386 and 001748 + this spec
