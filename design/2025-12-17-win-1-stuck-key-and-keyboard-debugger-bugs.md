# Win+1 Stuck Key Bug & Keyboard Debugger Layer Visibility

**Date:** 2025-12-17
**Status:** Investigation
**Category:** Keyboard Input Bugs
**Author:** Claude (AI) + Luke

## Executive Summary

Two related keyboard issues have been identified:

1. **Win+1 Stuck Key Bug**: When pressing Win+1, then releasing 1 (while Win is still held), then releasing Win, the 1 key appears "stuck" in the keyboard debugger UI
2. **Keyboard Debugger Only Shows Wolf Layer**: The UI shows three layers (Wolf, Inputtino, Evdev) but only ever populates the Wolf layer

## Bug 1: Win+1 Stuck Key

### Reproduction Steps

1. Connect to a sandbox session via **browser-based Moonlight Web** from Mac (Command key = Win key)
2. Open the keyboard debugger panel
3. Press and hold Win (Command on Mac)
4. While holding Win, press 1
5. While still holding Win, release 1
6. Release Win
7. **Observe:** The 1 key shows as stuck in the keyboard debugger

### Expected Behavior

After step 6, no keys should be shown as pressed in the keyboard debugger.

### Key Facts

- **Browser-based**: This is Moonlight Web running in the browser, NOT native Moonlight client
- **Reliable transport**: WebSocket over TCP - no packet loss possible
- **100% reproducible**: Happens every single time

### Analysis

#### Browser-Side Keyboard Input Flow

The complete flow from browser to Wolf:

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    BROWSER KEYBOARD INPUT FLOW                               │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  1. Browser receives physical keypress                                       │
│     └── May intercept for browser shortcuts (Cmd+1 = Switch to Tab 1!)     │
│                                                                              │
│  2. KeyboardEvent dispatched to document                                     │
│     └── stream.ts: document.addEventListener("keydown/keyup", handler)       │
│         └── ViewerApp.onKeyDown/onKeyUp                                      │
│             └── event.preventDefault() ← attempts to block browser default   │
│                                                                              │
│  3. StreamInput.onKeyDown/onKeyUp (input.ts:158-163)                         │
│     └── sendKeyEvent(isDown, event)                                          │
│                                                                              │
│  4. convertToKey(event) (keyboard.ts:594-600)                                │
│     └── VK_MAPPINGS[event.code] → returns VK code or null                    │
│     └── If null: EARLY RETURN, event silently dropped!                       │
│                                                                              │
│  5. convertToModifiers(event) (keyboard.ts:3-20)                             │
│     └── Checks event.shiftKey, ctrlKey, altKey, metaKey flags                │
│                                                                              │
│  6. sendKey(isDown, key, modifiers) (input.ts:177-185)                       │
│     └── Format: U8(0) + Bool(isDown) + U8(modifiers) + U16(key)             │
│                                                                              │
│  7. WebSocket Transport (websocket-stream.ts:1222-1228)                      │
│     └── sendInputMessage(WsMessageType.KeyboardInput, payload)               │
│     └── Format: type(1) + subType(1) + isDown(1) + modifiers(1) + key(2)    │
│                                                                              │
│  8. Wolf receives and tracks key state                                       │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

#### Primary Hypothesis: Browser Intercepts Command+1 Shortcut

**Command+1 is a standard browser shortcut on Mac: "Switch to Tab 1"**

The likely sequence:

1. **Press Command** → `keydown` event fires → reaches JavaScript ✓ → sent to Wolf
2. **Press 1 (Command held)** → `keydown` event fires → reaches JavaScript ✓ → sent to Wolf
3. **Release 1 (Command held)** → **BROWSER INTERCEPTS for tab switching** → `keyup` event **NEVER reaches JavaScript**
4. **Release Command** → `keyup` event fires → reaches JavaScript ✓ → sent to Wolf

**Result:** Wolf receives KEY_PRESS for '1' but **never receives KEY_RELEASE**, causing the stuck key.

#### Why `event.preventDefault()` Doesn't Help

