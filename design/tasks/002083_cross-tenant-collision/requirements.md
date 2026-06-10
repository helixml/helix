# Requirements: Make per-Worker Git Repo ID Collision-Proof Across Orgs

## Background

When two different orgs hire a worker with the same id (e.g. `w-mt`) within
the same wall-clock second, the second org's desktop activation fails with:

```
err="create per-Worker repo: create git repo: 500 Internal Server Error:
  Failed to create repository: ERROR: duplicate key value violates unique
  constraint \"git_repositories_pkey\" (SQLSTATE 23505)"
repo_id=code-w-mt-1781019169
```

The org-graph rows (`org_roles`, `org_workers`) land correctly — both orgs are
isolated. The defect is one layer down, in the git-repo service: per-Worker
repo ids are minted as `<repoType>-<sanitizedName>-<unixSeconds>`, and
`git_repositories.id` is a **global** primary key (not org-scoped). Two orgs
hiring identically-named workers in the same second mint the identical id and
collide on insert.

This is the same id-collision class as #2570 (spawner / activation-queue /
mirror singletons leaking across orgs), one layer down. It is not a data leak
— the second org's row simply fails to be created — but it is a hard,
user-visible activation failure with a misleading 500 error.

## User stories

**As an operator of multi-tenant Helix**, when two of my orgs hire workers
with the same id at the same time, both orgs' desktops should provision
successfully on the first try. Today the second one fails with a 5xx and a
foreign-key-shaped error message that is hard to map back to "wait one
second and retry".

**As a developer working on the org runtime**, when I write tests that
fire identical `hire_worker` calls at two orgs back-to-back, the test
should reflect the production behaviour I want — both succeed — rather than
catching the SQLSTATE 23505 and treating it as expected.

## Acceptance criteria

1. **No collision under load.** Calling `generateRepositoryID` 10,000 times
   in tight succession (with arbitrary repeated `name` inputs) produces
   10,000 distinct ids. No `time.Now().Unix()`-shaped second-granularity
   collisions.

2. **Cross-org repro from the bug report passes.** Two orgs each hiring a
   worker with id `w-mt` back-to-back via the helix-org MCP `hire_worker`
   tool both successfully provision a per-Worker repo and a desktop.
   No `git_repositories_pkey` violation in the API logs for either
   activation.

3. **Id is still human-debuggable.** A repo id of the form
   `code-<workerID>-<suffix>` keeps its `code-` prefix and the worker id,
   so an operator grepping logs for `code-w-mt-` continues to find the
   relevant repo. Only the trailing time-based segment changes.

4. **Backward compatible with existing rows.** Existing repos in the
   `git_repositories` table keep their current ids; the change only
   affects newly-generated ids. No migration required.

5. **Regression test in place.** A unit test in
   `api/pkg/services/git_repository_service_test.go` covers the
   colliding-id case (same name + same logical instant, no collision).

## Out of scope

- Changing `git_repositories.id` to a composite `(id, org_id)` primary key.
  This would be a much larger schema change and is not needed if ids are
  globally collision-proof.
- Renaming or backfilling existing repo ids.
- The unrelated `coord_%d_%d`, `event_%d`, `test_*_%d` Unix-time id
  generators elsewhere in the codebase (grep finds several). They are
  scoped to test or short-lived contexts; if any later turns out to
  collide, fix it in a separate task.
