# fix(frontend): stop 1x1 clipboard placeholder hijacking chat paste

## Summary

Copying text inside the streamed desktop and pasting it into the Helix chat box attached a
1x1 transparent PNG instead of inserting the text.

Two halves, both in the frontend:

1. **Copy side.** `DesktopStreamViewer`'s Cmd+C handler must declare the `ClipboardItem`'s
   MIME types synchronously to stay anchored to the user gesture (Safari), before the
   remote clipboard poll tells us whether the copy was text or an image. So it always
   declares both `text/plain` and `image/png`, and a text copy resolves the image half to
   `PLACEHOLDER_PNG_BASE64` — a 70-byte 1x1 transparent PNG. That placeholder is
   deliberate (commit `2161143e2`): Chrome sanitises every image representation and rejects
   the *entire* `clipboard.write()`, discarding the valid text, if the image blob can't be
   decoded. Net effect: after any text copy the clipboard legitimately holds the text *and*
   a transparent pixel.
2. **Paste side.** `RobustPromptInput.handlePaste` called `filesFromClipboard` and
   unconditionally won if anything came back, so the sentinel PNG was attached and the text
   was dropped. `InferenceTextField.handlePaste` had the same shape.

Terminals and editors read `text/plain`, which is why the bug was invisible everywhere
except our own chat inputs.

The primary fix is on the paste side, so the chat inputs are protected against any
clipboard producer — not just ours.

## Changes

- **New `frontend/src/components/common/clipboardPlaceholder.ts`** — single home for the
  sentinel. Exports `PLACEHOLDER_PNG_BASE64`, a `PLACEHOLDER_PNG_BYTE_LENGTH` *derived*
  from it via `atob()` (never hardcoded), and `isPlaceholderPng(file)`.
  `DesktopStreamViewer` now imports the constant instead of declaring its own copy — the
  base64 string appears exactly once in the tree.
- **`chatAttachments.ts`** — `filesFromClipboard` drops files matching the sentinel. Done
  in the helper rather than at call sites so every consumer is protected by construction.
  The match is the cheap synchronous `type === 'image/png' && size === 70`: the paste
  handler must decide `preventDefault()` synchronously, so an async byte comparison can't
  gate it, and a 70-byte PNG can only encode a 1x1 single-colour pixel. This is keyed to
  the exact sentinel we produce — not a "small PNG" or "1x1 image" heuristic.
- **`RobustPromptInput.tsx`** — comment documenting the rule. **A real file on the
  clipboard still wins over text.** We deliberately did *not* add a blanket
  text-over-image preference: Finder/Explorer file copies carry the filename in
  `text/plain` and Chrome's "Copy image" carries `text/html`, so a blanket rule would turn
  legitimate file pastes into filename strings. Dropping the sentinel alone fully resolves
  the bug.
- **`InferenceTextField.tsx`** — replaced its hand-rolled `clipboardData.items` walk with
  `filesFromClipboard(...)` + an `image/*` filter, inheriting the sentinel drop and
  removing a duplicated clipboard walk. *Behaviour change beyond the fix:* it previously
  ignored `clipboardData.files` entirely and now honours it, so images dragged/copied from
  Finder attach as they already do in the other input.
- **`DesktopStreamViewer.tsx`** — once the poll resolves and the result is text, a
  best-effort `navigator.clipboard.writeText()` re-writes the clipboard text-only so the
  placeholder never reaches other applications. Chained strictly *after* the
  gesture-anchored `write()` resolves (running it in parallel would let the slower write
  land last and restore the sentinel), only for text copies, with its own `.catch` so a
  rejection can't flip the toast or affect the write that already works. Safari will likely
  reject it; the paste-side sentinel drop covers that.

## Testing

**Ran and passing:**
- `yarn tsc` (full TypeScript project build) — clean.
- Full `vitest` suite: **112 files, 596 tests passed, 1 skipped**.
- New coverage: text + sentinel PNG → text preserved, nothing attached; a real pasted
  image → still attaches; large text paste → still becomes a `.txt` attachment; plus
  `filesFromClipboard` unit tests and an assertion that the sentinel decodes to exactly 70
  bytes with a valid PNG signature and trailing `IEND`.
- **The three new paste tests were confirmed to be genuine regression tests**: with the
  one-line filter in `filesFromClipboard` reverted, all three fail; restored, all three
  pass.

**Verified live in the browser** (inner Helix at `localhost:8080`, registered
`test@helix.ml`, completed onboarding, created project + spec task, opened the task detail
page with the desktop stream). At the **live production chat input**, a real
`ClipboardEvent` carrying a real `DataTransfer` with `text/plain` plus the exact 70-byte
sentinel `File`:

| Clipboard | `defaultPrevented` | Attachment |
|---|---|---|
| text + 70-byte sentinel PNG | `false` (native text insertion proceeds) | none ✅ |
| real 85-byte PNG | `true` | attaches as image ✅ |

The 85-byte control is the important one — it shows the match is the exact sentinel size,
not "any small PNG".

**NOT verified — stated plainly:**
- **The copy-side `writeText` re-write was not exercised live.** It needs a real copy
  performed by an app inside the streamed desktop, and the desktop stream could not hold a
  connection on this host (`Connection stale - no data received`, `Render stalled`,
  continuous reconnect attempts) under load averages of 130–358 on 4 CPUs, which also
  OOM-killed Vite builds and the frontend container's esbuild worker. Faking it via
  `POST /external-agents/{id}/clipboard` does not work either: on GNOME the bridge writes
  the selection over D-Bus `SetSelection`, the desktop then owns the selection, and the
  matching `GET` that the copy handler polls reads back empty.
- **The image-copy-from-desktop path was likewise not exercised end-to-end**, for the same
  reason. The paste half of it *is* covered by the real-image browser check above and by
  unit tests.
- `yarn build` could not be completed here: `frontend/dist` is a root-owned bind mount the
  sandbox user cannot write to, and builds redirected to a temp `--outDir` were OOM-killed
  under the host load. `yarn tsc` and the full test suite both pass, and the changed
  modules ran through Vite in the live browser, but I have not seen a green `vite build`
  and am not claiming one — CI will confirm.

The copy-side change is guarded (`kind === "text" && d?.data && navigator.clipboard?.writeText`),
swallows its own errors, and runs only after the existing write resolves, so the worst case
if it misbehaves is the status quo — but it is the one change here without live coverage.

## Screenshots

![Real image still attaches at the live chat input](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002742_fix-pasting-a-desktop/screenshots/02-real-image-still-attaches.png)
