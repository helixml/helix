# Client-Side Cursor Rendering

**Date:** 2026-01-16
**Status:** Draft
**Author:** Claude (with Luke)

## Overview

Replace server-side cursor compositing with client-side cursor rendering. The server sends cursor image data when the cursor changes, and the client renders the cursor locally at the current mouse position. This eliminates cursor latency entirely since the cursor moves with local input, not with video frames.

## Goals

1. Zero cursor latency - cursor follows local mouse movement instantly
2. Support cursor image changes (pointer, text, resize handles, custom app cursors)
3. Support cursor hiding (text input, games, full-screen video)
4. Work on both GNOME (Mutter) and Sway compositors
5. Minimal bandwidth - only send cursor data when it changes

## Current State

- Cursor is composited into the video stream by the compositor
- Cursor appears with video latency (encoder + network + decode)
- No way to hide cursor on client side
- Cursor position in video doesn't match local mouse position during movement

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                         Server (Desktop Container)              │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  ┌──────────────┐     ┌─────────────────┐     ┌──────────────┐ │
│  │  Compositor  │────▶│  Cursor Monitor │────▶│  WebSocket   │ │
│  │ (Mutter/Sway)│     │  (New Component)│     │  (Existing)  │ │
│  └──────────────┘     └─────────────────┘     └──────────────┘ │
│         │                     │                      │         │
│         ▼                     ▼                      ▼         │
│  ┌──────────────┐     ┌─────────────────┐     ┌──────────────┐ │
│  │ Hide cursor  │     │ Cursor Changed  │     │ Send Cursor  │ │
│  │ from video   │     │ Event Detection │     │ Image + Meta │ │
│  └──────────────┘     └─────────────────┘     └──────────────┘ │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
                                │
                                ▼ WebSocket
┌─────────────────────────────────────────────────────────────────┐
│                         Client (Browser)                        │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  ┌──────────────┐     ┌─────────────────┐     ┌──────────────┐ │
│  │  Receive     │────▶│  Cursor Cache   │────▶│  Render      │ │
│  │  Cursor Msg  │     │  (by hash/name) │     │  Cursor      │ │
│  └──────────────┘     └─────────────────┘     └──────────────┘ │
│                                                                 │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │                    Canvas/Overlay                         │  │
│  │  - Video layer (no cursor)                               │  │
│  │  - Cursor layer (CSS cursor or canvas overlay)           │  │
│  └──────────────────────────────────────────────────────────┘  │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

## Implementation Components

### 1. Hide Cursor from Video Stream

#### GNOME (Mutter) ✅ SOLVED
Use the `cursor-mode` D-Bus parameter when creating ScreenCast session:
- `cursor-mode: 0` = Hidden (no cursor in stream)
- `cursor-mode: 1` = Embedded (current behavior)
- `cursor-mode: 2` = Metadata (cursor sent as PipeWire `SPA_META_Cursor`)

**Current code** in `session.go:137`:
```go
"cursor-mode": dbus.MakeVariant(uint32(1)), // Embedded cursor
```
**Change to:**
```go
"cursor-mode": dbus.MakeVariant(uint32(0)), // Hidden - render client-side
```

For Portal API (`session_portal.go`), use `cursor_mode: 1` (hidden) instead of `2` (embedded).

#### Sway (ext-image-copy-capture) ✅ SOLVED
The `paint_cursors` option controls cursor compositing:
- Default: **Cursor NOT included** (no flag set)
- With flag: `Options::PaintCursors` composites cursor onto frames

**Current code** in `ext_image_copy_capture.rs`:
```rust
let options = ext_image_copy_capture_manager_v1::Options::PaintCursors;
```
**Change to:**
```rust
let options = ext_image_copy_capture_manager_v1::Options::empty(); // No cursor
```

### 2. Cursor Change Detection

#### GNOME (Mutter) - Recommended Approach
**Best option: Use `cursor-mode: 2` (Metadata)**
- PipeWire stream includes `SPA_META_Cursor` with cursor image, position, hotspot
- No need for XFixes or polling
- Works for ALL apps (Wayland-native and XWayland)

If metadata mode not available, fallback options:
- **XFixes extension:** Works for XWayland apps ONLY
- **Cursor theme lookup:** Map cursor name to theme files

