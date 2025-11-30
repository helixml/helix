# Keyboard Layout Reset Bug - Investigation Notes

**Date:** 2025-11-30
**Status:** ROOT CAUSE IDENTIFIED - WLROOTS PATCH REQUIRED
**Category:** Bug Investigation

## EXECUTIVE SUMMARY

### The Problem
Pressing any modifier key (Shift, Ctrl, Alt) resets non-US keyboard layouts to US in Sway inside Wolf-streamed sandbox.

### Root Cause
The Wayland protocol's `wl_keyboard.modifiers` event bundles modifier state (Shift/Ctrl/Alt) together with layout group. There is no way to send one without the other. When the outer compositor sends this event, wlroots calls `xkb_state_update_mask(..., group)` which resets Sway's active layout.

### Why Simple Fixes Don't Work
1. **Single "us" layout on outer compositor**: Tested and FAILED. Even sending `group=0` still resets Sway's layout.
2. **Synchronized layout switching**: REJECTED by user as too complex and fragile.
3. **Skipping modifiers events**: Would break Shift/Ctrl/Alt entirely because wlroots Wayland backend uses `update_state=false` for key events.

### REJECTED Solution: Patch wlroots
Patching wlroots to use `update_state=true` was considered but **REJECTED** because:
- We need to support multiple desktop environments (Sway, GNOME, KDE, Gamescope)
- Each has its own Wayland backend implementation
- Maintaining patches for all of them is not sustainable

### Recommended Solution: Outer Compositor Layout Management
Move keyboard layout management entirely to the **outer compositor** (wayland-display-core/Smithay). The inner compositor (Sway, GNOME, etc.) should be configured with a single layout matching the outer's current layout, effectively delegating layout control to the outer compositor.

**Key insight:** The outer compositor is the one we control. If IT owns layout management, the inner compositor just receives the already-translated keycodes.

**Implementation location:** Wolf lobby creation and runtime API + wayland-display-core XKB configuration.

---

## Problem

When using a non-US keyboard layout (e.g., French or British) in Sway inside a Wolf-streamed sandbox, pressing any modifier key (Shift, Ctrl, Alt) causes the keyboard layout to reset to US.

**Steps to reproduce:**
1. Start a sandbox session with Sway
2. Sway is configured with multiple keyboard layouts: `us,gb,fr`
3. Switch to French or British layout using Sway's layout switching (keyboard buttons in panel, or Alt+Shift if configured)
4. Press Left Shift (or any modifier key)
5. **Result:** Layout resets to US

## CRITICAL: What We Know vs What We've Assumed

### What We KNOW for certain:

1. **The bug exists** - pressing Shift resets non-US layout to US in Sway
2. **The bug existed BEFORE any changes today** - user confirmed this
3. **Container architecture:**
   - `helix-sandbox` container = Runs Wolf + the outer Wayland compositor (`waylanddisplaysrc` GStreamer element using `wayland-display-core` Rust code)
   - Wolf spawns `helix-sway` container = Runs Sway (inner Wayland compositor)
   - Architecture: `Moonlight → helix-sandbox (Wolf + outer compositor) → helix-sway (Sway)`

4. **The outer compositor (wayland-display-core) sends `wl_keyboard.modifiers` events to Sway** - these include a `group` field (layout index)

5. **Smithay's code serializes `layout_effective`:**
   ```rust
   // In modifiers_state.rs
   let layout_effective = state.serialize_layout(xkb::STATE_LAYOUT_EFFECTIVE);
   ```
   This is the current layout index in the outer compositor's XKB state.