From `stream.ts:193-204`:
```typescript
onKeyDown(event: KeyboardEvent) {
    this.onUserInteraction()
    event.preventDefault()  // ← This is on keydown, not keyup!
    this.stream?.getInput().onKeyDown(event)
}
onKeyUp(event: KeyboardEvent) {
    this.onUserInteraction()
    event.preventDefault()  // ← Can't prevent what never arrives
    this.stream?.getInput().onKeyUp(event)
}
```

The `preventDefault()` on `keydown` tells the browser "don't do the default action for this keydown." But:
- The browser may still be tracking the key combo internally
- When the key is released, the browser processes the shortcut action first
- The `keyup` event may never dispatch to JavaScript at all

#### Evidence Needed

1. **Add console.log to keyup handler to verify events arrive:**
   ```typescript
   onKeyUp(event: KeyboardEvent) {
       console.log('[DEBUG] keyup received:', event.code, event.key)
       // ... rest of handler
   }
   ```

2. **Test the hypothesis:**
   - If console.log for '1' keyup is missing → Browser interception confirmed
   - If console.log appears but Wolf still shows stuck → Bug is elsewhere

3. **Test different key combos:**
   - Ctrl+1 (not a browser shortcut) → Should work correctly
   - Command+T (new tab) → Likely also stuck
   - Command+A (select all) → Likely also stuck

#### Alternative Hypotheses

1. **Silent drop in convertToKey()**: If `VK_MAPPINGS` returns `null`, the event is silently dropped. But `Digit1` is mapped: `Digit1: StreamKeys.VK_KEY_1` (keyboard.ts:27)

2. **Modifier flag mismatch on keyup**: The keyup event might have different modifier flags than expected, but this shouldn't cause a complete drop.

3. **Race condition in event dispatch**: Unlikely since this is 100% reproducible and WebSocket is reliable.

### Relevant Files (Browser-Side)

- `/prod/home/luke/pm/helix/frontend/src/lib/moonlight-web-ts/stream.ts` - Document event listeners
- `/prod/home/luke/pm/helix/frontend/src/lib/moonlight-web-ts/stream/input.ts` - StreamInput class
- `/prod/home/luke/pm/helix/frontend/src/lib/moonlight-web-ts/stream/keyboard.ts` - Key code conversion
- `/prod/home/luke/pm/helix/frontend/src/lib/moonlight-web-ts/stream/websocket-stream.ts` - WebSocket transport

### Relevant Files (Wolf-Side)

- `/prod/home/luke/pm/wolf/src/moonlight-server/control/input_handler.cpp` - keyboard event handling
- `/prod/home/luke/pm/wolf/src/moonlight-server/control/keyboard_state.hpp` - state tracker

### Recommended Fix

If browser interception is confirmed, possible solutions:

1. **Use `keydown` with `event.repeat` for synthetic keyup detection:**
   Track pressed keys in JavaScript. If we receive a keydown for a key we didn't see keyup for, synthesize the keyup first.

2. **Use `blur` event to release all keys:**
   When the window loses focus (tab switch), send KEY_RELEASE for all currently pressed keys.

3. **Capture keyboard at lower level:**
   Investigate if the Gamepad API or Pointer Lock API can capture keyboard more reliably.

4. **Document limitation:**
   Some browser shortcuts cannot be overridden. Users should avoid using Windows shortcuts that conflict with browser shortcuts.

---

## Bug 2: Keyboard Debugger Only Shows Wolf Layer

### Observed Behavior

The keyboard debugger UI shows three layers:
- **Wolf (X)** - Shows pressed key count
- **Inputtino (0)** - Always shows 0
- **Evdev (0)** - Always shows 0

### Root Cause

Looking at `wolf/src/moonlight-server/api/endpoints.cpp:1107-1228`, the `endpoint_KeyboardState` function only populates Inputtino and Evdev layers when the keyboard type is `wolf::core::input::Keyboard`:

