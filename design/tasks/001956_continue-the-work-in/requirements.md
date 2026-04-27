# Requirements: Continue Agent Screencast Recording (Round 3)

## Context

Third pass at landing the agent screencast recording feature.

- **Task 001386** (`feature/001386-oh-heres-a-killer`) built the core feature: `RecordingManager`, 5 MCP tools (`start_recording`, `stop_recording`, `add_subtitle`, `set_subtitles`, `get_recording_status`), HTTP endpoints, WebVTT generation, CGO/non-CGO build stubs.
- **Task 001748** (`feature/001748-continue-the-work-to`) merged main, fixed code-quality issues, and fixed two critical bugs (MP4 duration, missing SPS/PPS). Verified end-to-end in Helix-in-Helix but never merged upstream.
- **Task 001956 (this task)** picks up from 001748. Main has moved ~364 commits since 001748 was last synced — most importantly inside `api/pkg/desktop/`. We re-merge main, deal with whatever drift breaks, and re-verify end-to-end.

## What the Feature Does (still applies, copied from 001386)

Lets an AI agent record a screencast of its work and add subtitle narration, so users get a visual demo without watching live.

### MCP Tools
- `start_recording(title?)` → `recording_id`. Only one recording active per session.
- `stop_recording()` → MP4 path + optional VTT path.
- `add_subtitle(text, start_ms, end_ms)` — append one subtitle entry.
- `set_subtitles([{text, start_ms, end_ms}, ...])` — replace the entire subtitle track.
- `get_recording_status()` — frame count, duration, subtitle count.

### Output
- MP4 (H.264 in MP4 container, ffmpeg `-c:v copy` mux from raw H.264).
- WebVTT sidecar (when subtitles were added).
- Stored at `/tmp/helix-recordings/<session_id>/<recording_id>/`.

## What This Task Must Do

### R1: Branch from 001748 and re-merge main
- New branch `feature/001956-continue-the-work-in` based on `feature/001748-continue-the-work-to` (which already has the merge from 001386 + the 4 fix commits).
- Merge latest `main`. Resolve conflicts.

### R2: Confirm the recording feature still wires up after the merge
The post-001748 main commits that touch `api/pkg/desktop/` are:
- `2084e478a` GPU memory leak fix in `shared_video_source.go` — may change `Subscribe()` lifecycle.
- `5faaf574e` pipeline teardown timeout fix.
- `8d3989147` desktop-bridge crash on API restart.
- `65aa6c72d` + `487da8b06` `HELIX_ENCODER=vaapi` support in `gst_pipeline.go`.
- (`fc64e70b1`, `573f0fa5d` — git identity changes, not relevant.)

Verify the recording tap into `SharedVideoSource.Subscribe()` still works against the new lifecycle/cleanup behavior. The vaapi encoder change is upstream of the recording (we just consume H.264 frames), should not affect us — confirm by ffprobe-ing a test recording.

### R3: Build and re-verify end-to-end
- `./stack build-ubuntu` to rebuild `desktop-bridge`.
- Start a fresh session.
- Run `start_recording → add_subtitle → stop_recording`. Verify MP4 plays and matches expected duration.
- Run `set_subtitles → stop_recording`. Verify VTT sidecar.
- Run error cases: stop-without-start, double-start.

## Acceptance Criteria
- [ ] Branch is up to date with main (conflicts resolved).
- [ ] `go build ./api/pkg/desktop/...` passes (CGO and non-CGO).
- [ ] `go test ./api/pkg/desktop/...` passes.
- [ ] E2E: start → add_subtitle → stop produces a valid MP4 (correct duration via ffprobe) + VTT.
- [ ] E2E: error cases return clear messages.
- [ ] No regression in non-recording desktop functionality (window mgmt MCP tools, video stream).

## Out of Scope
- Filestore upload (still future work).
- Burned-in subtitles.
- Audio track.
- Multiple concurrent recordings per session.
