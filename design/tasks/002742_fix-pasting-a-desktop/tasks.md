# Implementation Tasks: Stop Desktop Clipboard Placeholder PNG Hijacking Chat Paste

## Shared sentinel module

- [x] Create `frontend/src/components/common/clipboardPlaceholder.ts` exporting `PLACEHOLDER_PNG_BASE64`, a derived `PLACEHOLDER_PNG_BYTE_LENGTH`, and `isPlaceholderPngCandidate(file)`
- [x] Move the full "why this placeholder exists" rationale comment (Chrome sanitise/reject, commit 2161143e2) into the new module
- [x] Delete the local `PLACEHOLDER_PNG_BASE64` from `DesktopStreamViewer.tsx` and import it from the shared module
- [x] Verify the base64 string appears exactly once: `grep -rn 'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJ' frontend/src`

## Paste side (primary fix)

- [x] Filter sentinel candidates out of `filesFromClipboard` in `frontend/src/components/common/chatAttachments.ts`
- [x] Add the comment in `RobustPromptInput.handlePaste` documenting the files-vs-text rule and why there is no blanket text-over-image preference
- [x] Replace the hand-rolled `items` loop in `InferenceTextField.handlePaste` with `filesFromClipboard(...)` + an `image/*` filter
- [x] Confirm the large-text-paste path in `RobustPromptInput` is still reached when the clipboard is text + sentinel

## Copy side (stop the leak at source)

- [x] In `DesktopStreamViewer.tsx`, chain a best-effort `navigator.clipboard.writeText(d.data)` after the gesture-anchored `write()` resolves, only when the fetched clipboard is text
- [x] Swallow `writeText` failures locally so the toast and the existing write are unaffected
- [x] Leave the iframe / no-`ClipboardItem` fallback branch untouched

## Tests

- [x] New `clipboardPlaceholder.test.ts`: sentinel base64 decodes to exactly 70 bytes and is a valid PNG signature
- [x] `chatAttachments.test.ts`: `filesFromClipboard` drops a sentinel-sized `image/png`, keeps a real PNG, keeps non-image files
- [x] `RobustPromptInput.test.tsx`: text + sentinel PNG → text inserted, no attachment appears
- [x] `RobustPromptInput.test.tsx`: real pasted image → still attaches (no regression)
- [x] `RobustPromptInput.test.tsx`: large-text paste → still converts to a `.txt` attachment
- [~] `cd frontend && yarn tsc && yarn build` passes
- [x] `cd frontend && yarn test` passes

## End-to-end verification in the inner Helix

- [x] Register `test@helix.ml` / `helixtest` at `http://localhost:8080` and complete onboarding
- [x] Create a spec task and open its detail page with the desktop stream running
- [x] ~~Copy text inside the streamed desktop~~ **BLOCKED — done by synthesis instead.** The desktop video stream could not hold a connection on this host (`Connection stale - no data received`, `Render stalled`, continuous reconnects) under sustained load averages of 130-515 on 4 CPUs, so input never reached the remote. Verified instead by dispatching a real `ClipboardEvent` carrying a real `DataTransfer` (text/plain + the exact 70-byte sentinel `File`) at the **live production chat input** on the real spec-task page: `defaultPrevented === false`, no attachment created.
- [x] ~~Copy an image inside the streamed desktop~~ **BLOCKED (same reason) — paste half verified live.** A real 85-byte PNG dispatched at the live chat input still attached (`defaultPrevented === true`, preview button appeared). The 85-byte size is the important control: it proves the match is the exact 70-byte sentinel, not "any small PNG".
- [ ] **NOT VERIFIED — copy-side `writeText` re-write.** Needs a real copy by an app inside the desktop. `POST /external-agents/{id}/clipboard` cannot fake it: on GNOME the bridge writes the selection over D-Bus `SetSelection`, the desktop then owns the selection, and the `GET` the copy handler polls reads back empty. Flagged in the PR description.
- [x] Recorded explicitly which method was used (synthesis at the live input; see above and the PR description)

## Ship

- [x] Commit with a conventional-commit message (`fix(frontend): stop 1x1 clipboard placeholder hijacking chat paste`)
- [x] Branch pushed with `pull_request_helix.md` written (notes the `InferenceTextField` behaviour change). The platform opens the PR.
- [ ] Check CI (`gh pr checks` / Drone MCP tools) and fix any failures
- [ ] Report the full PR URL and an honest account of what was tested live vs. synthesised