```cpp
std::visit([&state](auto &keyboard) {
  using T = std::decay_t<decltype(keyboard)>;

  if constexpr (std::is_same_v<T, wolf::core::input::Keyboard>) {
    // Get inputtino and evdev state - ONLY for inputtino keyboard
    auto inputtino_keys = keyboard.get_pressed_keys();
    auto evdev_keys = keyboard.get_evdev_pressed_keys();
    // ... populate layers ...
  } else {
    // WaylandKeyboard - NO introspection available
    logs::log(logs::debug, "[KEYBOARD] Using WaylandKeyboard - NO introspection available");
  }
}, running.keyboard->value());
```

### Why This Happens

Lobbies use `WaylandKeyboard` (not `inputtino::Keyboard`):

From `wolf/src/moonlight-server/sessions/lobbies.cpp:235-238`:
```cpp
// switch mouse and keyboard in session to use the lobby wayland server
auto wl_state = lobby->wayland_display->load();
session->keyboard->emplace(virtual_display::WaylandKeyboard(wl_state));
```

`WaylandKeyboard` is a different implementation that:
- Sends keyboard events directly to the Wayland compositor
- Does NOT have `get_pressed_keys()` or `get_evdev_pressed_keys()` methods
- Cannot expose internal state for debugging

### Fix Options

#### Option 1: Add State Tracking to WaylandKeyboard

Add pressed key tracking to `WaylandKeyboard` class:

```cpp
class WaylandKeyboard {
  std::set<short> pressed_keys_;

  void press(short key) {
    pressed_keys_.insert(key);
    // ... existing Wayland logic ...
  }

  void release(short key) {
    pressed_keys_.erase(key);
    // ... existing Wayland logic ...
  }

  std::vector<short> get_pressed_keys() const {
    return std::vector<short>(pressed_keys_.begin(), pressed_keys_.end());
  }
};
```

**Pros:** Simple implementation, consistent API
**Cons:** Doesn't give true Wayland compositor state, only Wolf's view

#### Option 2: Query Wayland Compositor State

Have the outer compositor (wayland-display-core) expose keyboard state via a protocol:

```cpp
// Add to wayland-display-core Command enum
SetKeyboardLayout(...),
GetKeyboardState() -> Vec<u32>,  // Currently pressed keycodes
```

**Pros:** Shows actual compositor state
**Cons:** More complex, requires changes to wayland-display-core

#### Option 3: Keep Only Wolf Layer for WaylandKeyboard

Accept that WaylandKeyboard doesn't support introspection and clearly indicate this in the UI:

```typescript
// In KeyboardObservabilityPanel.tsx
{keyboardState?.sessions?.map(session => (
  session.inputtino_state.pressed_keys.length === 0 &&
  session.evdev_state.pressed_keys.length === 0 && (
    <Typography>
      Inputtino/Evdev layers unavailable (using Wayland backend)
    </Typography>
  )
))}
```

**Pros:** Accurate representation, no code changes to Wolf
**Cons:** Less debugging capability

### Recommendation

**Implement Option 1** - Add state tracking to WaylandKeyboard. This provides:
- Consistent API across keyboard types
- Ability to compare Wolf tracker state with WaylandKeyboard state
- Detection of mismatches between what Wolf received and what was sent to Wayland

The Evdev layer would still show 0 for WaylandKeyboard (no direct kernel device), but Inputtino layer would show WaylandKeyboard's internal state.

---

