# Keyboard Modifier Stuck Issue - Deep Dive Analysis

**Date:** 2025-11-25
**Issue:** Keyboard modifiers (Ctrl/Caps Lock) get stuck when using Moonlight streaming
**Severity:** High (worst on RHEL clients)
**Author:** Claude (AI) + Luke

## Executive Summary

The keyboard modifier stuck issue is caused by a **missing modifier release mechanism** in Wolf's input handler combined with **unreliable KEY_RELEASE event delivery** from Moonlight clients (especially RHEL).

## Architecture Overview

```
Moonlight Client (RHEL/Windows/Mac)
    ↓ (UDP packets with KEYBOARD_PACKET)
Wolf Server
    ↓ input_handler.cpp::keyboard_key()
inputtino::Keyboard
    ↓ libevdev_uinput_write_event()
Linux uinput device
    ↓ /dev/input/eventX
Sway compositor (libinput)
    ↓ Wayland events
Application (Zed, Firefox, etc.)
```

## Root Cause Analysis

### The Bug Location

**File:** `/prod/home/luke/pm/wolf/src/moonlight-server/control/input_handler.cpp`
**Function:** `keyboard_key()` (lines 340-374)

```cpp
void keyboard_key(const KEYBOARD_PACKET &pkt, events::StreamSession &session) {
  short moonlight_key = (short)boost::endian::little_to_native(pkt.key_code) & (short)0x7fff;
  if (session.keyboard->has_value()) {
    if (pkt.type == KEY_PRESS) {
      // Press the virtual modifiers
      if (pkt.modifiers & KEYBOARD_MODIFIERS::SHIFT && moonlight_key != M_SHIFT)
        keyboard.press(M_SHIFT);
      if (pkt.modifiers & KEYBOARD_MODIFIERS::CTRL && moonlight_key != M_CTRL)
        keyboard.press(M_CTRL);
      // ... more modifiers ...

      // Press the actual key
      keyboard.press(moonlight_key);

      // Release the virtual modifiers
      if (pkt.modifiers & KEYBOARD_MODIFIERS::SHIFT && moonlight_key != M_SHIFT)
        keyboard.release(M_SHIFT);
      // ... more modifiers ...
    } else {
      // *** THE BUG IS HERE ***
      // On KEY_RELEASE, only the actual key is released
      // Modifiers are NOT checked or released!
      keyboard.release(moonlight_key);
    }
  }
}
```

### Inputtino Keyboard State Tracking

**File:** `/prod/home/luke/pm/wolf/build/_deps/inputtino-src/src/uinput/keyboard.cpp`

```cpp
// Keys are tracked in cur_press_keys vector
void Keyboard::press(short key_code) {
  if (auto key = press_btn(keyboard, key_code)) {
    _state->cur_press_keys.push_back(key_code);  // Added to tracking
  }
}

void Keyboard::release(short key_code) {
  this->_state->cur_press_keys.erase(
      std::remove(..., key_code), ...);  // Removed from tracking
  // ... write release event ...
}

// CRITICAL: Repeat thread keeps re-pressing held keys!
auto repeat_thread = std::thread([state, millis_repress_key]() {
  while (!state->stop_repeat_thread) {
    std::this_thread::sleep_for(std::chrono::milliseconds(millis_repress_key));
    for (auto key : state->cur_press_keys) {
      press_btn(keyboard, key);  // Keys in cur_press_keys are constantly re-pressed!
    }
  }
});
```

### The Stuck Modifier Scenario

1. **User presses Ctrl key on Moonlight client**
   - Moonlight sends: `KEY_PRESS, key=M_CTRL, modifiers=0`
   - Wolf calls: `keyboard.press(M_CTRL)`
   - Ctrl added to `cur_press_keys`

2. **User releases Ctrl key on client**
   - **Expected:** Moonlight sends `KEY_RELEASE, key=M_CTRL`
   - **Actual (bug):** Release event is lost/delayed/never sent

3. **Result:**
   - Ctrl stays in `cur_press_keys`
   - Repeat thread keeps pressing Ctrl every N milliseconds
   - Every subsequent keypress acts like Ctrl+key

### Why RHEL is Worse

Possible factors (needs further investigation):
- RHEL's X11/Wayland keyboard driver handles events differently
- Moonlight Qt on RHEL may have different event coalescing
- SELinux or other security policies may interfere
- Different kernel event timing

## Existing Workaround

**File:** `/prod/home/luke/pm/helix/wolf/sway-config/startup-app.sh` (lines 220-222)

