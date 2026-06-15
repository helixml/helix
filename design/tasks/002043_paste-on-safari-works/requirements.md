# Requirements: Fix Safari Copy from Remote Desktop to System Clipboard

## Background

In the remote desktop stream viewer, paste (Cmd+V) on Safari already works
acceptably — Safari surfaces its native "Paste" affordance which the user
explicitly clicks. That is Safari's security model and is fine.

However, **copy (Cmd+C) is broken on Safari**: the UI shows the green
"Copied" toast, but **nothing is actually written to the macOS system
clipboard**. Paste from Safari into another macOS app yields the previous
clipboard content, not what was just "copied". The same flow works in
Chrome.

## Root cause (suspected — to be confirmed during implementation)

`DesktopStreamViewer.tsx` handles Cmd+C by:

1. Forwarding Ctrl+C to the remote desktop (good).
2. Waiting `setTimeout(300ms)` for the remote clipboard to update.
3. Fetching the new clipboard from the API.
4. Calling `navigator.clipboard.writeText(text)`.

Safari requires `navigator.clipboard.writeText()` to be invoked
**synchronously within the same user-gesture task** that originated the
keystroke. After a 300 ms `setTimeout`, the gesture is gone — Safari
rejects/ignores the write. The catch block then runs
`showClipboardToast("Copied", "success")` regardless (see comment
`// Still show success - the remote copy likely worked even if sync
failed`), so the user is told the copy succeeded when locally it did not.

## User stories

- **As a Safari user of the remote desktop**, when I press Cmd+C inside the
  desktop stream, I want the selected text to land in my macOS clipboard
  so I can paste it into any local app — the same as Chrome.
- **As a Safari user**, when the local clipboard write genuinely fails, I
  want the toast to tell me so, instead of being told "Copied" when
  nothing was copied.
- **As a Chrome user**, I want existing copy behaviour to keep working
  exactly as it does today (no regression).
- **As a user inside the macOS Wails app (iframe / WKWebView)**, I want
  the existing postMessage clipboard bridge to keep working unchanged.

## Acceptance criteria

1. In Safari on macOS, selecting text in the remote desktop and pressing
   Cmd+C results in that text being available in the macOS system
   clipboard (verified by pasting into a native macOS app such as Notes
   or TextEdit).
2. In Safari on macOS, copying an image on the remote desktop and
   pressing Cmd+C results in that image being available in the macOS
   system clipboard (verified by pasting into Preview → File → New
   from Clipboard, or into a chat app).
3. The success toast is only shown when the local system clipboard was
   actually written, and reflects what was copied ("Copied text" or
   "Copied image"). If only the remote copy succeeded but the local
   write failed, the toast text and colour must reflect the partial
   failure (e.g. "Copied on remote — local clipboard blocked by browser",
   warning/error style).
4. Chrome on macOS and Linux continues to behave as it does today, for
   both text and image copy.
5. The macOS Wails app (iframe + postMessage bridge) gains image
   clipboard support in **both directions** as part of this change:
   - Copying an image on the remote desktop with Cmd+C lands the
     image on the macOS system clipboard (verified by pasting into
     Preview / a chat app from inside *and* outside the Wails app).
   - Pasting an image from the macOS system clipboard with Cmd+V
     transfers the image to the remote desktop's clipboard (verified
     by pasting into an image-capable remote app).
   The existing text postMessage bridge keeps working unchanged.
6. No regression in the existing paste flows (Safari paste button, Chrome
   keyboard paste, native `paste` DOM event, iframe paste).
7. The 2.7-second background clipboard polling loop (`syncClipboard`
   inside the `useEffect` near line 2664) is **removed** as part of this
   change. Rationale: it cannot work on Safari at all (no user gesture
   → silent fail), it races with explicit Cmd+C on slow networks (stale
   poll response overwriting fresh user copy), and it adds a periodic
   HTTP round-trip per active session for a Chrome-only convenience
   feature. Removing it also lets us delete the `lastRemoteClipboardHash`
   and `lastAutoSyncedText` refs and the related paste-flow branch that
   exists only to compensate for the auto-sync side-effects. See the
   design doc for the trade-off in full.

## Accepted UX regression from removing auto-sync

If a user copies text **inside** the remote desktop through a path that
does not flow through our Cmd+C interceptor (e.g. right-click → Copy in
the remote app, or the agent itself copying programmatically), local
clipboard will no longer auto-populate. The user can recover by giving
focus to the desktop stream and pressing Cmd+C explicitly, which routes
through the new gesture-anchored copy path. This was a Chrome-only
side-effect of the polling loop and never worked on Safari.

## Image clipboard (in scope)

Images must also work — Cmd+C of an image on the remote desktop in
Safari must land that image in the macOS system clipboard, pasteable
into Preview / chat apps / image editors. Same gesture-anchored
`clipboard.write()` mechanism, multi-MIME `ClipboardItem`. See
`design.md` decision 4 for the trade-off (paste of a text-copy into
an image-only destination, or vice versa, yields nothing instead of
the right content — user retries with the matching destination).

Image copy/paste also works inside the macOS Wails app via an
extended postMessage bridge backed by a new cgo binding to
NSPasteboard. See `design.md` decision 6 for the protocol.

## Out of scope

- Changing Safari's "Paste" button affordance — that is Safari's native
  security UI and we accept it.
- A server-side "wait for clipboard change" endpoint that would let
  the frontend skip its 500 ms polling loop. The current bounded
  client polling is good enough; the server-side fix is tracked as
  *Future work* in `design.md` and is a sensible follow-up.
- Backend / Go-side changes to the existing
  `v1ExternalAgentsClipboardDetail` endpoint. The bug is purely
  frontend.
- Replacing the polling with a push-based remote-clipboard-changed
  notification over the existing WebSocket. That would re-introduce the
  same Safari-gesture problem and the same race; if we ever want
  remote→local notification, it should be a deliberate separate piece
  of work, not bundled with this fix.
