# Mobile keyboards typed `@` as `2` on the streamed desktop

Reported as "I can't log in to LinkedIn on my phone" — typing `@` in the email field
produced `2`. Every Shift-level character was affected: `! # $ % ^ & * ( ) _ + { } | : " < >
? ~` and uppercase letters.

## Why

A hardware keyboard sends a separate `ShiftLeft` keydown that the remote compositor latches,
so replaying the *physical key position* (`Digit2` → `KEY_2`) reproduces `@`. A mobile
on-screen keyboard sends **one event for the symbol and no Shift event at all**: `key="@"`,
`code="Digit2"`, `shiftKey=false`. Replaying the position types the unshifted `2`.

Four defects stacked. The first is what users hit; the rest are why the code that was
supposed to handle this never ran.

1. **`StreamInput.sendKeyEvent` maps position + reported modifiers.** `convertToEvdevKey`
   reads `event.code`, `convertToEvdevModifiers` reads `event.shiftKey` (false).
2. **`handleWSKeyboardKeycode` ignores the modifiers byte** (`data[2]`). Even a client that
   set SHIFT would not get Shift pressed.
3. **The keysym path existed on both ends and was never transmitted.** `sendKeysym` /
   `sendKeysymTap` (subTypes 2/3) were implemented in `input.ts` and handled in
   `ws_input.go`, but `WebSocketStream.patchInputMethods()` and its duplicate in
   `getInput()` only patched `sendKey`, the mouse methods and `sendTouch`. The keysym
   methods kept writing to an `RTCDataChannel` that is `null` in WebSocket mode, and
   `trySendChannel` drops those **with no log line**. The whole feature was dead in the only
   transport the product uses (`useEvdevCodes: true`).
4. **The Wayland fallback dropped the shift level.** `keysymToEvdev` maps `'@' → KEY_2`,
   `'A' → KEY_A` with no Shift. `XKBKeysymNeedsShift` in `xkb.go` was written for exactly
   this and was **called from nowhere in the repo**.

Also dead: subType 1 (`sendText`) had no backend handler at all, yet `DesktopStreamViewer`
called it from three hidden-input handlers.

## Fix

On a virtual keyboard, send the character the user meant, not the key position they didn't
press — which is what a keysym is.

- `keysym.ts`: `shouldUseKeysym` also fires on a **shift-level mismatch** — a shifted
  character reported with `shiftKey=false`, which a real US-layout keyboard cannot produce.
  Desktop behaviour is unchanged because there `shiftKey` is true.
  **CapsLock is carved out**: it legitimately yields `A` with `shiftKey=false`, and since the
  remote already tracks the CapsLock we forwarded, rewriting those as Shift+A would type
  *lowercase* there. Symbols still fire — CapsLock does not produce `@`.
- `input.ts`: mismatch characters send a keysym **tap** and the keyup is swallowed. A virtual
  keyboard may never deliver `keyup`, which would latch the key down on the remote (see
  `2025-11-25-keyboard-modifier-stuck-analysis.md`). Auto-repeat still works — each repeated
  keydown emits another tap.
- `websocket-stream.ts`: added `sendKeysym`/`sendKeysymTap` and patched them in.
  `getInput()` now **delegates to `patchInputMethods()`** instead of duplicating the list.
- `ws_input.go`: `keysymNeedsShift` + `resolveKeysym`; the Wayland fallback holds Shift
  around shift-level keysyms. The **GNOME D-Bus branch is deliberately untouched** —
  `NotifyKeyboardKeysym` resolves the level from the compositor's keymap, so synthesizing
  Shift there would double-apply it.
- Replaced `sendText` with `sendTextAsKeysyms` for swipe/IME text.

The subType-0 modifiers byte still goes unused, now with a comment saying why: physical
keyboards send real Shift events, and a second synthesized one risks the stuck-modifier bug.
One mechanism per problem.

## Traps for the next person

- **Adding a `StreamInput.sendX` means patching it onto the transport in
  `websocket-stream.ts`.** Otherwise it writes to a null RTCDataChannel and is dropped
  silently — no error, no log. This is the easiest way to write input code in this repo that
  looks correct and does nothing. `getInput()` delegating to `patchInputMethods()` now makes
  one list instead of two; keep it that way.
- **A test that stubs the send methods cannot catch a "this code never runs" bug.**
  `websocket-stream.keyboard.test.ts` drives the real `WebSocketStream` against a fake
  `globalThis.WebSocket` and asserts the literal wire bytes. A matching Go test
  (`ws_input_keysym_test.go`) pins the same 7-byte frame, so the two ends cannot drift.
- **GNOME and Sway take different branches in every `ws_input.go` handler**
  (`s.conn && s.rdSessionPath` → D-Bus, else `s.waylandInput`). A fix in one is not a fix in
  the other, and the D-Bus path silently does more for you.
- **Virtual keyboards send no modifier keys.** Any code reconstructing a character from
  `event.shiftKey` is wrong on mobile. Prefer `event.key` over `event.code` when the source
  is a soft keyboard.
- The viewer already logs every hidden-input `keydown` and `beforeinput` — remote-debug a
  real device against those logs before theorising about event shapes.

## Verification status

Green: full frontend suite (1122), the three new stream test files, `go test ./pkg/desktop/`,
`yarn build`, `go build`, `go vet`.

**Not verified on a real device.** The fix assumes iOS reports `key: "@"`, `code: "Digit2"`,
`shiftKey: false`. If it instead reports an empty `code`, the pre-existing "no code" branch
routes to keysym and the fix still applies — both shapes now reach the transport, which
neither did before. A live streamed-desktop test was not possible in the dev environment:
`./stack build-zed release` is OOM-killed under the 24 GiB container cgroup, so there is no
`helix-ubuntu` image and the sandbox crash-loops.
