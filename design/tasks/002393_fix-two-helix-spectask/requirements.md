# Requirements: Fix helix spectask start -n/--name and Atomic --attach Uploads

## Background

Two bugs in `helix spectask start`, both hit for real while dispatching an
incident-response task on 2026-07-29. Every claim in the brief was verified
against the code at commit `8b1a5ce29`:

| Claim | Verified |
|---|---|
| `spectask.go:217` registers `-n` with default `"CLI Test Task"` | ✅ exact |
| `spectask.go:500-502` puts `name` in the POST payload | ✅ `payload := map[string]string{"name": name, ...}` |
| `spec_driven_task_handlers.go:115` decodes into `types.CreateTaskRequest` | ✅ `json.NewDecoder(r.Body).Decode(&req)` |
| `types.CreateTaskRequest` has no `Name` field | ✅ `simple_spec_task.go:86-114` — no `Name` |
| Server derives name via `GenerateTaskNameFromPrompt` | ✅ `spec_driven_task_service.go:196` |
| Attachment upload validates *and* commits in one loop | ✅ `spec_task_attachments_handlers.go:104-170` |
| Five early-returns can fire mid-batch | ✅ oversize (×2), bad filename, unsupported mime, SVG-with-script, filestore error, DB error |
| `created` slice discarded on every early return | ✅ only encoded on the success path (line 189) |
| Gzip is rejected | ✅ `SpecTaskAttachmentAllowedMimeTypes` = 5 image types + pdf + plain/markdown/csv |

## User Stories

### 1. Name a task from the CLI

**As a** user dispatching a task with `helix spectask start`,
**I want** `-n "Fix desktop-bridge GPU leak"` to actually name the task,
**so that** the task list shows a readable title instead of 57 characters of prompt.

Acceptance criteria:
- `helix spectask start -n "My Task" --prompt "..."` creates a task whose `name` is exactly `My Task`.
- Omitting `-n` still derives the name from the prompt via `GenerateTaskNameFromPrompt` (unchanged behaviour for the frontend and every other caller).
- `-n ""` or whitespace-only is treated as "not supplied".
- The flag default changes from `"CLI Test Task"` to `""`.
- Names longer than the DB column are handled without a 500 (see design).

### 2. All-or-nothing attachment uploads

**As a** user running `--attach a.log --attach b.gz --attach c.md`,
**I want** a batch with any invalid file to upload **nothing**,
**so that** I never get a half-populated task I have to archive and delete by hand.

Acceptance criteria:
- A batch where ≥1 file fails validation writes **zero** rows to `spec_task_attachments` and **zero** blobs to the filestore.
- The 400 response lists **every** offending file with its reason, not just the first.
- A fully valid batch behaves exactly as today (201 + array of created attachments).
- Mid-write failures (filestore/DB) roll back everything already written in that request; if rollback itself fails, the response says so explicitly and names the leaked paths.
- The CLI prints every rejected file and its reason, and tells the user what state the task is in.

### 3. Compressed logs can be attached

**As a** user attaching an 82 MB log,
**I want** to send `leaker.log.gz`,
**so that** I don't have to hand-filter it down to fit an allowlist that excludes the one format built for large logs.

Acceptance criteria:
- `application/gzip`, `application/x-gzip` and `application/zip` are accepted.
- `.gz`, `.tgz`, `.zip` map to the right mime type regardless of what `http.DetectContentType` sniffs.
- Archives are served with `Content-Disposition: attachment`, never `inline`.
- The `--attach` help text states the size cap, the per-task cap and the accepted formats.

### 4. Archive/delete errors say what to do

**As an** API caller,
**I want** `PATCH /spec-tasks/{id}/archive` with an empty body to work (or say why not),
**so that** I'm not guessing at an undocumented `{"archived":true}`.

Acceptance criteria:
- Empty body on `PATCH …/archive` defaults to `archived=true` and succeeds.
- A malformed (non-empty, non-JSON) body still 400s, with a message naming the expected shape.
- The delete guard message tells the caller the exact two-step call to make.

### 5. No more silently-dropped fields

**As a** maintainer,
**I want** a test that fails when the CLI sends a field the server struct doesn't define,
**so that** this class of bug can't recur silently.

Acceptance criteria:
- A contract test asserts every key in the CLI's `/from-prompt` payload has a matching JSON tag on `types.CreateTaskRequest`.
- Any other CLI→API payload found to send undefined fields is either fixed or documented.

## Out of Scope

- Server-side decompression or inspection of archive contents.
- A standalone `helix spectask attach` subcommand (does not exist today).
- Changing how the agent generates `short_title` after planning.

## Open Questions

1. **`short_title` when `-n` is given.** The UI renders `user_short_title || short_title || name` (`frontend/src/components/tasks/TabsView.tsx:805,956,1394`), so fixing `Name` alone is enough for the title to display correctly. The design therefore sets **only** `Name` and leaves `short_title`/`user_short_title` empty, so the agent's auto-generated short title still wins for tab display, as it does for frontend-created tasks. If you would rather `-n` pin the tab title permanently, we should set `UserShortTitle` instead — say so at review.
2. **Rollback vs 207 for mid-write failures.** Design picks **rollback + 500** (never 207), because a partial success the caller must parse is exactly the failure mode being fixed. Confirm you don't want 207.
3. **Zip/gzip security.** Accepted on the grounds that nothing server-side ever decompresses them (no zip-bomb surface) and they're served as downloads. Note this does allow a 100 MB archive into the `helix-specs` git repo via attachment staging — the same exposure a 100 MB `.txt` already has. Flag if the repo-bloat angle changes your view.
4. **Strict decoding on `/from-prompt`.** The design adds a *test*, not `DisallowUnknownFields`, because turning strict decoding on would break any existing client sending an extra key (including older CLI binaries in the field). Confirm that's the tradeoff you want.
5. **Truncating over-long `-n` values.** `SpecTask.Name` has no explicit size tag but `short_title` is `size:100`; the design truncates `Name` at 200 runes to stay clear of any column limit. Adjust if there's a canonical cap.