#### Sway (wlroots)
**Best option: Use `create_pointer_cursor_session`**
- ext-image-copy-capture-v1 protocol provides cursor capture session
- Receives cursor image, position, and hotspot events
- Works for all apps

#### XFixes Limitation ⚠️
XFixes (`XFixesGetCursorImage`) does NOT work for Wayland-native apps:
- Only sees cursors when XWayland apps have focus
- GTK4, Qt6 Wayland, etc. will show stale/incorrect cursor
- Should only be used as fallback for legacy X11 apps

### 3. Cursor Image Extraction

#### Methods to Get Cursor Pixels
1. **XFixes (X11/XWayland):**
   ```c
   XFixesCursorImage *cursor = XFixesGetCursorImage(display);
   // cursor->pixels, cursor->width, cursor->height, cursor->xhot, cursor->yhot
   ```

2. **Cursor theme files:**
   - Parse `/usr/share/icons/<theme>/cursors/*`
   - Xcursor format: multiple sizes, animation frames
   - Cache common cursors at startup

3. **wlr-cursor-shape-v1 (Sway):**
   - Get shape hints, map to cursor theme
   - Not pixel data, but cursor name

### 4. Wire Protocol Extension

Add new message types to the existing WebSocket binary protocol:

```
// Message type 0x10: Cursor Image
// Sent when cursor image changes (not position - that's local)
struct CursorImageMessage {
    type: u8 = 0x10,
    cursor_id: u32,      // Hash/ID for caching
    hotspot_x: u16,      // Click point X offset
    hotspot_y: u16,      // Click point Y offset
    width: u16,
    height: u16,
    format: u8,          // 0=RGBA, 1=PNG compressed
    data_len: u32,
    data: [u8],          // RGBA pixels or PNG bytes
}

// Message type 0x11: Cursor Visibility
struct CursorVisibilityMessage {
    type: u8 = 0x11,
    visible: u8,         // 0=hidden, 1=visible
    cursor_id: u32,      // Which cursor to show (if visible)
}

// Message type 0x12: Use Cached Cursor
// Sent when switching to a previously-sent cursor
struct CursorSwitchMessage {
    type: u8 = 0x12,
    cursor_id: u32,      // Reference to cached cursor
}
```

### 5. Client-Side Rendering

#### How CSS Custom Cursors Work
CSS custom cursors ARE rendered as native OS cursors:
- Browser converts your image to a platform cursor
- Zero latency - follows OS mouse input directly
- Size limits: typically 32x32 to 128x128 (browser/OS dependent)
- Animated cursors NOT supported via CSS

#### Option A: CSS Custom Cursor (Recommended for local user)
```javascript
// Create data URL from received cursor image
const blob = new Blob([cursorData], { type: 'image/png' });
const url = URL.createObjectURL(blob);
// Set CSS cursor with hotspot coordinates
canvas.style.cursor = `url(${url}) ${hotspotX} ${hotspotY}, auto`;
```
- **Pros:** Zero latency, browser-native, no position tracking needed
- **Cons:** Static images only, size limits apply

This replaces the current circle indicator (`#custom-cursor` div) with the actual cursor image.

#### Option B: Canvas/DOM Overlay (Required for remote cursors)
```javascript
// Render cursor image at tracked position
// Used for: remote users, animated cursors, oversized cursors
const cursorDiv = document.createElement('div');
cursorDiv.style.pointerEvents = 'none';
cursorDiv.style.position = 'absolute';
cursorDiv.style.backgroundImage = `url(${cursorDataUrl})`;
// Update position on mousemove or remote cursor events
```
- **Pros:** Full control, animation, no size limits, multiple cursors
- **Cons:** Requires position tracking, slight overhead

#### Option C: CSS + Overlay Hybrid (Best approach)
- Use CSS cursor for local user's cursor (zero latency)
- Use DOM overlay for remote users' cursors (Figma-style)
- Fall back to overlay for animated/oversized cursors

### 6. Multi-Player Cursors (Figma-style)

Support multiple users viewing/controlling the same desktop session.

