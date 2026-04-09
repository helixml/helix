# Design: Continue Agent Screencast Recording

## Current Architecture

The recording feature follows a two-layer architecture:

```
Agent (Zed/Claude) → MCP Tools → API Gateway (RevDial) → MCPServer → HTTP → desktop.Server → RecordingManager
```

- **MCPServer** (`mcp_server.go`): Registers 5 recording tools, proxies to HTTP endpoints via `desktopURL` (localhost:9876)
- **desktop.Server** (`desktop.go`): Hosts HTTP endpoints, owns `RecordingManager`
- **RecordingManager** (`recording.go`): Core logic — subscribes to `SharedVideoSource`, writes H.264, converts with ffmpeg
- Both MCPServer and desktop.Server run in the same `desktop-bridge` process

## Key Findings from Review

### 1. Merge Conflict Resolution (DONE)

Main introduced two changes that conflicted:
- **MCP handler mount** (`desktop.go:491-493`): `mux.Handle("/mcp", s.mcpHandler)` — mounts the MCP StreamableHTTP handler on the desktop HTTP server. Added alongside recording endpoints.
- **SSE → StreamableHTTP** (`mcp_server.go`): Main replaced `SSEServer` with `StreamableHTTPServer`. Recording tools now use the new transport.

### 2. Code Issues to Fix

**Race condition (recording_handlers.go:20-30)**:
```go
// Current (buggy): no synchronization on lazy init
if s.recordingManager == nil {
    s.recordingManager = NewRecordingManager(sessionID, nodeID)
}
```
Fix: Use `sync.Once` on the Server struct.

**Non-deferred inner mutex (recording.go:214-220)**:
```go
// Current (panic-unsafe):
m.active.mu.Lock()
m.active.Subtitles = append(...)
m.active.mu.Unlock()
```
Fix: Use `defer m.active.mu.Unlock()` or restructure to avoid nested locks.

**Logging**: Uses `fmt.Printf("[RECORDING] ...")` instead of structured logging. Should use the server's `slog.Logger`.

### 3. MCP Routing Compatibility

The MCP routing change (PR #1850) routes MCP traffic through the API gateway via RevDial. The recording tool handlers are registered on `MCPServer.mcpServer` which is served via `StreamableHTTPServer`. When a tool is called:

1. Request arrives via API gateway → RevDial → desktop-bridge `/mcp` endpoint
2. `StreamableHTTPServer` dispatches to the tool handler
3. Tool handler makes HTTP POST to `desktopURL` (localhost:9876) — same process
4. desktop.Server handler calls `RecordingManager` directly

This architecture is sound — the internal HTTP call is within the same process. No changes needed for MCP routing compatibility.

### 4. Codebase Patterns Observed

- **Build tags**: CGO code uses `//go:build cgo && linux`, stubs use `//go:build !cgo || !linux`
- **Error responses**: HTTP handlers use `http.Error()` with JSON bodies
- **MCP tools**: Use `mcp.NewTool()` + `m.mcpServer.AddTool()` pattern
- **Video frames**: `SharedVideoSource.Subscribe()` returns `(frameCh, errorCh, clientID, err)`
- **Replay frames**: GOP warmup frames have `frame.IsReplay = true` — must be skipped in recordings

### 5. Testing Strategy

**Unit tests exist for**: WebVTT generation, struct creation (trivial)

**Missing tests**: 
- Integration test with real SharedVideoSource
- E2E test via MCP tools
- Edge cases: concurrent start, stop without start, empty recording

**E2E testing requires**:
1. `./stack build-ubuntu` to rebuild desktop-bridge
2. Start new session (old sessions use old image)
3. Call MCP tools through running agent
4. Verify MP4 in `/tmp/helix-recordings/`

## Decision: Scope of This Task

Focus on making the existing implementation mergeable:
1. Fix the identified code quality issues (race condition, mutex patterns, logging)
2. Verify compilation after merge
3. Attempt E2E test if environment permits
4. Do NOT add new features (filestore upload, burned-in subtitles, etc.)
