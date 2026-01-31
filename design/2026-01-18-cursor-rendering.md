# Cursor Rendering Design

Date: 2026-01-18

## Summary

Client-side cursor rendering to eliminate compositor cursor from video stream. Cursor shape/bitmap captured from compositor and rendered in frontend.

## Current State

### Sway (ext-image-copy-capture-cursor-session-v1)

**Status: WORKING**

The Wayland ext-image-copy-capture protocol provides cursor bitmap capture.

**Bugs Fixed:**
1. **Hotspot/bitmap race condition** - FIXED in `wayland_cursor.go`
   - Problem: `cursor_session_hotspot()` callback sent cursor with OLD bitmap + NEW hotspot
   - Solution: Removed callback from hotspot and position events, only send from `frame_ready`

2. **Missing cursor updates** - FIXED in `wayland_cursor.go`
   - Problem: After fix #1, cursor updates were missed because frame capture only happened on 100ms timer
   - Solution: Added `wl_cursor_client_capture_frame()` call in hotspot handler to immediately request new frame when cursor shape changes

**Key Code Locations:**
- `api/pkg/desktop/wayland_cursor.go` - Wayland cursor capture client
- `api/pkg/desktop/ws_stream.go:monitorCursorWayland()` - Cursor monitoring goroutine

### GNOME (Meta.CursorTracker + GNOME Shell Extension)

**Status: IMPLEMENTED with multi-layer fallback**

**Root Issue:** In GNOME 49 headless mode, `Cogl.Texture.get_data()` fails to transfer GPU texture data to CPU memory.

**Solution Implemented (3-layer approach):**

1. **Cogl.Offscreen + read_pixels()** (Primary)
   - Creates offscreen framebuffer, draws cursor texture to it
   - Uses `read_pixels()` to force GPU→CPU transfer
   - May work where `get_data()` fails

2. **Direct texture.get_data()** (Fallback)
   - Standard Cogl texture read
   - Works in non-headless mode

3. **Hotspot Fingerprinting** (Final fallback)
   - When pixels unavailable, detect cursor shape from hotspot position
   - Map hotspot patterns to CSS cursor names (default, pointer, text, etc.)
   - Send cursor name instead of pixels to frontend
   - Frontend renders SVG cursors matching the shape

**Wire Protocol Extension:**
- New message type `0x51` (StreamMsgCursorName)
- Format: `type(1) + hotspotX(4) + hotspotY(4) + nameLen(1) + name(...)`
- Frontend event: `{ type: "cursorName", cursorName: string, hotspotX, hotspotY }`

**GNOME Shell Extension:**
- Location: `desktop/ubuntu-config/gnome-extension/helix-cursor@helix.ml/`
- Listens to `Meta.CursorTracker.cursor-changed` signal
- Tries Cogl.Offscreen, then get_data(), then fingerprinting
- Sends either pixels OR cursor_name to socket

**Socket Protocol (JSON):**
```json
{
  "hotspot_x": number,
  "hotspot_y": number,
  "width": number,
  "height": number,
  "pixels": string,       // Base64 RGBA (if available)
  "cursor_name": string   // CSS cursor name fallback
}
```

### Frontend Cursor Rendering

**Location:** `frontend/src/components/external-agent/DesktopStreamViewer.tsx`

**State:**
- `cursorImage: CursorImageData | null` - Contains imageUrl, hotspot, dimensions
- `cursorCssName: string | null` - CSS cursor name for fallback rendering
- `cursorPosition: {x, y}` - Tracked locally from mouse events

**Rendering Priority:**
1. If `cursorImage` available → render bitmap cursor
2. Else if `cursorCssName` available → render SVG cursor matching name
3. Else → render circle fallback indicator

**SVG Cursors Implemented:**
- `default` - Arrow pointer
- `pointer` - Hand/link cursor
- `text` - I-beam text cursor
- All others → fallback to default arrow

**Pixel Format Handling:** `frontend/src/lib/helix-stream/stream/websocket-stream.ts`
- `convertCursorBitmapToDataUrl()` handles ARGB8888, RGBA8888, BGRA8888, ABGR8888
- DRM format codes to Canvas RGBA conversion

