# Requirements: Fix Shifted Characters From Mobile Keyboards on the Streamed Desktop

## Background

A user driving a Helix streamed desktop (`DesktopStreamViewer`) from an iPhone cannot log
in to LinkedIn: typing `@` in the email field produces `2` on the remote desktop. On a
physical keyboard `@` is Shift+2; the mobile keyboard emits the symbol without any Shift
key press, and Helix forwards the *physical key position* (`Digit2` → evdev `KEY_2`)
without the Shift level, so the remote desktop types `2`.

This is not specific to `@`. Every character that lives on a shifted level of the US
layout is affected the same way: `! # $ % ^ & * ( ) _ + { } | : " < > ? ~` and — depending
on how the mobile browser reports capitals — uppercase letters. Any password or email
address containing one of these is impossible to type on a phone, which makes the streamed
desktop unusable for exactly the logins users need it for.

## User Stories

### US1 — Type shifted symbols from a mobile keyboard
As a user viewing a streamed desktop on my phone, I want the symbol I tap on the on-screen
keyboard to appear on the remote desktop, so that I can enter email addresses, passwords
and URLs.

**Acceptance criteria**
- Tapping `@` on an iPhone on-screen keyboard (system keyboard and Gboard) inserts `@` in
  the focused field on the remote desktop, not `2`.
- The same holds for `! " # $ % & ' ( ) * + : < > ? _ { | } ~ ^` and for `A`–`Z`.
- Unshifted characters (`a`–`z`, `0`–`9`, `.`, `-`, `/`, space) continue to work unchanged.
- Backspace, Enter, and arrow keys continue to work unchanged.
- A character is delivered exactly once — no duplicated or dropped characters when several
  input paths (`keydown`, `beforeinput`, `input`, `compositionend`) fire for one tap.

### US2 — Log in on a phone
As a user, I want to complete a real login form (email + password containing symbols) on
the streamed desktop from my phone, so that I can use sites like LinkedIn from a session.

**Acceptance criteria**
- Typing `user@example.com` into a form field on the remote desktop from a phone produces
  exactly `user@example.com`.
- Typing a password containing `@`, `!` and an uppercase letter produces the exact string
  (verify against a field where the text is visible, e.g. a text editor or the URL bar).

### US3 — No regression for physical keyboards
As a desktop user, I want Shift+2 and every other shortcut to keep working exactly as
today.

**Acceptance criteria**
- Shift+2 on a physical keyboard still produces `@`.
- Modifier chords (Ctrl+C, Ctrl+V, Cmd+Tab handling, Alt combos) are unchanged.
- No new stuck-modifier behaviour (see `design/2025-11-25-keyboard-modifier-stuck-analysis.md`
  in the helix repo — a known past failure mode in this area).

### US4 — Shifted characters work on non-GNOME desktops
As a user on a session whose compositor does not expose GNOME's D-Bus
`NotifyKeyboardKeysym` (Sway/wlroots experimental desktops), I want shifted characters to
arrive correctly through the Wayland virtual-keyboard fallback.

**Acceptance criteria**
- On the Wayland fallback path, a keysym that requires Shift on the active layout is
  delivered with Shift pressed and released around the key.
- Uppercase letters arrive uppercase on that path.

## Out of Scope

- Dead keys / accented character composition (already documented as unsupported in
  `keysym.ts`).
- Non-US keyboard layouts beyond what xkbcommon already resolves.
- Emoji and other non-BMP input.
- Any redesign of the mobile stream UI (toolbar, keyboard toggle, zoom behaviour).

## Open Questions

1. **Exact iOS event shape.** The analysis below is derived from the code paths, not from
   a captured iPhone trace. The design assumes that on iOS the on-screen keyboard fires a
   `keydown` whose `event.key` is the real character (`"@"`) while `event.shiftKey` is
   `false` and `event.code` is either empty or the US-layout position (`"Digit2"`). If the
   real device instead fires only `beforeinput`/`input` with no `keydown`, the fix still
   works (both paths are covered by this design), but the *primary* path differs. **Please
   confirm, or capture the console output** — the viewer already logs
   `[DesktopStreamViewer] Hidden input keydown: <key> <code>` and
   `beforeinput: <inputType> <data>` for exactly this purpose.
2. **Which browser?** The report says "iPhone Google keyboard". Gboard on iOS runs inside
   whatever browser is used (Safari or Chrome-for-iOS, both WebKit). Is the browser Safari
   or Chrome? Behaviour of synthesized key events differs slightly between them.
3. **Which desktop image?** GNOME (`helix-ubuntu`, the production desktop) and Sway/wlroots
   (experimental) take different backend code paths and have *different* bugs here. This
   spec fixes both, but confirming which one the user hit tells us which fix actually
   closes the report.
4. **Uppercase letters** — are they already broken for the user too, or only symbols?
   That distinguishes "shift level dropped everywhere" from "symbols only" and would
   confirm the root cause without a device trace.
5. **Verification without an iPhone.** No physical iOS device is available in the dev
   environment. The plan is to verify with synthesized events matching the assumed iOS
   event shape plus Chrome DevTools mobile emulation, which exercises the code path but
   *not* the real WebKit event quirk. Assumed acceptable; final confirmation needs the
   reporter to retest on their phone.
