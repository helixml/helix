# Design: Attach Screenshots and Documents to Spec Tasks

## Summary

Add a SpecTask-scoped attachments feature. Files uploaded by the user
land in the Helix filestore on upload, get committed to the
`helix-specs` branch under `design/tasks/<task-dir>/attachments/` when
spec generation starts, and are therefore present inside the agent's
sandbox container at a stable, well-known path
(`/home/retro/work/helix-specs/design/tasks/<task-dir>/attachments/`)
because the `helix-specs` repo is already cloned into the workspace by
the container's startup script.

## Why this approach (git-as-transport)

There are three obvious places to make attachments visible to the
agent:

| Option | How files reach container | Pros | Cons |
|---|---|---|---|
| **A. Commit to `helix-specs` branch** *(chosen)* | Standard `git clone` in startup script | Zero new transport. Files appear in the same tree as the design docs. Naturally visible in the helix-specs git history, browsable in the UI's existing design-docs viewer. PublicDesignDocs gating already works. Survives container restart. Cloned tasks already use this branch — `prepopulateClonedSpecs()` shows the pattern. | Git bloat for binary files; mitigated by 10 MB cap + 10-file cap. Couples task lifecycle to a git push. |
| B. Write to sandbox workspaceDir via hydra | Direct copy via existing hydra file APIs | No git bloat. | New cross-component transport. Doesn't survive container rebuild unless re-staged. Not viewable through the existing helix-specs viewer. |
| C. Have the agent fetch files via an API at startup | Bash + curl in startup script | No commits, no git bloat. | New endpoint, new auth model for agent → API, files not visible until the agent fetches them. |

**Option A wins on simplicity.** The infrastructure already exists:
`prepopulateClonedSpecs()` in `spec_driven_task_service.go:2082`
already commits files to the helix-specs branch via
`WithExternalRepoWrite()`. We extend that pattern. The agent's startup
script already clones helix-specs into `/home/retro/work/helix-specs/`.
No new transport, no new endpoint, no new mount.

The 10 MB × 10 file cap means ~100 MB worst-case per task, which is
well below the threshold where git starts struggling. If we hit users
who legitimately need larger files (UI mocks at 4K, big PDFs), the
follow-up is **git-LFS on the helix-specs branch** — not a rip-and-
replace.

## Data Model

### New table: `spec_task_attachments`

A row per uploaded file. Lives separately from `spec_tasks` so we
don't bloat the main row, and so we can stream the list back without
loading the whole task.

```go
type SpecTaskAttachment struct {
    ID            string    `gorm:"primaryKey;size:255"`         // att_01k...
    SpecTaskID    string    `gorm:"size:255;index;not null"`
    ProjectID     string    `gorm:"size:255;index;not null"`     // denormalised for fast org-scoped queries / authz
    UserID        string    `gorm:"size:255;index"`              // who uploaded
    Filename      string    `gorm:"size:512;not null"`           // original filename, sanitised
    MimeType      string    `gorm:"size:128;not null"`
    SizeBytes     int64     `gorm:"not null"`
    Caption       string    `gorm:"size:1024"`                   // optional user note
    FilestorePath string    `gorm:"size:1024;not null"`          // absolute filestore path
    CommittedSHA  string    `gorm:"size:64"`                     // helix-specs commit hash once staged (empty until StartSpecGeneration runs)
    CreatedAt     time.Time
}
```

GORM AutoMigrate adds the table; no migration script needed (matches
project convention).

### Filestore layout

Uploaded blobs live under a SpecTask-scoped prefix in the user's
filestore tree:

```
spec-tasks/<task-id>/attachments/<att-id>__<sanitised-filename>
```

Why the `att-id__` prefix? It survives collisions when two attachments
happen to share a filename and lets the path stay stable even if the
filename is later edited.

A new `IsSpecTaskAttachmentPath()` helper (mirroring `IsAppPath`) lets
the filestore upload handler enforce that the user has write access to
the underlying spec task.

## API Surface

All endpoints are gated by `authorizeUserToProjectByID` against the
parent task's project — same pattern as every other spec-task handler.

| Method | Path | Purpose |
|---|---|---|
| POST | `/api/v1/spec-tasks/{taskID}/attachments` | Multipart upload, one or more files (`files[]`). Returns `[]SpecTaskAttachment`. Rejects if task is past `spec_review`. |
| GET | `/api/v1/spec-tasks/{taskID}/attachments` | List attachments. |
| GET | `/api/v1/spec-tasks/{taskID}/attachments/{attID}/content` | Stream raw file bytes (auth-checked). Used by the lightbox and image `<img>` tags. Supports `?presigned=true` to return a short-lived signed URL instead. |
| DELETE | `/api/v1/spec-tasks/{taskID}/attachments/{attID}` | Remove file. Rejects if task is past `spec_review`. Removes filestore blob, deletes the row, and (if `CommittedSHA != ""`) commits a removal to helix-specs. |

