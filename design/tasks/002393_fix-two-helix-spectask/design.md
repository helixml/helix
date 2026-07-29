# Design: Fix helix spectask start -n/--name and Atomic --attach Uploads

## Codebase notes (verified at commit `8b1a5ce29`)

Findings worth keeping for anyone touching this area later:

- **The CLI POSTs a loose `map[string]string`, not a typed struct** (`api/pkg/cli/spectask/spectask.go:500`). That is *why* the name was silently dropped: `encoding/json` discards unknown fields on decode, and nothing on either side declares the contract. The CLI lives in the same module as `api/pkg/types`, so it can simply use the real request struct — that is the root fix, not just adding a field.
- **Task title rendering:** `frontend/src/components/tasks/TabsView.tsx` uses `user_short_title || short_title || name` in every place a title is shown (lines 805, 956, 1394, 1423, 1854). So `Name` alone is sufficient; no `short_title` write is needed.
- **`GenerateTaskNameFromPrompt`** (`api/pkg/services/spec_driven_task_service.go:1794`) collapses whitespace and truncates to 60 runes (rune-safe — there's already a regression test for byte truncation at `spec_driven_task_service_test.go:255`).
- **Attachment rollback primitives already exist:** `Controller.FilestoreSpecTaskAttachmentDelete(absolutePath)` (`api/pkg/controller/filestore.go:318`) and `Store.DeleteSpecTaskAttachment`. No new plumbing needed for rollback.
- **Attachment limits:** 100 MB/file, 500 files/task (`api/pkg/types/simple_spec_task.go:460-461`). `ParseMultipartForm` is deliberately given one file's worth of memory; don't scale it with the per-task cap.
- **Uploads are staged into the `helix-specs` git repo** by `StageUploadedAttachments` (`api/pkg/services/spec_task_attachments.go:71`) right after the loop — so anything committed to the DB also lands in git. Another reason partial batches are bad.
- **The dev stack is at `localhost:8080`, never `api:8080`** (per repo `CLAUDE.md`). At spec time no stack was running locally — the implementer must `./stack start` before the end-to-end verification. Air hot-reloads Go changes.

## Bug 1 — `-n/--name`

### Change 1a: type the request (root fix)

Add to `types.CreateTaskRequest` (`api/pkg/types/simple_spec_task.go`):

```go
Name string `json:"name,omitempty"` // Optional: explicit task name; empty ⇒ derived from prompt
```

Replace the CLI's `map[string]string` payload in `createSpecTask` with `types.CreateTaskRequest`
so the compiler enforces the contract from now on. This is the change that
actually prevents recurrence — the contract test below is the backstop.

### Change 1b: honour it in the service

`SpecDrivenTaskService.CreateTaskFromPrompt` (`spec_driven_task_service.go:196`):

```go
Name: resolveTaskName(req.Name, req.Prompt),
```

with a small helper in the same package:

```go
func resolveTaskName(name, prompt string) string {
    name = strings.TrimSpace(name)
    if name == "" {
        return GenerateTaskNameFromPrompt(prompt)
    }
    // collapse whitespace/newlines the same way, cap length
    ...
}
```

Empty/whitespace-only ⇒ existing behaviour, so the frontend and every other
caller is untouched. Also strip null bytes from `Name` in the handler alongside
the existing `req.Prompt` strip (Postgres SQLSTATE 22021).

`short_title` / `user_short_title` are deliberately left alone — see
requirements Open Question 1.

### Change 1c: flag default

`spectask.go:217`: `StringVarP(&taskName, "name", "n", "", "Task name (default: derived from the prompt)")`.

### Change 1d: contract test

`api/pkg/cli/spectask/payload_contract_test.go` — marshal the CLI's request
struct, and assert every JSON key it emits exists as a JSON tag on
`types.CreateTaskRequest` (reflection over the struct's tags). Once the CLI uses
the typed struct this is trivially true, but the test locks it in for future
hand-rolled maps. Audit the other CLI payloads while here
(`interact.go:241`, `spectask.go:1742`, `benchmark_cmd.go:460`) and note or fix
any that hit endpoints with typed request structs.

We do **not** enable `DisallowUnknownFields` on `/from-prompt` — it would reject
older CLI binaries and any client sending an extra key. Open Question 4.

## Bug 2 — atomic `--attach`

### Change 2a: split validate from commit

Rewrite `uploadSpecTaskAttachments` (`api/pkg/server/spec_task_attachments_handlers.go:104-170`)
as two passes over `r.MultipartForm.File["files"]`:

**Pass 1 — validate everything, write nothing.** For each file: size, sanitised
filename, read body, mime detect, allowlist, SVG-script check. Collect
`[]fileError{Filename, Reason}` instead of returning on the first failure. Also
fold in the per-task count check. Keep the decoded body in memory per file
(already bounded: `Size` is checked before `io.ReadAll`, and the per-task cap ×
100 MB is why the parser spills to disk — if holding N bodies in memory is a
concern, re-open each `*multipart.FileHeader` in pass 2 instead of caching, at
the cost of a second read).

If `len(fileErrors) > 0`: respond `400` with a JSON body listing all of them:

```json
{"error": "3 of 7 attachments rejected", "rejected": [
  {"filename": "leaker-full.log.gz", "reason": "unsupported mime type: application/x-gzip"},
  ...
]}
```

Status code: `400` for any validation failure, including oversize — the existing
`413` is dropped for batches, since one response can now carry mixed reasons.
Keep `413` when oversize is the *only* class of failure if it matters to a
client; simplest is 400 everywhere, and no current caller branches on 413.

**Pass 2 — commit.** Only reached when pass 1 is clean. Same
filestore-then-DB-row sequence as today.

### Change 2b: roll back mid-write failures

Genuine filestore/DB errors can still hit pass 2. On any error there, undo what
this request created, in reverse order:

- for each already-created row: `Store.DeleteSpecTaskAttachment`
- for each already-written blob (including the one whose DB insert failed):
  `Controller.FilestoreSpecTaskAttachmentDelete(item.Path)`

Then `500` with a message saying the batch was rolled back. If a rollback step
itself fails, log the leaked path at ERROR and include the leaked filenames in
the response — never leave the caller guessing. Chosen over `207 Multi-Status`
because a partial success the client must parse is precisely the failure this
task exists to remove.

Note: `StageUploadedAttachments` runs only after a fully successful commit, so
rollback needs no git-side undo.

### Change 2c: CLI reporting + preflight

`uploadSpecTaskAttachments` in the CLI (`spectask.go:544`): parse the structured
error body and print one line per rejected file. Then, because the task is
created *before* the upload (`spectask.go:118` then `:133`), an outright
rejection leaves an empty backlog task — the CLI must say so and print the
task ID.

Add a local preflight in the CLI **before** `createSpecTask`: for each
`--attach` path check readability and size against
`types.SpecTaskAttachmentMaxBytes`, and check the extension/sniffed mime against
`types.SpecTaskAttachmentAllowedMimeTypes` — importing the shared constants, not
copying them. This means the common case (a bad file) never creates a task at
all. The server-side check stays canonical.

### Change 2d: allow compressed archives

In `types.SpecTaskAttachmentAllowedMimeTypes` add `application/gzip`,
`application/x-gzip` (what `http.DetectContentType` actually returns for gzip)
and `application/zip`. In `detectAttachmentMime` map `.gz`/`.tgz` →
`application/gzip` and `.zip` → `application/zip` so the canonical name is
stored regardless of sniffing.

Rationale: nothing server-side ever decompresses these, so there is no
zip-bomb/path-traversal surface — the blob is stored and served verbatim, and
the agent unpacks it itself inside its own sandbox. The 100 MB per-file cap is
unchanged. Extend the SVG `Content-Disposition: attachment` branch to cover
archive types too (`inline` for an archive is pointless and mildly risky).

Update the `--attach` help text with the caps and formats.

## Minor — archive/delete messages

`archiveSpecTask` (`spec_driven_task_handlers.go:1313`): treat an empty body as
`{"archived": true}` — read the body, and only attempt `json.Unmarshal` when it
is non-empty. A non-empty malformed body still 400s, with
`invalid request body: expected {"archived": true|false}`.

Delete guard (`:1274`): `task is not archived — archive it first: PATCH /api/v1/spec-tasks/{id}/archive {"archived":true}, then retry DELETE`.

## Testing

Unit/integration (Go):
- `resolveTaskName`: explicit name wins; empty/whitespace falls back; newlines collapsed; over-long truncated.
- CLI-payload contract test (Change 1d).
- Multi-file upload handler test: 3 files, middle one an unsupported mime ⇒ 400, response lists that file, **`ListSpecTaskAttachments` returns 0 rows and the filestore prefix is empty**.
- Same shape with two bad files ⇒ both listed in one response.
- Happy path with a `.gz` ⇒ 201, mime stored as `application/gzip`.
- Rollback test: inject a `CreateSpecTaskAttachment` failure on file 2 of 3 ⇒ 500, 0 rows, 0 blobs.
- Archive handler: empty body ⇒ 200 + archived; garbage body ⇒ 400.

End-to-end against the running stack (`./stack start`, `http://localhost:8080` — **not** `api:8080`):
1. `helix spectask start -n "Spec 002393 name test" --prompt "<long prompt>"` ⇒ verify `SELECT name, short_title FROM spec_tasks WHERE id='…'` shows exactly `Spec 002393 name test`.
2. Same without `-n` ⇒ name still derived from prompt.
3. `--attach` with 4 valid files + 1 rejected ⇒ command fails, all rejects listed, then verify `SELECT count(*) FROM spec_task_attachments WHERE spec_task_id='…'` is `0` and the filestore attachments dir for that task is absent/empty.
4. `--attach` with a real `.log.gz` ⇒ uploads, appears under `design/tasks/<task>/attachments/`.
5. `curl -X PATCH …/archive` with no body ⇒ 200.

Per repo rules: anything not actually run must be stated as untested, explicitly.
