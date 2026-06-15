# Implementation Tasks: Fix Safari Copy from Remote Desktop to System Clipboard

## Frontend (DesktopStreamViewer.tsx)

- [ ] Reproduce the bug in Safari on macOS: select text in the remote desktop, press Cmd+C, paste into a native macOS app, confirm previous clipboard content is pasted (not the new selection) while the UI shows "Copied"
- [ ] Refactor the Cmd+C / Ctrl+C branch in `handleKeyDown` (around lines 3905–4049) so the local clipboard write is initiated synchronously inside the user-gesture handler
- [ ] Construct a single `ClipboardItem` that declares **both** `text/plain` and `image/png` synchronously, with each MIME's `Promise<Blob>` resolving to the real Blob if the fetched type matches or a 0-byte Blob otherwise
- [ ] Replace the hard-coded `setTimeout(300)` with bounded adaptive polling: snapshot the pre-copy clipboard hash (in parallel with forwarding Ctrl+C), then poll `v1ExternalAgentsClipboardDetail` every ~30 ms for up to ~500 ms, return as soon as the hash differs
- [ ] Use `ClipboardItem.supports("image/png")` to feature-detect image support; on browsers that lack it, drop the image representation and write text-only
- [ ] Replace `clipboardWriteText(text)` with `clipboardWrite({ mime, text? | base64? })` so the dispatcher can carry images as well as text. Update all call sites
- [ ] Extend `clipboardReadText` to `clipboardReadAny` returning `{ mime: "text/plain" | "image/png" | "empty", text?, base64? }`; feed image results into the paste-upload path so paste-image-into-iframe works
- [ ] Feature-detect `ClipboardItem` and `navigator.clipboard.write`; fall back to the existing text-only path when missing
- [ ] Fix the misleading toast: show green "Copied text" / "Copied image" only when the local clipboard write actually succeeds; error variant when the local write fails or is rejected
- [ ] Remove the 2.7-second background polling `useEffect` (lines 2664–2740)
- [ ] Delete the `lastRemoteClipboardHash` ref and all references
- [ ] Delete the `lastAutoSyncedText` ref and the paste-flow short-circuit at ~line 4091 that reads it
- [ ] Preserve existing `[Clipboard]` / `[Paste DEBUG]` logging; add a `[Clipboard] poll resolved in NNms` log inside the new poll loop

## macOS Wails app (for-mac/)

- [~] Add `for-mac/clipboard_darwin.go` with cgo + AppKit (mirror `cursor_darwin.go` pattern). Implement `(a *App) SetClipboardImagePNG(base64PNG string) error` writing `NSPasteboardTypePNG`, and `(a *App) GetClipboardImagePNG() (string, error)` reading the same type and returning base64 or `""`
- [ ] Run `wails dev` (or `wails build`) to regenerate `for-mac/frontend/wailsjs/go/main/App.d.ts` and `.js`; include the regenerated files in the PR
- [ ] Extend the `handleMessage` event handler in `for-mac/frontend/src/App.tsx`:
  - [ ] Accept `{ type: "helix-clipboard-write", mime: "image/png", base64: string }` and call `SetClipboardImagePNG`
  - [ ] For `helix-clipboard-read`, query `GetClipboardImagePNG` first; if non-empty respond with `mime: "image/png"`, else fall back to `ClipboardGetText` and respond with `mime: "text/plain"`, else `mime: "empty"`
  - [ ] Keep accepting the old `{ type, text }` write shape (treat as `mime: "text/plain"`) for back-compat

## Manual testing

- [ ] Safari on macOS — text copy lands in macOS clipboard, pastes into Notes / TextEdit
- [ ] Safari on macOS — image copy lands in macOS clipboard, pastes into Preview via File → New from Clipboard
- [ ] Safari on macOS — accepted UX trade-off: text copy → paste into Preview (image-only) gets nothing; retry into text destination works
- [ ] Safari on macOS — toast goes red/orange when clipboard permission is denied
- [ ] Chrome on macOS — text and image copy still work identically
- [ ] macOS Wails app — text copy and paste still work via postMessage bridge
- [ ] macOS Wails app — **image copy** (new): Cmd+C on a remote image, paste into Preview / Messages outside the Wails app
- [ ] macOS Wails app — **image paste** (new): copy an image on the Mac (`Cmd+Shift+Ctrl+4`), focus the desktop stream inside the Wails window, Cmd+V, verify image lands on remote clipboard and pastes into a remote image app
- [ ] Paste flows on Safari (paste button, native `paste` DOM event, keyboard fallback) — none should regress
- [ ] Auto-sync removal does NOT regress Cmd+C → ⌘V
- [ ] Poll loop typically resolves in 30–90 ms on a healthy local desktop (console logs)
- [ ] Backward-compat: old Wails app + new iframe → image bridge fails gracefully, text still works
- [ ] Backward-compat: new Wails app + old iframe → text still works via old message shape

## Build & release

- [ ] `cd frontend && yarn build` succeeds with no new TypeScript errors and no unused-symbol warnings
- [ ] `for-mac` cgo build succeeds on macOS (Apple Silicon + Intel)
- [ ] Open PR against `helixml/helix` with a concise description and manual test results, calling out: auto-sync removal, ClipboardItem-with-Promise + dual MIME, bounded polling, and the new iframe image bridge
