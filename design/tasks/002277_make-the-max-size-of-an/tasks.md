# Implementation Tasks: Raise Spec-Task Attachment Max Size to 100 MB

- [x] Change `SpecTaskAttachmentMaxBytes` to `100 * 1024 * 1024` (update the `// 100 MB per file` comment) in `api/pkg/types/simple_spec_task.go`
- [x] Change `SPEC_TASK_ATTACHMENT_MAX_BYTES` to `100 * 1024 * 1024` (update the `// 100 MB` comment) in `frontend/src/services/specTaskAttachmentsService.ts`
- [x] Decouple the `ParseMultipartForm` in-memory buffer from the per-file limit in `api/pkg/server/spec_task_attachments_handlers.go` — pass a fixed 32 MB budget instead of `MaxPerTask * MaxBytes`, and update the comment
- [x] Grep `api/pkg/server/*_test.go` for `SpecTaskAttachmentMaxBytes` / hardcoded `10 * 1024 * 1024` — no existing boundary assertions found (no upload-handler HTTP test harness exists; existing test file only covers pure helpers)
- [x] Add a guard test asserting the constant is 100 MB (`TestSpecTaskAttachmentLimits`) — proportionate to a two-constant change; a full multipart-upload handler test would need a test server + store mock + filestore + auth, disproportionate here
- [x] Build backend: `cd api && go build ./pkg/server/ ./pkg/types/` — OK; `TestSpecTaskAttachmentLimits` passes
- [x] Build frontend: `yarn vite build` — OK (built to temp outDir; the repo's `frontend/dist` is a root-owned bind mount)
- [x] End-to-end test in inner Helix: registered, onboarded, created a spec task, opened its attachments panel — helper text reads **"100.0 MB each"** (screenshot 01), and uploading a **25 MB** file (2.5× the old limit) returned **HTTP 201** and now shows as "big-attach-25mb.txt — 25.0 MB" in "Attachments (1/10)" (screenshot 02). Did not exercise the >100 MB rejection path (generating/uploading a 100 MB+ file is disproportionate; the 413 guard is unchanged code that already references the constant).
