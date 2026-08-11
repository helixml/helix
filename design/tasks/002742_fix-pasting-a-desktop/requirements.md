# Requirements: Stop Desktop Clipboard Placeholder PNG Hijacking Chat Paste

## Background

When a user presses Cmd+C/Ctrl+C inside the streamed desktop on
https://meta.helix.ml, `DesktopStreamViewer` writes a `ClipboardItem` that declares
**both** `text/plain` and `image/png` synchronously (required to stay anchored to the
user gesture for Safari). When the remote copy turns out to be text, the `image/png`
representation resolves to `PLACEHOLDER_PNG_BASE64` — a 70-byte 1x1 transparent PNG
(`frontend/src/components/external-agent/DesktopStreamViewer.tsx:264`). The placeholder
is deliberate: commit `2161143e2` added it because Chrome sanitises/decodes every image
representation and rejects the *entire* `navigator.clipboard.write()` — discarding the
valid text — if the image blob is empty or undecodable.

So after any text copy from the desktop, the system clipboard legitimately holds both
the real text and a 70-byte transparent PNG. Terminals and editors read `text/plain` and
look fine. Our own chat inputs read files first and unconditionally win:

- `RobustPromptInput.handlePaste` (`frontend/src/components/common/RobustPromptInput.tsx:1109`)
  calls `filesFromClipboard()` and, if anything comes back, `preventDefault()`s and
  attaches — text is never reached.
- `filesFromClipboard` (`frontend/src/components/common/chatAttachments.ts:84`) returns
  the placeholder PNG.
- `InferenceTextField.handlePaste` (`frontend/src/components/create/InferenceTextField.tsx:146`)
  has the same shape (any `kind === 'file'` + `image/*` item wins).

Result: the user's text is dropped and a 1x1 transparent PNG is attached.

The codebase already solved this in one place: the **paste-into-desktop** path in
`DesktopStreamViewer.tsx:165-175` explicitly reads `text/plain` before `image/png` with a
comment naming this exact placeholder problem. The chat inputs never got that treatment.

## User Stories

### US-1: Pasting copied desktop text into chat inserts the text
**As a** Helix user working in the streamed desktop
**I want** text I copied inside the desktop to paste into the chat input as text
**So that** I can quote code and terminal output to the agent.

Acceptance criteria:
- [ ] Given the clipboard carries a non-empty `text/plain` **and** an `image/png` file
      that is byte-identical to the placeholder sentinel, when I paste into the Helix
      chat input, then the text is inserted into the input.
- [ ] No image attachment appears in the attachment tray.
- [ ] Given the pasted text exceeds `LARGE_TEXT_THRESHOLD`, the existing
      large-text-to-`.txt`-attachment behaviour still applies (the sentinel must not
      short-circuit that path either).
- [ ] The same holds for `InferenceTextField` (the create/inference prompt box).

### US-2: Real image pastes are unaffected
**As a** user pasting a screenshot or an image copied from the desktop
**I want** the image to attach exactly as it does today
**So that** the fix does not regress image workflows.

Acceptance criteria:
- [ ] A pasted real PNG/JPEG still attaches as an image attachment.
- [ ] A pasted PDF / non-image file still attaches (unchanged).
- [ ] An image copied *inside the streamed desktop* and pasted into chat still attaches
      as an image.
- [ ] A clipboard containing a real image *and* the sentinel attaches only the real image.

### US-3: The placeholder stops leaking into other applications
**As a** user copying text out of the streamed desktop into any third-party app
**I want** the local clipboard to end up holding text only
**So that** no other application can be confused by a stray transparent pixel.

Acceptance criteria:
- [ ] After the gesture-anchored `clipboard.write()` resolves and the fetched remote
      clipboard is text, a best-effort `navigator.clipboard.writeText()` re-writes the
      clipboard as text-only.
- [ ] The re-write is strictly best-effort: any failure (Safari will likely reject it
      because transient activation is gone) is swallowed and does not change the toast,
      the existing write, or the copy outcome.
- [ ] The re-write never runs for image copies (it must not clobber a real image).
- [ ] The re-write is chained strictly *after* the `ClipboardItem` write resolves, so it
      cannot be overwritten by the slower gesture-anchored write.

### US-4: One sentinel constant, no duplication
**As a** maintainer
**I want** the placeholder defined once
**So that** the copy side and the paste side can never drift apart.

