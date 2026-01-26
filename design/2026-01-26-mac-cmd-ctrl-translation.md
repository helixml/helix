# Mac Cmd to Ctrl Translation for Remote Desktop

**Date:** 2026-01-26
**Status:** Abandoned
**Author:** Claude (with Luke)

## Goal

Make Mac Cmd+C/V keyboard shortcuts work seamlessly on Ubuntu/Sway remote desktop by translating them to Ctrl+C/V.

## Background

When Mac users connect to a Linux remote desktop, they expect Cmd+C/V to work like Ctrl+C/V. The challenge is that:
1. The Cmd key maps to Meta/Super on Linux
2. Meta/Super has its own behavior (e.g., GNOME Activities overview)
3. The keyboard events fire in sequence: Cmd down → C down → C up → Cmd up

## What We Discovered

### The Input Pipeline

```
Browser KeyboardEvent
  → DesktopStreamViewer.tsx (handleKeyDown/handleKeyUp)
  → StreamInput.onKeyDown()
  → WebSocketStream.sendKey(isDown, evdevKeycode, modifiers)
  → WebSocket binary message [subType, isDown, modifiers, keycode_hi, keycode_lo]
  → Backend ws_input.go handleWSKeyboardKeycode()
  → GNOME D-Bus NotifyKeyboardKeycode or Wayland virtual keyboard
```

### Key Files

- `frontend/src/components/external-agent/DesktopStreamViewer.tsx` - Keyboard event handling
- `frontend/src/lib/helix-stream/stream/input.ts` - StreamInput class
- `frontend/src/lib/helix-stream/stream/websocket-stream.ts` - WebSocket transport
- `frontend/src/lib/helix-stream/stream/evdev-keys.ts` - Evdev keycode conversion
- `api/pkg/desktop/ws_input.go` - Backend keyboard handling

### Original Problem

The backend was ignoring modifier flags in keyboard messages:
```go
// Line 133 in ws_input.go
modifiers := data[2] // Was commented out as "Currently unused"
```

When Cmd+C was pressed on Mac, the frontend correctly set `ctrlKey: true` in the synthetic event, and the WebSocket message included `modifiers=2` (Ctrl flag), but the backend wasn't using it.

## Approaches Attempted

### Approach 1: Backend Modifier Synthesis

**Idea:** Make the backend synthesize modifier key presses based on the modifier flags in the message.

**Implementation:**
- Track `currentModifiers` state in `wsInputState`
- On keyDown: press modifiers that are in the message but not already pressed
- On keyUp: release modifiers that were pressed for this keystroke

**Problem:** This worked for the immediate Ctrl+C, but the Mac Cmd key had already been sent to the remote as Meta keydown before we detected Cmd+C.

### Approach 2: Release Meta Before Sending Ctrl+C (Frontend)

**Idea:** When we detect Cmd+C, send a MetaLeft keyup to release the Meta key, then send Ctrl+C.

**Problem:** Releasing Meta after it was pressed triggers GNOME Activities overview. GNOME interprets Meta down → Meta up (without another key in between) as a "Super tap" which opens Activities.

### Approach 3: Suppress Meta Entirely on Mac

**Idea:** Never send Meta keydown to the remote on Mac. Translate all Cmd+key to Ctrl+key.

**Problem:** User also wanted to use Cmd as Super key sometimes (e.g., to open GNOME Activities).

### Approach 4: Buffered Meta with Conditional Behavior

**Idea:** Buffer the Meta keydown:
- If another key is pressed while Cmd held → translate to Ctrl+key, don't send Meta
- If Cmd is released without another key → send Super tap

**Implementation:**
- `metaKeyBuffered` flag when Meta keydown is received
- `metaKeyUsedForShortcut` flag set when Cmd+key is translated
- On Meta keyup: send Super tap only if not used for shortcut

**Problems Found:**
1. Clipboard handlers (Cmd+C/V) returned early without setting `metaKeyUsedForShortcut`, so Super tap still fired
2. Cmd+A caused repeated 'a' characters - unclear if browser `event.repeat` flag issue or something else
3. The interaction between clipboard sync, synthetic events, and modifier state became complex

## Complexity Factors

1. **Event Timing:** Cmd keydown fires before we know if it's a shortcut or standalone
2. **Multiple Code Paths:** Clipboard handlers (copy/paste) have special logic separate from general keyboard handling
3. **State Synchronization:** Need to track what modifiers the remote thinks are pressed vs what we've actually sent
4. **Browser Differences:** `event.repeat` behavior may vary
5. **Synthetic Events:** Creating `new KeyboardEvent()` objects doesn't perfectly replicate real events

## Recommendations for Future Attempt

1. **Consider a timeout-based approach:** Buffer Meta for ~100ms. If no other key, send Super. If key pressed, translate to Ctrl. This is how macOS handles Cmd internally.

2. **Unified modifier handling:** All keyboard events should go through a single translation layer, not have special cases scattered (clipboard handlers, general handler).

3. **Test with logging first:** Add comprehensive logging to understand the exact event sequence before implementing fixes.

4. **Consider OS-level solution:** A browser extension or system-level key remapping might be more reliable than JavaScript event manipulation.

## Files Modified (All Reverted)

- `api/pkg/desktop/ws_input.go` - Added modifier state tracking and synthesis
- `api/pkg/desktop/desktop.go` - Added `streamKeyboardState` field
- `api/pkg/desktop/ws_stream.go` - Pass keyboard state to handler
- `frontend/src/components/external-agent/DesktopStreamViewer.tsx` - Mac Cmd handling
- `frontend/src/lib/helix-stream/stream/websocket-stream.ts` - Debug logging

## Commits Made (All Reverted)

1. `ad2d55062` - feat(desktop): translate Mac Cmd+C/V to Ctrl+C/V for Linux desktop
2. `f0fb37159` - fix(desktop): release Meta key before sending Ctrl+C/V for Cmd translation
3. `de339ae45` - fix(desktop): suppress Meta key on Mac instead of releasing it
4. `30cae495a` - fix(desktop): buffered Cmd handling - tap for Super, hold for Ctrl translation
5. `5f7cedab6` - fix(desktop): mark Cmd as used for shortcut in clipboard handlers