OpenAPI annotations on the handlers → regenerate the typed client with
`./stack update_openapi`.

### Create-task convenience

`POST /api/v1/spec-tasks/from-prompt` stays JSON. The frontend's new
flow is:

```
1. POST /api/v1/spec-tasks/from-prompt   → task created (backlog, no agent yet)
2. POST /api/v1/spec-tasks/{id}/attachments (multipart)  → for each batch
3. (existing) POST /api/v1/spec-tasks/{id}/start-planning → kicks off agent
```

Task creation is fast (no agent boot yet), so the two-call flow is
fine. It also gives us idempotent retries on upload failure without
re-creating the task. The UI ties the steps together in a single
"Create" click.

## Backend: commit to helix-specs at planning time

`StartSpecGeneration()` in
`api/pkg/services/spec_driven_task_service.go` is the entry point. We
add a new step *before* the prompt is built and the agent is launched:

```go
// New: stage attachments into helix-specs branch
if err := s.stageAttachmentsToHelixSpecs(ctx, task); err != nil {
    log.Error().Err(err).Str("task_id", task.ID).Msg("stage attachments failed")
    s.markTaskFailed(ctx, task, fmt.Sprintf("stage attachments: %v", err))
    return
}
```

`stageAttachmentsToHelixSpecs`:

1. Loads `SpecTaskAttachment` rows where `SpecTaskID == task.ID` and
   `CommittedSHA == ""`.
2. Opens a `WithExternalRepoWrite()` session on the project's
   helix-specs branch (the same helper `prepopulateClonedSpecs` uses).
3. For each attachment: streams the blob from filestore into
   `design/tasks/<task.DesignDocPath>/attachments/<filename>`.
4. Writes a single commit
   `"chore(specs): attach N files for <task name>"`, pushes.
5. Updates each row's `CommittedSHA` to the new HEAD.

If staging is partially complete (the API restarted), step 1 keys on
`CommittedSHA == ""` so we don't re-commit already-staged files.

For **cloned tasks**, `prepopulateClonedSpecs()` is extended to also
copy `attachments/*` from the source task directory in the *same*
commit it pushes for the cloned specs.

## Planning Prompt Changes

`BuildPlanningPrompt()` in
`api/pkg/services/spec_task_prompts.go` gets a new template variable
`AttachmentsSection`, populated by a new helper
`BuildAttachmentsSection(attachments []*types.SpecTaskAttachment, taskDirName string) string`:

```
## Attachments

The user attached {{N}} file(s) for context. They are in your workspace at:

- `/home/retro/work/helix-specs/design/tasks/<task-dir>/attachments/screenshot-bug.png` (image/png, 248 KB) — "shows the misaligned dropdown"
- `/home/retro/work/helix-specs/design/tasks/<task-dir>/attachments/design.pdf` (application/pdf, 1.2 MB)

**Read or view them BEFORE asking clarifying questions.** They are
evidence of the bug or feature, not decoration. For images, use the
Read tool which supports PNG/JPG/GIF/WebP visually.
```

If there are no attachments, the section is empty (no header,
no whitespace pollution). The template guards with `{{if
.AttachmentsSection}}`.

## Frontend

### `NewSpecTaskForm.tsx`

Add a `FileUpload` widget below the prompt textarea, gated on a
`taskAttachmentsEnabled` flag (default true; allows quick rollback).
Reuse the existing
`frontend/src/components/widgets/FileUpload.tsx` (react-dropzone) and
the `useUploadFilestoreFiles` hook pattern. Constraints: 10 MB / file,
10 files / task, accepted MIME types listed in `requirements.md`.

State held in component:
```ts
const [pendingAttachments, setPendingAttachments] = useState<File[]>([])
```

Submit flow becomes:
```ts
const task = await api.getApiClient().v1SpecTasksFromPromptCreate(createTaskRequest)
if (pendingAttachments.length > 0) {
  await uploadAttachments(task.id, pendingAttachments)  // FormData multipart
}
if (startImmediately) {
  await api.getApiClient().v1SpecTasksStartPlanningCreate(task.id)
}
```

If `uploadAttachments` fails, we surface the error and offer "Retry
upload" / "Edit task" — the task already exists at this point so the
user can recover from the task detail page.

### `SpecTaskDetailContent.tsx`

New `<TaskAttachmentsPanel />` component rendered above the
description block. It lists attachments returned from
`GET /api/v1/spec-tasks/{id}/attachments` via a React-Query hook.
Add/remove buttons are disabled if `task.status` is past
`spec_review`. Image thumbnails use the `…/content` endpoint
directly; PDFs/text files render as a generic icon and open in a new
tab on click.

