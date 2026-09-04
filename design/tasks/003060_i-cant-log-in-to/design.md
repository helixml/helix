# Design: Fix Shifted Characters From Mobile Keyboards on the Streamed Desktop

## Where the code lives

All paths relative to `/home/retro/work/helix/`.

| File | Role |
|---|---|
| `frontend/src/components/external-agent/DesktopStreamViewer.tsx` | Stream viewer. Wires `keydown`/`keyup`/`beforeinput` on the container (~L4080–4620) and owns the hidden `<input>` used for the iOS/phone on-screen keyboard (~L4757–4869). |
| `frontend/src/lib/helix-stream/stream/input.ts` | `StreamInput` — turns DOM events into wire messages: `sendKey` (subType 0), `sendText` (1), `sendKeysym` (2), `sendKeysymTap` (3); `onBeforeInput` (L268). |
| `frontend/src/lib/helix-stream/stream/evdev-keys.ts` | `event.code` → evdev keycode; `convertToEvdevModifiers` reads `event.shiftKey`. |
| `frontend/src/lib/helix-stream/stream/keysym.ts` | `event.key` → X11 keysym; `shouldUseKeysym()` gates keysym mode on "`event.code` is empty". |
| `frontend/src/lib/helix-stream/stream/websocket-stream.ts` | WebSocket transport. `patchInputMethods()` (L333) and `getInput()` (L2277) monkey-patch `StreamInput`'s send methods onto the WebSocket. |
| `api/pkg/desktop/ws_input.go` | Backend. `handleWSKeyboard` dispatches subType 0/2/3 → GNOME D-Bus `NotifyKeyboardKeycode`/`NotifyKeyboardKeysym`, else Wayland virtual keyboard. Static `keysymToEvdev` at L755. |
| `api/pkg/desktop/xkb.go` | xkbcommon layout-aware keysym table. `XKBKeysymToEvdev` (L296) and `XKBKeysymNeedsShift` (L315). |

Prior art worth reading before touching this: `design/2025-11-26-keyboard-input-deep-dive.md`,
`design/2025-11-25-keyboard-modifier-stuck-analysis.md`, `design/2026-01-08-direct-input-websocket.md`.

## Root cause

Four defects stack up. The first is what the user hit; the rest are why the code that was
*supposed* to handle this never runs.

**1. Physical-position mapping loses the shift level (the user-visible bug).**
`StreamInput.sendKeyEvent` prefers evdev mode: `convertToEvdevKey(event)` maps
`event.code === "Digit2"` → `KEY_2`, and `convertToEvdevModifiers(event)` reads
`event.shiftKey`. A hardware keyboard also delivers a separate `ShiftLeft` keydown, which
the compositor latches, so `@` works. A mobile on-screen keyboard delivers **one event for
the symbol and no Shift event at all**, with `shiftKey === false`. The remote compositor
receives a bare `KEY_2` press → types `2`. Same mechanism turns `A` into `a`, `_` into `-`,
`:` into `;`.

**2. The backend ignores the modifiers byte on the keycode path.**
`handleWSKeyboardKeycode` (`ws_input.go:133`) has `// modifiers := data[2] // Currently unused`.
So even if the frontend *had* set the SHIFT bit, nothing would press Shift.

**3. The correct path exists but its messages are never transmitted.**
`onBeforeInput` → `sendKeysymTap` and the keysym branch of `sendKeyEvent` → `sendKeysym`
are exactly the right mechanism (a keysym names the *character*, and GNOME's
`NotifyKeyboardKeysym` resolves the level itself). Both are implemented on the frontend and
handled on the backend (subTypes 2 and 3). **But `WebSocketStream.patchInputMethods()` and
`getInput()` only patch `sendKey`, the mouse methods and `sendTouch`.** `sendKeysym`,
`sendKeysymTap` and `sendText` still call `trySendChannel(this.keyboard, …)` against an
`RTCDataChannel` that is `null` in WebSocket mode — `trySendChannel` returns silently. The
whole keysym feature is dead code in the transport the product actually uses
(`useEvdevCodes: true`, `websocket-stream.ts:322`).

**4. The Wayland fallback drops the shift level too.**
`handleWSKeyboardKeysym`/`Tap` call `XKBKeysymToEvdev` (or static `keysymToEvdev`) and press
the bare keycode. `keysymToEvdev` maps `'@' → KEY_2`, `'A' → KEY_A`, `'_' → KEY_MINUS` with
no Shift. `XKBKeysymNeedsShift` exists in `xkb.go` and returns exactly the missing bit — and
is **called from nowhere in the repo**. This only affects non-GNOME compositors (Sway,
wlroots); GNOME's D-Bus keysym call handles the level itself.

