# Implementation Tasks: Fix Safari Copy from Remote Desktop to System Clipboard

- [ ] Reproduce the bug in Safari on macOS: select text in the remote desktop, press Cmd+C, paste into a native macOS app, confirm previous clipboard content is pasted (not the new selection) while the UI shows "Copied"
- [ ] Refactor the Cmd+C / Ctrl+C branch in `handleKeyDown` in `frontend/src/components/external-agent/DesktopStreamViewer.tsx` (around lines 3905–4049) so the local clipboard write is initiated synchronously inside the user-gesture handler
- [ ] Use `navigator.clipboard.write([new ClipboardItem({ "text/plain": promise })])` where `promise` resolves to a `Blob` after the 300 ms wait and the `v1ExternalAgentsClipboardDetail` API call returns
- [ ] Feature-detect `ClipboardItem` and `navigator.clipboard.write`; fall back to the existing `clipboardWriteText` path when missing, and keep the iframe / postMessage branch (`isInIframe`) unchanged
- [ ] Fix the misleading toast: only show green "Copied" when the local clipboard write actually succeeds; show the `"error"` toast variant (e.g. "Copied on remote — local clipboard blocked") when the local write fails or is rejected by the browser
- [ ] Remove the 2.7-second background polling `useEffect` (around lines 2664–2740) that calls `v1ExternalAgentsClipboardDetail` and writes the result to local clipboard — it can't work on Safari and races with explicit Cmd+C on slow networks
- [ ] Delete the `lastRemoteClipboardHash` ref (line ~403) and all references to it (lines ~2692, ~2723, ~4034)
- [ ] Delete the `lastAutoSyncedText` ref (line ~404) and the paste-flow short-circuit at ~line 4091 that reads it (`if (text === lastAutoSyncedText.current) { ... skip upload ... }`) — with auto-sync gone, paste always uploads then sends Ctrl+V
- [ ] Preserve all remaining `[Clipboard]` and `[Paste DEBUG]` console logging for future debugging (only remove the auto-sync-specific log lines)
- [ ] Verify in Safari on macOS that Cmd+C in the remote desktop now lands the selected text in the macOS system clipboard (paste into Notes / TextEdit)
- [ ] Verify Safari toast goes red/orange when clipboard permission is denied or the local write fails
- [ ] Regression test Chrome on macOS: Cmd+C still works identically to today
- [ ] Regression test the macOS Wails app (iframe): postMessage clipboard bridge unchanged
- [ ] Regression test paste flows on Safari (paste button, native `paste` DOM event, keyboard fallback) — none should regress
- [ ] Confirm the auto-sync removal does NOT regress Cmd+C → ⌘V (it shouldn't — that path now does the local write itself); the only intentional regression is "copy on remote via right-click then ⌘V locally without pressing Cmd+C," which we accept
- [ ] `cd frontend && yarn build` succeeds with no new TypeScript errors and no unused-symbol warnings from the deleted refs
- [ ] Open PR against `helixml/helix` with a concise description and the manual test results, calling out the auto-sync removal explicitly