### Visual consistency

Follow the existing list/card style rules in `CLAUDE.md`:
- File rows use `<Typography variant="body2" color="text.secondary">`
  for non-primary metadata.
- The "remove" action is in a vertical-dot menu (not a row of icon
  buttons).
- Thumbnails sit at 48×48, square-cropped via CSS `object-fit: cover`.

## Edge cases / gotchas

- **Filename collisions.** Two attachments with the same filename:
  later one wins inside the commit (overwrites the earlier one in
  helix-specs). The DB still keeps both rows. The `att-id__` prefix
  on the filestore path prevents loss of the older bytes. UI shows a
  warning when a user adds a file whose name already exists.
- **Path traversal.** Sanitise filenames with `filepath.Base()` +
  reject anything containing `..` or starting with `.`. Filestore
  paths are constructed server-side from `att-id` + sanitised name —
  never trust the upload's `Content-Disposition`.
- **SVG XSS.** Serve SVGs with `Content-Type: image/svg+xml` and
  `Content-Disposition: attachment` (force download, never inline
  render) **or** strip scripts server-side. Simpler: forbid `<script>`
  via a regex check during upload — SVGs that contain it are rejected.
- **Filestore quotas.** Add the attachment bytes to the project's
  existing filestore quota counter (if one is enabled). For projects
  without a quota, the per-task cap (100 MB) is the only ceiling.
- **Reaping orphaned blobs.** If a task is deleted, a hook in
  `DeleteSpecTask` deletes the attachment rows AND the filestore
  prefix `spec-tasks/<task-id>/`. Helix-specs commits are left in
  history (consistent with how design-docs deletions are handled
  today — git is append-only for that branch).
- **MIME spoofing.** Use `net/http.DetectContentType` on the first
  512 bytes of the uploaded file and reject if it doesn't match the
  declared `Content-Type` family (image/, application/pdf, text/).

## Test plan

- **Go unit tests** in `api/pkg/services/spec_task_attachments_test.go`
  cover: upload validation, MIME sniffing, size limits, per-task cap,
  status-gated mutation (rejects after `spec_review`), commit
  idempotency (re-running `stageAttachmentsToHelixSpecs` is a no-op
  when `CommittedSHA` already set).
- **HTTP handler tests** in `api/pkg/server/spec_task_attachments_test.go`
  using `suite.Suite` + `gomock` (per project convention).
- **End-to-end** in the inner Helix at `http://localhost:8080`:
  register `test@helix.ml` / `helixtest`, create a task with a real PNG
  attached, watch the agent's planning session and confirm the
  attachment appears at the documented path and the agent reads it
  before exploring.
- **Frontend**: snapshot-only — there are no frontend test
  conventions in this repo. Manual end-to-end verification per CLAUDE.md.

## Rollout

No feature flag at the API level (additive endpoints, opt-in by
calling them). Frontend gating via `taskAttachmentsEnabled` (default
true) so we can switch the UI off without an API redeploy if needed.

## Files touched (estimate)

- `api/pkg/types/simple_spec_task.go` — new `SpecTaskAttachment` type, request types.
- `api/pkg/store/store_spec_task_attachments.go` (new) — CRUD.
- `api/pkg/store/store.go` — interface additions for mocks.
- `api/pkg/server/spec_task_attachments_handlers.go` (new) — HTTP layer.
- `api/pkg/server/server.go` — route registration.
- `api/pkg/services/spec_driven_task_service.go` — call `stageAttachmentsToHelixSpecs`; extend `prepopulateClonedSpecs` for cloned tasks.
- `api/pkg/services/spec_task_attachments.go` (new) — staging logic.
- `api/pkg/services/spec_task_prompts.go` — add `AttachmentsSection` template var + `BuildAttachmentsSection`.
- `api/pkg/controller/filestore.go` — `FilestoreSpecTaskUploadFile`, `IsSpecTaskAttachmentPath`.
- `frontend/src/components/tasks/NewSpecTaskForm.tsx` — attachment widget.
- `frontend/src/components/tasks/TaskAttachmentsPanel.tsx` (new).
- `frontend/src/components/tasks/SpecTaskDetailContent.tsx` — mount the panel.
- `frontend/src/services/taskAttachmentsService.ts` (new) — React-Query hooks.
- `api/pkg/server/swagger.yaml` — regenerated via `./stack update_openapi`.

Roughly: ~12 new files, ~7 modified.

## Implementation Notes (what we learned while building)

