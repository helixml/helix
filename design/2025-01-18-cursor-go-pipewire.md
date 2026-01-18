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
- `monitorCursorPipeWire()` creates `CursorSocketListener` when client connects
- Extension retries until socket exists, then sends cursor data
- No CGO required, no PipeWire complexity

### Code Locations
- `desktop/ubuntu-config/gnome-extension/helix-cursor@helix.ml/` - GNOME Shell extension
- `api/pkg/desktop/cursor_socket.go` - Go Unix socket listener
- `api/pkg/desktop/ws_stream.go` - `monitorCursorPipeWire()` uses socket listener

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

## Design Principles
1. **Minimize CGO** - Hard to debug, memory issues
2. **Use pure Go** where libraries exist
3. **Fallback to D-Bus** when PipeWire metadata unavailable

## References
- [Mutter MR !2393](https://gitlab.gnome.org/GNOME/mutter/-/merge_requests/2393) - Fix cursor metadata with unthrottled input
- [spa_meta_cursor docs](https://docs.pipewire.org/structspa__meta__cursor.html)
- [XDG Desktop Portal ScreenCast](https://flatpak.github.io/xdg-desktop-portal/docs/doc-org.freedesktop.impl.portal.ScreenCast.html)
- [Meta.CursorTracker API](https://mutter.gnome.org/meta/class.CursorTracker.html)
