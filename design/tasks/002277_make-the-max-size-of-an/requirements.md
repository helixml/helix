# Requirements: Raise Spec-Task Attachment Max Size to 100 MB

## Background

Spec-task attachments are currently capped at **10 MB per file**. Users want to
attach larger files (e.g. logs, PDFs, datasets). This task raises the per-file
limit to **100 MB**. The per-task file count (10 files) is unchanged.

The limit is defined by two source-of-truth constants that must stay in sync:

- Backend: `SpecTaskAttachmentMaxBytes` in `api/pkg/types/simple_spec_task.go`
- Frontend: `SPEC_TASK_ATTACHMENT_MAX_BYTES` in
  `frontend/src/services/specTaskAttachmentsService.ts`

All user-facing "max size" text and error messages are derived from these
constants (via `humanAttachmentSize` / `humanSize`), so no copy needs manual
editing — it will read "100.0 MB" automatically once the constant changes.

## User Stories

### US-1: Upload files up to 100 MB
**As a** user creating or editing a spec task,
**I want** to attach files up to 100 MB each,
**so that** I can include larger logs, PDFs, and datasets without hitting the
10 MB wall.

**Acceptance criteria:**
- A file between 10 MB and 100 MB uploads successfully via the New Spec Task
  form and the Task Attachments panel.
- A file over 100 MB is rejected client-side with a clear message that states
  the "100.0 MB" limit.
- The backend independently rejects a file over 100 MB with HTTP 413, even if
  the client check is bypassed.
- The "10 files, X each" helper text and the pre-upload error snackbars display
  "100.0 MB" (derived from the constant, not hardcoded).

### US-2: Existing limits unchanged
**As a** maintainer,
**I want** only the per-file byte limit changed,
**so that** the per-task count (10 files), the MIME-type allowlist, and the
read-only-after-review behaviour are all unaffected.

**Acceptance criteria:**
- `SpecTaskAttachmentMaxPerTask` remains 10.
- The MIME allowlist is unchanged.
- No regression in existing 10 MB-and-under uploads.

## Out of Scope

- Increasing the per-task file count.
- Changing the MIME-type allowlist.
- Streaming/chunked uploads or resumable upload UX.

## Open Questions

1. **100 MB = 100 × 1024 × 1024 (binary MiB) vs 100 × 1000 × 1000 (decimal MB)?**
   The existing constant uses `10 * 1024 * 1024` (MiB). The plan follows the
   same convention: `100 * 1024 * 1024`. Confirm this is intended.
2. **Multipart in-memory buffer.** The upload handler calls
   `ParseMultipartForm(MaxPerTask * MaxBytes)`, whose argument is the amount
   buffered **in memory** before spilling to disk. Today that is 10 × 10 MB =
   100 MB; naively it would become 10 × 100 MB = **1 GB** in memory per request,
   a real memory-pressure risk. The design proposes decoupling the
   `ParseMultipartForm` buffer from the per-file limit (keep a modest in-memory
   buffer, let the rest spill to disk). Confirm this is acceptable — see
   design.md §2.
3. Any reverse proxy / load balancer in front of the production API with a
   request-body size limit (e.g. nginx `client_max_body_size`) that would need
   raising to allow 100 MB uploads? The repo's `frontend/nginx.conf` only serves
   static assets and does not proxy the API, so it is not a factor here, but the
   production ingress is outside this repo.