## Wire Protocol

### StreamMsgCursorImage (0x50)

**Format:**
```
type(1) + lastMoverID(4) + posX(4) + posY(4) + hotspotX(4) + hotspotY(4) + bitmapSize(4) + [bitmap_header + pixels]

bitmap_header (16 bytes):
  format(4) + width(4) + height(4) + stride(4)

pixels:
  Raw pixel data in format specified by format field
```

### StreamMsgCursorName (0x51) - NEW

**Format:**
```
type(1) + hotspotX(4) + hotspotY(4) + nameLen(1) + name(...)
```

Used when pixel capture fails and only cursor shape name is available.

**DRM Format Codes:**
- `0x34325241` - ARGB8888 (AR24) - Little-endian bytes: B,G,R,A
- `0x34324152` - RGBA8888 (RA24) - Little-endian bytes: R,G,B,A
- `0x34324142` - BGRA8888 (BA24) - Little-endian bytes: B,G,R,A
- `0x34324241` - ABGR8888 (AB24) - Little-endian bytes: R,G,B,A

## CLI Debug Tools

**ASCII Art Cursor Rendering:** `api/pkg/cli/spectask/spectask.go`
- `renderCursorASCII()` function displays cursor bitmap as ASCII art
- Shows sample pixel values for format debugging
- Usage: `helix spectask stream <session-id> -v`

## Known Issues

### GNOME 30fps on Static Screen
- User reports constant 30fps even with static screen
- Expected: ~10fps keepalive on damage-based ScreenCast
- TODO: Investigate PipeWire stream configuration

### GNOME Cursor Flicker
- Hardware cursor visible even with cursor-mode=2
- Solution: Use transparent cursor theme + client-side rendering
- Requires fixing client-side cursor first

## Hotspot Fingerprinting Table

For Adwaita theme at 48x48 scale:
| Hotspot (x,y) | Size | Cursor Name |
|---------------|------|-------------|
| (7,7) | 48x48 | default |
| (0,0) | 48x48 | default |
| (24,24) | 48x48 | text (or crosshair/move) |
| (14,5) | 48x48 | pointer |

**Heuristic Fallback:**
- Hotspot in top-left quarter → `default`
- Hotspot centered → `text`
- Hotspot at top-center → `pointer`

## Files Modified

### Backend (Go)
- `api/pkg/desktop/wayland_cursor.go` - Sway cursor capture fixes
- `api/pkg/desktop/ws_stream.go` - Cursor monitoring, wire protocol, sendCursorName()
- `api/pkg/desktop/cursor_socket.go` - GNOME extension socket listener (now with cursorName)
- `api/pkg/cli/spectask/spectask.go` - ASCII art cursor debug

### GNOME Shell Extension
- `desktop/ubuntu-config/gnome-extension/helix-cursor@helix.ml/extension.js`
  - Added Cogl.Offscreen approach for pixel capture
  - Added hotspot fingerprinting fallback
  - Added cursor_name field to socket protocol

### Frontend (TypeScript/React)
- `frontend/src/lib/helix-stream/stream/websocket-stream.types.ts` - Added CursorName message type
- `frontend/src/lib/helix-stream/stream/websocket-stream.ts` - handleCursorName() method
- `frontend/src/components/external-agent/DesktopStreamViewer.tsx` - SVG cursor rendering

## Testing

### Sway
```bash
# Start a new session
/tmp/helix spectask start --project $HELIX_PROJECT -n "sway-cursor-test"

# Run stream with verbose mode to see cursor updates
/tmp/helix spectask stream ses_xxx -v

# Move mouse over different UI elements (text fields, buttons, links)
# Verify ASCII art cursor changes and hotspot values are correct
```

### GNOME (Ubuntu)
```bash
# Start a new session
/tmp/helix spectask start --project $HELIX_PROJECT -n "gnome-cursor-test"

# Check extension logs
docker compose exec -T sandbox-nvidia docker logs <container> 2>&1 | grep HelixCursor

# Look for either:
# - "Offscreen read_pixels worked" (pixel capture working)
# - "Using fingerprint fallback: cursor_name=default" (using CSS fallback)
```
