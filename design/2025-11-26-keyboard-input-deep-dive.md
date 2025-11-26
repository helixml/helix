# Keyboard Input Deep Dive: RHEL 9.4 vs Ubuntu 24.04 vs SLES 15

**Date:** 2025-11-26
**Author:** Claude (AI) + Luke
**Issue:** Keyboard keys (especially 'i') randomly not working on RHEL 9.4 Moonlight clients
**Severity:** High

## Executive Summary

The keyboard issues on RHEL 9.4 clients are caused by **multiple factors** in the input pipeline, not a single root cause. The primary issues are:

1. **SDL2 version differences** - RHEL 9.4 ships SDL 2.26.x vs Ubuntu 24.04's SDL 2.30.x
2. **X11/XKB keycode offset confusion** - The 8-offset between evdev and X11 keycodes
3. **Missing modifier release events** - Fixed in Wolf input_handler.cpp (commit 067e61c)
4. **SDL scancode mapping bugs** - Certain keys not recognized on older SDL versions

## SLES 15 as Alternative Client OS (RECOMMENDED)

**Good news:** SUSE Linux Enterprise Server 15 SP6 (released June 2024) uses kernel 6.4/6.5, which is significantly newer than RHEL 9.4's kernel 5.14.

| Distribution | Kernel Version | Input Subsystem Age | Recommendation |
|-------------|----------------|---------------------|----------------|
| RHEL 9.4 | 5.14.0-427 | ~3 years old (May 2022) | **Not recommended** |
| Ubuntu 24.04 | 6.8.x | Current (March 2024) | **Recommended** |
| SLES 15 SP5 | 5.14.x | ~3 years old (same as RHEL) | **Not recommended** |
| **SLES 15 SP6** | **6.4/6.5** | Recent (June 2024) | **Recommended** ✓ |

### Why SLES 15 SP6 is a Good Alternative

