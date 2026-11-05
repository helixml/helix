# Stuck Caps Lock / Ctrl on shared (multi-viewer) desktops

**Date:** 2026-10-11
**Status:** Fixed (see PR for this branch)

## Symptom

Several people viewed one spec-task desktop. Caps lock got stuck on, including
for a viewer whose OS has CapsLock disabled. Separately, Ctrl got stuck, so
every viewer's typing came out Ctrl-modified.

## Root causes

### 1. The CapsLock→Ctrl remap never existed

`startup-app.sh` / `dconf-settings.ini` (and the Sway config) set
`caps:ctrl_nocaps`, which is not an xkeyboard-config option. Every GNOME
desktop logged `xkbcommon: ERROR: Unrecognized RMLVO option "caps:ctrl_nocaps"
was ignored`, so CapsLock was a real lock toggle. Removed; `altwin:ctrl_win`
stays. Fixing it to `ctrl:nocaps` would be worse: on macOS, caps-on sends
keydown with no keyup until caps turns off, so Ctrl would stay held for every
viewer.

### 2. Lock state was forwarded as edges into one shared keyboard

Every viewer's input goes through desktop-bridge into the same Mutter
RemoteDesktop session (`Server.rdSessionPath`) and therefore the same virtual
input device and the same seat xkb state. There is exactly one remote lock.
The browser forwarded CapsLock as evdev 58 down/up, and nothing reconciled the
remote lock with any browser's lock:

- macOS Chrome/Safari fire CapsLock `keydown` only when caps turns on and
  `keyup` only when it turns off. The remote toggles on press, so it flips on
  every other local toggle, and KEY_CAPSLOCK stays *held* on the virtual device
  between the two.
- Toggling caps while the stream lacks focus is never seen.
- A viewer with CapsLock disabled in the OS can never send a toggle.
- `resetInputState` (window focus, canvas mouseenter, double-Escape) sent a
  fake CapsLock keyup, releasing a KEY_CAPSLOCK another viewer was holding.

### 3. Key edges with no guaranteed release (stuck Ctrl)

- The browser listens for keys on the stream container. When focus leaves while
  a key is held (Cmd/Ctrl+Tab, clicking the chat box, an OS shortcut), the keyup
  goes elsewhere. The only resets were window focus, canvas mouseenter and
  double-Escape.
- desktop-bridge had no per-connection state. A viewer that closed the tab or
  lost network with Ctrl held left Ctrl held for everyone.

  Reproduced live on the old desktop-bridge: one connection sent Ctrl down and
  was closed; the next connection typed `b` + Enter into a terminal running
  `cat > /tmp/out.txt`, and the file got `^B`.

## Mutter semantics this relies on

From `src/backends/native/meta-virtual-input-device-native.c` and
`meta-seat-impl.c` (GNOME/mutter main):

- Each virtual device keeps a per-key press count. A press of a key already down
  and a release of a key already up are **ignored and the count is restored**
  (`key_count < 0 || key_count > 1` → "Received multiple virtual … (ignoring)").
  So the device is idempotent per key: a key is simply down or up.
- The seat additionally drops presses/releases that are not seat-wide changes.
- Consequence: when two viewers both hold Ctrl, the first one to release it
  releases it for both. There is no per-viewer ref-counting in Mutter.
- `NotifyKeyboardKeysym` (`notify_keyval_in_impl`) picks the first keycode and
  shift level that produces the keysym from the keymap, and presses Shift_L for
  level 1. **It does not consult the lock state.**

## Explaining "Abi uppercase while Luke lowercase"

A single shared lock cannot make two viewers' keycode events diverge. The
keysym path can, because of the last bullet above. Verified live on the old
desktop-bridge with a terminal running `cat`:

| Remote caps | keycode `a` | keysym `a` | keysym `A` |
|---|---|---|---|
| unlocked | `a` | `a` | `A` |
| locked   | `A` | `A` | **`a`** |

The frontend sends a keysym instead of a keycode when
`hasShiftLevelMismatch(event)` is true: an uppercase letter with
`shiftKey=false` **and** `getModifierState('CapsLock')=false` (the heuristic for
virtual keyboards). Such an event goes out as a keysym tap of `A`. Mutter
presses Shift+a for it, so it types `A` while the remote lock is off. Another
viewer typing on the keycode path in the same instant types `a`. That matches
the report exactly: remote caps unlocked, Luke (keycode path) lowercase, Abi
(keysym `A`) uppercase.

What remains unverified is why Abi's browser would report an uppercase letter
with the CapsLock modifier state false. Candidates:

- macOS Chrome reporting a stale CapsLock modifier state after the lock was
  toggled while the window was unfocused, which is the reported sequence;
- a leftover Shift-level remap or input method on Abi's machine.

We had no macOS browser available to confirm either. The fix does not depend on
which one it was.

Also ruled out:

- The cursor RemoteDesktop session that `session.go` creates is never used for
  input. All keyboard input goes through `rdSessionPath`, so every viewer
  shares one device.