Acceptance criteria:
- [ ] `PLACEHOLDER_PNG_BASE64` lives in a single shared module; `DesktopStreamViewer.tsx`
      imports it rather than declaring it.
- [ ] The base64 string appears exactly once in the frontend source tree.
- [ ] The byte length used for the cheap synchronous match is *derived* from the base64
      constant, not hardcoded separately.
- [ ] No dead code or superseded fallback paths left behind (CLAUDE.md: no
      fallback-stacking, no duplicated constants).

## Non-Goals

- Changing the copy handler's gesture-anchored dual-MIME `ClipboardItem`. It is required
  for Chrome/Safari compatibility and stays as-is.
- Any heuristic broader than the exact sentinel (e.g. "drop any small PNG" or "drop any
  1x1 image"). Explicitly out of scope.
- Backend/API changes. This is entirely frontend.

## Testing Requirements

1. `cd frontend && yarn build` passes.
2. `cd frontend && yarn test` passes, with `RobustPromptInput.test.tsx` extended to cover:
   (a) text + sentinel PNG → text inserted, no attachment;
   (b) real image → still attaches (no regression);
   (c) large text paste → still converts to a `.txt` attachment;
   plus a unit test asserting the sentinel constant decodes to exactly 70 bytes (guards
   the derived-length match).
3. End-to-end in the inner Helix at `http://localhost:8080` with the `chrome-devtools`
   MCP tools: register `test@helix.ml` / `helixtest`, complete onboarding, create a spec
   task, open its detail page with the desktop stream, copy text inside the streamed
   desktop with Cmd+C/Ctrl+C, paste into the chat input, confirm **text** lands.
   If any link in that chain is genuinely impossible in the sandbox, synthesising the
   clipboard state (dispatching a real paste event carrying a `text/plain` item plus the
   exact 70-byte placeholder PNG file at the live chat input) is an acceptable fallback —
   but the report must state clearly which of the two was done, and the copy-side change
   must still be exercised against the live `DesktopStreamViewer`.
4. Confirm the image copy path still works end-to-end: copy an image inside the desktop,
   paste into chat, it attaches as an image.
5. The final report must state honestly what was run and what was not. No "compiles and
   unit tests pass" hand-off.

## Deliverable

A PR against `helixml/helix` with a conventional-commit title, full PR URL reported.

## Open Questions

1. **Text-vs-image preference when both are real.** The brief asks us to "prefer real
   text when both are present". The design deliberately does **not** add a blanket
   text-over-image preference to the chat inputs, because after the sentinel is dropped
   the remaining cases are ones where preferring text would regress:
   Finder/Explorer file copies carry the filename in `text/plain` alongside the file, and
   Chrome's "Copy image" carries `text/html`. A blanket rule would turn a legitimate file
   paste into a filename string. The proposed rule is therefore: *drop the sentinel; if
   any real file remains, files win (unchanged behaviour); text is reached only when the
   file list is empty after the drop.* Is that acceptable, or do you want the broader
   text-wins-over-image rule despite the file-copy regression risk?
2. **Exact-byte confirmation.** The design uses a synchronous `type === 'image/png' &&
   size === <70>` match, without an async byte-for-byte comparison. Rationale: the paste
   handler must decide `preventDefault()` synchronously, so an async check cannot gate
   that decision without either double-handling the event or introducing a visible flash;
   and a 70-byte PNG can only ever be a 1x1 single-colour pixel, so a false positive
   costs the user nothing. Is dropping the async byte confirmation acceptable, or do you
   want the exact-bytes check wired in (attach-then-remove, or defer the attach)?
3. **Copy-side re-write reliability.** Step 3 (`writeText` after the poll resolves) is
   proposed as best-effort. On Chrome the ~5s transient activation window comfortably
   covers the 500ms poll deadline; on Safari it will likely be rejected and the
   sentinel-drop covers it. Confirm you want it shipped as best-effort rather than
   dropped entirely (the paste-side fix alone already fixes the reported bug).
4. **`InferenceTextField` scope.** It is a separate prompt box (create/inference flow,
   used by `CreateContent.tsx` and `PreviewPanel.tsx`) and is not reachable from the
   streamed-desktop spec-task page, so it cannot be verified through the reported
   repro. Plan is to fix it identically and cover it with the shared helper + unit tests
   only. Confirm that is sufficient verification for that component.