Also noted, lower severity: subType `1` (`sendText`) has **no backend handler at all**
(`handleWSKeyboard` only switches on 0/2/3, logging "Unknown keyboard subType"), yet the
hidden `<input>` in `DesktopStreamViewer` calls `input.sendText(char)` in three places
(`onBeforeInput`, `onInput`, `onCompositionEnd`). And `StreamInput.sendText` never calls
`this.buffer.reset()` before writing, unlike `sendKeysym`/`sendKeysymTap`. It is dead in
both directions.

## Approach

**Principle: on a virtual keyboard, send the character the user meant, not the key position
they didn't press.** Keysyms already express that. The fix is to make the keysym path
actually reach the backend, choose it for virtual-keyboard input, and make the one backend
path that ignores shift level stop ignoring it. Per the helix `CLAUDE.md` rule (*NO
FALLBACKS — one approach, fix properly, no dead code paths*), the dead `sendText`/subType-1
path is removed rather than left as a third alternative.

### 1. Transport: make keysym messages transmittable (`websocket-stream.ts`)

Add `sendKeysym(isDown, keysym, modifiers)` and `sendKeysymTap(keysym, modifiers)` to
`WebSocketStream`, framed exactly as the backend already parses:

- subType 2: `[2][isDown:1][modifiers:1][keysym:4 BE]` (7 bytes)
- subType 3: `[3][modifiers:1][keysym:4 BE]` (6 bytes)

sent via `sendInputMessage(WsMessageType.KeyboardInput, …)`, mirroring `sendKey`
(`websocket-stream.ts:1700`). Patch both into `StreamInput` in `patchInputMethods()` **and**
`getInput()` — the two patch sites are duplicated today and must stay in sync (a good
follow-up is to have `getInput()` just return `this.input` since `patchInputMethods()` runs
in the constructor, but keep this change mechanical).

### 2. Frontend: route virtual-keyboard characters through keysyms

Widen the keysym gate in `keysym.ts`:

```
shouldUseKeysym(event):
  # existing: no usable event.code  →  keysym
  # new:      the event carries a printable single character whose value cannot be
  #           reproduced from event.code + the reported modifier state
```

