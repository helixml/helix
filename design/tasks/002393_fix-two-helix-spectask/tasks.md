# Implementation Tasks: Fix helix spectask start -n/--name and Atomic --attach Uploads

## Bug 1 — `-n/--name`

- [ ] Add `Name string \`json:"name,omitempty"\`` to `types.CreateTaskRequest` (`api/pkg/types/simple_spec_task.go`)
- [ ] Add `resolveTaskName(name, prompt)` helper and use it at `api/pkg/services/spec_driven_task_service.go:196` (falls back to `GenerateTaskNameFromPrompt`, trims/collapses whitespace, caps length)
- [ ] Strip null bytes from `req.Name` in `createTaskFromPrompt` alongside the existing `req.Prompt` strip
- [ ] Replace the CLI's `map[string]string` payload in `createSpecTask` (`api/pkg/cli/spectask/spectask.go:500`) with `types.CreateTaskRequest`
- [ ] Change the `-n` flag default from `"CLI Test Task"` to `""` and update its help text (`spectask.go:217`)
- [ ] Add CLI-payload/server-struct contract test (`api/pkg/cli/spectask/payload_contract_test.go`)
- [ ] Audit other CLI payloads (`interact.go:241`, `spectask.go:1742`, `benchmark_cmd.go:460`) for the same silent-drop class; fix or document findings

## Bug 2 — atomic `--attach`

- [ ] Rewrite `uploadSpecTaskAttachments` (`api/pkg/server/spec_task_attachments_handlers.go:104-170`) as validate-all-then-commit
- [ ] Return `400` with a JSON body listing **every** rejected file and reason
- [ ] Add rollback of blobs + rows on mid-write filestore/DB failure; `500` on rollback, naming any path that could not be cleaned up
- [ ] Parse the structured rejection body in the CLI uploader and print one line per rejected file, plus the created task's ID/state
- [ ] Add a CLI-side preflight (readable, size, mime allowlist via shared `types` constants) before `createSpecTask` so bad input never creates a task

## Compressed attachments

- [ ] Add `application/gzip`, `application/x-gzip`, `application/zip` to `types.SpecTaskAttachmentAllowedMimeTypes`
- [ ] Map `.gz`/`.tgz`/`.zip` extensions in `detectAttachmentMime`
- [ ] Force `Content-Disposition: attachment` for archive mime types in the download handler
- [ ] Update the `--attach` help text with size cap, per-task cap and accepted formats

## Minor — archive/delete messages

- [ ] Treat an empty `PATCH /spec-tasks/{id}/archive` body as `{"archived": true}`; keep 400 for malformed non-empty bodies with an explanatory message
- [ ] Improve the delete guard message to spell out the archive-then-delete sequence

## Tests

- [ ] Unit tests for `resolveTaskName` (explicit wins, empty falls back, whitespace, truncation)
- [ ] Handler test: multi-file batch with one bad file ⇒ 400, all rejects listed, 0 rows and 0 blobs
- [ ] Handler test: two bad files ⇒ both reported in one response
- [ ] Handler test: rollback on injected DB failure mid-commit ⇒ 500, 0 rows, 0 blobs
- [ ] Handler test: `.gz` happy path ⇒ 201 with `application/gzip`
- [ ] Handler tests for archive with empty and malformed bodies
- [ ] `go test ./api/pkg/server/... ./api/pkg/services/... ./api/pkg/cli/...` green

## End-to-end verification (running stack at `localhost:8080`)

- [ ] `./stack start` and confirm the API is up on `localhost:8080` (never `api:8080`)
- [ ] Create a task with `-n "…"` ⇒ DB `name` matches exactly
- [ ] Create a task without `-n` ⇒ name still derived from the prompt
- [ ] `--attach` with 4 valid + 1 invalid file ⇒ all rejects reported; `spec_task_attachments` has 0 rows and the task's filestore dir is empty
- [ ] `--attach` a real `.log.gz` ⇒ uploads and appears under `design/tasks/<task>/attachments/`
- [ ] `curl -X PATCH …/archive` with no body ⇒ 200
- [ ] Record results in the PR description; state explicitly anything that could not be tested
