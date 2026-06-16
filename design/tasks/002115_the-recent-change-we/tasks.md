# Implementation Tasks: Fix Chrome Copy-Paste Regression in Remote Desktop Clipboard Sync

- [x] Root-cause confirmed: zero-byte `image/png` Blob in the gesture-anchored `ClipboardItem` makes Chrome's image sanitizer reject the whole `navigator.clipboard.write()`, so the valid `text/plain` never lands (confirmed via code + 002043's own design.md stating the unused MIME is a zero-byte Blob)
- [x] Add a `PLACEHOLDER_PNG_BASE64` constant (1×1 transparent PNG) near the clipboard helpers in `DesktopStreamViewer.tsx`
- [x] Validate the placeholder PNG string before committing: regenerate it or decode and confirm valid signature, all chunk CRC-32s, trailing `IEND`, and a clean decode (verified from source: 70 bytes, IHDR/IDAT/IEND, CRCs OK)
- [x] In the copy handler's `imageBlobPromise`, replace the zero-byte `image/png` fallback with a valid PNG built from `PLACEHOLDER_PNG_BASE64` via `base64ToBytes`
- [x] In `clipboardReadAny()` `navigator.clipboard.read()` branch, prefer non-empty `text/plain` over `image/png`
- [x] In `clipboardReadAny()` iframe `helix-clipboard-response` branch, mirror the same precedence (non-empty `text` before `base64`)
- [x] Run frontend type-check + build: `yarn tsc` passes (exit 0); `vite build` compiles all 21,652 modules and builds clean to a writable outDir
- [x] Add per-repo PR description (`pull_request_helix.md`), noting Safari manual verification is deferred to the reviewer (no Safari in CI)
- [x] Merge latest `origin/main` into the feature branch (already up to date) and push `feature/002115-fix-chrome-copy-paste`

## Notes

- Reproduction in the inner Helix requires a live desktop container + real OS
  clipboard + remote text selection, which cannot be driven reliably through
  the devtools automation (clipboard is OS-level, not DOM-level). Root cause
  is instead established from the code and the original 002043 design. Manual
  Chrome/Safari end-to-end verification is deferred to the PR reviewer.
- `frontend/dist` is owned by root in this environment, so the default
  `yarn build` fails at the final copy-into-dist step (`EACCES mkdir
  dist/external-libs`). This is unrelated to the change — the build compiles
  fine; verified with `vite build --outDir /tmp/...` (exit 0).
