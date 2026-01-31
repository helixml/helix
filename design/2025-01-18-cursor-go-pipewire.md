# Cursor Handling: GNOME Shell Extension

**Status:** Implemented
**Decision:** Use GNOME Shell extension with Unix socket. NO PipeWire cursor metadata. NO file-based IPC.

## Goal
Read cursor sprite/hotspot from GNOME, send to frontend via WebSocket.

## Architecture
```
Meta.CursorTracker → GNOME Shell Extension (JS) → Unix Socket → Go Process → WebSocket → Frontend
```

## What NOT To Do
- **NO file-based IPC** (`/tmp/helix-cursor.bin`)
- **NO Rust-to-Go file passing**
- **NO PipeWire cursor metadata** (doesn't work in GNOME headless mode)
- **NO CGO PipeWire clients** (complex, hard to debug)
- The Rust GStreamer plugin handles VIDEO only

## Key Research Findings

### 1. Cursor Updates Require Video Buffers
You cannot receive cursor updates without also receiving video buffers:
- Cursor info (position, hotspot, pixmap) is sent as SPA metadata **attached to each video buffer**
- New cursor update only arrives when a new frame is produced or cursor moves
- **Workaround:** Pull frames but immediately discard video data after extracting cursor metadata

### 2. Each Client Needs Its Own Stream
Cannot attach a second PipeWire client to an existing node:
- Each screen-sharing client creates its own PipeWire stream
- Even if targeting same screen, Mutter spawns separate source node for each client session
- Must initiate new session via `org.freedesktop.portal.ScreenCast` D-Bus interface

### 3. Implementation for Go Process
1. **D-Bus Initiation:** Use XDG Desktop Portal to start screencast session
2. **PipeWire Client:** Connect to provided FD using PipeWire bindings
3. **Buffer Processing:**
   - Subscribe to `SPA_META_Cursor` metadata
   - When buffer arrives, ignore video pixels (`datas` array)
   - Extract `spa_meta_cursor` struct from metadata
   - **CRITICAL:** Return buffer immediately via `pw_stream_queue_buffer` or stream stalls

## Current Implementation

### GNOME Shell Extension
The extension monitors `Meta.CursorTracker` for cursor changes:
- `desktop/ubuntu-config/gnome-extension/helix-cursor@helix.ml/extension.js`
- Uses GNOME 49 ESM format (`export default class extends Extension`)
- Gets cursor tracker via `global.backend.get_cursor_tracker()` (GNOME 49 API)
- Connects to `cursor-changed` signal
- Extracts sprite texture via `get_sprite()` (returns `Cogl_Texture2D` in GNOME 49)
- Gets hotspot via `get_hot()`
- Reads pixel data using `texture.get_data(Cogl.PixelFormat.RGBA_8888, rowstride, buffer)`
  - Must pass pre-allocated `Uint8Array` buffer (GJS introspection quirk)
- Encodes pixels as base64 and sends JSON to Unix socket
- **Retry mechanism:** Retries sending every 2s until socket is available (socket created when client connects)

### Go Socket Listener
- `api/pkg/desktop/cursor_socket.go` - Unix socket server
- Listens on `/run/user/1000/helix-cursor.sock`
- Parses JSON messages with base64-encoded pixel data
- Calls callback with hotspot, dimensions, and RGBA pixels

### Integration in ws_stream.go
- `monitorCursorPipeWire()` registers with `SharedCursorBroadcaster` (singleton)
- Broadcaster manages single socket listener shared by all VideoStreamers
- Extension retries until socket exists, then sends cursor data
- No CGO required, no PipeWire complexity

### Code Locations
- `desktop/ubuntu-config/gnome-extension/helix-cursor@helix.ml/` - GNOME Shell extension
- `api/pkg/desktop/cursor_socket.go` - Go Unix socket listener + `SharedCursorBroadcaster`
- `api/pkg/desktop/ws_stream.go` - `monitorCursorPipeWire()` registers with broadcaster

### Rust Plugin (Video Only)
The Rust `pipewirezerocopysrc` plugin handles VIDEO ONLY:
- No cursor extraction
- Video-only processing in `on_stream_process`

## Frontend Note
Frontend already tracks cursor POSITION client-side. Server only needs to provide cursor SHAPE (bitmap/hotspot) when it changes.

## Historical Note: GNOME Headless Mode Issue

PipeWire cursor metadata (SPA_META_Cursor) does NOT appear in buffers in GNOME headless mode.
We tried:
- cursor-mode=2 (Metadata) on session
- pw_stream_update_params requesting SPA_META_Cursor with CHOICE_RANGE
- Both linked and standalone sessions

Root cause: GNOME headless mode doesn't provide cursor metadata in PipeWire buffers.

**Solution:** Use GNOME Shell extension (Meta.CursorTracker) instead of PipeWire metadata.

## Cursor Compositing for Screenshots (MCP/Agent)

### Problem
With the Helix-Invisible cursor theme, the system cursor is transparent. Even when capturing
screenshots with `cursor-mode=1` (Embedded), the cursor isn't visible. AI agents using the
MCP screenshot tool couldn't see cursor position.

### Solution: Server-Side Cursor Compositing
Composite cursor sprites onto screenshots before sending to agents.

#### Implementation (api/pkg/desktop/)

1. **`cursor_state.go`** - Global cursor state singleton
   - Tracks cursor position (x, y) and shape (cursorName)
   - Updated by VideoStreamer (shape) and input handlers (position)
   - Read by screenshot functions when compositing

2. **`cursor_sprites.go`** - Cursor sprite generation
   - Programmatically generates cursor images in Go
   - **Standard Adwaita style** (white body, black outline) for LLM recognition
   - Supports 20+ cursor types matching CSS cursor names
   - `CompositeCursorOnImage()`, `CompositeCursorOnPNG()`, `CompositeCursorOnJPEG()`

3. **`screenshot.go`** - Integration
   - `captureGNOMEScreenshotWithCursor()` composites cursor when `includeCursor=true`
   - Gets cursor state from `GetGlobalCursorState().Get()`
   - Composites after capture, before returning to client

4. **`ws_stream.go`** - Shape tracking
   - `sendCursorName()` calls `GetGlobalCursorState().UpdateShape()`
   - Updates global state whenever cursor shape changes

5. **`ws_input.go`** - Position tracking
   - `handleWSMouseAbsoluteWithClient()` calls `GetGlobalCursorState().UpdatePosition()`
   - Updates global state on every mouse movement

### Note: Sway Does Not Need Compositing
Sway uses a normal visible cursor theme. The `grim -c` flag captures the actual cursor.
Compositing is only needed for GNOME with the Helix-Invisible transparent cursor theme.

## Multi-User Cursor Shape Fix

### Problem
When User A moves the mouse and hovers over a button, User A's local cursor doesn't update
to the new shape (e.g., "pointer"). But User B can see User A's remote cursor change correctly.

### Root Cause
Each `VideoStreamer` was creating its own `CursorSocketListener` using the same Unix socket
path (`/run/user/1000/helix-cursor.sock`). When User B connected:
1. User B's VideoStreamer calls `NewCursorSocketListener()`
2. This removes User A's existing socket file
3. Creates a new socket listener
4. User A's listener is orphaned - never receives cursor events

Result: Only the last-connected user received cursor shape updates.

### Fix: SharedCursorBroadcaster
Created a shared cursor broadcaster singleton that:
1. Manages a single Unix socket listener
2. All VideoStreamers register callbacks with the broadcaster
3. When cursor events arrive, broadcasts to ALL registered callbacks

```go
// cursor_socket.go - SharedCursorBroadcaster
type SharedCursorBroadcaster struct {
    callbacks map[uint64]CursorSocketCallback
    // ...
}

// ws_stream.go - monitorCursorPipeWire now uses shared broadcaster
broadcaster := GetSharedCursorBroadcaster(v.logger)
callbackID := broadcaster.Register(func(...) { ... })
defer broadcaster.Unregister(callbackID)
```

### Frontend Logic (Unchanged)
The frontend correctly attributes cursor shapes using `lastMoverID`:
- If `lastMoverID === selfClientId`: update local cursor
- If `lastMoverID !== selfClientId`: update remote cursor for that user

This creates the "multiple cursors" illusion where each user sees their own cursor
reflecting their actions, while also seeing other users' cursors.

## GNOME Animation Frame Rate Issue

### Problem
Desktop animations (alt-tabbing, window transitions) run at ~1 FPS instead of 60 FPS
in GNOME headless mode with ScreenCast.

### Experiments Tried

**1. max_framerate=60/1**
- Result: Judder, every other frame skipped, caps at ~30-40 FPS
- Why: Mutter's rate limiting skips frames that come within 16.6ms of each other

**2. max_framerate=360/1 (high value)**
- Theory: Keep follow-up mechanism active while having short min_interval (2.7ms)
- Result: WORSE - still capped at ~30-40 FPS with judder
- Conclusion: Any non-zero max_framerate triggers Mutter's problematic rate limiting

**3. max_framerate=0/1 (CURRENT)**
- Result: Full 60 FPS for normal content, but animations run at ~1 FPS
- Why: Disables rate limiting entirely, but also disables the follow-up mechanism
- Trade-off: We accept slow animations because the alternative is worse

### Root Cause (Mutter Internals)
The `max_framerate` parameter controls the ScreenCast virtual monitor's refresh rate:

```c
// meta-screen-cast-virtual-stream-src.c:661-662
refresh_rate = ((float) video_format->max_framerate.num /
                video_format->max_framerate.denom);
info = meta_virtual_monitor_info_new(width, height, refresh_rate, ...);
```

- With `max_framerate=0/1`: refresh_rate=0, which paradoxically allows smooth 60 FPS
  (possibly inherits from the main `--virtual-monitor WxH@60` setting)
- With `max_framerate=360/1`: refresh_rate=360 Hz, causes conflicts that cap FPS at 30-40

The exact mechanism is unclear, but the observation is consistent: any non-zero
max_framerate causes frame rate issues in GNOME headless ScreenCast.

### Current Solution
- Use `max_framerate=0/1` to disable rate limiting (no judder, full 60 FPS)
- Accept that animations run at ~1 FPS (follow-up mechanism disabled)
- Use 100ms keepalive to provide 10 FPS minimum for static screens

**Code location:** `desktop/gst-pipewire-zerocopy/src/pipewire_stream.rs`

## Design Principles
1. **Minimize CGO** - Hard to debug, memory issues
2. **Use pure Go** where libraries exist
3. **Fallback to D-Bus** when PipeWire metadata unavailable
4. **Standard cursor appearance** - Use Adwaita style for LLM training data recognition

## References
- [Mutter MR !2393](https://gitlab.gnome.org/GNOME/mutter/-/merge_requests/2393) - Fix cursor metadata with unthrottled input
- [spa_meta_cursor docs](https://docs.pipewire.org/structspa__meta__cursor.html)
- [XDG Desktop Portal ScreenCast](https://flatpak.github.io/xdg-desktop-portal/docs/doc-org.freedesktop.impl.portal.ScreenCast.html)
- [Meta.CursorTracker API](https://mutter.gnome.org/meta/class.CursorTracker.html)
