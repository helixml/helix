# Add drag-and-drop file upload to the New SpecTask form

## Summary
You can now drag files/screenshots straight onto the New SpecTask form's
attachment area instead of only clicking the "Attach files" button. This reuses
`react-dropzone` (already a dependency, used elsewhere in the app) and routes
dropped files through the exact same validation as the file picker.

## Changes
- `frontend/src/components/tasks/NewSpecTaskForm.tsx`:
  - Extracted the file-picker validation into a shared `handleAddFiles()` so the
    picker and the dropzone behave identically (remaining-slot count + per-file
    size limit; the count is read from the state updater to avoid stale closures).
  - Added a `react-dropzone` dropzone around the attachment row
    (`accept` = the spec-task MIME set, `noClick`/`noKeyboard` so the existing
    button stays the only click path — no double file dialog, `disabled` at the
    10-file cap).
  - Added a drag-active overlay (dashed border + cloud icon + "Drop files to
    attach") shown only while dragging.
  - Updated the helper hint to mention drag & drop.
- No backend, API, accepted-types, or size-limit changes. No new dependencies.

## Testing
Verified end-to-end in the inner Helix (Vite HMR):
- Drag-over shows the overlay; dropping a valid PNG adds a deletable chip;
  overlay clears after drop.
- Wrong type (`.exe`) rejected by `accept`; oversize (11 MB) rejected with the
  existing "too large" snackbar.
- Single `<input type=file>` in the dropzone + label `htmlFor` intact → the
  "Attach files" button still opens the native picker.
- `tsc --noEmit` clean; `vite build` to a writable out dir exits 0 (repo
  `dist/` is root-owned in this env).

## Screenshots
![Form with drag & drop hint](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002188_it-should-be-possible-to/screenshots/01-form-with-dropzone-hint.png)
![Drag overlay active](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002188_it-should-be-possible-to/screenshots/02-drag-overlay-active.png)
![File attached after drop](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002188_it-should-be-possible-to/screenshots/03-file-attached-after-drop.png)
![Oversize file rejected](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002188_it-should-be-possible-to/screenshots/04-oversize-rejected-snackbar.png)