#### Architecture
```
┌─────────────────────────────────────────────────────────────────┐
│                         Server                                   │
├─────────────────────────────────────────────────────────────────┤
│  User A ─────┐                                                   │
│              ├──▶ Cursor Position Broadcast ──▶ All Users       │
│  User B ─────┘                                                   │
│                                                                  │
│  Desktop cursor image ──▶ All Users (when changed)              │
└─────────────────────────────────────────────────────────────────┘
```

#### Wire Protocol Extension
```
// Message type 0x13: Remote User Cursor Position
// Broadcast to all connected users when any user moves cursor
struct RemoteCursorMessage {
    type: u8 = 0x13,
    user_id: u32,        // Unique user identifier
    x: u16,              // Cursor X position (in stream coordinates)
    y: u16,              // Cursor Y position (in stream coordinates)
}

// Message type 0x14: Remote User Joined/Left
struct RemoteUserMessage {
    type: u8 = 0x14,
    user_id: u32,
    event: u8,           // 0=left, 1=joined
    name_len: u8,
    name: [u8],          // UTF-8 display name
    color: u32,          // RGBA cursor color (assigned by server)
    avatar_url_len: u16, // Length of avatar URL (0 if none)
    avatar_url: [u8],    // Avatar image URL (from user profile)
}
```

#### Visual Design

```
     ┌─ Colored cursor arrow (SVG, user's assigned color)
     │
     ▼
    ╱╲
   ╱  ╲    ┌──────────────────┐
  ╱    ╲   │ ┌────┐           │
 ╱──────╲  │ │ 😀 │  Luke M.  │  ◀── Name pill with avatar
           │ └────┘           │
           └──────────────────┘
```

**Colored Cursor Arrow:**
- SVG cursor shape, dynamically colored per user
- Colors auto-assigned from palette: red, blue, green, purple, orange, teal, pink, yellow
- Same user always gets same color (hash of user_id)

**Name Pill:**
- Rounded rectangle with user's assigned color as background
- Avatar (24x24 circle) on left - from user profile or initials fallback
- Display name on right
- Appears on hover or always visible (user preference)
- Fades after 3 seconds of cursor inactivity

#### Client Implementation
```typescript
interface RemoteUser {
  userId: string;
  userName: string;
  avatarUrl?: string;     // From user profile
  color: string;          // Assigned color (#FF5733, etc.)
}

interface RemoteCursor {
  user: RemoteUser;
  x: number;
  y: number;
  lastUpdate: number;     // For fade-out on idle
  visible: boolean;       // False when user leaves or idle timeout
}

// Color palette for cursor assignment
const CURSOR_COLORS = [
  '#E53935', // Red
  '#1E88E5', // Blue
  '#43A047', // Green
  '#8E24AA', // Purple
  '#FB8C00', // Orange
  '#00ACC1', // Teal
  '#D81B60', // Pink
  '#FDD835', // Yellow
];

function getColorForUser(userId: string): string {
  const hash = hashCode(userId);
  return CURSOR_COLORS[hash % CURSOR_COLORS.length];
}

// Render colored cursor with avatar pill
function RemoteCursorOverlay({ cursor }: { cursor: RemoteCursor }) {
  return (
    <div style={{
      position: 'absolute',
      left: cursor.x,
      top: cursor.y,
      pointerEvents: 'none',
      opacity: cursor.visible ? 1 : 0,
      transition: 'opacity 0.3s',
    }}>
      {/* Colored arrow cursor */}
      <svg width="24" height="24" style={{ color: cursor.user.color }}>
        <path fill="currentColor" d="M0,0 L0,16 L4,12 L8,20 L10,19 L6,11 L12,11 Z"/>
      </svg>

      {/* Name pill with avatar */}
      <div style={{
        display: 'flex',
        alignItems: 'center',
        backgroundColor: cursor.user.color,
        borderRadius: 12,
        padding: '2px 8px 2px 2px',
        marginLeft: 8,
        marginTop: -4,
      }}>
        {cursor.user.avatarUrl ? (
          <img
            src={cursor.user.avatarUrl}
            style={{ width: 20, height: 20, borderRadius: 10 }}
          />
        ) : (
          <div style={{
            width: 20, height: 20, borderRadius: 10,
            backgroundColor: 'rgba(255,255,255,0.3)',
            display: 'flex', alignItems: 'center', justifyContent: 'center',
            fontSize: 10, fontWeight: 'bold', color: 'white',
          }}>
            {cursor.user.userName.charAt(0).toUpperCase()}
          </div>
        )}
        <span style={{
          marginLeft: 4,
          color: 'white',
          fontSize: 12,
          fontWeight: 500,
          textShadow: '0 1px 2px rgba(0,0,0,0.3)',
        }}>
          {cursor.user.userName}
        </span>
      </div>
    </div>
  );
}
```

