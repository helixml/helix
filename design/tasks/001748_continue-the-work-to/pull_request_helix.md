# Fix agent screencast recording: merge main, code quality, and SPS/PPS bug

## Summary
Continues work from task 001386 to make the agent screencast recording feature mergeable. Merges main, fixes code quality issues found in review, and fixes two critical bugs: incorrect MP4 duration and missing SPS/PPS NAL units that caused recordings to fail entirely.

## Changes
- **Merge main**: Resolved conflicts in `desktop.go` (recording endpoints + MCP handler mount) and `mcp_server.go` (recording tools + StreamableHTTPServer)
- **Race condition fix**: Use `sync.Once` for lazy `RecordingManager` initialization instead of unsynchronized nil check
- **Mutex safety**: Use `defer` for inner mutex unlocks in `AddSubtitle`/`SetSubtitles` (panic-safe)
- **Logging**: Replace `fmt.Printf("[RECORDING]...")` with structured `slog.Logger`
- **Error handling**: Check `file.Close()` error in `StopRecording()`
- **MP4 duration fix**: Raw H.264 has no timestamps — ffmpeg used SPS VUI timing instead of actual framerate. Fixed with `-fflags +genpts -r <calculated_fps>` to generate correct PTS values
- **SPS/PPS fix**: Recording skipped ALL replay frames including the initial keyframe with SPS/PPS. ffmpeg cannot parse H.264 without these stream parameters. Fix extracts SPS/PPS from the first replay keyframe and writes them at the start of the file.

## Testing
E2E tested in Helix-in-Helix environment:
- `start_recording` -> returns recording ID
- `add_subtitle` / `set_subtitles` -> subtitles buffered and written as WebVTT
- `get_recording_status` -> reports frame count, duration, subtitle count
- `stop_recording` -> produces valid MP4 (verified with ffprobe: 14.8s, 9fps, 1920x1080, H.264 Constrained Baseline)
- Verified video captures real motion (GNOME overview toggle, app window openings)
- Error cases: stop without start, double start — both return clear error messages
