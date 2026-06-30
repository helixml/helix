# Design: Drag-and-Drop File Upload in New SpecTask Form

## Overview

Add a drag-and-drop dropzone around the existing "Attach files" row in
`NewSpecTaskForm.tsx`, reusing `react-dropzone` (already a dependency,
v14.2.3). Dropped files flow through the **same** validation/state logic that
the native file picker already uses.

## Current State (researched)

- Form: `frontend/src/components/tasks/NewSpecTaskForm.tsx`
  - State: `pendingAttachments: File[]` (line ~160), hidden `<input>` +
    "Attach files" label + chips (lines ~781–860).
  - The `<input onChange>` handler (lines ~789–812) contains the validation:
    remaining-slot check, per-file size check, then
    `setPendingAttachments(...)`.
  - Upload happens after task creation via `useUploadSpecTaskAttachments`.
- Constants: `frontend/src/services/specTaskAttachmentsService.ts`
  - `SPEC_TASK_ATTACHMENT_MAX_BYTES` (10 MB), `SPEC_TASK_ATTACHMENT_MAX_PER_TASK`
    (10), `SPEC_TASK_ATTACHMENT_ACCEPTED_MIME` (png/jpeg/gif/webp/svg/pdf/
    txt/md/csv).
- Existing dropzone patterns:
  - `components/widgets/FileUpload.tsx` — thin `useDropzone` wrapper, but only
    supports `onlyImages`/`onlyDocuments` accept maps (not the spec-task MIME
    set) and provides no drag overlay. **Not a clean fit** for the custom accept
    + visual feedback we need.
  - `components/external-agent/SandboxDropZone.tsx` — full overlay/visual
    feedback pattern (manual handlers). Good reference for the overlay styling.

## Approach

Use `react-dropzone`'s `useDropzone` hook **directly inside NewSpecTaskForm**
(reusing the same library, consistent with the codebase), because we need:
1. A custom `accept` map = `SPEC_TASK_ATTACHMENT_ACCEPTED_MIME`.
2. A drag-active visual overlay.
3. To route dropped files through the existing validation.

### Key decisions

1. **Extract a shared `addFiles(files: File[])` handler.** Pull the validation
   body out of the inline `<input onChange>` into a single function (e.g.
   `handleAddFiles`). Both the input's `onChange` and the dropzone's `onDrop`
   call it. This guarantees identical behaviour (DRY) and is the core change.

2. **`useDropzone` config:**
   - `onDrop: handleAddFiles`
   - `accept: SPEC_TASK_ATTACHMENT_ACCEPTED_MIME`
   - `noClick: true` and `noKeyboard: true` — the existing label button already
     opens the picker; disabling click/keyboard on the dropzone avoids a
     double-trigger (satisfies US-3) and keeps the native `<input>` as the click
     path.
   - Keep the existing hidden `<input>` for the click path (do **not** rely on
     react-dropzone's `getInputProps` input), so the label/button wiring is
     untouched. The dropzone only handles drag events.

3. **Wrap the attachment `Box` (lines ~781–860) with `getRootProps()`** and add
   `position: 'relative'`. Render a drag overlay (MUI `Fade` + dashed border +
   `CloudUploadIcon` / "Drop files to attach") shown when `isDragActive`,
   styled like `SandboxDropZone`'s overlay with `pointerEvents: 'none'`.

4. **Disable when full.** When
   `pendingAttachments.length >= SPEC_TASK_ATTACHMENT_MAX_PER_TASK`, pass
   `disabled: true` to `useDropzone` (drops are ignored) — mirrors the disabled
   state already shown on the button.

### Data flow

```
drag files over Box ──▶ isDragActive=true ──▶ overlay shows
drop ──▶ react-dropzone onDrop(acceptedFiles) ──▶ handleAddFiles(files)
                                                      │ (existing validation)
                                                      ▼
                                              setPendingAttachments(...)
                                                      │
click "Attach files" ──▶ <input onChange> ──▶ handleAddFiles(files)
```

Drop and click converge on `handleAddFiles`; nothing downstream (chips, upload,
reset) changes.

## Files to Change

- `frontend/src/components/tasks/NewSpecTaskForm.tsx` — extract
  `handleAddFiles`, add `useDropzone`, wrap the attachment Box, add overlay.

No backend, API, or constants changes. No new dependencies.

## Risks / Notes
- `react-dropzone`'s `accept` rejects unlisted types before `onDrop`; the
  existing size/count validation in `handleAddFiles` still runs for accepted
  files — keep both layers.
- Ensure `getRootProps()` doesn't swallow clicks on the label — `noClick: true`
  handles this; verify the picker still opens after wiring.

## Testing
- Build: `cd frontend && yarn build`.
- E2E in inner Helix (localhost:8080): register/onboard, open New SpecTask form,
  drag a screenshot onto the attachment area → chip appears; verify overlay on
  drag-over; verify oversize/too-many/wrong-type rejection; verify the button
  still opens the picker; create the task and confirm the attachment uploads.

## Implementation Notes (as built)

Implemented exactly as designed. One file changed:
`frontend/src/components/tasks/NewSpecTaskForm.tsx`.

- Added imports: `Fade` (MUI), `CloudUpload as CloudUploadIcon`
  (`@mui/icons-material`), `useDropzone` (`react-dropzone`).
- Extracted the inline `<input onChange>` validation into
  `handleAddFiles = useCallback((files: File[]) => {...}, [snackbar])`. It does
  the remaining-slot check and per-file size check inside the
  `setPendingAttachments` updater (using `prev.length` so the count is always
  current — avoids a stale-closure bug). The `<input onChange>` now just calls
  `handleAddFiles(Array.from(e.target.files || []))` after clearing the value.
- `useDropzone({ onDrop: handleAddFiles, accept: SPEC_TASK_ATTACHMENT_ACCEPTED_MIME,
  noClick: true, noKeyboard: true, disabled: attachmentsFull })`.
  `attachmentsFull = pendingAttachments.length >= SPEC_TASK_ATTACHMENT_MAX_PER_TASK`.
- Wrapped the attachment `Box` with `{...getAttachmentRootProps()}` +
  `position: relative`; added a `Fade in={isAttachmentDragActive}` overlay
  (dashed primary border, translucent fill, cloud icon + "Drop files to attach",
  `pointerEvents: none`, `m: -1` so the highlight extends slightly past the row).
- Updated the empty-state hint to "drag & drop or click to add …".

### Verified end-to-end (inner Helix, Vite HMR)
- Drag-over → overlay appears (screenshot 02); drop a valid `image/png` → chip
  added (03); overlay clears after drop.
- Wrong type (`.exe`) → rejected by react-dropzone `accept` (no chip).
- Oversize (11 MB png) → rejected with snackbar "huge.png is too large
  (max 10.0 MB)." (04).
- Only one `<input type=file>` in the dropzone root + label `htmlFor` intact →
  no double file-dialog; the "Attach files" button still opens the picker.

### Gotchas
- `frontend/dist/` is root-owned (prod bind mount per CLAUDE.md), so the
  repo-local `yarn build` fails at the final copy step with EACCES. The code
  itself compiles: `tsc --noEmit` is clean and `vite build --outDir /tmp/...`
  exits 0. Don't `rm -rf frontend/dist`.
- Native file drops can't be done via the chrome-devtools click/drag tools;
  simulate by dispatching `DragEvent`s with a constructed `DataTransfer` via
  `evaluate_script` (React's delegated listeners pick them up).