#### Avatar Sources
1. **Helix user profile** - if user has uploaded avatar
2. **OAuth provider** - Google/GitHub avatar from SSO
3. **Gravatar fallback** - hash of email
4. **Initials** - first letter of name in colored circle

#### Cursor Ownership
- Each user has their own color + avatar
- When a remote user moves cursor, only THEIR remote cursor moves
- Local user's cursor is always CSS (zero latency)
- Remote cursors are DOM overlays (network latency visible)
- Local user sees their own colored cursor in the "who's here" list but not as overlay

### 7. AI Agent Cursor

Show the AI agent's cursor when it interacts with the desktop via MCP tools.

#### Visual Design
```
    ╱╲
   ╱  ╲    ┌─────────────────────┐
  ╱    ╲   │  🤖  Qwen Agent     │  ◀── Distinct AI styling
 ╱──────╲  │      typing...      │
    │      └─────────────────────┘
    │
    ▼ Pulsing/animated to show activity
```

**AI Cursor Appearance:**
- Distinct color: Cyan/Electric blue (`#00D4FF`) - stands out from human colors
- Robot emoji or AI icon instead of avatar
- Agent name displayed (e.g., "Qwen Agent", "Claude")
- Activity indicator: "clicking...", "typing...", "scrolling..."
- Subtle pulse/glow animation when actively interacting

#### Wire Protocol
```
// Message type 0x15: Agent Cursor Update
// Sent when AI agent performs mouse actions via MCP
struct AgentCursorMessage {
    type: u8 = 0x15,
    agent_id: u32,       // Agent identifier
    x: u16,              // Target X position
    y: u16,              // Target Y position
    action: u8,          // 0=move, 1=click, 2=double-click, 3=drag, 4=scroll
    visible: u8,         // 0=hide, 1=show
}
```

#### Integration with MCP Tools
When the agent calls MCP tools, broadcast cursor position:

| MCP Tool | Cursor Behavior |
|----------|-----------------|
| `computer_mouse_move` | Show cursor moving to target position |
| `computer_left_click` | Show cursor + click animation at position |
| `computer_double_click` | Show cursor + double-click animation |
| `computer_drag` | Show cursor + drag trail from start to end |
| `computer_scroll` | Show cursor + scroll indicator |
| `computer_type` | Show cursor at last position + "typing..." label |

#### Animation States
```typescript
type AgentAction = 'idle' | 'moving' | 'clicking' | 'typing' | 'scrolling' | 'dragging';

interface AgentCursor {
  agentId: string;
  agentName: string;      // "Qwen Agent", "Claude", etc.
  agentIcon: string;      // Robot emoji or icon URL
  x: number;
  y: number;
  action: AgentAction;
  visible: boolean;
  trailPoints?: {x: number, y: number}[];  // For drag visualization
}

// Click animation: ripple effect at click point
function ClickRipple({ x, y, color }: { x: number, y: number, color: string }) {
  return (
    <div style={{
      position: 'absolute',
      left: x - 20,
      top: y - 20,
      width: 40,
      height: 40,
      borderRadius: '50%',
      border: `2px solid ${color}`,
      animation: 'ripple 0.6s ease-out forwards',
    }} />
  );
}

// Typing indicator: dots animation
function TypingIndicator() {
  return <span className="typing-dots">typing...</span>;
}
```

#### Server-Side: MCP to Cursor Events
```go
// In MCP tool handler (api/pkg/mcp/)
func (h *Handler) handleMouseMove(x, y int) error {
    // Execute the mouse move
    if err := h.desktop.MouseMove(x, y); err != nil {
        return err
    }

    // Broadcast agent cursor position to all viewers
    h.broadcastAgentCursor(AgentCursorEvent{
        AgentID:   h.agentID,
        AgentName: h.agentName,
        X:         x,
        Y:         y,
        Action:    ActionMove,
        Visible:   true,
    })
    return nil
}
```

