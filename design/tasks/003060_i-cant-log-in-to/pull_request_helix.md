# Fix shifted characters typed from mobile keyboards on the streamed desktop

## Summary

Typing `@` on a phone produced `2` on the streamed desktop, which made it impossible to
enter an email address — the reported symptom was being unable to log in to LinkedIn from
an iPhone. Every character on the Shift level was affected the same way: `! # $ % ^ & * ( )
_ + { } | : " < > ? ~` and uppercase letters.

A hardware keyboard sends a separate `ShiftLeft` keydown that the remote compositor
latches, so replaying the physical key position works. A mobile on-screen keyboard sends
**one event for the symbol and no Shift event at all** — `@` arrives as `key="@"`,
`code="Digit2"`, `shiftKey=false` — so replaying the position typed the unshifted `2`.

Four defects stacked up. The first is what users hit; the rest are why the code that was
supposed to handle this never ran:

1. **Position-based mapping loses the shift level.** `convertToEvdevKey` maps `Digit2` →
   `KEY_2` and `convertToEvdevModifiers` reads `event.shiftKey`, which is `false`.
2. **The backend ignores the modifiers byte** on the keycode path.
3. **The correct path existed but was never transmitted.** `sendKeysym`/`sendKeysymTap`
   were implemented on both ends (subTypes 2/3), but `WebSocketStream` only patched
   `sendKey`, the mouse methods and `sendTouch` onto the transport. The keysym methods kept
   writing to a `null` RTCDataChannel, and `trySendChannel` drops those **with no log
   line** — the whole keysym feature was dead in the transport the product actually uses.
4. **The Wayland fallback dropped the shift level too.** `XKBKeysymNeedsShift` was written
   for exactly this and was called from nowhere in the repo.

The fix: on a virtual keyboard, send the character the user meant rather than the key
position they didn't press. Keysyms already express that.

## Changes

**Frontend**
- `keysym.ts`: `shouldUseKeysym` also selects keysym mode on a *shift-level mismatch* — a
  shifted character reported with `shiftKey=false`, which a real US-layout keyboard cannot
  produce. Physical-keyboard behaviour is bit-for-bit unchanged. CapsLock is carved out,
  since it legitimately yields `A` with `shiftKey=false` and the remote already tracks the
  CapsLock we forwarded.
- `input.ts`: mismatch characters send a keysym **tap** and the keyup is swallowed, so a
  virtual keyboard that never delivers `keyup` cannot latch a key down on the remote
  (cf. `design/2025-11-25-keyboard-modifier-stuck-analysis.md`). Auto-repeat still works.
- `websocket-stream.ts`: added `sendKeysym`/`sendKeysymTap` and patched them onto
  `StreamInput`. `getInput()` now delegates to `patchInputMethods()` instead of keeping a
  second hand-maintained copy of the patch list — that duplication is how the keysym
  methods came to be unpatched in the first place.
- Replaced the dead `sendText` (subType 1 has **no backend handler**) with
  `sendTextAsKeysyms`, used by the hidden input and `onBeforeInput` for swipe/IME text.

**Backend**
- `ws_input.go`: added `keysymNeedsShift` + `resolveKeysym`, and the Wayland fallback now
  holds Shift around shift-level keysyms. The GNOME D-Bus path is untouched — it resolves
  the level from the compositor's own keymap, so synthesizing Shift there would
  double-apply it.
- Documented why the keycode path deliberately ignores its modifiers byte.

## Testing

Ran and green:

- Full frontend suite: **1119 passed, 1 skipped, 0 failed** — no regressions.
- 31 new/existing tests in `lib/helix-stream/stream/`, including
  `websocket-stream.keyboard.test.ts`, which drives the **real `WebSocketStream`** against a
  fake socket and asserts the literal bytes `[0x10, 3, 0, 0, 0, 0, 0x40]`. This one matters:
  the original bug was invisible to any test that stubs the send methods, because the defect
  was that those methods were never called over the transport.
- A Go test pins the same 7-byte frame from the backend side, so the two ends cannot drift.
- Physical Shift+2 is asserted to still emit `[0x10, 0, 1, 1, 0, 3]` (KEY_2 + SHIFT).
- `yarn build`, `go build`, `go vet`, `go test ./pkg/desktop/`.

**NOT tested: the live streamed desktop.** This environment cannot provide one. The session
startup script's `./stack build-zed release` was OOM-killed (24 GiB cgroup, ~19 GiB already
resident) and `set -e` aborted it before `build-ubuntu`/`stack start`; without that image
`helix-sandbox-nvidia-1` crash-loops with `Aborting sandbox boot: required production
desktop 'ubuntu' is not available`. I brought the rest of the stack up by hand and it is
healthy (registered, onboarded, project created), but there is no desktop session to type
into.

**Outstanding: confirmation on a real iPhone.** The fix is driven by the assumed iOS event
shape (`key: "@"`, `code: "Digit2"`, `shiftKey: false`). If iOS instead reports an empty
`event.code`, the pre-existing "no code" branch routes to keysym and the fix still applies —
both shapes are now wired to the transport, which neither was before. The viewer already
logs `[DesktopStreamViewer] Hidden input keydown: <key> <code>`, so a remote-debug session
against a phone will confirm which path fires.
