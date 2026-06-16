# Implementation Tasks: Fix Chrome Copy-Paste Regression in Remote Desktop Clipboard Sync

- [ ] Reproduce the regression on Chrome in the inner Helix: copy text from a remote desktop session, confirm the red "Copied on remote — local clipboard blocked" toast and the `[Clipboard] local write blocked:` console error
- [ ] Add a `PLACEHOLDER_PNG_BASE64` constant (1×1 transparent PNG) near the clipboard helpers in `DesktopStreamViewer.tsx`
- [ ] In the copy handler's `imageBlobPromise`, replace the zero-byte `image/png` fallback with a valid PNG built from `PLACEHOLDER_PNG_BASE64` via `base64ToBytes`
- [ ] In `clipboardReadAny()` `navigator.clipboard.read()` branch, prefer non-empty `text/plain` over `image/png`
- [ ] In `clipboardReadAny()` iframe `helix-clipboard-response` branch, mirror the same precedence (non-empty `text` before `base64`)
- [ ] Run `cd frontend && yarn build` and confirm it passes (TS strict)
- [ ] Verify on Chrome: copy text → green "Copied text" toast → paste into a local field works → paste back into remote pastes text
- [ ] Verify on Chrome: copy a real image → green "Copied image" toast → image copy/paste still works
- [ ] Note in the PR description that Safari manual verification is deferred to the reviewer (no Safari in CI)
- [ ] Open the PR against `helixml/helix` and confirm CI is green
