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
- **Status:** Partially implemented (as stream-stats)
- Connects to Moonlight WebSocket
- Shows real-time statistics: message rate, bitrate, message sizes
- Supports `--duration` for timed runs
- Supports `--output` to save video data to file

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

## CLI Stream-Stats Testing Results (2026-01-04)

### Fixed: Lobby-Based Session Detection

The stream-stats CLI initially failed with "AppNotFound" because external agent sessions use Wolf **lobbies**, not apps. The session's `wolf_app_id` is a lobby config ID, not a Wolf app.

**Fix:** Detect lobby-based sessions and use the placeholder app ID:
1. Check `session.config.wolf_lobby_id` to detect lobby-based sessions
2. For lobby sessions, fetch `/api/v1/wolf/ui-app-id?session_id=XXX`
3. Use "Select Agent" app (ID 985743958) for lobby attachment

### Fixed: AlreadyStreaming Handling

When a Wolf stream session already exists (from previous CLI test), "create" mode returns "AlreadyStreaming" error.

**Fix:** Added `--join` flag to use "join" mode in AuthenticateAndInit message.

### Signaling Stability Test Results

| Duration | Mode | Result | Notes |
|----------|------|--------|-------|
| 15s | create | ✅ | Full signaling stages: Launch Streamer → WebRTC Peer Setup → Negotiation |
| 30s | join | ✅ | Stable connection, received WebRTC config |
| 60s | join | ✅ | Stable connection, no disconnects |
| 2 min | join | ✅ | Stable connection, no disconnects |

**Observation:** Only 1 message received (WebRTC config) because CLI doesn't complete WebRTC negotiation (no SDP answer, no ICE candidates). This is expected - we're testing signaling stability, not video streaming.

### Moonlight Signaling Protocol

The AuthenticateAndInit message format for lobby-based sessions:

```json
{
  "AuthenticateAndInit": {
    "credentials": "<token>",
    "session_id": "agent-<helix_session_id>",
    "mode": "create|join",
    "client_unique_id": "cli-<timestamp>",
    "host_id": 0,
    "app_id": 985743958,  // Select Agent placeholder app
    "bitrate": 10000,
    "fps": 60,
    "width": 1920,
    "height": 1080,
    "video_supported_formats": 1,  // H264
    "video_colorspace": "Rec709",
    "video_color_range_full": true
  }
}
```

## Files Modified

- `api/pkg/desktop/session.go` - Standalone ScreenCast fallback
- `api/pkg/desktop/desktop.go` - Added standaloneScreenCast flag
- `api/pkg/cli/spectask/spectask.go` - Added stream/stream-stats commands, lobby detection, --join flag
