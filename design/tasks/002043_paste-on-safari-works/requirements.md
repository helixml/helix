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
2. The "Copied" green toast is only shown when the local system clipboard
   was actually written. If only the remote copy succeeded but the local
   write failed, the toast text and colour must reflect the partial
   failure (e.g. "Copied on remote — local clipboard blocked by browser",
   warning/error style).
3. Chrome on macOS and Linux continues to behave as it does today.
4. The macOS Wails app (iframe + postMessage bridge) continues to work
   as it does today.
5. No regression in the existing paste flows (Safari paste button, Chrome
   keyboard paste, native `paste` DOM event, iframe paste).
6. The 2-second auto-sync loop (`v1ExternalAgentsClipboardDetail` polling)
   that copies the remote clipboard back to local in the background is
   not weakened — it must still update the local clipboard when the
   browser permits.

## Out of scope

- Changing Safari's "Paste" button affordance — that is Safari's native
  security UI and we accept it.
- Image clipboard handling on Safari — text is the priority. If image
  copy on Safari is straightforward to fix with the same mechanism it
  may be included, otherwise it stays as-is.
- Any backend / Go-side changes to the
  `v1ExternalAgentsClipboardDetail` endpoint. The bug is purely
  frontend.
