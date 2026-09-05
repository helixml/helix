# Implementation Tasks: Fix Shifted Characters From Mobile Keyboards on the Streamed Desktop

## Transport (frontend)

- [x] Add `sendKeysym(isDown, keysym, modifiers)` to `WebSocketStream` in `frontend/src/lib/helix-stream/stream/websocket-stream.ts`, framed `[2][isDown:1][modifiers:1][keysym:4 BE]` via `sendInputMessage(WsMessageType.KeyboardInput, …)`, mirroring `sendKey`
- [x] Add `sendKeysymTap(keysym, modifiers)` to `WebSocketStream`, framed `[3][modifiers:1][keysym:4 BE]`
- [x] Patch both methods onto `StreamInput` in `patchInputMethods()`; `getInput()` now delegates to it instead of duplicating the list (removes the drift that caused this bug)
- [ ] Verify in the browser console that a keysym message actually leaves the socket (today it is silently swallowed by `trySendChannel` on a null RTCDataChannel)

## Character routing (frontend)

- [x] In `keysym.ts`, extend `shouldUseKeysym()` to also select keysym mode when `event.key` is a single printable character whose shift level contradicts the reported modifier state (shifted-level character with `event.shiftKey === false`), keeping the existing "empty `event.code`" rule
- [x] Confirm physical-keyboard events (Shift+2 with `shiftKey: true`) still take the evdev path unchanged
- [x] In `DesktopStreamViewer.tsx`, replace the three `input.sendText(char)` calls in the hidden `<input>` (`onBeforeInput`, `onInput`, `onCompositionEnd`) with the keysym path
- [x] Replace `StreamInput.sendText` (subType 1, no backend handler) with `sendTextAsKeysyms`, used by the hidden input and `onBeforeInput`
- [x] Audit the `keydown` / `beforeinput` / `input` / `compositionend` handlers — no duplication: the container handler guards on `document.activeElement !== container`, so it and the hidden input are mutually exclusive
- [x] Send a keysym **tap** (not a down/up pair) for mismatch characters and swallow the keyup, so a virtual keyboard that never sends keyup cannot latch a key down on the remote

## Backend shift level (Go)

- [x] Add `keysymNeedsShift` + `resolveKeysym` in `api/pkg/desktop/ws_input.go` marking `A`–`Z` and `! @ # $ % ^ & * ( ) _ + { } | : " < > ? ~` as needing Shift (chose a pure predicate over threading a tuple through ~130 `return KEY_X` statements — less churn and nothing to drift)
- [x] In `handleWSKeyboardKeysym` and `handleWSKeyboardKeysymTap`, on the `s.waylandInput` branch only, resolve shift via `XKBKeysymNeedsShift(keysym)` first and the static result second, merge it with the message's modifiers byte, and press/release `KEY_LEFTSHIFT` around the key in the existing reverse order
- [x] Leave the GNOME D-Bus `NotifyKeyboardKeysym` branch untouched — it resolves the level itself; add a comment saying so
- [x] Add a comment to `handleWSKeyboardKeycode` explaining why `data[2]` (modifiers) stays unused: physical keyboards send real Shift events, and synthesizing a second one risks the stuck-modifier bug in `design/2025-11-25-keyboard-modifier-stuck-analysis.md`

## Tests

- [x] Frontend vitest: `shouldUseKeysym` / `convertToKeysym` for `@` (`code: "Digit2"`, `shiftKey: false`) → keysym `0x40`; for desktop Shift+2 → evdev path; plus `A`, `_`, `:`, `a`, `2`, `Enter`, `Backspace`
- [x] Go test: `keysymNeedsShift` / `resolveKeysym` shift flags for `'@'`, `'A'`, `'a'`, `'_'`, `'2'`
- [x] ~~Go test for `handleWSKeyboardKeysymTap` emission order~~ **dropped**: `s.waylandInput` is a concrete cgo-bound `*WaylandInput`, so this needs an interface indirection in the input hot path purely for a test. Not worth the risk for 6 lines of straight-line code; the shift *decision* is unit-tested and ordering is covered end-to-end
- [x] Frontend vitest: wire-level routing (`input.keyboard.test.ts`) — `@` emits one keysym tap and never the Digit2 position; keyup swallowed; Shift+2 unchanged

## Verification

- [x] `cd frontend && yarn build` and `yarn test` (1119 passed); `go build ./pkg/desktop/` + `go test ./pkg/desktop/`
- [~] End-to-end in the inner Helix (`http://localhost:8080`): start a spec task for a live desktop, open the stream viewer, dispatch synthesized iOS-shaped `KeyboardEvent`s at the stream container via chrome-devtools, and confirm `user@example.com` appears verbatim on the remote desktop
- [ ] Regression on a physical keyboard in the same session: Shift+2 → `@`, Ctrl+C / Ctrl+V, and a held key auto-repeating
- [ ] Chrome DevTools mobile emulation pass over the hidden-input focus flow
- [ ] State explicitly in the PR which legs ran and that real-iPhone confirmation is outstanding (no device available in the dev environment)

## Documentation

- [ ] Add `design/2026-09-04-mobile-keyboard-shifted-characters.md` to the helix repo recording the four stacked root causes, the "patch both `patchInputMethods()` and `getInput()`" gotcha, and the GNOME-vs-Wayland path split