6. **wlroots (Sway's backend) passes the `group` to `xkb_state_update_mask`:**
   ```c
   xkb_state_update_mask(keyboard->xkb_state,
       mods_depressed, mods_latched, mods_locked,
       0, 0, group);  // group parameter updates layout state
   ```

### What We DON'T KNOW:

1. **Whether the current "fix" actually works** - IT HAS NOT BEEN TESTED

2. **What the actual difference is between `XkbConfig::default()` and `XkbConfig { layout: "us", .. }`:**
   - According to [xkbcommon docs](https://xkbcommon.org/doc/current/structxkb__rule__names.html), empty string causes xkbcommon to check:
     1. `XKB_DEFAULT_LAYOUT` environment variable
     2. System default (compiled-in or from `/etc/default/keyboard`)
   - The `helix-sandbox` container has NO `/etc/default/keyboard` file
   - The host has `XKBLAYOUT="gb"` in `/etc/default/keyboard`
   - **UNKNOWN:** What layout does xkbcommon actually use when layout="" and no env var is set inside helix-sandbox?

3. **Why the bug exists if the original code was `XkbConfig::default()`:**
   - If `default()` results in a single "us" layout, and my "fix" is explicit `layout: "us"`, they should be identical
   - **UNLESS:** xkbcommon's default includes multiple layouts somehow, or there's environment variable inheritance we don't understand

## Timeline of Changes

1. **>2 hours ago (original state):**
   - Code: `seat.add_keyboard(XkbConfig::default(), 200, 25)`
   - Bug was present (user confirmed)

2. **~1 hour ago (commit 1cac15d):**
   - Changed to: `XkbConfig { layout: "us,gb,fr", options: Some("caps:ctrl_nocaps".into()), .. }`
   - This was an attempt to "match" the inner Sway's layouts - this was WRONG because it doesn't help (outer's layout_effective still gets sent)

3. **Now (commit 9d71aa4):**
   - Changed to: `XkbConfig { layout: "us", ..XkbConfig::default() }`
   - This is CLAIMED to fix the bug but HAS NOT BEEN TESTED

## Key Files

1. **Outer compositor XKB config:**
   - `/prod/home/luke/pm/wolf/gst-wayland-display/wayland-display-core/src/comp/mod.rs`
   - Line ~226: `seat.add_keyboard(XkbConfig { layout: "us", .. }, 200, 25)`

2. **Smithay modifiers serialization:**
   - `/prod/home/luke/.cargo/git/checkouts/smithay-afc82e866f6972a4/a166cf4/src/input/keyboard/modifiers_state.rs`
   - Line 136: `let layout_effective = state.serialize_layout(xkb::STATE_LAYOUT_EFFECTIVE);`

3. **Inner compositor (Sway) config:**
   - `/prod/home/luke/pm/helix/wolf/sway-config/config`
   - Contains: `xkb_layout "us,gb,fr"`

4. **Sandbox environment setup:**
   - `/prod/home/luke/pm/helix/api/pkg/external-agent/wolf_executor.go`
   - Passes `XKB_DEFAULT_LAYOUT=us,gb,fr` to the helix-sway container (inner compositor)
   - **BUT NOT** to the helix-sandbox container (outer compositor where waylanddisplaysrc runs)

## Theory: Why Single Layout Might Fix It

**IF** the outer compositor has only one layout:
- `layout_effective` is always 0
- When sent to Sway via `wl_keyboard.modifiers` with `group=0`
- Sway's `xkb_state_update_mask` receives `group=0`
- If Sway is currently on layout 0 (us), no change
- If Sway is on layout 1 (gb) or 2 (fr), it MIGHT get reset to 0

**BUT** this theory doesn't explain why the original `XkbConfig::default()` (which should also result in single layout) had the bug.

## Possible Root Causes We Haven't Investigated

1. **XkbConfig::default() might pick up multiple layouts from somewhere:**
   - Could xkbcommon be reading X11 config files?
   - Could there be a system-wide XKB default that includes multiple layouts?

2. **The host's /etc/default/keyboard might be inherited:**
   - Host has `XKBLAYOUT="gb"`
   - If xkbcommon reads this somehow, the outer compositor would have "gb" layout, not "us"
   - Then `layout_effective` would depend on which layout the outer thinks it's on

3. **There might be XKB environment variables we don't know about:**
   - `XKB_DEFAULT_RULES`, `XKB_DEFAULT_MODEL`, etc.
   - These could affect behavior

## NEXT STEPS (for next session)

1. **BUILD the current code into helix-sandbox image**
   - Current Wolf images are from BEFORE commit 9d71aa4
   - Run `./stack build-sandbox` or `./stack rebuild-wolf`

2. **TEST whether the fix works:**
   - Start a sandbox session
   - Switch Sway to French layout
   - Press Shift
   - Does layout reset to US? If no, fix works. If yes, fix doesn't work.

3. **If fix doesn't work, investigate:**
   - What layout does the outer compositor actually have?
   - Add logging to wayland-display-core to print the XkbConfig it's using
   - Check if any XKB env vars are set in helix-sandbox container

4. **Key command to rebuild:**
   ```bash
   ./stack build-sandbox  # Rebuilds helix-sandbox with new Wolf code
   ```

## References

- [xkbcommon xkb_rule_names documentation](https://xkbcommon.org/doc/current/structxkb__rule__names.html) - explains empty string behavior
- [xkbcommon xkb_state_update_mask](https://xkbcommon.org/doc/current/group__state.html) - how state is updated
- [wlroots wlr_keyboard.c](https://github.com/swaywm/wlroots/blob/master/types/wlr_keyboard.c) - Sway's keyboard handling

## Wolf Commits

- `9d71aa4` - Current "fix" (explicit `layout: "us"`) - UNTESTED
- `1cac15d` - Wrong approach (multi-layout `us,gb,fr`) - REVERTED
- `2e593d2` - Original state (`XkbConfig::default()`) - HAD BUG

## UNTESTED CONFIGURATIONS

### Multi-layout on outer compositor (commit 1cac15d)

**Configuration tested but NOT with thorough analysis:**
```rust
XkbConfig {
    layout: "us,gb,fr",
    options: Some("caps:ctrl_nocaps".into()),
    ..XkbConfig::default()
}
```

**Why this was attempted:** The idea was to "match" the inner Sway's layout configuration so `layout_effective` would correspond to the same layouts.

**Why it was reverted:** Theoretically, this doesn't help because:
1. Outer compositor's layout state is independent of Sway's layout state
2. User changes layout in Sway (inner), not in the outer compositor
3. Outer's `layout_effective` would still be stuck on whatever layout the outer thinks it's on (likely 0/us)
4. When user switches Sway to French (index 2) and presses Shift, outer still sends `group=0` (its current layout), resetting Sway back to US

**BUT WE NEVER ACTUALLY TESTED THIS CONFIGURATION.** The reasoning was done theoretically. It's possible that:
- Having matching layouts somehow helps with layout synchronization
- There's behavior we don't understand about how wl_keyboard.modifiers is interpreted
- The outer compositor might respond to layout changes from the client somehow

**Recommendation:** If the single "us" layout fix doesn't work, this multi-layout configuration should be tested with logging enabled to see what actually happens.

## Logging Added

Detailed keyboard event logging was added to `wayland-display-core/src/comp/input.rs`:
- `🎹 KEYBOARD EVENT:` - logs every key press/release
- `🎹 XKB STATE BEFORE:` - logs layout_effective, layout_locked, layout_latched, and layout names before key processing
- `🎹 MODIFIERS:` - logs modifier state during key callback
- `🎹 XKB STATE AFTER:` - logs layout and modifier state after processing
- `🎹 WILL SEND TO CLIENT: group/layout_effective=X` - logs the exact value that will be sent to Sway

This logging is now built into the latest helix-sandbox image.

## WAYLAND_DEBUG Option

For even more detailed tracing, `WAYLAND_DEBUG=1` can be set in the helix-sway container environment to see all Wayland protocol messages including `wl_keyboard.modifiers` events with the `group` value that Sway receives.

## TEST RESULTS (2025-11-30)

**Single layout fix DID NOT WORK.**

Logs from the test clearly show:
```
sandbox-1  | 🎹 XKB CONFIG: layout='us' variant='' model='' rules='' options=None
sandbox-1  | 🎹 KEYBOARD INITIALIZED with explicit 'us' layout config
sandbox-1  | 🎹 XKB STATE BEFORE: layouts=1 effective=0 locked=0 latched=0 names=[0:English (US)]
sandbox-1  | 🎹 WILL SEND TO CLIENT: group/layout_effective=0
```

The outer compositor correctly has a single "us" layout and sends `group=0`. **BUT THE BUG STILL OCCURS.**

## ROOT CAUSE CONFIRMED

The problem is NOT the outer compositor sending the wrong layout index. The problem is:

1. Sway (inner compositor) manages its own keyboard layouts (us,gb,fr) for its clients
2. User switches to French layout (index 2) using Sway's layout switcher
3. When user presses Shift, outer compositor sends `wl_keyboard.modifiers` with `group=0`
4. wlroots (Sway's backend) calls `xkb_state_update_mask(..., group=0)`
5. This resets Sway's active layout to index 0 (US)

**The fundamental issue:** The Wayland protocol requires sending a `group` value with every modifiers event. There's no "don't change layout" value. Whatever value we send (even 0) will update Sway's layout state.

## POTENTIAL SOLUTIONS

### Option 1: Suppress modifiers events (NOT RECOMMENDED)
Don't send `wl_keyboard.modifiers` events at all. This would break modifier key functionality (Shift, Ctrl, Alt wouldn't work properly).

### Option 2: Match outer compositor to Sway's layouts (PARTIAL FIX)
Configure outer compositor with same layouts as Sway (`us,gb,fr`). When user switches layout in Sway, somehow synchronize the outer compositor's layout to match.

**Problem:** There's no Wayland protocol for Sway to notify the outer compositor of layout changes.

### Option 3: Track client layout state (COMPLEX)
Implement a custom protocol or use an existing one (zwp_virtual_keyboard_v1?) to track what layout Sway is currently using, and send that value back.

### Option 4: Patch wlroots to ignore group from parent compositor (SWAY FORK)
Modify Sway/wlroots to NOT update the layout group when receiving `wl_keyboard.modifiers` from the parent compositor. Only honor layout changes from internal sources.

**This requires forking wlroots/Sway.**

### Option 5: Use virtual keyboard protocol (INVESTIGATION NEEDED)
The zwp_virtual_keyboard_v1 protocol might allow more control over keyboard events. Need to investigate if it bypasses the modifiers→group issue.

### Option 6: Per-window layout tracking
Some wlroots compositors support per-window layout (see [wlroots-kbdd](https://github.com/avarvit/wlroots-kbdd)). This might provide a workaround but would require Sway configuration changes.

## NEXT STEPS

1. **Investigate Option 2 more deeply:** Can we make the outer compositor track and sync with Sway's layout? Even if there's no protocol, could we use a side channel (IPC, shared state)?

2. **Investigate Option 5:** Does zwp_virtual_keyboard_v1 offer any advantage?

3. **Consider Option 4:** If patching wlroots is the only solution, document what the patch would need to do.

## KEY INSIGHT: WE ALREADY SEND layout_effective=0

**IMPORTANT:** We verified that the outer compositor sends `layout_effective=0` (because it only has a single "us" layout). But the bug STILL occurs.

This means the problem is NOT that we're sending the wrong layout index. The problem is that we're sending ANY layout index at all. When wlroots receives `group=0` in `wl_keyboard.modifiers`, it calls `xkb_state_update_mask(..., group=0)` which resets Sway's layout to index 0.

**The fundamental problem:** There is no "neutral" value for `group` that means "don't change the layout." Any value we send (0, 1, 2, etc.) will update Sway's layout state.

## User Suggestion: Send Only Raw Keycodes (No Modifiers Events)

The user observed: **Physical keyboards don't send layout information - they just send raw keycodes. The compositor is supposed to apply its own XKB keymap to interpret those scancodes.**

Question: Can we simply NOT send `wl_keyboard.modifiers` events at all?

### How Wayland Keyboard Events Work

The `wl_keyboard` protocol has these events:
1. `keymap` - sends the XKB keymap to the client (once when focus is gained)
2. `enter` - sent when surface gains keyboard focus
3. `leave` - sent when surface loses keyboard focus
4. `key` - sends raw scancode + key state (pressed/released)
5. `modifiers` - sends current modifier state (depressed, latched, locked, **group**)

### Option 7: Skip Modifiers Events Entirely

**Theory:** If we don't send `wl_keyboard.modifiers` events, wlroots/Sway would track modifier state based on which keys are pressed (from `key` events), similar to how it would work with a physical keyboard.

**Investigation needed:**
1. Does wlroots update its XKB state from key events, or does it rely on modifiers events?
2. What happens when a compositor (like Sway) doesn't receive modifiers events?
3. Smithay's `KeyboardGrab::input()` takes `modifiers: Option<ModifiersState>` - if `None`, modifiers event is not sent

**Code path in Smithay:**
```rust
// In KeyboardInnerHandle::input() at mod.rs:1295-1297
if let Some(mods) = modifiers {
    focus.modifiers(self.seat, data, mods, serial);
}
```
If `modifiers` is `None`, the modifiers event is NOT sent to the client.

**We need to test:**
1. Can we modify the outer compositor to pass `modifiers: None` to `handle.input()`?
2. If so, does Sway still correctly track Shift/Ctrl/Alt state from key events?

### Why This Might Work

When wlroots receives keyboard input from a physical device, it calls `xkb_state_update_key()` on its own XKB state. This updates modifier state automatically based on which keys are pressed.

The question is whether wlroots does the same thing when receiving `wl_keyboard.key` events from a parent compositor. If it does, then NOT sending modifiers events should work.

### Why This Might NOT Work

wlroots might be designed to:
1. NOT call `xkb_state_update_key()` for key events from Wayland clients
2. Rely entirely on `wl_keyboard.modifiers` events for modifier state

In this case, not sending modifiers events would break Shift/Ctrl/Alt completely.

## REFERENCES

- [wlroots keyboard.c](https://github.com/swaywm/wlroots/blob/master/types/wlr_keyboard.c) - Shows `xkb_state_update_mask(..., group)` call
- [wlroots keyboard.h](https://github.com/swaywm/wlroots/blob/master/include/wlr/types/wlr_keyboard.h) - `wlr_keyboard_modifiers` struct with `group` field
- [Virtual keyboard protocol](https://wayland.app/protocols/virtual-keyboard-unstable-v1) - Potential alternative for keyboard input
- [wlroots issue #287](https://github.com/swaywm/wlroots/issues/287) - Keyboard interface redesign discussion

## NEXT EXPERIMENT: Skip Modifiers Events

To test Option 7, we need to:

1. **Modify the outer compositor** to not send modifiers events
2. **Test** whether Shift/Ctrl/Alt still work in Sway
3. **Test** whether the layout reset bug is fixed

**Code change location:** `wayland-display-core/src/comp/input.rs` in `keyboard_input()` function

**Two approaches:**
1. Create a custom `KeyboardGrab` that passes `None` for modifiers
2. Modify the keyboard event path to skip modifiers

**Risk:** If wlroots doesn't update XKB state from key events, this will completely break modifier keys.

## Option 8: Use inputtino Virtual Keyboard (Bypass Wayland Protocol Entirely)

**PROMISING APPROACH DISCOVERED**

Wolf already has TWO keyboard input paths:

1. **WaylandKeyboard** - Sends keyboard events through Wayland protocol to the outer compositor
   - Code: `wolf/src/core/src/platforms/linux/virtual-display/gst-wayland-display.cpp`
   - Flow: `Moonlight → Wolf → GStreamer message → outer compositor → wl_keyboard events → Sway`
   - **Problem:** This path sends `wl_keyboard.modifiers` with `group` which resets Sway's layout

2. **inputtino::Keyboard** - Creates a virtual `/dev/input/eventX` device via uinput
   - Code: `wolf/third_party/inputtino/src/uinput/keyboard.cpp`
   - Flow: `Moonlight → Wolf → inputtino → /dev/input/eventX → (could go to) libinput → Sway`
   - **This behaves like a physical keyboard - sends raw keycodes, NO layout information!**

### How Wolf Chooses Keyboard Type

In `wolf/src/moonlight-server/sessions/lobbies.cpp` lines 235-238:
```cpp
// switch mouse and keyboard in session to use the lobby wayland server
auto wl_state = lobby->wayland_display->load();
session->mouse->emplace(virtual_display::WaylandMouse(wl_state));
session->keyboard->emplace(virtual_display::WaylandKeyboard(wl_state));  // <- THE PROBLEM
```

When a session joins a lobby, Wolf uses `WaylandKeyboard` which goes through the outer compositor.

### How Touch/Joypad Devices Work (Evidence This Can Work)

Wolf already passes inputtino devices to Sway for touch screens and joypads:

```cpp
// From input_handler.cpp lines 229-234 (touch screen)
if (auto wl = *session.wayland_display->load()) {
    for (const auto node : touch_screen->get_nodes()) {
        add_input_device(*wl, node);
    }
}
```

The `add_input_device()` function tells the outer compositor to make the `/dev/input/eventX` device accessible to Sway via libinput.

### Proposed Solution

1. **Create inputtino::Keyboard at session/lobby start** (like we do for touch screen)
2. **Call add_input_device() to pass the virtual keyboard to Sway**
3. **Route keyboard events through inputtino instead of WaylandKeyboard**

The virtual keyboard would behave exactly like a physical USB keyboard:
- Sends raw key scancodes
- No layout information sent
- Sway uses its own XKB configuration to interpret keys
- **Layout changes in Sway are completely independent of the outer compositor**

### Key Code Locations

1. **inputtino Keyboard creation:** `wolf/third_party/inputtino/src/uinput/keyboard.cpp`
   - `Keyboard::create()` creates the virtual device
   - `Keyboard::press()`/`release()` send raw keycodes

2. **Device passthrough:** `wolf/src/core/src/platforms/linux/virtual-display/gst-wayland-display.cpp`
   - `add_input_device()` tells outer compositor to make device available to Sway

3. **Session keyboard setup:** `wolf/src/moonlight-server/sessions/lobbies.cpp`
   - Lines 235-238: Where we need to switch to inputtino::Keyboard

### Why This Should Work

Physical keyboards work perfectly with nested Wayland compositors because:
- They send raw scancodes via `/dev/input/eventX`
- Each compositor applies its own XKB keymap independently
- No layout synchronization needed between compositors

The inputtino virtual keyboard is designed to behave identically to a physical keyboard.

### Implementation Steps

1. In `lobbies.cpp`, when joining a lobby:
   ```cpp
   // Instead of:
   session->keyboard->emplace(virtual_display::WaylandKeyboard(wl_state));

   // Do:
   auto keyboard = inputtino::Keyboard::create({...});
   add_input_device(*wl_state, keyboard->get_nodes()[0]);
   session->keyboard->emplace(std::move(*keyboard));
   ```

2. The keyboard type variant already supports `input::Keyboard` (inputtino):
   ```cpp
   using KeyboardTypes = std::variant<input::Keyboard, virtual_display::WaylandKeyboard>;
   ```

3. The `keyboard_key()` function in `input_handler.cpp` already handles both types via `std::visit`

### Container Architecture Problem

**CRITICAL FINDING:** This approach has a fundamental problem with our container architecture.

**The nested compositor architecture:**
```
helix-sandbox container (Wolf + outer Smithay compositor)
    └── helix-sway container (Sway as Wayland CLIENT of outer compositor)
```

**Why Sway can't read from bind-mounted `/dev/input/` devices:**

1. **Sway uses Wayland backend, not libinput**: When `WAYLAND_DISPLAY` is set (which it is - Sway connects to outer compositor), Sway auto-detects and uses the wayland backend exclusively. It doesn't try to read from `/dev/input/` devices.

2. **libinput requires a session**: Even if we set `WLR_BACKENDS=wayland,libinput`, the libinput backend requires a session via:
   - `logind` (systemd-logind)
   - `seatd`/`libseat`
   - Or running as root with `CAP_SYS_ADMIN`

   The container has none of these.

3. **Reference:** [wlroots session documentation](https://github.com/swaywm/wlroots/blob/master/include/wlr/backend/session.h) - "A session is required when running on bare metal with libinput"

**Why touch screens work but keyboards wouldn't:**
Touch screen devices are passed to the OUTER compositor's libinput (in wayland-display-core), not to Sway. The touch events then flow through Wayland protocol to Sway. This doesn't help with keyboard layouts because the Wayland protocol still sends `group` in modifiers events.

### Alternative: Modify Outer Compositor to NOT Send Layout Group

Since we control the outer compositor (wayland-display-core / Smithay), the real fix is:

**Option 9: Send dummy `group=current` instead of `group=layout_effective`**

Instead of sending the outer compositor's layout_effective, send whatever value Sway's wlroots most recently told US (via the client's keyboard layout setting, if any), or just don't update it at all.

Actually, this is still problematic because the Wayland protocol requires us to send SOME value for `group`.

**Option 10: Use zwp_virtual_keyboard_v1 protocol**

The [virtual keyboard protocol](https://wayland.app/protocols/virtual-keyboard-unstable-v1) allows:
- Creating a virtual keyboard for a specific seat
- Sending key events without the compositor's XKB state interfering
- NOT sending modifiers events at all if we don't want to

**CRITICAL FINDING from [wlroots wlr_keyboard.c](https://github.com/swaywm/wlroots/blob/master/types/wlr_keyboard.c):**

The virtual keyboard implementation sets `update_state = false` for key events. This means:
1. `xkb_state_update_key()` is NOT called
2. XKB state doesn't change from key events
3. `keyboard_modifier_update()` finds no difference
4. **No `wl_keyboard.modifiers` event is sent to clients!**

**The problem:** If Sway's wlroots doesn't update its XKB state from key events, how would Shift+A produce 'A'?

**Answer:** The virtual keyboard client is expected to send `modifiers` requests when modifier state changes. But those requests still include `group`, which would reset the layout.

**WAIT - the virtual keyboard protocol is CLIENT→COMPOSITOR, not COMPOSITOR→CLIENT!**

The virtual keyboard protocol is designed for INPUT METHOD apps (like on-screen keyboards) that want to SEND keys TO the compositor. It's not for the compositor to SEND events to clients.

In our case:
- The outer compositor (Smithay/wayland-display-core) is the SERVER
- Sway is a CLIENT of the outer compositor
- The outer compositor sends `wl_keyboard` events TO Sway

The virtual keyboard protocol would allow Sway to become a virtual keyboard client and send keys BACK to the outer compositor - that's backwards!

### Correct Understanding of the Architecture

```
Moonlight → Wolf → outer compositor (wayland-display-core) → wl_keyboard events → Sway (client)
                                                          ↑
                                   We control this! Smithay sends wl_keyboard.modifiers
```

**We need to modify the outer compositor (Smithay/wayland-display-core) to NOT send `wl_keyboard.modifiers` with a changing `group`.**

### Option 11: Don't Send Modifiers Events At All

Looking at Smithay's keyboard handling, we could:
1. Modify the keyboard input handling to NOT send modifiers events
2. Let Sway track modifier state itself from key events

**Risk:** Sway might not properly track Shift/Ctrl/Alt state without modifiers events.

### Option 12: Send Modifiers with Sway's Current Group

The modifiers event includes `group` (layout index). Instead of sending the outer compositor's `layout_effective`, we could:
1. Track what layout Sway is currently using (maybe via side channel or just always send 0)
2. Send modifiers with THAT group value

**Problem:** How do we know what group Sway is on? There's no standard protocol for a compositor to tell its parent what layout it's using.

### Option 13: Always Send group=0 But Match Sway's Layout

If we configure BOTH compositors with the same layouts (`us,gb,fr`) and always send `group=0`:
- When user switches to French (index 2) in Sway
- Outer compositor still sends `group=0`
- Sway's wlroots calls `xkb_state_update_mask(..., group=0)`
- This RESETS Sway's layout to US (index 0) ← BUG

**This is exactly what we already tested and it didn't work!**

### Option 14: Synchronized Layout Switching (Revisited)

Go back to the synchronized switching approach but make it work:

1. **Configure outer compositor with same layouts as Sway** (`us,gb,fr`)
2. **When user switches layout in Sway:**
   - Waybar sends `swaymsg input type:keyboard xkb_switch_layout N`
   - ALSO notify outer compositor to switch to layout N
   - This could be via a fifo, HTTP API, or other IPC
3. **Now when outer sends `group=N`, it matches Sway's current layout**

This was attempted before but the notification mechanism wasn't implemented.

**Implementation:**
1. Create a fifo at `/run/user/wolf/keyboard-layout-fifo` (or HTTP endpoint)
2. Waybar layout buttons also echo the layout index to this fifo
3. Outer compositor reads from fifo and calls `xkb_state_set_group()` or similar
4. Now `layout_effective` matches Sway's active layout

**Next steps:**
1. Implement layout notification from Sway to outer compositor
2. Test synchronized layout switching

## CRITICAL FINDING: Raw Keycode Approach WILL NOT WORK

### wlroots Wayland Backend Uses `update_state = false`

Examined wlroots source code: `backend/wayland/seat.c` (https://gitlab.freedesktop.org/wlroots/wlroots/)

```c
static void keyboard_handle_key(void *data, struct wl_keyboard *wl_keyboard,
		uint32_t serial, uint32_t time, uint32_t key, uint32_t state) {
	struct wlr_keyboard *keyboard = data;

	struct wlr_keyboard_key_event wlr_event = {
		.keycode = key,
		.state = state,
		.time_msec = time,
		.update_state = false,  // <-- CRITICAL!
	};
	wlr_keyboard_notify_key(keyboard, &wlr_event);
}
```

When `update_state = false`:
- `xkb_state_update_key()` is NOT called
- XKB modifier state is NOT tracked from key events
- Modifier state comes ONLY from `wl_keyboard.modifiers` events

**This means if we don't send modifiers events, Shift/Ctrl/Alt WILL NOT WORK AT ALL.**

### Why This Design Exists

The Wayland backend in wlroots is designed for nested compositors (like Sway running inside another compositor). The parent compositor is authoritative for keyboard state - it sends:
1. `wl_keyboard.key` - raw keycodes
2. `wl_keyboard.modifiers` - current modifier/layout state

The child compositor (Sway) does NOT track XKB state from key events; it trusts the parent's modifiers events.

This is different from libinput input (physical keyboards) where `update_state = true`.

### Why Skipping Modifiers Events Doesn't Work

If we skip `wl_keyboard.modifiers`:
- Sway never knows Shift is held
- Shift+A produces 'a' not 'A'
- All modifier combinations fail

### Remaining Options

1. **Synchronized Layout Switching (Option 14)** - REJECTED
   - Too complex and fragile
   - Requires bidirectional IPC between Sway and outer compositor
   - Race conditions between layout switch and key events
   - Any desync causes the same bug to reappear

2. **Patch Sway/wlroots to ignore `group` from parent** - Requires fork
   - Modify wlroots to use `update_state = true` for Wayland backend
   - Or modify `wlr_keyboard_notify_modifiers` to not update layout
   - Significant maintenance burden

3. **Alternative input path** - Needs more investigation
   - Use different protocol or mechanism
   - None identified yet that avoids the modifiers→group issue

## OPEN QUESTION: Is `update_state = false` Really the Blocker?

The claim is that wlroots' Wayland backend sets `update_state = false` for key events, meaning:
- `xkb_state_update_key()` is NOT called
- Modifier state relies entirely on `wl_keyboard.modifiers` events
- Therefore skipping modifiers events would break Shift/Ctrl/Alt

**Verification completed - CONFIRMED:**

Examined Sway's keyboard.c:
- `keyboard_keysyms_raw()` calls `wlr_keyboard_get_modifiers(keyboard->wlr)`
- `keyboard_keysyms_translated()` calls `wlr_keyboard_get_modifiers(keyboard->wlr)`
- Both rely on `keyboard->modifiers` which is only updated via:
  1. `wl_keyboard.modifiers` events (via `wlr_keyboard_notify_modifiers`)
  2. OR when `update_state = true` for key events (NOT the case for Wayland backend)

**Sway does NOT have separate modifier tracking.** It relies entirely on wlroots' `keyboard->modifiers`.

## The Core Problem

The `wl_keyboard.modifiers` event bundles TWO pieces of information:
1. **Modifier state** (Shift, Ctrl, Alt, etc.) - REQUIRED for keyboard to work
2. **Layout group** - Causes the layout reset bug

These cannot be separated in the standard Wayland protocol.

## Remaining Options for Raw Keycode Delivery

1. **Patch wlroots to use `update_state = true` for Wayland backend**
   - Would allow wlroots to track modifiers from key events
   - Then we could skip sending modifiers events entirely
   - Requires forking wlroots

2. **Send modifiers events WITHOUT the group value**
   - Modify Smithay/outer compositor to always send `group=0`
   - But wlroots still calls `xkb_state_update_mask(..., 0, 0, group)`
   - This still resets layout to 0 every time!

3. **Only send modifiers events when modifiers change (not layout)**
   - Current behavior: send modifiers when ANY of depressed/latched/locked/group changes
   - Modified: only send when depressed/latched/locked changes
   - Sway still gets modifier info for Shift/Ctrl/Alt
   - Group value becomes stale but maybe wlroots doesn't always update it?

4. **Investigate if wlroots caches the group and only updates when it changes**
   - ANSWER: No. `xkb_state_update_mask()` ALWAYS takes group parameter.
   - From [xkbcommon docs](https://xkbcommon.org/doc/current/group__state.html):
     "All parameters must always be passed, or the resulting state may be incoherent."

## MOST PROMISING: Patch wlroots Wayland Backend

The cleanest fix is to modify wlroots' Wayland backend to use `update_state = true`.

**Current behavior (in `backend/wayland/seat.c`):**
```c
static void keyboard_handle_key(...) {
    struct wlr_keyboard_key_event wlr_event = {
        .update_state = false,  // XKB state NOT updated from key events
    };
}
```

**Proposed change:**
```c
static void keyboard_handle_key(...) {
    struct wlr_keyboard_key_event wlr_event = {
        .update_state = true,   // XKB state IS updated from key events
    };
}
```

**What this would enable:**
1. wlroots would track modifier state from key press/release (like libinput backend)
2. We could then IGNORE `wl_keyboard.modifiers` events entirely
3. Outer compositor could skip sending modifiers events (or send them without affecting layout)
4. Sway would manage its own layout independently

**Risk:**
- This changes fundamental wlroots behavior
- May break compatibility with other compositors as parents
- Need to understand why `update_state = false` was chosen originally

**Investigation needed:**
1. Why did wlroots choose `update_state = false` for Wayland backend?
2. What would break if we changed it to `true`?
3. Can we make this configurable via environment variable?

### Why wlroots uses `update_state = false`

From [wlroots issue #1769](https://github.com/swaywm/wlroots/issues/1769) and related discussions:

**The reason is for keyboard enter/leave synchronization.** When the nested compositor window gains/loses focus:
- `wl_keyboard.enter` includes list of currently-pressed keys
- wlroots generates synthetic key press events to sync internal state
- These synthetic events use `update_state = false` so they don't:
  - Trigger compositor bindings (like switching workspaces)
  - Incorrectly update XKB state

**The same `update_state = false` is used for ALL key events** in the Wayland backend for consistency.

**This means:** For regular key events (not enter/leave sync), `update_state = true` might actually work fine! The enter/leave cases could be kept as `update_state = false` while regular key events use `true`.

### Potential Fix: Modify ONLY regular key events

```c
// In keyboard_handle_key() - regular key events
static void keyboard_handle_key(...) {
    struct wlr_keyboard_key_event wlr_event = {
        .update_state = true,   // Regular events CAN update state
    };
}

// In keyboard_handle_enter() - keep as false
struct wlr_keyboard_key_event event = {
    .update_state = false,  // Synthetic events should NOT update state
};
```

### Combined with skipping modifiers events

If we patch wlroots Wayland backend to use `update_state = true` for key events:
1. Key press/release updates XKB modifier state internally
2. We can skip sending `wl_keyboard.modifiers` from outer compositor
3. Or we send modifiers but wlroots ignores them (needs investigation)
4. Layout is managed by Sway independently

---

## RECOMMENDED SOLUTION: Outer Compositor Layout Management

### Why This Approach

The wlroots patching approach was **REJECTED** because:
1. We need to support Sway, GNOME, KDE, Gamescope, and other compositors
2. Each has its own Wayland backend implementation
3. Maintaining patches for all of them is not sustainable

Instead, we should move keyboard layout management to the **outer compositor** (wayland-display-core/Smithay), which we fully control.

### How It Works

1. **Outer compositor** is configured with the user's desired layout(s): `us,gb,fr`
2. **Inner compositor** (Sway, GNOME, etc.) is configured with a **single layout** that matches the outer's **current** layout
3. When user switches layout in Helix UI, outer compositor changes layout and inner compositor is reconfigured
4. `wl_keyboard.modifiers` events from outer correctly reflect the current layout (because both are on the same layout)

### Implementation Plan

#### Phase 1: Add keyboard layout to lobby creation

1. **Wolf API changes** (`wolf/src/moonlight-server/events/events.hpp`):
   ```cpp
   struct KeyboardSettings {
     std::string layout;       // "us", "gb", "fr", etc.
     std::string variant;      // "azerty", "", etc.
     std::string model;        // "pc104", "", etc.
     std::string options;      // "caps:ctrl_nocaps", etc.
   };

   struct CreateLobbyEvent {
     // ... existing fields ...
     KeyboardSettings keyboard_settings;  // NEW
   };
   ```

2. **wayland-display-core changes** (`gst-wayland-display/wayland-display-core/src/lib.rs`):
   ```rust
   pub enum Command {
       // ... existing commands ...
       SetKeyboardLayout(String, String, String, String),  // layout, variant, model, options
   }
   ```

3. **Smithay XKB configuration** (`wayland-display-core/src/comp/mod.rs`):
   - Accept keyboard layout from lobby creation
   - Configure XKB with user's layout
   - Pass layout to inner compositor via environment variable

4. **Inner compositor configuration**:
   - Pass `XKB_DEFAULT_LAYOUT` matching outer's current layout
   - Configure Sway config with single layout (not multi-layout)

#### Phase 2: Runtime layout switching

1. **Wolf API endpoint** (`wolf/src/moonlight-server/api/endpoints.cpp`):
   ```cpp
   void endpoint_SetKeyboardLayout(const HTTPRequest &req, std::shared_ptr<UnixSocket> socket);
   ```

2. **Helix frontend UI**:
   - Add keyboard layout selector to session panel
   - Call Wolf API to switch layout

3. **Runtime sync mechanism**:
   - When outer compositor layout changes, update inner compositor's `XKB_DEFAULT_LAYOUT`
   - For Sway: call `swaymsg input type:keyboard xkb_switch_layout next` or configure single layout
   - For GNOME/KDE: Use their respective D-Bus APIs

### Key Files to Modify

1. **Wolf events**: `wolf/src/moonlight-server/events/events.hpp` - Add KeyboardSettings
2. **Wolf API**: `wolf/src/moonlight-server/api/endpoints.cpp` - Add layout switch endpoint
3. **wayland-display-core lib**: `gst-wayland-display/wayland-display-core/src/lib.rs` - Add Command::SetKeyboardLayout
4. **wayland-display-core comp**: `gst-wayland-display/wayland-display-core/src/comp/mod.rs` - Handle layout in XKB config
5. **Helix API**: `helix/api/pkg/external-agent/wolf_executor.go` - Pass keyboard layout to Wolf
6. **Helix frontend**: Add layout selector component

### Current State

Currently, Wolf does NOT have keyboard layout options in:
- `CreateLobbyEvent` - No keyboard settings
- `VideoSettings` - Only resolution/refresh rate
- `ClientSettings` - Only controller/mouse settings

The outer compositor (`wayland-display-core`) has XKB config but it's hardcoded:
```rust
let xkb_config = XkbConfig {
    layout: "us,gb,fr",
    options: Some("caps:ctrl_nocaps".into()),
    ..XkbConfig::default()
};
```

### Why Single Layout on Inner Compositor?

The key insight is that when the outer compositor sends `wl_keyboard.modifiers` with `group=0`, and the inner compositor also has `group=0` as its only layout, there's no layout mismatch and no reset occurs.

The layout *interpretation* (which physical key produces which character) happens in the outer compositor's XKB handling. The inner compositor just needs to apply that interpretation to its clients.

---

## ARCHITECTURE ANALYSIS: How Wayland Keyboard Protocol Works

### Question: Does Wayland send raw keycodes or translated characters?

**Answer: RAW KEYCODES (evdev scancodes)**

The `wl_keyboard.key` event sends raw keycodes, NOT translated characters. Evidence from Smithay (`wayland/seat/keyboard.rs:219`):

```rust
kbd.key(serial.into(), time, key.raw_code().raw() - 8, state.into())
//                           ^^^^^^^^^^^^^^^^^^^^^^^^
//                           Raw keycode (evdev scancode)
```

### How XKB Translation Works in Nested Compositors

```
Moonlight client (Windows scancode 0x1E = 'A' key)
    ↓
Wolf converts to evdev scancode (30)
    ↓
Smithay outer compositor:
  - Receives keycode 30
  - Applies its XKB keymap internally for its own purposes
  - Sends wl_keyboard.key(keycode=30) to Sway  ← RAW KEYCODE!
  - Also sends wl_keyboard.modifiers() with layout group  ← THE BUG SOURCE
    ↓
Sway inner compositor:
  - Receives keycode 30
  - Applies its OWN XKB keymap to translate 30 → 'a' or 'é' or 'q'
  - Sends wl_keyboard.key(keycode=30) to applications
    ↓
Application (e.g., Firefox):
  - Receives keycode 30
  - Uses the keymap that Sway sent during wl_keyboard.enter to translate to character
```

**Key insight:** BOTH compositors have their own XKB keymaps and apply them independently. The outer compositor does NOT translate keycodes to characters before sending to inner compositor.

### Why This Matters for the Layout Reset Bug

1. The outer compositor sends `wl_keyboard.modifiers` with `layout_effective` (its current layout index)
2. Sway's wlroots receives this and calls `xkb_state_update_mask(..., group=layout_effective)`
3. This OVERRIDES whatever layout Sway's user selected
4. The keycodes themselves are unchanged - only the group/layout index is problematic

### Multi-User Layout Architecture

**Current architecture:** ONE Wayland display per LOBBY

From `lobbies.cpp:113-116`:
```cpp
auto wl_state = virtual_display::create_wayland_display(...);
lobby->wayland_display->store(wl_state);
```

From `lobbies.cpp:235-238`:
```cpp
// ALL sessions in a lobby share the same Wayland display
auto wl_state = lobby->wayland_display->load();
session->keyboard->emplace(virtual_display::WaylandKeyboard(wl_state));
```

**This means all users in the same lobby share the same XKB state and cannot have different layouts.**

### Theoretical Per-User Layout Solution

To support per-user layouts (French user + American user in same lobby), we'd need:

**Option A: Per-session XKB translation**
1. Apply each user's XKB layout in Wolf session layer (before the shared Wayland display)
2. Convert keycode → keysym/character at session level
3. Use text input protocol (zwp_text_input_v3) to inject text
4. **Limitation:** Only works for text entry, breaks shortcuts/gaming

**Option B: Per-connection Wayland display**
1. Create one Wayland display per connection instead of per lobby
2. Each user has their own isolated XKB state
3. **Problem:** Would require significant architectural changes to video/audio muxing

**Option C: Accept shared layout (RECOMMENDED)**
1. Lobby has ONE keyboard layout configured at creation time
2. All users in that lobby use the same layout
3. Layout is controlled via Helix UI when creating/managing the lobby
4. Inner compositor (Sway) is configured with SINGLE matching layout

For now, Option C is the pragmatic solution. Multi-user different layouts could be explored later.

---

## IMPLEMENTED SOLUTION: Patched wlroots

### Summary

We patched wlroots to add a `WLR_IGNORE_PARENT_KEYBOARD_LAYOUT` environment variable that, when set to "1", causes the Wayland backend to preserve its own keyboard layout group instead of synchronizing to the parent compositor's layout.

### The Fix

**File modified:** `backend/wayland/seat.c`

**Original code:**
```c
static void keyboard_handle_modifiers(void *data, struct wl_keyboard *wl_keyboard,
		uint32_t serial, uint32_t mods_depressed, uint32_t mods_latched,
		uint32_t mods_locked, uint32_t group) {
	struct wlr_keyboard *keyboard = data;
	wlr_keyboard_notify_modifiers(keyboard, mods_depressed, mods_latched,
		mods_locked, group);  // ← Uses parent's group, causing layout reset
}
```

**Patched code:**
```c
static bool should_ignore_parent_layout(void) {
	static int cached = -1;
	if (cached == -1) {
		const char *env = getenv("WLR_IGNORE_PARENT_KEYBOARD_LAYOUT");
		cached = (env != NULL && strcmp(env, "1") == 0) ? 1 : 0;
	}
	return cached == 1;
}

static void keyboard_handle_modifiers(void *data, struct wl_keyboard *wl_keyboard,
		uint32_t serial, uint32_t mods_depressed, uint32_t mods_latched,
		uint32_t mods_locked, uint32_t group) {
	struct wlr_keyboard *keyboard = data;

	/* Preserve our own layout group if configured */
	if (should_ignore_parent_layout()) {
		group = keyboard->modifiers.group;  // ← Use OUR group, not parent's
	}

	wlr_keyboard_notify_modifiers(keyboard, mods_depressed, mods_latched,
		mods_locked, group);
}
```

### Implementation Details

1. **Patch file:** `wolf/wlroots-patches/0001-wayland-backend-ignore-parent-layout-group.patch`

2. **Dockerfile changes:** `Dockerfile.sway-helix` now includes:
   - A `wlroots-build` stage that clones wlroots 0.17.4 and applies the patch
   - Builds only the wlroots shared library (not Sway)
   - Copies the patched `libwlroots.so.12` to replace the stock version
   - Sets `WLR_IGNORE_PARENT_KEYBOARD_LAYOUT=1` environment variable

3. **Why wlroots is a shared library:**
   - Sway dynamically links to `libwlroots.so.12`
   - We only need to replace this single library file
   - Stock Sway binary from Ubuntu packages continues to work

### How It Works

1. User starts a sandbox session with Sway
2. Sway runs with `WLR_IGNORE_PARENT_KEYBOARD_LAYOUT=1` environment variable
3. When user switches keyboard layout in Sway (e.g., to French)
4. When user presses Shift (or any modifier key):
   - Outer compositor (Smithay) sends `wl_keyboard.modifiers` with `group=0`
   - Patched wlroots receives this but **ignores the group value**
   - Sway's layout remains on French (or whatever user selected)

### Testing

To verify the fix works:
1. Start a sandbox session
2. Switch Sway to a non-US layout (e.g., French via Waybar buttons)
3. Press Shift or any modifier key
4. **Expected:** Layout should remain on French
5. **Before fix:** Layout would reset to US

### Maintenance Notes

- The patch is specific to wlroots 0.17.x (used by Sway 1.9)
- When Ubuntu upgrades to a newer wlroots, the patch may need to be updated
- The patch is minimal (~15 lines) and unlikely to conflict with upstream changes
- The environment variable approach allows disabling the fix if needed
