# Implementation Tasks: Drag-and-Drop File Upload in New SpecTask Form

- [~] Extract the inline `<input onChange>` validation in `NewSpecTaskForm.tsx` into a shared `handleAddFiles(files: File[])` function (remaining-slot check, per-file size check, then `setPendingAttachments`).
- [~] Point the existing hidden file `<input>` `onChange` at `handleAddFiles`.
- [~] Add `useDropzone` from `react-dropzone` with `onDrop: handleAddFiles`, `accept: SPEC_TASK_ATTACHMENT_ACCEPTED_MIME`, `noClick: true`, `noKeyboard: true`, and `disabled` when at the max file count.
- [~] Wrap the attachment `Box` (attach button + chips + hint) with `getRootProps()` and `position: 'relative'`.
- [~] Add a drag-active overlay (MUI `Fade` + dashed border + `CloudUploadIcon` + "Drop files to attach", `pointerEvents: 'none'`) shown when `isDragActive`, styled like `SandboxDropZone`.
- [ ] Verify the "Attach files" button still opens the native picker (no double-trigger from the dropzone).
- [ ] Build the frontend (`cd frontend && yarn build`).
- [ ] E2E test in inner Helix: drag-drop adds files, overlay shows on drag-over, size/count/type validation works, button still works, attachment uploads after task creation.