- **Service didn't have a `controller` dependency.** `SpecDrivenTaskService` lives in `services/` and deliberately doesn't import `controller/`. Other services there (e.g. `git_repository_service.go`) work with raw filestore paths via a `filestoreBase` string, not via the controller. Solution: added a function field `ReadAttachmentBlob AttachmentBlobReader` on the service struct, wired from `server.go` (`apiServer.readSpecTaskAttachmentBlob`). Mirrors how `GetProjectSecrets`, `ExecInDesktop`, `RegisterRequestMapping` etc. are wired.
- **Staging is best-effort, not fatal.** The plan said "mark task failed on staging error". On reflection that's the wrong policy — if helix-specs is briefly unreachable, the user shouldn't lose their task. We log a warning and continue; the prompt section still lists the files (paths inside the repo) so the agent can request a re-stage manually if needed. Idempotency via `CommittedSHA` makes a retry path trivial.
- **Multi-file upload sidesteps the typed client.** swag's generated TypeScript `v1SpecTasksAttachmentsCreate(taskId, data)` only allows `files: File` (single). The backend accepts `files[]`. Used raw axios for the multipart POST so we can attach all files in one request. List/delete still use the generated client.
- **Read-only statuses live in the handler, not on the task type.** No need to add a method on `SpecTask` — both `uploadSpecTaskAttachments` and `deleteSpecTaskAttachment` consult a private `specTaskAttachmentsLocked` map.
- **Cloning: don't duplicate the filestore blob.** When cloning a task, the new attachment row reuses the source's `FilestorePath`. The agent reads attachments from the cloned task's helix-specs commit (which has its own copy of the bytes), not from filestore — so this dedupe is safe. If the user replaces a file later via the UI, the new upload writes a fresh blob under the cloned task's prefix.
- **Detail-page panel is below the description, not above.** Plan said "above the description block". In testing, putting it below kept the eye on the task title/description first and treated attachments as supporting context — the more natural reading order.
- **Frontend prod build is blocked by a pre-existing root-owned `frontend/dist/` directory.** Not caused by this change. `yarn tsc` clean. Vite HMR is the dev path; production builds work in CI where dist isn't pre-staged.
- **No `presigned=true` query param on the content endpoint.** Plan called for it; turned out unnecessary because the regular content endpoint is already streamed cheap and the public-design-docs gate is enough for anonymous shares. Dropping it kept the surface smaller.

### Files actually touched

Backend (Go):
- `api/pkg/types/simple_spec_task.go` — new `SpecTaskAttachment` type + constants.
- `api/pkg/system/uuid.go` — `SpecTaskAttachmentPrefix` + `GenerateSpecTaskAttachmentID`.
- `api/pkg/store/store.go` — interface additions.
- `api/pkg/store/store_spec_task_attachments.go` (new) — CRUD.
- `api/pkg/store/store_mocks.go` — regenerated.
- `api/pkg/store/postgres.go` — AutoMigrate registration.
- `api/pkg/filestore/filestore.go` — `GetSpecTaskAttachmentsPrefix`.
- `api/pkg/controller/filestore.go` — `FilestoreSpecTaskAttachment*` helpers.
- `api/pkg/services/spec_driven_task_service.go` — `AttachmentBlobReader` type + field; call `stageAttachmentsAndBuildPromptSection` from `StartSpecGeneration`; extended `prepopulateClonedSpecs` to also copy attachments.
- `api/pkg/services/spec_task_attachments.go` (new) — staging + clone helpers.
- `api/pkg/services/spec_task_prompts.go` — `AttachmentsSection` field + template var + `BuildAttachmentsSection` + `humanSize`. `BuildPlanningPrompt` got a 5th parameter.
- `api/pkg/services/spec_task_prompts_test.go` — updated existing test for new signature.
- `api/pkg/services/spec_task_attachments_prompt_test.go` (new) — unit tests for prompt section.
- `api/pkg/server/spec_task_attachments_handlers.go` (new) — REST handlers.
- `api/pkg/server/spec_task_attachments_validation_test.go` (new) — unit tests.
- `api/pkg/server/spec_driven_task_handlers.go` — `deleteSpecTask` GCs attachment rows + filestore prefix.
- `api/pkg/server/server.go` — route registration (3 routes on authRouter, 1 on subRouter for public reads), wired `ReadAttachmentBlob` callback.
- `api/pkg/server/swagger.{json,yaml}`, `docs.go` — regenerated.

Frontend (TypeScript):
- `frontend/src/api/api.ts` — regenerated.
- `frontend/src/services/specTaskAttachmentsService.ts` (new) — hooks + constants.
- `frontend/src/components/tasks/TaskAttachmentsPanel.tsx` (new) — grid + lightbox.
- `frontend/src/components/tasks/SpecTaskDetailContent.tsx` — mount panel.
- `frontend/src/components/tasks/NewSpecTaskForm.tsx` — attach button + chip strip; upload after task create.
