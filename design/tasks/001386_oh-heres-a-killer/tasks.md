# Implementation Tasks: Agent Screencast Recording

## Phase 1: Core Recording Infrastructure

- [ ] Create `api/pkg/desktop/recording.go` with `RecordingManager` struct
- [ ] Implement `Recording` struct with ID, title, start time, output path, subtitle buffer
- [ ] Add method to subscribe to `SharedVideoSource` frame channel
- [ ] Create GStreamer pipeline: `appsrc → h264parse → mp4mux → filesink`
- [ ] Implement frame forwarding from SharedVideoSource to recording appsrc

## Phase 2: MCP Tools

- [ ] Add `start_recording` tool to `mcp_server.go`
  - Parameters: `title` (optional string)
  - Creates Recording, starts pipeline, returns recording ID
- [ ] Add `stop_recording` tool to `mcp_server.go`
  - Finalizes MP4 file, generates VTT from subtitle buffer
  - Uploads files to filestore
  - Returns filestore URL
- [ ] Add `add_subtitle` tool to `mcp_server.go`
  - Parameters: `text` (required), `duration_ms` (optional, default 3000)
  - Appends timestamped subtitle to active recording

## Phase 3: Subtitle Generation

- [ ] Implement `Subtitle` struct with text, timestamp, duration
- [ ] Create `generateWebVTT()` function to convert subtitle buffer to VTT format
- [ ] Write VTT file alongside MP4 on stop

## Phase 4: File Upload & Cleanup

- [ ] Use existing `/upload` endpoint to push MP4 and VTT to filestore
- [ ] Clean up `/tmp/helix-recordings/` files after successful upload
- [ ] Add automatic cleanup on session end (desktop-bridge shutdown hook)

## Phase 5: Testing

- [ ] Unit test: RecordingManager start/stop lifecycle
- [ ] Unit test: WebVTT generation from subtitle buffer
- [ ] Integration test: MCP tool E2E (start → add subtitles → stop → verify files)
- [ ] Manual test: Agent records demo, subtitles appear correctly

## Phase 6: Documentation

- [ ] Add recording tools to MCP server tool list in README/docs
- [ ] Document filestore path convention for recordings
- [ ] Add example agent workflow using recording tools