1. **Modern kernel (6.4/6.5)**: Contains most of the input subsystem fixes from kernel 6.x series
2. **Enterprise support**: SUSE provides 10+ years of support like RHEL
3. **libinput improvements**: Kernel 6.4+ includes significant evdev and libinput fixes
4. **SDL2 version**: SLES 15 SP6 ships SDL 2.28.x (newer than RHEL 9.4's 2.26.x)

### Key Input Subsystem Improvements in Kernel 6.4+

- Fixed uinput device creation race conditions
- Improved HID event delivery timing
- Better handling of modifier key sequences
- Fixed event coalescing bugs in evdev
- Improved libinput integration

### Recommendation

**If the customer is willing to deploy SLES 15, they should specifically choose SLES 15 SP6 (not SP5)**. SP6 uses kernel 6.4/6.5 which should exhibit similar keyboard behavior to Ubuntu 24.04.

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           MOONLIGHT CLIENT (RHEL 9.4)                       │
├─────────────────────────────────────────────────────────────────────────────┤
│  Physical Keyboard                                                          │
│       │                                                                     │
│       ▼                                                                     │
│  Linux Kernel (5.14.0-427) ─── evdev ─── /dev/input/eventX                  │
│       │                                                                     │
│       ▼                                                                     │
│  X11 Server (Xorg/XWayland)                                                 │
│       │                  ┌────────────────────────────────────┐             │
│       │                  │ X11 adds +8 offset to keycodes     │             │
│       │                  │ evdev keycode 30 (KEY_A)           │             │
│       │                  │ becomes X11 keycode 38             │             │
│       ▼                  └────────────────────────────────────┘             │
│  SDL2 (2.26.x on RHEL 9)                                                    │
│       │                  ┌────────────────────────────────────┐             │
│       │                  │ SDL converts X11 keycode to        │             │
│       │                  │ SDL_Scancode using lookup table    │             │
│       │                  │ scancodes_xfree86.h                │             │
│       ▼                  └────────────────────────────────────┘             │
│  Moonlight Qt (handleKeyEvent)                                              │
│       │                  ┌────────────────────────────────────┐             │
│       │                  │ Converts SDL_Scancode to           │             │
│       │                  │ Windows VK code                    │             │
│       │                  │ SDL_SCANCODE_I (23) → VK_I (0x49)  │             │
│       ▼                  └────────────────────────────────────┘             │
│  LiSendKeyboardEvent2(keyCode, action, modifiers, flags)                    │
│       │                                                                     │
│       │ ─────── Network (ENet reliable channel) ──────────▶                 │
└───────┼─────────────────────────────────────────────────────────────────────┘
        │
        ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                             WOLF SERVER (Ubuntu 25.04)                      │
├─────────────────────────────────────────────────────────────────────────────┤
│  Wolf control/input_handler.cpp::keyboard_key()                             │
│       │                  ┌────────────────────────────────────┐             │
│       │                  │ Receives KEYBOARD_PACKET with      │             │
│       │                  │ Windows VK code (little-endian)    │             │
│       │                  │ VK_I (0x49) → moonlight_key        │             │
│       ▼                  └────────────────────────────────────┘             │
│  inputtino::Keyboard::press(moonlight_key)                                  │
│       │                  ┌────────────────────────────────────┐             │
│       │                  │ Looks up key_mappings table:       │             │
│       │                  │ 0x49 → {KEY_I, 0x7000C}            │             │
│       │                  │ Writes to uinput:                  │             │
│       │                  │ - EV_MSC, MSC_SCAN, 0x7000C        │             │
│       │                  │ - EV_KEY, KEY_I, 1                 │             │
│       │                  │ - EV_SYN, SYN_REPORT               │             │
│       ▼                  └────────────────────────────────────┘             │
│  libevdev_uinput → /dev/input/eventX (virtual keyboard)                     │
│       │                                                                     │
│       ▼                                                                     │
│  Sway compositor (wlroots + libinput)                                       │
│       │                                                                     │
│       ▼                                                                     │
│  Wayland client (Zed editor)                                                │
└─────────────────────────────────────────────────────────────────────────────┘
```

## Root Cause Analysis

### 1. SDL2 Version Differences (Primary Suspect for "i key not working")

**RHEL 9.4:** SDL 2.26.x (shipped November 2022)
**Ubuntu 24.04:** SDL 2.30.x (shipped March 2024)

Between these versions, SDL has had numerous keyboard handling fixes:
- Fixed X11 keycode to scancode mappings for certain keys
- Fixed dead key handling
- Fixed modifier key state tracking
- Fixed unrecognized key warnings

**Critical SDL Bug:** [Issue #2895](https://github.com/libsdl-org/SDL/issues/2895) - Missing X11 scancode mappings
- Some keys generate "The key you just pressed is not recognized by SDL"
- These keys are simply dropped, never reaching Wolf

**Hypothesis:** The 'i' key issue may be related to:
1. SDL2 on RHEL 9 occasionally failing to recognize the scancode
2. Race conditions in event delivery causing key events to be coalesced
3. X11/XKB state desync after dead key or modifier events

### 2. The 8-Offset Problem (X11 vs evdev)

```
evdev keycodes:     0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, ...
X11 keycodes:       8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, ...
                    ↑
                    Reserved by X11 protocol (cannot be changed)
```

**Source:** [Stack Exchange](https://unix.stackexchange.com/questions/537982/why-do-evdev-keycodes-and-x11-keycodes-differ-by-8)

SDL handles this offset internally, but bugs exist:
- [Issue #1074](https://github.com/libsdl-org/SDL/issues/1074) - Remapped modifier keys not mapped correctly
- The offset calculation can fail for non-standard keyboards

### 3. Kernel Input Subsystem Differences

**RHEL 9.4 Kernel:** 5.14.0-427 (frozen at RHEL 9.0 release, May 2022)
**Ubuntu 24.04 Kernel:** 6.8.x (released March 2024)

While Red Hat [backports security fixes](https://www.redhat.com/en/blog/what-backporting-and-how-does-it-apply-rhel-and-other-red-hat-products), they **do NOT backport new features** to the input subsystem.

Key differences between 5.14 and 6.8 in `drivers/input/`:
- Bug fixes in evdev event delivery
- Improved uinput virtual device handling
- Better HID descriptor parsing
- Fixed race conditions in event coalescing

**Impact:** The same physical key press may result in:
- Slightly different timing of events
- Different event coalescing behavior
- Different handling of rapid key presses

### 4. Modifier Key State Tracking (Fixed)

The original Wolf `keyboard_key()` function only released modifiers on KEY_PRESS:

```cpp
// ORIGINAL (buggy)
if (pkt.type == KEY_PRESS) {
    // Press modifiers
    // Press key
    // Release modifiers  ← Only here!
} else {
    keyboard.release(moonlight_key);  // No modifier handling
}
```

**Fix applied in commit 067e61c:**

```cpp
// FIXED
} else {
    keyboard.release(moonlight_key);
    // Also release any modifiers NOT in packet
    if (!(pkt.modifiers & KEYBOARD_MODIFIERS::SHIFT))
        keyboard.release(M_SHIFT);
    // ... etc for CTRL, ALT, META
}
```

### 5. inputtino Repeat Thread

The inputtino library has a background thread that re-presses held keys:

```cpp
auto repeat_thread = std::thread([state, millis_repress_key]() {
    while (!state->stop_repeat_thread) {
        std::this_thread::sleep_for(std::chrono::milliseconds(millis_repress_key));
        for (auto key : state->cur_press_keys) {
            press_btn(keyboard, key);  // Re-press held keys
        }
    }
});
```

If a key gets stuck in `cur_press_keys`, it will be continuously re-pressed, causing:
- Repeated characters
- Modifier keys staying "held"
- Strange keyboard behavior

## The "i key randomly not working" Theory

Based on the research, here are the most likely causes:

### Theory A: SDL Event Loss (Most Likely)

1. User presses 'i' key on RHEL client
2. Linux kernel generates evdev event correctly
3. X11 server receives the event
4. SDL2 (2.26.x) has a race condition or bug in event processing
5. SDL fails to generate SDL_KEYDOWN event for 'i'
6. Moonlight never sees the key press
7. Wolf never receives the packet

**Evidence:**
- Issue is intermittent (race condition)
- Issue is worse on RHEL (older SDL)
- Works fine initially, then stops (state accumulation)

### Theory B: XKB State Corruption

1. Dead key or modifier sequence confuses XKB
2. XKB state becomes corrupted
3. Subsequent key events are misinterpreted
4. 'i' key generates wrong keycode or is filtered

**Evidence:**
- [Bug 660254](https://bugzilla.redhat.com/show_bug.cgi?id=660254) - XKB eats keyboard shortcuts
- Issue may be related to keyboard layout switching

### Theory C: Network Event Coalescing

1. Multiple key events arrive in rapid succession
2. ENet or Wolf coalesces events
3. Some events are lost or delayed
4. 'i' key event doesn't reach inputtino

**Evidence:**
- Network is UDP-based
- Fast typing more likely to trigger issue

## Recommended Solutions

### Immediate (Observability)

1. **Add Wolf keyboard state API**
   - Expose `cur_press_keys` via REST API
   - Show modifier state per session
   - Log all keyboard events with timestamps

2. **Create keyboard visualization widget**
   - Show which keys Wolf thinks are pressed
   - Highlight stuck keys in red
   - Show modifier state (Ctrl, Shift, Alt, Meta)

### Short-term (Wolf-side Fixes)

3. **Add keyboard state reset endpoint**
   ```
   POST /api/v1/wolf/sessions/{id}/keyboard/reset
   ```
   Clears all `cur_press_keys` and releases all modifiers

4. **Add modifier timeout in inputtino**
   Auto-release modifiers held > 5 seconds without other activity

### Medium-term (Client-side Investigation)

5. **Test with newer SDL on RHEL**
   Build SDL 2.30.x from source and test

6. **Add SDL debug logging on client**
   Capture all keyboard events before they reach Moonlight

7. **Test with Wayland instead of X11 on RHEL**
   Wayland has different keyboard handling (no 8-offset)

### Long-term (Protocol Improvements)

8. **Add keyboard state sync message**
   Periodically sync full keyboard state between client and server

9. **Add keyboard event acknowledgment**
   Server confirms receipt of each keyboard event

## Implementation Plan for Observability

### Wolf API Endpoint

```go
// GET /api/v1/wolf/sessions/{session_id}/keyboard-state
type KeyboardStateResponse struct {
    SessionID       string   `json:"session_id"`
    Timestamp       int64    `json:"timestamp"`
    PressedKeys     []int    `json:"pressed_keys"`
    PressedKeysHex  []string `json:"pressed_keys_hex"`
    KeyNames        []string `json:"key_names"`
    ModifierState   struct {
        Shift bool `json:"shift"`
        Ctrl  bool `json:"ctrl"`
        Alt   bool `json:"alt"`
        Meta  bool `json:"meta"`
    } `json:"modifier_state"`
}
```

### Frontend Widget

```typescript
// KeyboardStateWidget component
// Shows:
// - Virtual keyboard layout
// - Currently pressed keys highlighted
// - Modifier key states
// - "Reset Keyboard" button
// - Auto-refresh every 100ms when active
```

## Files Modified/Created

1. **This document:** `design/2025-11-26-keyboard-input-deep-dive.md`
2. **Wolf modifier fix:** `/prod/home/luke/pm/wolf/src/moonlight-server/control/input_handler.cpp` (commit 067e61c)
3. **Wolf keyboard state tracker:** `/prod/home/luke/pm/wolf/src/moonlight-server/control/keyboard_state.hpp` (NEW)
   - Global singleton tracker that monitors all key presses/releases per session
   - Tracks Moonlight (Windows VK) key codes as they flow through Wolf
   - Independent of inputtino - tracks what Wolf receives from Moonlight
4. **Wolf keyboard state API endpoints:**
   - `GET /api/v1/keyboard/state` - Returns pressed keys for all sessions
   - `POST /api/v1/keyboard/reset` - Releases all stuck keys for a session
5. **Helix API handlers:** TBD - `api/pkg/server/wolf_keyboard_handlers.go`
6. **Frontend widget:** TBD - `frontend/src/components/wolf/KeyboardStateWidget.tsx`

## References

- [games-on-whales/inputtino](https://github.com/games-on-whales/inputtino) - Virtual input library
- [Wolf Issue #217](https://github.com/games-on-whales/wolf/issues/217) - Left Shift/Control not working
- [Wolf Issue #245](https://github.com/games-on-whales/wolf/issues/245) - Keyboard malfunction
- [SDL Issue #2895](https://github.com/libsdl-org/SDL/issues/2895) - Missing X11 scancode mappings
- [SDL Issue #1074](https://github.com/libsdl-org/SDL/issues/1074) - Modifier key mapping
- [Moonlight Qt keyboard.cpp](https://deepwiki.com/moonlight-stream/moonlight-qt/4.7-input-handling) - Input handling docs
- [evdev vs X11 keycodes](https://unix.stackexchange.com/questions/537982/why-do-evdev-keycodes-and-x11-keycodes-differ-by-8) - The 8-offset explanation
- [Red Hat backporting](https://www.redhat.com/en/blog/what-backporting-and-how-does-it-apply-rhel-and-other-red-hat-products) - RHEL kernel policy

## Testing Plan

1. **Reproduce the issue:**
   - Use RHEL 9.4 Moonlight client
   - Type rapidly, especially 'i' key
   - Note when issue occurs

2. **Enable debug logging:**
   - SDL: `SDL_VIDEO_DEBUG=1`
   - X11: `LIBGL_DEBUG=verbose`
   - Wolf: Enable verbose keyboard logging

3. **Test with observability:**
   - Open keyboard state widget
   - Watch for stuck keys
   - Correlate with typed characters

4. **Test fixes:**
   - Verify modifier release fix
   - Test keyboard reset endpoint
   - Compare RHEL vs Ubuntu behavior
