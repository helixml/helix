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

- [~] Register `test@helix.ml` / `helixtest` at `http://localhost:8080` and complete onboarding
- [ ] Create a spec task and open its detail page with the desktop stream running
- [ ] Copy text inside the streamed desktop (Cmd+C/Ctrl+C), paste into the chat input, confirm the text lands and no attachment appears
- [ ] Copy an image inside the streamed desktop, paste into the chat input, confirm it still attaches as an image
- [ ] Verify the copy-side re-write against the live `DesktopStreamViewer` (clipboard holds text only after a text copy)
- [ ] If any step above is genuinely impossible in the sandbox, synthesise the paste event with a real 70-byte sentinel `File` + `text/plain` at the live chat input, and record explicitly which method was used

## Ship

- [ ] Commit with a conventional-commit message (e.g. `fix(frontend): stop 1x1 clipboard placeholder hijacking chat paste`)
- [ ] Open a PR against `helixml/helix`, note the `InferenceTextField` behaviour change (now honours `clipboardData.files`) in the description
- [ ] Check CI (`gh pr checks` / Drone MCP tools) and fix any failures
- [ ] Report the full PR URL and an honest account of what was tested live vs. synthesised
