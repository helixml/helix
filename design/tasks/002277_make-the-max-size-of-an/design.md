# Design: Raise Spec-Task Attachment Max Size to 100 MB

## 1. Change the two constants

The limit lives in exactly two places that must stay in sync:

| File | Line | Current | New |
|------|------|---------|-----|
| `api/pkg/types/simple_spec_task.go` | ~460 | `SpecTaskAttachmentMaxBytes = 10 * 1024 * 1024 // 10 MB per file` | `100 * 1024 * 1024 // 100 MB per file` |
| `frontend/src/services/specTaskAttachmentsService.ts` | 10 | `SPEC_TASK_ATTACHMENT_MAX_BYTES = 10 * 1024 * 1024 // 10 MB` | `100 * 1024 * 1024 // 100 MB` |

Everything downstream reads these constants:

- **Backend enforcement** (`api/pkg/server/spec_task_attachments_handlers.go`):
  per-file size check (`fh.Size > SpecTaskAttachmentMaxBytes`, HTTP 413), the
  `io.LimitReader(src, MaxBytes+1)` read cap, and the post-read length check all
  reference the constant — no change needed beyond the constant itself.
- **Frontend enforcement** (`NewSpecTaskForm.tsx`, `TaskAttachmentsPanel.tsx`):
  the pre-upload `f.size > SPEC_TASK_ATTACHMENT_MAX_BYTES` guard, the error
  snackbar text, and the "10 files, X each" helper text all format the constant
  via `humanAttachmentSize` / `humanSize`, so they will show "100.0 MB"
  automatically.

No UI copy is hardcoded to "10 MB", so no string edits are required.

## 2. Multipart in-memory buffer (key decision)

`spec_task_attachments_handlers.go` currently does:

```go
// Enforce a request-body cap matching per-task budget (10 files * 10 MB + overhead).
if err := r.ParseMultipartForm(int64(types.SpecTaskAttachmentMaxPerTask) * types.SpecTaskAttachmentMaxBytes); err != nil {
```

`ParseMultipartForm(maxMemory)`'s argument is **how many bytes of file parts are
held in memory** before the remainder spills to temp files on disk — it is *not*
a hard request-size cap. Today that budget is `10 * 10 MB = 100 MB`. If we leave
this expression tied to the raised constant it becomes `10 * 100 MB = 1 GB` held
in memory per upload request, which is a memory-pressure / DoS risk.

**Decision:** decouple the in-memory buffer from the per-file limit. Pass a
small fixed in-memory budget (e.g. `32 << 20`, 32 MB — Go's own default is
`10 << 20`) so larger parts spill to disk. The real per-file limit is still
enforced by the existing `fh.Size > MaxBytes` check and the
`io.LimitReader(src, MaxBytes+1)` read, so correctness is unchanged; only the
buffering strategy changes. Update the comment accordingly.

Rationale: raising the *per-file* limit is the intent; ballooning in-memory
multipart buffering to 1 GB is an unintended side effect of coupling the two.
This is the only non-mechanical part of the change.

## 3. Tests

- Existing Go handler tests (if any assert on the 10 MB boundary) must be
  updated to the new limit. Grep for `SpecTaskAttachmentMaxBytes` in
  `api/pkg/server/*_test.go` and any hardcoded `10 * 1024 * 1024` in tests.
- Add/adjust a test asserting a ~11 MB file is now accepted and a >100 MB file
  is rejected with 413 (the boundary the change moves).

## 4. Verification

- `cd api && go build ./pkg/server/ ./pkg/types/`
- `cd frontend && yarn build`
- End-to-end in the inner Helix: create a spec task, attach a file between 10
  and 100 MB (should succeed), attempt >100 MB (should be rejected with the
  "100.0 MB" message). Confirm the helper text reads "100.0 MB each".

## Notes / Learnings

- The attachment size limit is intentionally a single source-of-truth constant
  per side (Go + TS). Keep them equal.
- All user-facing size copy is formatted from the constant — never hardcode the
  number in JSX or error strings.
- `frontend/nginx.conf` only serves static assets (port 8081) and does not proxy
  API upload requests, so it imposes no body-size limit on uploads. The
  production ingress is out of this repo's scope (see requirements Open
  Question 3).
