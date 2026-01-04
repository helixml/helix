# Video Streaming CLI and PipeWire Debugging

**Date:** 2026-01-04
**Status:** In Progress

## Summary

Implementing a comprehensive CLI tool for video streaming that supports:
1. Screenshot capture
2. Video stream statistics
3. VLC-compatible HTTP streaming server
4. Keyboard input redirection

## Current Findings

### PipeWire Video Producer Exit (Critical Bug)

The PipeWire video stream disconnects after ~27 seconds, causing Wolf to stop the lobby and kill the container:

```
10:31:05 INFO  | [LOBBY] PipeWire node ID 45 received for lobby, starting pipewiresrc video producer
...27 seconds later...
error: streaming stopped, reason error (-5)
10:31:32 ERROR | [GSTREAMER] Pipeline error: Internal data stream error.
10:31:32 WARN  | [LOBBY] PipeWire video producer exited for lobby, stopping lobby
10:31:34 DEBUG | [DOCKER] Stopping container
```

**Root Cause Investigation:**
- GStreamer pipeline error: "Internal data stream error" with reason `-5` (source disconnected)
- The PipeWire stream source is being lost after ~27 seconds
- This suggests the ScreenCast D-Bus session is ending prematurely
- Possible causes:
  1. screenshot-server crashes or exits
  2. GNOME Shell exits (would kill D-Bus session)
  3. ScreenCast session times out
  4. PipeWire daemon issue

**Timeline (confirmed):**
1. Container starts, GNOME Shell launches in headless mode (`--headless --virtual-monitor`)
2. screenshot-server starts, creates standalone ScreenCast session (GNOME 49 mode)
3. PipeWire node ID 45 is assigned and reported to Wolf at ~T+4s
4. Wolf starts pipewiresrc GStreamer pipeline with this node ID
5. Stream runs normally for ~27 seconds with healthy heartbeats
6. At T+27s: GStreamer receives "streaming stopped, reason error (-5)"
7. Pipeline error triggers lobby shutdown and container kill

**NOT the watchdog:** The thread monitor shows normal heartbeats (3001 over 26s) before exit.

**Key Log Evidence:**
```
10:31:32.028280975 INFO  | [THREAD_MONITOR] Unregistered thread: TID=28428 name=GStreamer-Pipeline (lived 26s, 3001 heartbeats)
```

**Next Debug Steps:**
1. Check `/tmp/screenshot-server.log` inside container for Go binary output
2. Check if GNOME Shell or the D-Bus session is exiting
3. Add more logging to screenshot-server to track ScreenCast session state

### Desktop Type Configuration

When testing, ensure you're using the correct agent configuration:

- **Ubuntu desktop** (`desktop_type: "ubuntu"`): Uses PipeWire mode with GNOME 49 headless
- **KDE desktop** (`desktop_type: "kde"`): Uses Wayland mode with KDE Plasma

Agent `app_01kchs65wezc7ewxemj9px0gcv` uses Ubuntu desktop (correct for PipeWire testing).
Agent `app_01kcgh9ek4w96kws0fke5b7q30` uses KDE desktop (Wayland mode).

KDE containers may fail with Qt Wayland shell integration errors:
```
qt.qpa.wayland: Loading shell integration failed.
startplasma-wayland: Shutting down...
```

### GNOME 49 Standalone ScreenCast Fix

Implemented fallback for GNOME 49 which doesn't support linking ScreenCast to RemoteDesktop sessions:

```go
// In session.go:createSession()
if !linkedOK {
    s.logger.Info("creating standalone ScreenCast session (GNOME 49+ mode)...")
    emptyOptions := map[string]dbus.Variant{}
    scObj.Call(screenCastIface+".CreateSession", 0, emptyOptions).Store(&scSessionPath)
    s.standaloneScreenCast = true
}

// In session.go:startSession()
if s.standaloneScreenCast {
    scSession.Call(screenCastSessionIface+".Start", 0)
} else {
    rdSession.Call(remoteDesktopSessionIface+".Start", 0)
}
```