## Architecture Reference

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    BROWSER → WOLF KEYBOARD INPUT FLOW                        │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  Physical Keyboard (Mac)                                                     │
│       │                                                                      │
│       ▼                                                                      │
│  macOS (translates Command → Meta key)                                       │
│       │                                                                      │
│       ▼                                                                      │
│  Browser (Chrome/Safari/Firefox)                                             │
│       │                                                                      │
│       ├── Browser may intercept shortcuts (Cmd+1, Cmd+T, etc.)              │
│       │   └── If intercepted: keyup may NEVER reach JavaScript! ← BUG #1    │
│       │                                                                      │
│       ▼                                                                      │
│  JavaScript KeyboardEvent                                                    │
│       │                                                                      │
│       ▼                                                                      │
│  stream.ts: ViewerApp.onKeyDown/onKeyUp                                      │
│       │                                                                      │
│       ▼                                                                      │
│  input.ts: StreamInput.sendKeyEvent()                                        │
│       │                                                                      │
│       ├── keyboard.ts: convertToKey() - maps event.code → VK code           │
│       │   └── Returns null for unmapped keys (silent drop)                  │
│       │                                                                      │
│       ├── keyboard.ts: convertToModifiers() - checks modifier flags         │
│       │                                                                      │
│       ▼                                                                      │
│  websocket-stream.ts: sendKey(isDown, key, modifiers)                        │
│       │                                                                      │
│       │ Binary: type(1) + subType(1) + isDown(1) + modifiers(1) + key(2)    │
│       │                                                                      │
│       ▼ (WebSocket over TCP - reliable, no packet loss)                      │
│                                                                              │
│  Wolf keyboard_key()                                                         │
│       │                                                                      │
│       ├─── KeyboardStateTracker ──────────────────── [Layer 1: Wolf]        │
│       │    (tracks moonlight_key presses/releases)                           │
│       │                                                                      │
│       └─── session.keyboard (variant)                                        │
│            │                                                                 │
│            ├── input::Keyboard (inputtino)                                   │
│            │   └── cur_press_keys ──────────────── [Layer 2: Inputtino]     │
│            │   └── libevdev uinput ──────────────── [Layer 3: Evdev]        │
│            │                                                                 │
│            └── WaylandKeyboard (lobby mode)                                  │
│                └── Wayland wl_keyboard events                                │
│                    (NO internal state tracking) ← BUG #2                     │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

## Related Files

### Wolf
- `wolf/src/moonlight-server/control/input_handler.cpp` - Main keyboard handling
- `wolf/src/moonlight-server/control/keyboard_state.hpp` - State tracker
- `wolf/src/moonlight-server/api/endpoints.cpp` - Keyboard state API
- `wolf/src/core/src/platforms/linux/virtual-display/gst-wayland-display.cpp` - WaylandKeyboard

### Helix API
- `api/pkg/wolf/client.go` - Wolf client keyboard state types

### Frontend
- `frontend/src/components/external-agent/KeyboardObservabilityPanel.tsx` - UI
- `frontend/src/services/wolfService.ts` - React Query hooks

## Previous Related Work

- `design/2025-11-25-keyboard-modifier-stuck-analysis.md` - Modifier stuck issue analysis
- `design/2025-11-26-keyboard-input-deep-dive.md` - Deep dive on keyboard architecture
- `design/2025-11-30-keyboard-layout-reset-bug-fix.md` - Layout reset bug

## Next Steps

### For Win+1 Bug (Priority: High)

1. **Verify browser interception hypothesis:**
   ```typescript
   // Add to stream.ts onKeyUp handler temporarily:
   onKeyUp(event: KeyboardEvent) {
       console.log('[DEBUG] keyup:', event.code, event.key, 'meta:', event.metaKey)
       // ... rest
   }
   ```
   - Reproduce Win+1 sequence
   - Check if console.log for 'Digit1' keyup appears
   - If missing → Browser interception confirmed

2. **Test control cases:**
   - Ctrl+1 (not a browser shortcut) → Should work
   - Win+2, Win+3, etc. → Browser shortcuts, likely also stuck
   - Win+A (select all) → Browser shortcut, likely stuck
   - Win+K (arbitrary) → Not a shortcut, should work

3. **Implement fix if confirmed:**
   - Option A: Track pressed keys in JS, send synthetic keyup on window blur
   - Option B: Track pressed keys in JS, send keyup before duplicate keydown
   - Option C: Use keyboard lock API if available (requires secure context)

### For Keyboard Debugger Bug (Priority: Medium)

1. Implement Option 1: Add state tracking to WaylandKeyboard
2. Update endpoints.cpp to query WaylandKeyboard state
3. Test with lobby sessions