- XKB `LockMods` (lock on press, unlock on the release that follows a press made
  while locked) combined with a held KEY_CAPSLOCK plus other viewers' fake
  keyups makes the shared lock flip at surprising moments. That explains
  "stuck", but it is still one state for everyone, so it cannot explain
  simultaneous divergence.

## Fix

### Client (`input.ts`, `evdev-keys.ts`, `DesktopStreamViewer.tsx`, `websocket-stream.ts`)

- CapsLock key events (`code` or `key` = `CapsLock`) are never forwarded.
- `EvdevModifiers.CAPS_LOCK = 1 << 4` is set in the modifiers byte from
  `event.getModifierState('CapsLock')` on every key event.
- `StreamInput` tracks the evdev codes (and keysyms) it has sent as down.
  `releaseAllKeys()` sends keyups for exactly that set; a keyup for a key never
  sent as down is dropped.
- Held keys are released on container `focusout`, `visibilitychange`→hidden,
  `pagehide`, window focus, double-Escape and `WebSocketStream.close()`. On an
  unexpected socket close the set is just forgotten, because desktop-bridge
  releases them.
- `resetInputState` no longer sends blind keyups for a hard-coded modifier list
  (including the fake CapsLock keyup). Canvas mouseenter now only releases mouse
  buttons: a key held while the pointer enters (Ctrl+click) is still physically
  down.

### desktop-bridge (`keyboard_state.go`, `ws_input.go`, `ws_stream.go`)

- `Server.keyboard` (`sharedKeyboard`) tracks `capsLocked` (false at start) and,
  for each evdev key, how many connections hold it. Each input connection (stream
  WebSocket, `/ws/input`) has a `keyboardClient` with its held keys. All
  injection happens under one mutex, so decisions and injection order cannot
  diverge between viewers.
- Keycode press: if the caps bit differs from `capsLocked`, tap KEY_CAPSLOCK
  (GNOME), or set the locked modifier mask on the virtual keyboard (Sway: the
  zwp_virtual_keyboard client owns the modifier state, and a tap would be
  overwritten by the next modifiers event). Then inject the key. Releases never
  sync. Raw KEY_CAPSLOCK from old clients is dropped, so tracking stays exact.
  Old clients send caps bit 0, so caps stays off for them.
- Keysym press (subType 2) and keysym tap (subType 3): sync the lock **off**.
  A keysym names the exact character, and Mutter ignores the lock when it
  resolves one (table above).
- **Held-key policy:** a key goes down remotely when the first connection
  presses it and up when the last connection holding it releases it or
  disconnects. A repeat press from the same connection (browser auto-repeat) is
  not forwarded on GNOME, which repeats held keys itself and would ignore it
  anyway. It is forwarded on Sway, where compositor repeat is disabled. A
  release from a connection that never pressed the key is dropped. Keysym taps
  skip pressing/releasing a modifier that some viewer holds.
- When a connection closes, every key it still holds is released, unless
  another viewer also holds it.

Only the caps-lock bit of the modifiers byte is used. The rest stays ignored,
for the reasons in `design/2025-11-25-keyboard-modifier-stuck-analysis.md`.

## Known limits

- If desktop-bridge restarts while the compositor keeps running with caps
  locked, `capsLocked` starts false and the first press with the bit set taps
  the lock off. The next press re-syncs it. This is self-correcting.
- Keysym holds (subType 2, iPad) are tracked per connection but are not shared
  across connections the way keycode holds are.

## Live verification (inner Helix, desktop image `helix-ubuntu:d92631`)

Two browser tabs (A, B) viewed one spec-task desktop. Output was a ghostty
terminal running `cat > /tmp/out.txt`. Keyboard events were dispatched to the
stream container, so they went through the real `DesktopStreamViewer` handlers.

| Test | Result |
|---|---|
| Container log | 0 × `Unrecognized RMLVO option`; `xkb-options` = `['altwin:ctrl_win']` |
| A types with CapsLock state on, B with it off, A again | `ABC` / `abc` / `DEF`; wire modifiers byte 16 from A, 0 from B |
| macOS sequence: CapsLock keydown only, later keyup only | nothing sent; next typing `ghi` lowercase |
| Old-client raw KEY_CAPSLOCK tap, then `j` | `j` (raw lock edge dropped) |
| After a caps-on press locked the remote: keysym `A`, `a`, taps `A`, `a` | `KAaLAa` (the old bridge typed `a` for keysym `A` here) |
| Abi/Luke case: A sends `X` with no Shift and no CapsLock state (keysym tap), B types `y`, A `Z` with caps, B `w` | `XyZw` |
| Ctrl+U in terminal | kills the line |
| B holds Ctrl, focus leaves container | exactly one keyup (Ctrl) sent; A then types plain |
| B holds Ctrl, socket dropped with no keyup | bridge logs `releasing key held by disconnected viewer evdev=29`; A types plain (old bridge: `^B`) |
| A and B both hold Ctrl, B releases and disconnects | A's Ctrl+U still works (Ctrl stayed down for A) |
| B holds Ctrl, tab closed | A types plain |
