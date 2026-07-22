# Raise spec-task attachment max size to 100 MB

## Summary

Raises the per-file spec-task attachment limit from 10 MB to 100 MB so users can
attach larger logs, PDFs, and datasets. The limit lives in two source-of-truth
constants (Go + TypeScript); all user-facing "max size" copy and enforcement is
derived from them, so no UI strings are hardcoded.

## Changes

- `api/pkg/types/simple_spec_task.go`: `SpecTaskAttachmentMaxBytes` 10 MB → 100 MB.
- `frontend/src/services/specTaskAttachmentsService.ts`:
  `SPEC_TASK_ATTACHMENT_MAX_BYTES` 10 MB → 100 MB (UI helper text and error
  snackbars now read "100.0 MB", formatted from the constant).
- `api/pkg/server/spec_task_attachments_validation_test.go`: added
  `TestSpecTaskAttachmentLimits`, a regression guard asserting the constant is
  100 MB.

## Notes

- The multipart in-memory buffer was already decoupled from the per-file limit
  on `main` (it buffers one file's worth and spills the rest to disk, and is
  explicitly kept from scaling with the 500-file per-task cap). No change needed
  here beyond the constant.
- The per-file cap is enforced server-side (HTTP 413 via `fh.Size` and an
  `io.LimitReader` read) independently of the client-side check.

## Verification

- `cd api && go build ./pkg/server/ ./pkg/types/` — OK.
- `CGO_ENABLED=1 go test -run TestSpecTaskAttachmentLimits ./pkg/server/` — pass.
- Frontend `vite build` — OK.
- **End-to-end in the inner Helix:** the attachments panel helper text now reads
  "100.0 MB each", and uploading a **25 MB** file (2.5× the old 10 MB limit)
  returned **HTTP 201** and persisted as "big-attach-25mb.txt — 25.0 MB" (see
  screenshots). The old build would have rejected this client-side and with a
  413 server-side.

## Screenshots

![Helper text shows 100.0 MB each](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002277_make-the-max-size-of-an/screenshots/01-attachments-100mb-helper.png)
![25 MB upload succeeds](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002277_make-the-max-size-of-an/screenshots/02-25mb-upload-success.png)