#### Why This Is Awesome
1. **Transparency** - Users see exactly what the AI is doing
2. **Debugging** - Easy to see if AI is clicking wrong elements
3. **Trust** - No hidden automation, everything visible
4. **Collaboration** - Human and AI cursors visible simultaneously
5. **Training data** - Could record agent actions for analysis

### 8. Touch Event Visualization

Show touch events from mobile/tablet users and multi-touch gestures.

#### Visual Design
```
        ┌─ User color ring
        │
   ┌────▼────┐
   │  ╭───╮  │
   │  │ 👆 │  │  ◀── Touch point with finger ripple
   │  ╰───╯  │
   └─────────┘
        │
        ▼
   Expanding ripple animation on tap
```

**Touch Indicators:**
- Circle with user's color border
- Finger emoji or filled circle
- Ripple animation on tap/release
- Multiple simultaneous touch points for pinch/zoom

#### Wire Protocol
```
// Message type 0x16: Touch Event
// Sent when user touches/drags on mobile/tablet
struct TouchEventMessage {
    type: u8 = 0x16,
    user_id: u32,
    touch_id: u8,        // Finger ID (0-9 for multi-touch)
    event_type: u8,      // 0=start, 1=move, 2=end, 3=cancel
    x: u16,
    y: u16,
    pressure: u8,        // 0-255 pressure (if available)
}

// Message type 0x17: Gesture Event
// Higher-level gesture recognition
struct GestureEventMessage {
    type: u8 = 0x17,
    user_id: u32,
    gesture: u8,         // 0=tap, 1=double-tap, 2=long-press, 3=pinch, 4=rotate, 5=swipe
    x: u16,              // Center point X
    y: u16,              // Center point Y
    scale: u16,          // For pinch: 100 = 1.0x, 200 = 2.0x
    rotation: i16,       // For rotate: degrees * 10
    direction: u8,       // For swipe: 0=up, 1=right, 2=down, 3=left
}
```

#### Multi-Touch Visualization
```typescript
interface TouchPoint {
  touchId: number;       // Finger ID
  userId: string;
  userColor: string;
  x: number;
  y: number;
  pressure: number;      // 0-1
  active: boolean;
}

// Render touch point with ripple
function TouchPointOverlay({ touch }: { touch: TouchPoint }) {
  const size = 32 + (touch.pressure * 16);  // Bigger with more pressure

  return (
    <div style={{
      position: 'absolute',
      left: touch.x - size/2,
      top: touch.y - size/2,
      width: size,
      height: size,
      borderRadius: '50%',
      border: `3px solid ${touch.userColor}`,
      backgroundColor: `${touch.userColor}40`,  // 25% opacity fill
      animation: touch.active ? 'none' : 'touch-release 0.3s forwards',
    }}>
      {/* Center dot */}
      <div style={{
        position: 'absolute',
        left: '50%', top: '50%',
        transform: 'translate(-50%, -50%)',
        width: 8, height: 8,
        borderRadius: '50%',
        backgroundColor: touch.userColor,
      }} />
    </div>
  );
}

// Pinch gesture visualization: two circles + line between
function PinchGestureOverlay({ touch1, touch2, userColor }: {
  touch1: {x: number, y: number},
  touch2: {x: number, y: number},
  userColor: string,
}) {
  return (
    <svg style={{ position: 'absolute', top: 0, left: 0, width: '100%', height: '100%', pointerEvents: 'none' }}>
      {/* Line connecting touch points */}
      <line
        x1={touch1.x} y1={touch1.y}
        x2={touch2.x} y2={touch2.y}
        stroke={userColor}
        strokeWidth={2}
        strokeDasharray="4 4"
      />
      {/* Touch circles */}
      <circle cx={touch1.x} cy={touch1.y} r={20} fill={`${userColor}40`} stroke={userColor} strokeWidth={2} />
      <circle cx={touch2.x} cy={touch2.y} r={20} fill={`${userColor}40`} stroke={userColor} strokeWidth={2} />
    </svg>
  );
}
```

#### Gesture Indicators
| Gesture | Visual |
|---------|--------|
| Tap | Single ripple expanding outward |
| Double-tap | Two quick ripples |
| Long-press | Growing circle that fills |
| Pinch | Two circles connected by dashed line |
| Rotate | Two circles with rotation arrow |
| Swipe | Arrow showing direction |