```bash
# Workaround for Moonlight keyboard modifier state desync bug
# Press Super+Escape to reset all modifier keys if they get stuck
bindsym $mod+Escape exec swaymsg 'input type:keyboard xkb_switch_layout 0'
```

This only resets the **Sway** keyboard layout state, not the **inputtino** `cur_press_keys` state.

## Proposed Solutions

### Solution 1: Modifier Timeout (Recommended)

Add a timeout mechanism in inputtino that auto-releases modifier keys if held for too long without any other key activity.

```cpp
// In KeyboardState struct
std::chrono::steady_clock::time_point last_modifier_press;
static constexpr auto MODIFIER_TIMEOUT = std::chrono::seconds(5);

// In repeat thread
void check_modifier_timeout() {
  auto now = std::chrono::steady_clock::now();
  if (now - last_modifier_press > MODIFIER_TIMEOUT) {
    for (auto key : {M_CTRL, M_SHIFT, M_ALT, M_META}) {
      if (std::find(cur_press_keys.begin(), cur_press_keys.end(), key) != cur_press_keys.end()) {
        release(key);
      }
    }
  }
}
```

**Pros:**
- Automatic recovery without user action
- Works even if release events are completely lost

**Cons:**
- Might interfere with legitimate long modifier holds (rare in practice)
- Requires inputtino modification

### Solution 2: Periodic Modifier Sync

Periodically request modifier state from Moonlight client and sync.

**Pros:**
- Accurate to actual client state

**Cons:**
- Requires Moonlight protocol extension
- More complex implementation

### Solution 3: Sway-level Modifier Reset Command

Create a Wolf API endpoint that forces all modifier keys to be released.

```go
// In wolf API
POST /api/v1/wolf/reset-modifiers
```

Then call this periodically or on demand from Sway.

**Pros:**
- No inputtino modification needed
- Can be triggered automatically

**Cons:**
- Requires API round-trip
- May interrupt legitimate modifier holds

### Solution 4: Fix Wolf input_handler.cpp (Quick Fix)

Modify `keyboard_key()` to also release modifiers on KEY_RELEASE if they're not in the packet's modifier flags.

```cpp
} else {  // KEY_RELEASE
  // Release actual key
  keyboard.release(moonlight_key);

  // Also release any modifiers that are NOT in the packet's modifier flags
  // This ensures modifiers don't stay stuck
  if (!(pkt.modifiers & KEYBOARD_MODIFIERS::SHIFT))
    keyboard.release(M_SHIFT);
  if (!(pkt.modifiers & KEYBOARD_MODIFIERS::CTRL))
    keyboard.release(M_CTRL);
  if (!(pkt.modifiers & KEYBOARD_MODIFIERS::ALT))
    keyboard.release(M_ALT);
  if (!(pkt.modifiers & KEYBOARD_MODIFIERS::META))
    keyboard.release(M_META);
}
```

**Pros:**
- Simple fix
- Works immediately

**Cons:**
- May cause issues if modifier is intentionally held
- Need to test thoroughly

## Recommended Implementation

1. **Immediate (Quick Fix):** Implement Solution 4 in Wolf's input_handler.cpp
2. **Short-term:** Add modifier timeout (Solution 1) to inputtino
3. **Long-term:** Investigate RHEL-specific issues in Moonlight client

## Testing Plan

1. Test on RHEL client:
   - Press and release Ctrl rapidly 10 times
   - Press Ctrl+C, verify Ctrl releases
   - Hold Ctrl for 5 seconds, verify auto-release

2. Test on Windows/Mac clients for regression

3. Test with Zed editor (common stuck key reports)

## Files to Modify

1. `/prod/home/luke/pm/wolf/src/moonlight-server/control/input_handler.cpp` - Quick fix
2. `/prod/home/luke/pm/wolf/build/_deps/inputtino-src/src/uinput/keyboard.cpp` - Timeout mechanism
3. `/prod/home/luke/pm/helix/wolf/sway-config/startup-app.sh` - Enhanced workaround

## References

- [Wolf Issue #217](https://github.com/games-on-whales/wolf/issues/217) - Left Shift/Control not working
- [Wolf Issue #245](https://github.com/games-on-whales/wolf/issues/245) - Left shift and control malfunction
- [inputtino library](https://github.com/games-on-whales/inputtino) - Virtual input handling
- [Stack Overflow: libevdev stuck key bug](https://stackoverflow.com/questions/75945088/libevdev-key-remapping-stuck-key-bug)