Concretely, prefer keysym mode when `event.key` is a single printable character **and**
either `event.code` is missing/`Unidentified` (today's rule) or the event came from a
virtual keyboard. Detect the latter conservatively — do **not** switch desktop browsers to
keysym mode. Two signals are available and should be combined:

- `isMobileOrTablet()` from `frontend/src/utils/isMobileOrTablet.ts` (already used across
  the viewer), and
- a "shift level mismatch" check: `event.key` is a shifted-level character (uppercase
  letter or one of the shift-only symbols) while `event.shiftKey === false`. This is
  impossible from a real keyboard on a US layout and is the exact signature of the bug.

Using the mismatch check as the trigger keeps physical-keyboard behaviour bit-for-bit
identical (there `shiftKey` is true, so the check never fires) and also fixes any desktop
browser that mis-reports modifier state.

`onBeforeInput` (`input.ts:268`) needs no logic change once its messages can be sent — it
already does `charToKeysym` + `sendKeysymTap`. It must, however, be reachable: keep the
existing `preventDefault` discipline so a single tap does not produce both a `keydown`-path
keysym and a `beforeinput`-path keysym tap. **Deduplication is the main regression risk of
this change** and needs an explicit rule: if `keydown` handled the character, the
subsequent `beforeinput` for the same character must be suppressed, and vice versa.

Replace the three `input.sendText(char)` calls in the hidden `<input>` handlers with
`input.onBeforeInput(...)` / a keysym tap, and delete `StreamInput.sendText` plus its
subType-1 framing (it has no backend handler at all).

### 3. Backend: honour shift level on the Wayland keysym fallback (`ws_input.go`)

In `handleWSKeyboardKeysym` and `handleWSKeyboardKeysymTap`, on the `s.waylandInput` branch
only, ask whether the keysym needs Shift and wrap the key press:

- Prefer `XKBKeysymNeedsShift(keysym)` (layout-aware, already written and tested against the
  live layout, currently unused).
- When xkbcommon is unavailable, the static fallback must report shift too. Change
  `keysymToEvdev` to return `(evdevCode int, needsShift bool)` — uppercase `A`–`Z`, the
  shift-only symbols `! @ # $ % ^ & * ( ) _ + { } | : " < > ? ~`, and nothing else.
  Update the two call sites.
- Merge the derived Shift with the modifiers byte already in the message, and release it in
  the same reverse order the existing code uses, so a keysym that needs Shift plus an
  explicit Ctrl modifier still works and nothing stays latched.

**The GNOME D-Bus branch is deliberately left alone.** `NotifyKeyboardKeysym` resolves the
level from the compositor's own keymap; synthesizing an extra `XK_Shift_L` around it would
double-apply Shift.

### 4. Explicitly not changing: the subType-0 modifiers byte

`handleWSKeyboardKeycode` will keep ignoring `data[2]`, with a comment explaining why:
physical keyboards already send real `ShiftLeft` press/release events, so synthesizing a
second Shift from the modifiers byte would fight the real one and risk the stuck-modifier
class of bug documented in `design/2025-11-25-keyboard-modifier-stuck-analysis.md`. Virtual
keyboards go through the keysym path instead. One mechanism per problem.

## Alternatives considered

- **Set the SHIFT bit on the frontend and honour it in the keycode path.** Rejected: it
  makes the physical and virtual keyboard paths interact (double Shift, early release), it
  is US-layout-hardcoded on the client, and it re-opens a known stuck-modifier failure mode.
- **Clipboard-paste the text instead of typing it.** Rejected: doesn't work for password
  fields that block paste, and doesn't fix the general typing experience.
- **Send the whole string as text (subType 1) and have the backend type it.** Rejected:
  requires a new backend handler and a second text-injection mechanism alongside keysyms,
  which already do this correctly. The existing subType-1 stub is being deleted, not grown.

## Verification

No physical iPhone is available in the dev environment, so verification is layered:

1. **Unit tests (frontend, vitest — `yarn test`).** `shouldUseKeysym`/`convertToKeysym`
   given the assumed iOS event shape (`key: "@"`, `code: "Digit2"`, `shiftKey: false`) →
   keysym `0x40`; given a real desktop Shift+2 (`shiftKey: true`) → *not* keysym mode,
   unchanged evdev output. Cover `A`, `_`, `:`, `a`, `2`, `Enter`, `Backspace`.
2. **Unit tests (Go).** `keysymToEvdev` returns `(KEY_2, true)` for `'@'`, `(KEY_A, true)`
   for `'A'`, `(KEY_A, false)` for `'a'`; `handleWSKeyboardKeysymTap` emits
   Shift-down → key-down → key-up → Shift-up in order on the Wayland path.
3. **End-to-end in the inner Helix** (`http://localhost:8080`, credentials in the helix
   `CLAUDE.md`): start a spec task to get a live desktop session, open the stream viewer,
   focus a text field on the remote desktop, and dispatch synthesized `KeyboardEvent`s
   matching the assumed iOS shape at the stream container via
   `mcp__chrome-devtools__evaluate_script`. Assert the remote desktop shows
   `user@example.com`. This is the closest reproduction available without the device, and
   it exercises the real wired-up component, not a DOM harness.
4. **Chrome DevTools mobile emulation** for the touch/hidden-input focus flow. Note in the
   PR that emulation does **not** reproduce WebKit's synthesized-event quirk — it verifies
   the code path, not the trigger.
5. **Regression check on a physical keyboard**: Shift+2, Ctrl+C/Ctrl+V, and a held key
   (auto-repeat) in the same session.

Report honestly which of these ran (the helix `CLAUDE.md` is explicit: never claim
quantified confidence; say "NOT tested: <what/why>" for the real-device leg).

## Learnings for future agents

- **The streamed desktop has four keyboard wire formats** (evdev keycode, text, keysym,
  keysym tap) but the WebSocket transport only ever patched *one* of them onto the
  transport. Whenever you add a `StreamInput.sendX`, you must also add it to
  **both** `patchInputMethods()` and `getInput()` in `websocket-stream.ts`, or it silently
  writes to a null RTCDataChannel and is dropped with no error. `trySendChannel` returns
  early on a null channel — there is no log line. This is the single easiest way to write
  input code in this repo that appears correct and does nothing.
- **`XKBKeysymNeedsShift` was written for exactly this bug and never called.** When
  something looks missing in `api/pkg/desktop`, grep for it before writing it.
- **GNOME vs Sway take different backend paths** in every handler in `ws_input.go`
  (`s.conn && s.rdSessionPath` → D-Bus, else `s.waylandInput`). A fix in one is not a fix
  in the other, and the D-Bus path silently does more for you (keysym level resolution).
- **Virtual keyboards do not send modifier keys.** Any code that reads `event.shiftKey` to
  reconstruct a character is wrong on mobile. Prefer `event.key` (what the user meant) over
  `event.code` (where they'd have pressed it) whenever the source is a soft keyboard.
- The viewer already `console.log`s every hidden-input `keydown`/`beforeinput` — remote-debug
  a real device against those logs before theorising.
