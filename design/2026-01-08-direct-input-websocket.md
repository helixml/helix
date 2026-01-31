# Direct WebSocket Input for GNOME/PipeWire Desktop Sessions

## Overview

Redesign input handling to bypass Moonlight Web and Wolf for GNOME/Ubuntu desktop sessions,
sending browser input events directly to the Go screenshot-server via WebSocket.

## Current Architecture (Complex)

```
Browser WheelEvent
    ↓ deltaY in CSS pixels (e.g., -120 for scroll down)
Moonlight Web Frontend (input.ts)
    ↓ Normalize deltaMode, negate Y, clamp to i16
    ↓ sendMouseWheelHighRes(deltaX, -deltaY)
WebSocket Stream (websocket-stream.ts)
    ↓ Pack as: [subType=3, deltaX:i16, deltaY:i16] big-endian
Wolf WebSocket Server
    ↓ Decode, create MOUSE_SCROLL_PACKET
Wolf Input Handler (input_handler.cpp)
    ↓ scroll_amount = big_to_native(pkt.scroll_amt1) * v_scroll_acceleration
    ↓ mouse.vertical_scroll(scroll_amount)
InputBridge (input_bridge.cpp)
    ↓ Send JSON: {"type":"scroll_smooth","dx":0,"dy":scroll_amount}
Go screenshot-server (input.go)
    ↓ mutterDX = -event.DX, mutterDY = -event.DY
    ↓ NotifyPointerAxis(dx, dy, flags=0)
GNOME Mutter D-Bus API
```

**Problems:**
1. 6 translation layers, each potentially negating or scaling values
2. Hard to trace which layer introduced bugs
3. Moonlight protocol expects Windows WHEEL_DELTA (±120 per notch)
4. GNOME expects libinput-style values (different units)
5. No scroll source information (wheel vs finger) passed through

## Proposed Architecture (Direct)

```
Browser WheelEvent
    ↓ Raw event data
Frontend detects PipeWire mode
    ↓ Connect to /api/v1/sessions/{id}/input WebSocket
Direct WebSocket to Helix API
    ↓ Proxy to container's :9876/ws/input
Go screenshot-server (input.go)
    ↓ Convert browser values → GNOME D-Bus values
GNOME Mutter D-Bus API
```

**Benefits:**
1. Single translation point (browser → GNOME)
2. Full browser event context available (deltaMode, wheelDelta, etc.)
3. Can send scroll source (wheel/finger) based on device detection
4. Easy to debug and iterate on

## Browser WheelEvent API

