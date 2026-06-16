# fix(frontend): restore remote-desktop clipboard copy on Chrome

## Summary

PR #2607 (Safari copy-from-remote fix, spec task 002043) regressed copy on
Chrome — the browser most users are on. Copying from the remote desktop no
longer reached the local system clipboard on Chrome, so paste-back and paste
into local apps were broken (red "Copied on remote — local clipboard blocked"
toast).

**Root cause:** the gesture-anchored `ClipboardItem` declares both
`text/plain` and `image/png` up front (it doesn't know which type the remote
produced until the async fetch resolves) and filled the non-matching MIME with
a **zero-byte Blob**. Chrome decode/sanitizes every image written to the
clipboard; a zero-byte Blob isn't a valid PNG, so Chrome **rejects the entire
`navigator.clipboard.write()`** — discarding the valid `text/plain` with it.
Safari tolerated the empty representation, so the regression shipped unnoticed.

## Changes

All in `frontend/src/components/external-agent/DesktopStreamViewer.tsx`:

- Add a verified `PLACEHOLDER_PNG_BASE64` constant (a real 1×1 transparent PNG,
  70 bytes, valid IHDR/IDAT/IEND with correct CRCs).
- Copy handler: the `image/png` fallback (when the copy was text, not an image)
  is now this valid placeholder PNG instead of a zero-byte Blob, so Chrome
  accepts the write and the `text/plain` lands.
- `clipboardReadAny()`: prefer **non-empty `text/plain` over `image/png`** in
  both the `navigator.clipboard.read()` branch and the iframe
  `helix-clipboard-response` branch. A text copy now also leaves the 1×1
  placeholder image on the clipboard; without this, paste-back would paste the
  transparent pixel instead of the text. Genuine image copies carry an empty
  `text/plain`, so they still resolve to the image.

The synchronous dual-MIME `ClipboardItem` architecture (the Safari fix) is
unchanged — only the contents of the representations are corrected.

## Trade-off

Copying *text* and pasting into a local *image-only* app yields a 1×1
transparent pixel. This is strictly better than 002043's accepted "0-byte
representation" cost and affects a rare flow.

## Verification

- `yarn tsc` passes (TS strict, exit 0); `vite build` compiles all 21,652
  modules cleanly.
- The embedded PNG is validated from source (signature, per-chunk CRC-32,
  trailing `IEND`, clean decode).
- Manual Chrome/Safari end-to-end clipboard verification is deferred to the
  reviewer: the inner-Helix automation cannot drive the OS-level clipboard +
  live remote desktop required to exercise this path. Expected after the fix:
  copy text on Chrome → green "Copied text" toast → paste into a local field
  and back into the remote both yield the text; image copy still works; Safari
  copy continues to work.
