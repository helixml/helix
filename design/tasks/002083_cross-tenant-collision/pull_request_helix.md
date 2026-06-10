# Make per-Worker git repo id collision-proof across orgs

## Summary

Two orgs hiring identically-named workers (e.g. both hire `w-mt`) within the same wall-clock second used to fail the second activation with `git_repositories_pkey` SQLSTATE 23505. `generateRepositoryID` minted `<repoType>-<name>-<unixSeconds>`, and `git_repositories.id` is a single-column global primary key, so identical inputs in the same second produced identical ids and the second `INSERT` aborted.

This swaps the second-granularity timestamp for `system.GenerateID()` (the existing lowercase-ULID helper used for every other entity id in the codebase). The repo id stays human-readable (`code-w-mt-01jx3vqz2j4n8m9p0r5t6w7x8y`), keeps the `code-<name>-` prefix for log grepping, and gains 80 bits of random entropy — collisions become astronomically unlikely without a schema change.

This is the same id-collision class as #2570 (spawner / activation-queue / mirror singletons), one layer down in the git-repo service.

## Changes

- `api/pkg/services/git_repository_service.go` — `generateRepositoryID` now suffixes with `system.GenerateID()` instead of `time.Now().Unix()`; added `api/pkg/system` import; added comment explaining the collision class being closed.
- `api/pkg/services/git_repository_service_test.go` — new `TestGenerateRepositoryID_NoCollisionUnderLoad` mints 10,000 ids with the same `(repoType, name)` and asserts all distinct. Fails immediately on the old implementation; passes on the new one.

## Compatibility

- No DB migration. Existing rows keep their `-<unixSeconds>` ids.
- No client impact. Repo ids are opaque strings everywhere except the service that mints them.
- ULIDs are still sortable by creation time (leading 48-bit ms timestamp), so any implicit alphabetic-equals-chronological sort still holds.

## Test plan

- [x] `go test -run TestGenerateRepositoryID -count=1 ./api/pkg/services/...` — green locally.
- [x] `go vet ./api/pkg/services/...` — clean.
- [ ] Manual e2e: in a fresh stack, hire `w-mt` into two different orgs back-to-back via the helix-org MCP `hire_worker` tool. Confirm both activations succeed and no `git_repositories_pkey` errors in `helix-api-1` logs.
