# Implementation Tasks: Fix Safari Copy from Remote Desktop to System Clipboard

- [ ] Reproduce the bug in Safari on macOS: select text in the remote desktop, press Cmd+C, paste into a native macOS app, confirm previous clipboard content is pasted (not the new selection) while the UI shows "Copied"
- [ ] Refactor the Cmd+C / Ctrl+C branch in `handleKeyDown` in `frontend/src/components/external-agent/DesktopStreamViewer.tsx` (around lines 3905–4049) so the local clipboard write is initiated synchronously inside the user-gesture handler
- [ ] Use `navigator.clipboard.write([new ClipboardItem({ "text/plain": promise })])` where `promise` resolves to a `Blob` after the 300 ms wait and the `v1ExternalAgentsClipboardDetail` API call returns
- [ ] Feature-detect `ClipboardItem` and `navigator.clipboard.write`; fall back to the existing `clipboardWriteText` path when missing, and keep the iframe / postMessage branch (`isInIframe`) unchanged
- [ ] Fix the misleading toast: only show green "Copied" when the local clipboard write actually succeeds; show the `"error"` toast variant (e.g. "Copied on remote — local clipboard blocked") when the local write fails or is rejected by the browser
- [ ] Update `lastRemoteClipboardHash.current` / `lastAutoSyncedText.current` from the new success path so the 2-second auto-sync loop doesn't immediately re-write the same value
- [ ] Preserve all existing `[Clipboard]` and `[Paste DEBUG]` console logging for future debugging
- [ ] Verify in Safari on macOS that Cmd+C in the remote desktop now lands the selected text in the macOS system clipboard (paste into Notes / TextEdit)
- [ ] Verify Safari toast goes red/orange when clipboard permission is denied or the local write fails
- [ ] Regression test Chrome on macOS: Cmd+C still works identically to today
- [ ] Regression test the macOS Wails app (iframe): postMessage clipboard bridge unchanged
- [ ] Regression test paste flows on Safari (paste button, native `paste` DOM event, keyboard fallback) — none should regress
- [ ] Regression test the 2-second clipboard auto-sync in Chrome — remote → local sync still works
- [ ] `cd frontend && yarn build` succeeds with no new TypeScript errors
- [ ] Open PR against `helixml/helix` with a concise description and the manual test results
