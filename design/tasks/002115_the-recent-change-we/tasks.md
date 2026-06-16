# Implementation Tasks: Fix Chrome Copy-Paste Regression in Remote Desktop Clipboard Sync

- [x] Root-cause confirmed: zero-byte `image/png` Blob in the gesture-anchored `ClipboardItem` makes Chrome's image sanitizer reject the whole `navigator.clipboard.write()`, so the valid `text/plain` never lands (confirmed via code + 002043's own design.md stating the unused MIME is a zero-byte Blob)
- [~] Add a `PLACEHOLDER_PNG_BASE64` constant (1×1 transparent PNG) near the clipboard helpers in `DesktopStreamViewer.tsx`
- [~] Validate the placeholder PNG string before committing: regenerate it or decode and confirm valid signature, all chunk CRC-32s, trailing `IEND`, and a clean decode (a malformed PNG silently re-introduces the Chrome bug)
- [~] In the copy handler's `imageBlobPromise`, replace the zero-byte `image/png` fallback with a valid PNG built from `PLACEHOLDER_PNG_BASE64` via `base64ToBytes`
- [~] In `clipboardReadAny()` `navigator.clipboard.read()` branch, prefer non-empty `text/plain` over `image/png`
- [~] In `clipboardReadAny()` iframe `helix-clipboard-response` branch, mirror the same precedence (non-empty `text` before `base64`)
- [ ] Run `cd frontend && yarn build` and confirm it passes (TS strict)
- [ ] Add per-repo PR description (`pull_request_helix.md`), noting Safari manual verification is deferred to the reviewer (no Safari in CI)
- [ ] Merge latest `origin/main` into the feature branch and push

## Notes

- Reproduction in the inner Helix requires a live desktop container + real OS
  clipboard + remote text selection, which cannot be driven reliably through
  the devtools automation (clipboard is OS-level, not DOM-level). Root cause
  is instead established from the code and the original 002043 design. Manual
  Chrome/Safari end-to-end verification is deferred to the PR reviewer.