## Planned CLI Features

### 1. `helix spectask screenshot <session-id>`
- **Status:** Implemented
- Takes screenshot via RevDial connection to desktop container
- Saves PNG to local file

### 2. `helix spectask stream <session-id>`
- **Status:** Implemented (using custom WebSocket video protocol)
- Connects to `/moonlight/host/api/ws/stream` WebSocket endpoint
- Uses simpler binary protocol (not WebRTC) with raw H.264/HEVC/AV1 frames
- Shows real-time statistics: FPS, bitrate, keyframes, codec, resolution
- Supports `--duration` for timed runs
- Supports `--output` to save raw video frames to file
- Supports `--verbose` to see individual frame details

### 3. VLC HTTP Streaming Server
- **Status:** Implemented
- Starts local HTTP server that VLC can connect to
- Streams raw video data from Moonlight WebSocket to HTTP clients
- Supports multiple simultaneous VLC connections
- Usage: `helix spectask stream <session-id> --vlc-server :8889`
- Then: `vlc http://localhost:8889/stream`
- Additional endpoints: `/` (info), `/stats` (connection count)

### 4. Keyboard Redirection
- **Status:** Planned
- Send keyboard input from CLI to remote desktop
- Capture terminal keystrokes and forward to Wolf
- Usage: `helix spectask stream <session-id> --keyboard`

## Next Steps

1. **Debug pipewiresrc exit** - Need to understand why GStreamer's pipewiresrc is exiting
   - Check if PipeWire daemon is crashing
   - Check if ScreenCast stream is ending prematurely
   - May need to keep screenshot-server running to maintain the D-Bus session

2. **Implement VLC HTTP server** - Add HTTP streaming endpoint for VLC

3. **Implement keyboard input** - Capture and forward keystrokes

## Technical Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│ CLI (helix spectask stream)                                          │
│  ├── WebSocket → Moonlight → Wolf → Video frames                    │
│  ├── HTTP Server → VLC connection                                   │
│  └── Keyboard capture → Wolf input socket                           │
└─────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────┐
│ Desktop Container                                                    │
│  ├── GNOME Shell (headless, PipeWire mode)                          │
│  ├── screenshot-server (D-Bus session management)                   │
│  │    ├── Standalone ScreenCast session (GNOME 49+)                 │
│  │    ├── PipeWire node registration                                │
│  │    └── Input socket bridge                                       │
│  └── PipeWire → Wolf → GStreamer encoding                           │
└─────────────────────────────────────────────────────────────────────┘
```

## CLI Stream Command (2026-01-04)

### Custom WebSocket Video Protocol

The `helix spectask stream` command uses a custom WebSocket-only video protocol instead of WebRTC. This is simpler and allows direct access to raw video frames for debugging.

**Endpoint:** `/moonlight/host/api/ws/stream?session_id=agent-{helix_session_id}`

**Binary Message Format:**
- VideoFrame (0x01): type(1) + codec(1) + flags(1) + pts(8) + width(2) + height(2) + NAL data
- AudioFrame (0x02): type(1) + codec(1) + flags(1) + pts(8) + audio data
- VideoBatch (0x03): type(1) + count(2) + [frames...]
- StreamInit (0x30): type(1) + codec(1) + width(2) + height(2) + fps(1) + ...

**Video Codecs:**
- 0x01 = H.264
- 0x02 = H.264 High 4:4:4
- 0x10 = HEVC
- 0x11 = HEVC Main10
- 0x20 = AV1 Main8
- 0x21 = AV1 Main10

**Statistics Displayed:**
- Resolution and codec
- Frame rate (FPS)
- Video bitrate (Mbps)
- Keyframe count
- Average/min/max frame sizes

## Files Modified

- `api/pkg/desktop/session.go` - Standalone ScreenCast fallback
- `api/pkg/desktop/desktop.go` - Added standaloneScreenCast flag
- `api/pkg/cli/spectask/spectask.go` - Added stream/stream-stats commands, lobby detection, --join flag