#### Touch vs Mouse
- **Mouse users**: Show colored cursor arrow with name pill
- **Touch users**: Show colored touch circles (no cursor arrow)
- **Same user colors**: Consistent color whether using mouse or touch
- **Can switch**: User might use mouse then touch - both work

### 9. Cursor Caching Strategy

#### Server Side
- Hash cursor image data (e.g., xxhash of pixels)
- Cache last N cursors sent (LRU, max ~50)
- Only send full image on first occurrence
- Send cursor_id reference for cached cursors

#### Client Side
- Cache received cursor images by cursor_id
- Preload common cursors on connection start
- Evict old cursors when cache exceeds limit

## Implementation Phases

### Phase 1: Cursor Hiding (Foundation)
1. Hide cursor from GNOME ScreenCast output
2. Hide cursor from Sway ext-image-copy-capture
3. Verify video stream has no cursor

### Phase 2: Basic Cursor Streaming (GNOME)
1. Implement XFixes cursor monitoring for GNOME
2. Send cursor images over WebSocket
3. Client renders cursor via CSS custom cursor
4. Test with common apps (terminal, browser, file manager)

### Phase 3: Cursor Caching & Optimization
1. Implement cursor hashing and caching
2. Add cursor switch messages (no re-send)
3. Compress cursor images (PNG)
4. Measure bandwidth impact

### Phase 4: Sway Support
1. Investigate Sway cursor APIs
2. Implement cursor monitoring for Sway
3. Test with Sway-native and XWayland apps

### Phase 5: Advanced Features
1. Animated cursor support (loading spinners)
2. Cursor hiding detection (games, video players)
3. High-DPI cursor support (2x, 3x)
4. Cursor confinement sync (when app grabs cursor)

### Phase 6: Multi-Player Cursors
1. Add user identification to WebSocket connections
2. Implement cursor position broadcasting
3. Render remote user cursors with labels/colors
4. Add user joined/left notifications
5. Implement cursor fade-out on idle

## Open Questions (Resolved)

| Question | Answer |
|----------|--------|
| MUTTER_DEBUG_HIDE_CURSOR? | Does NOT exist. Use `cursor-mode: 0` D-Bus param instead. |
| ScreenCast without cursor? | Yes, `cursor-mode: 0` (Mutter) or `cursor_mode: 1` (Portal) |
| Sway cursor default? | OFF by default. `paint_cursors` flag enables it. |
| XFixes for Wayland apps? | NO. Only works for XWayland apps. Use `SPA_META_Cursor` instead. |
| Cursor hotspot? | Provided by `SPA_META_Cursor` (GNOME) or cursor session events (Sway) |

## Remaining Questions

1. **SPA_META_Cursor parsing:**
   - How to parse cursor metadata from PipeWire stream in Rust/Go?
   - Is cursor image included or just position/hotspot?

2. **Performance:**
   - How often do cursors change in typical usage?
   - Bandwidth cost of cursor images (estimate: ~1-5 KB per cursor, ~50 unique cursors)

## Risks & Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| Can't hide cursor from video | High | Fall back to current behavior |
| XFixes doesn't work for Wayland apps | Medium | Use cursor theme lookup as fallback |
| High bandwidth for cursor images | Low | Caching + compression |
| Cursor rendering flicker | Medium | Double-buffering, CSS transitions |
| Different behavior GNOME vs Sway | Medium | Abstract cursor API, test both |

## Success Metrics

1. Cursor latency < 16ms (local input latency only)
2. Cursor changes reflected within 100ms
3. Bandwidth overhead < 10KB/s average
4. Works with 95%+ of common applications
5. No visible cursor in video stream

## References

- [XFixes Extension](https://www.x.org/releases/X11R7.7/doc/fixesproto/fixesproto.txt)
- [wlr-cursor-shape-v1](https://wayland.app/protocols/cursor-shape-v1)
- [Xcursor Format](https://man.archlinux.org/man/Xcursor.3)
- [GNOME Mutter Source](https://gitlab.gnome.org/GNOME/mutter)
- [Sway Cursor Handling](https://github.com/swaywm/sway/wiki)