Reference: [MDN WheelEvent](https://developer.mozilla.org/en-US/docs/Web/API/WheelEvent)

### Key Properties

| Property | Type | Description |
|----------|------|-------------|
| `deltaX` | double | Horizontal scroll amount in `deltaMode` units |
| `deltaY` | double | Vertical scroll amount in `deltaMode` units |
| `deltaZ` | double | Z-axis scroll (rare) |
| `deltaMode` | uint | Unit: 0=pixel, 1=line, 2=page |

### deltaMode Values

| Constant | Value | Description |
|----------|-------|-------------|
| `DOM_DELTA_PIXEL` | 0 | Values in CSS pixels |
| `DOM_DELTA_LINE` | 1 | Values in lines (~16-40 pixels/line) |
| `DOM_DELTA_PAGE` | 2 | Values in pages (viewport height) |

### Chrome on macOS Behavior

From research and testing:
- **Always uses `DOM_DELTA_PIXEL` (0)** - Chrome on Mac always sends pixel values
- **Trackpad two-finger scroll**: Small values (1-10 pixels) at high frequency (~60Hz)
- **Mouse wheel**: Larger values (~100-120 pixels per notch)
- **Acceleration**: macOS applies acceleration, values can be hundreds of pixels
- **Direction**: Positive deltaY = content should scroll DOWN (natural scrolling)

### Direction Convention

**Browser convention (WheelEvent):**
- `deltaY > 0`: User scrolled DOWN (finger moved up on trackpad)
- `deltaY < 0`: User scrolled UP (finger moved down on trackpad)

This is the "document scroll" perspective: positive delta = content moves up = scrolled down.

## GNOME Mutter RemoteDesktop D-Bus API

Reference: [gnome-remote-desktop D-Bus interface](https://github.com/GNOME/gnome-remote-desktop/blob/master/src/org.gnome.Mutter.RemoteDesktop.xml)

### NotifyPointerAxis Method

```xml
<method name="NotifyPointerAxis">
  <arg name="dx" type="d" direction="in"/>       <!-- horizontal delta -->
  <arg name="dy" type="d" direction="in"/>       <!-- vertical delta -->
  <arg name="flags" type="u" direction="in"/>    <!-- ClutterScrollFinishFlags + source flags -->
</method>
```

### Parameter Details

**dx, dy (double):**
- Values represent physical scroll distance
- Unit: "scroll units" - similar to libinput's normalized scroll
- For smooth scrolling: 1.0 = ~10 pixels of content scroll (approximate)
- Sign convention: **Negative = content scrolls UP** (opposite of browser!)

**flags (uint32):**
Combination of finish flags and source flags:

```
Bits 0-1: ClutterScrollFinishFlags
  0 = Continuous scrolling (no finish)
  1 = CLUTTER_SCROLL_FINISHED_HORIZONTAL
  2 = CLUTTER_SCROLL_FINISHED_VERTICAL
  3 = Both axes finished

Bits 2-3: Scroll source (since Mutter MR !1636)
  0x00 = FINGER (default, enables kinetic scrolling)
  0x04 = WHEEL (discrete clicks, no kinetic)
  0x08 = CONTINUOUS (on-button scroll, etc.)
```

### libinput v120 Standard

libinput uses "v120" normalization for wheel scrolling:
- One wheel notch = 120 units
- High-resolution wheels can send fractions (e.g., 15, 30, 60 for 8x resolution)
- Windows WHEEL_DELTA is also 120 per notch

**This is NOT what NotifyPointerAxis uses!** The D-Bus API uses floating-point
scroll deltas similar to libinput's `get_scroll_value()`, not `get_scroll_value_v120()`.

### Expected Value Ranges

Based on libinput documentation and GNOME source code analysis:

| Input Source | Typical Value Range | Frequency |
|-------------|---------------------|-----------|
| Mouse wheel (per notch) | ±10-15 | 1-5 Hz |
| Trackpad (per event) | ±1-5 | 60 Hz |
| High-res wheel | ±1-3 | 8-60 Hz |

## Mapping: Browser → GNOME

### Conversion Formula

```go
// Browser WheelEvent → GNOME NotifyPointerAxis

func convertBrowserScrollToGNOME(deltaX, deltaY float64, deltaMode int, isTrackpad bool) (gnomeDX, gnomeDY float64, flags uint32) {
    // Step 1: Normalize to pixels if needed
    var pixelX, pixelY float64
    switch deltaMode {
    case 0: // DOM_DELTA_PIXEL
        pixelX, pixelY = deltaX, deltaY
    case 1: // DOM_DELTA_LINE
        pixelX, pixelY = deltaX * 40, deltaY * 40  // ~40 pixels per line
    case 2: // DOM_DELTA_PAGE
        pixelX, pixelY = deltaX * 800, deltaY * 600  // approximate viewport
    }

    // Step 2: Convert pixel delta to GNOME scroll units
    // GNOME expects smaller values: ~10-15 units per wheel notch
    // Browser sends ~100-120 pixels per wheel notch
    // Ratio: 100 pixels ≈ 10 GNOME units → divide by 10
    gnomeDX = pixelX / 10.0
    gnomeDY = pixelY / 10.0

    // Step 3: Invert Y axis
    // Browser: +Y = scroll down (content moves up)
    // GNOME:   +Y = content moves down (scroll up)
    gnomeDY = -gnomeDY

    // Step 4: Set scroll source flag
    if isTrackpad {
        flags = 0x00  // FINGER source (enables kinetic scrolling)
    } else {
        flags = 0x04  // WHEEL source (discrete)
    }

    return gnomeDX, gnomeDY, flags
}
```

### Detecting Trackpad vs Mouse

The browser doesn't directly tell us if input is from trackpad or mouse.
Heuristics we can use:

1. **deltaMode**: Trackpads on Chrome always use DOM_DELTA_PIXEL
2. **Value magnitude**: Trackpad events are smaller (1-10px) and high frequency
3. **Event frequency**: Trackpads send ~60 events/sec, mice send ~1-5/sec
4. **WebHID API**: Can detect device type (future enhancement)

Simple heuristic:
```typescript
function isLikelyTrackpad(event: WheelEvent): boolean {
    // Trackpad events typically have small deltas and pixel mode
    if (event.deltaMode !== WheelEvent.DOM_DELTA_PIXEL) return false
    const magnitude = Math.abs(event.deltaX) + Math.abs(event.deltaY)
    return magnitude < 50  // Trackpad events are usually < 10px, wheel is > 100px
}
```

### Scroll Finish Detection

For smooth scrolling, GNOME needs to know when a scroll gesture ends:

```go
type scrollState struct {
    lastEventTime time.Time
    finishTimer   *time.Timer
    mu            sync.Mutex
}

func (s *scrollState) handleScroll(dx, dy float64, isTrackpad bool) {
    s.mu.Lock()
    defer s.mu.Unlock()

    // Cancel pending finish timer
    if s.finishTimer != nil {
        s.finishTimer.Stop()
    }

    // Send scroll event
    sendNotifyPointerAxis(dx, dy, sourceFlag(isTrackpad))
    s.lastEventTime = time.Now()

    // Schedule finish signal (only for trackpad - enables kinetic scrolling)
    if isTrackpad {
        s.finishTimer = time.AfterFunc(150*time.Millisecond, func() {
            s.mu.Lock()
            defer s.mu.Unlock()
            // Send finish with both axes completed
            sendNotifyPointerAxis(0, 0, 0x03)  // flags = 3 (both finished)
        })
    }
}
```

## WebSocket Protocol Design

### Endpoint

```
ws://{container}:9876/ws/input
```

Via Helix API with RevDial routing:
```
wss://app.tryhelix.ai/api/v1/sessions/{session_id}/input
    ↓ Helix API (authenticate, look up session)
    ↓ RevDial tunnel (screenshot-server already connected to API)
    ↓ Screenshot-server receives WebSocket
```

The screenshot-server already maintains a RevDial connection to the Helix API
for session sync and heartbeats. This same connection can be used to route
input WebSocket connections back to the container, eliminating the need for
direct container access.

**Routing Decision:**
The Helix API knows the session mode (PipeWire vs Wayland) from the lobby configuration:

```go
func (h *Handler) HandleInputWebSocket(w http.ResponseWriter, r *http.Request) {
    session := h.getSession(r)
    lobby := h.getLobby(session.ID)

    if lobby.Mode == "pipewire" || lobby.DesktopType == "ubuntu" {
        // Route to Go screenshot-server via RevDial
        h.proxyToScreenshotServer(session, w, r)
    } else {
        // Sway mode - return error, use Moonlight input path
        http.Error(w, "Direct input not supported for Sway mode", http.StatusNotImplemented)
    }
}
```

This allows the frontend to always try the direct input WebSocket first,
falling back to Moonlight if the endpoint returns an error.

### Message Format (Binary)

Using a simple binary format for low latency:

```
Message format (little-endian):
  [0]:     uint8   message_type
  [1-N]:   payload (type-specific)

Message Types:
  0x01 = Keyboard
  0x02 = Mouse Button
  0x03 = Mouse Move Absolute
  0x04 = Mouse Move Relative
  0x05 = Scroll (new! - direct browser values)
  0x06 = Touch

Scroll Message (type=0x05):
  [0]:     uint8   message_type (0x05)
  [1]:     uint8   delta_mode (0=pixel, 1=line, 2=page)
  [2]:     uint8   flags (bit 0 = is_trackpad)
  [3-6]:   float32 deltaX (little-endian)
  [7-10]:  float32 deltaY (little-endian)
```

### Frontend Integration

```typescript
class DirectInputWebSocket {
    private ws: WebSocket
    private buffer = new ArrayBuffer(11)
    private view = new DataView(this.buffer)

    sendScroll(event: WheelEvent) {
        const isTrackpad = this.detectTrackpad(event)

        this.view.setUint8(0, 0x05)  // message type
        this.view.setUint8(1, event.deltaMode)
        this.view.setUint8(2, isTrackpad ? 0x01 : 0x00)
        this.view.setFloat32(3, event.deltaX, true)  // little-endian
        this.view.setFloat32(7, event.deltaY, true)

        this.ws.send(new Uint8Array(this.buffer, 0, 11))
    }

    private detectTrackpad(event: WheelEvent): boolean {
        if (event.deltaMode !== WheelEvent.DOM_DELTA_PIXEL) return false
        const magnitude = Math.abs(event.deltaX) + Math.abs(event.deltaY)
        return magnitude < 50
    }
}
```

## Implementation Plan

### Phase 1: Go WebSocket Server
1. Add `/ws/input` endpoint to screenshot-server
2. Implement binary message parsing
3. Implement scroll conversion with correct GNOME mapping
4. Add scroll source flag support
5. Add finish detection timer

### Phase 2: Frontend Integration
1. Add DirectInputWebSocket class
2. Detect when to use direct WS vs Moonlight (based on container type)
3. Wire up scroll events to direct WS when available
4. Add trackpad detection heuristic

### Phase 3: API Proxy
1. Add WebSocket proxy route in Helix API
2. Route to correct container based on session ID
3. Handle authentication/authorization

### Phase 4: Testing & Tuning
1. Test with mouse wheel on various mice
2. Test with MacBook trackpad
3. Test with Magic Mouse
4. Tune conversion factors based on user feedback

## Reference Links

- [MDN WheelEvent](https://developer.mozilla.org/en-US/docs/Web/API/WheelEvent)
- [MDN deltaMode](https://developer.mozilla.org/en-US/docs/Web/API/WheelEvent/deltaMode)
- [libinput Wheel Scrolling](https://wayland.freedesktop.org/libinput/doc/latest/wheel-api.html)
- [libinput v120 API](https://wayland.freedesktop.org/libinput/doc/latest/api/group__event__pointer.html)
- [GNOME gnome-remote-desktop D-Bus XML](https://github.com/GNOME/gnome-remote-desktop/blob/master/src/org.gnome.Mutter.RemoteDesktop.xml)
- [Mutter MR !1636 - Custom scroll source](https://gitlab.gnome.org/GNOME/mutter/-/merge_requests/1636)
- [Clutter Reference - Events](https://developer-old.gnome.org/clutter/stable/clutter-Events.html)
