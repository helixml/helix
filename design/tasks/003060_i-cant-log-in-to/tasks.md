# Implementation Tasks: Fix Shifted Characters From Mobile Keyboards on the Streamed Desktop

## Transport (frontend)

- [~] Add `sendKeysym(isDown, keysym, modifiers)` to `WebSocketStream` in `frontend/src/lib/helix-stream/stream/websocket-stream.ts`, framed `[2][isDown:1][modifiers:1][keysym:4 BE]` via `sendInputMessage(WsMessageType.KeyboardInput, …)`, mirroring `sendKey`
- [~] Add `sendKeysymTap(keysym, modifiers)` to `WebSocketStream`, framed `[3][modifiers:1][keysym:4 BE]`
- [~] Patch both methods onto `StreamInput` in `patchInputMethods()` **and** in `getInput()` (both patch sites must stay in sync)
- [ ] Verify in the browser console that a keysym message actually leaves the socket (today it is silently swallowed by `trySendChannel` on a null RTCDataChannel)

## Character routing (frontend)

- [ ] In `keysym.ts`, extend `shouldUseKeysym()` to also select keysym mode when `event.key` is a single printable character whose shift level contradicts the reported modifier state (shifted-level character with `event.shiftKey === false`), keeping the existing "empty `event.code`" rule
- [ ] Confirm physical-keyboard events (Shift+2 with `shiftKey: true`) still take the evdev path unchanged
- [ ] In `DesktopStreamViewer.tsx`, replace the three `input.sendText(char)` calls in the hidden `<input>` (`onBeforeInput`, `onInput`, `onCompositionEnd`) with the keysym path
- [ ] Delete `StreamInput.sendText` and its subType-1 framing from `input.ts` (no backend handler exists for subType 1)
- [ ] Audit the `keydown` / `beforeinput` / `input` / `compositionend` handlers so one keystroke produces exactly one wire message — no duplicate and no dropped characters

## Backend shift level (Go)

- [ ] Change `keysymToEvdev` in `api/pkg/desktop/ws_input.go` to return `(evdevCode int, needsShift bool)`; mark `A`–`Z` and `! @ # $ % ^ & * ( ) _ + { } | : " < > ? ~` as needing Shift; update both call sites
- [ ] In `handleWSKeyboardKeysym` and `handleWSKeyboardKeysymTap`, on the `s.waylandInput` branch only, resolve shift via `XKBKeysymNeedsShift(keysym)` first and the static result second, merge it with the message's modifiers byte, and press/release `KEY_LEFTSHIFT` around the key in the existing reverse order
- [ ] Leave the GNOME D-Bus `NotifyKeyboardKeysym` branch untouched — it resolves the level itself; add a comment saying so
- [ ] Add a comment to `handleWSKeyboardKeycode` explaining why `data[2]` (modifiers) stays unused: physical keyboards send real Shift events, and synthesizing a second one risks the stuck-modifier bug in `design/2025-11-25-keyboard-modifier-stuck-analysis.md`

## Tests

- [ ] Frontend vitest: `shouldUseKeysym` / `convertToKeysym` for `@` (`code: "Digit2"`, `shiftKey: false`) → keysym `0x40`; for desktop Shift+2 → evdev path; plus `A`, `_`, `:`, `a`, `2`, `Enter`, `Backspace`
- [ ] Go test: `keysymToEvdev` shift flags for `'@'`, `'A'`, `'a'`, `'_'`, `'2'`
- [ ] Go test: `handleWSKeyboardKeysymTap` emits Shift-down → key-down → key-up → Shift-up in order on the Wayland path, and leaves no modifier latched

## Verification

- [ ] `cd frontend && yarn build` and `yarn test`; `go build ./pkg/desktop/`
- [ ] End-to-end in the inner Helix (`http://localhost:8080`): start a spec task for a live desktop, open the stream viewer, dispatch synthesized iOS-shaped `KeyboardEvent`s at the stream container via chrome-devtools, and confirm `user@example.com` appears verbatim on the remote desktop
- [ ] Regression on a physical keyboard in the same session: Shift+2 → `@`, Ctrl+C / Ctrl+V, and a held key auto-repeating
- [ ] Chrome DevTools mobile emulation pass over the hidden-input focus flow
- [ ] State explicitly in the PR which legs ran and that real-iPhone confirmation is outstanding (no device available in the dev environment)

## Documentation

- [ ] Add `design/2026-09-04-mobile-keyboard-shifted-characters.md` to the helix repo recording the four stacked root causes, the "patch both `patchInputMethods()` and `getInput()`" gotcha, and the GNOME-vs-Wayland path split
