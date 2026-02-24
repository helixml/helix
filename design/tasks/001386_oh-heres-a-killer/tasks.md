# Implementation Tasks: Agent Screencast Recording

## Phase 1: Core Recording Infrastructure

- [x] Create `api/pkg/desktop/recording.go` with `RecordingManager` struct
- [x] Implement `Recording` struct with ID, title, start time, output path, subtitle buffer
- [x] Add method to subscribe to `SharedVideoSource` frame channel
- [x] Write raw H.264 frames to temp file (simpler than GStreamer pipeline)
- [x] Use ffmpeg to mux H.264 to MP4 on stop (same pattern as spectask_mcp_test.go)

## Phase 2: MCP Tools

- [x] Add HTTP endpoints to desktop.Server for recording control
- [x] Add `start_recording` tool to `mcp_server.go`
  - Parameters: `title` (optional string)
  - Creates Recording, starts pipeline, returns recording ID
- [x] Add `stop_recording` tool to `mcp_server.go`
  - Finalizes MP4 file, generates VTT from subtitle buffer
  - Returns file paths
- [x] Add `add_subtitle` tool to `mcp_server.go`
  - Parameters: `text` (required), `start_ms` (required), `end_ms` (required)
  - Appends single subtitle entry to active recording
- [x] Add `set_subtitles` tool to `mcp_server.go`
  - Parameters: `subtitles` (required array of `{text, start_ms, end_ms}` objects)
  - Replaces entire subtitle track, allows agent to craft precise narration
- [x] Add `get_recording_status` tool to `mcp_server.go`

## Phase 3: Subtitle Generation

- [x] Implement `Subtitle` struct with text, start_ms, end_ms
- [x] Create `generateWebVTT()` function to convert subtitle buffer to VTT format
- [x] Write VTT file alongside MP4 on stop

## Phase 4: File Storage & Cleanup

- [x] Store recordings in `/tmp/helix-recordings/<session_id>/<recording_id>/` (accessible locally)
- [x] Add automatic cleanup on session end (desktop-bridge shutdown hook)
- [ ] (Future) Upload to filestore for persistent storage across sessions

## Phase 5: Testing

- [ ] Unit test: RecordingManager start/stop lifecycle
- [ ] Unit test: WebVTT generation from subtitle buffer
- [ ] Integration test: MCP tool E2E (start → add subtitles → stop → verify files)
- [ ] Manual test: Agent records demo, subtitles appear correctly

## Phase 6: Documentation

- [ ] Add recording tools to MCP server tool list in README/docs
- [ ] Document filestore path convention for recordings
- [ ] Add example agent workflow using recording tools