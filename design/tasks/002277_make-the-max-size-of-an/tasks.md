# Implementation Tasks: Raise Spec-Task Attachment Max Size to 100 MB

- [ ] Change `SpecTaskAttachmentMaxBytes` to `100 * 1024 * 1024` (update the `// 100 MB per file` comment) in `api/pkg/types/simple_spec_task.go`
- [ ] Change `SPEC_TASK_ATTACHMENT_MAX_BYTES` to `100 * 1024 * 1024` (update the `// 100 MB` comment) in `frontend/src/services/specTaskAttachmentsService.ts`
- [ ] Decouple the `ParseMultipartForm` in-memory buffer from the per-file limit in `api/pkg/server/spec_task_attachments_handlers.go` — pass a small fixed budget (e.g. `32 << 20`) instead of `MaxPerTask * MaxBytes`, and update the comment
- [ ] Grep `api/pkg/server/*_test.go` for `SpecTaskAttachmentMaxBytes` / hardcoded `10 * 1024 * 1024` and update any boundary assertions to the new limit
- [ ] Add/adjust a handler test: a ~11 MB file is accepted, a >100 MB file is rejected with HTTP 413
- [ ] Build backend: `cd api && go build ./pkg/server/ ./pkg/types/`
- [ ] Build frontend: `cd frontend && yarn build`
- [ ] End-to-end test in inner Helix: attach a 10–100 MB file (succeeds), attempt >100 MB (rejected with "100.0 MB" message), confirm helper text reads "100.0 MB each"
