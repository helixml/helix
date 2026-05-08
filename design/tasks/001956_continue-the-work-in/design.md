# Design: Continue Agent Screencast Recording (Round 3)

## Architecture (unchanged since 001748)

```
Agent (Zed/Claude)
  → MCP tool call
  → API gateway
  → RevDial tunnel
  → desktop-bridge :9876 /mcp        (StreamableHTTPServer mounted via SetMCPHandler)
  → MCP tool handler (mcp_server.go)
  → HTTP POST localhost:9876/recording/*
  → desktop.Server handler (recording_handlers.go)
  → RecordingManager (recording.go)
  → SharedVideoSource.Subscribe()    (taps existing video pipeline — no extra GPU encode)
  → raw H.264 → ffmpeg mux → MP4 + WebVTT
  → /tmp/helix-recordings/<session_id>/<recording_id>/
```

The HTTP indirection between the MCP handler and the recording handler is intentional and survives — both run in the same process, but the MCP handler is constructed before the desktop.Server is fully wired with the RecordingManager (lazy init), and the HTTP boundary keeps the seam thin.

## Decisions Inherited from 001748 (still valid)

These are the load-bearing decisions from the prior pass. Keep them.

1. **`sync.Once` for `RecordingManager` lazy init** — first `/recording/start` call creates the manager. `recordingManagerErr` propagates init failures.
2. **`defer` for inner mutex unlocks** in `AddSubtitle`/`SetSubtitles` (panic-safe).
3. **`slog.Logger` everywhere** — no more `fmt.Printf("[RECORDING]…")`.
4. **MP4 duration via `ffmpeg -fflags +genpts -r <calculated_fps>`** — raw H.264 has no PTS; SPS VUI timing in the stream overrides `-framerate`. Calculated FPS = `frame_count / (duration_ms / 1000)`.
5. **Extract SPS/PPS from the first replay keyframe** — recording skips replay frames (GOP warmup) but the *first* one carries SPS (NAL 7) and PPS (NAL 8). Without those NAL units written at the start of the raw H.264 file, ffmpeg cannot parse anything. Use `parseAnnexBNALUnits()` from `ws_stream.go`.

## What's New for This Pass

### Drift to absorb (~7 commits in `api/pkg/desktop/`)

Worth reading carefully before resolving conflicts:

| Commit       | What it touches                       | Likely interaction with recording                                                                                                |
| ------------ | ------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------- |
| `2084e478a`  | `shared_video_source.go` (~+100 line) | GPU memory leak fix — released GStreamer pipelines that weren't being torn down. Verify `Subscribe()` signature unchanged and lifecycle assumptions still hold for the recording's frame consumer goroutine. |
| `5faaf574e`  | pipeline lifecycle                    | Now exits instead of crashing on teardown timeout. Recording cleanup on shutdown (`StopRecording` from `desktop.go` Run-defer) should still fire before the timeout. |
| `8d3989147`  | desktop-bridge crash on API restart   | If the bridge process now survives API restarts more cleanly, an in-flight recording survives too — make sure that's actually OK. |
| `65aa6c72d` + `487da8b06` | `gst_pipeline.go` (vaapi support)         | We consume already-encoded H.264 frames downstream of the encoder choice. Should be transparent. ffprobe the test MP4 to confirm we still get H.264 in MP4. |
| `fc64e70b1`, `573f0fa5d`  | git/identity                              | Unrelated.                                                                                                                       |

### Conflict expectations

- `desktop.go` — most likely conflict. Both branches add fields to `Server`. Take the union: `recordingManager*` fields + whatever main added.
- `shared_video_source.go` — main's GPU leak fix is large. The recording uses `Subscribe()` and reads `frame.IsReplay` / `frame.IsKeyframe` / `frame.Data`. As long as that surface is intact, no recording-side change required.
- `gst_pipeline.go` — vaapi changes are encoder-internal. Unlikely to conflict with recording (which doesn't touch this file).

If the conflict resolution gets ugly, fall back to: `git merge -X theirs main` for non-recording files, then manually patch `desktop.go` and re-add recording lines.

## Files Touched (expected, same set as 001748)

| File                                          | Purpose                                                |
| --------------------------------------------- | ------------------------------------------------------ |
| `api/pkg/desktop/recording.go`                | RecordingManager, SPS/PPS extract, ffmpeg mux          |
| `api/pkg/desktop/recording_nocgo.go`          | Stub for non-CGO builds                                |
| `api/pkg/desktop/recording_handlers.go`       | HTTP endpoints + sync.Once init                        |
| `api/pkg/desktop/recording_handlers_nocgo.go` | Stub                                                   |
| `api/pkg/desktop/recording_test.go`           | WebVTT + struct tests                                  |
| `api/pkg/desktop/desktop.go`                  | recordingManager fields + /recording/* routes + cleanup on shutdown |
| `api/pkg/desktop/mcp_server.go`               | 5 recording tools                                      |

## Testing Strategy

Same as 001748 — that pass did E2E in Helix-in-Helix and produced playable MP4s with motion. Re-run those steps after the merge. The diagnostic that catches the most regressions: `ffprobe` on the output MP4 — if duration is wrong or it can't parse, one of the 001748 fixes regressed.

## Why Not Cherry-Pick Onto Fresh Main Instead?

Considered — would give a cleaner history. Rejected because:
- The 001748 branch is already the "merge of main + fixes" state, with proven E2E behavior.
- Re-merging from there preserves the fix commits' authorship and review trail.
- Cherry-picking would duplicate the merge work that 001748 already did once.

## Notes for Future-Future Continuations

If a fourth pass is needed, check whether main has converted MCP tool handlers to call `RecordingManager` directly (skipping the HTTP indirection). The original reason for the indirection — MCP being a separate process — went away when main mounted MCP at `/mcp` on the desktop server. The HTTP loopback still works but is redundant.
