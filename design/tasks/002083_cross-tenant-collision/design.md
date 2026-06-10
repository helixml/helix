# Design: Make per-Worker Git Repo ID Collision-Proof Across Orgs

## Where the bug lives

`api/pkg/services/git_repository_service.go:1117-1124`:

```go
func (s *GitRepositoryService) generateRepositoryID(repoType types.GitRepositoryType, name string) string {
    sanitizedName := strings.ReplaceAll(strings.ToLower(name), " ", "-")
    sanitizedName = strings.ReplaceAll(sanitizedName, "_", "-")

    timestamp := time.Now().Unix()
    return fmt.Sprintf("%s-%s-%d", repoType, sanitizedName, timestamp)
}
```

`GitRepository.ID` is `gorm:"primaryKey"` (single-column PK) in
`api/pkg/types/git_repositories.go:40`, so the `git_repositories_pkey`
constraint enforces global uniqueness on `id`. Two callers passing
`(code, "w-mt")` within the same Unix second compute identical ids and the
second `INSERT` aborts with SQLSTATE 23505.

The org runtime call site is
`api/pkg/org/infrastructure/runtime/helix/project.go:271` —
`a.Service.CreateGitRepo(...)` → `CreateRepository` →
`generateRepositoryID`. Per-Worker repo names are simply the worker id
(`string(workerID)`), which is shared across orgs (Workers like `w-mt`,
`w-owner` are common defaults), so the collision is structurally easy to
hit, not a rare edge case.

## Options considered

1. **Composite primary key `(id, org_id)`.** Real fix in principle, but
   touches every query that loads a repo by id (currently keyed on a
   string), every foreign key into `git_repositories` (e.g. project↔repo
   attachments), and ties the repo service tightly to org semantics. Large
   blast radius for a defect that has nothing to do with cross-org auth.
2. **Prefix the org id into the repo id (`code-<orgID>-<workerID>-<ts>`).**
   Removes the same-second cross-org collision but does not solve
   same-org / same-second collisions (still time-based) and bloats the id.
3. **Replace the second-granularity timestamp with a ULID.** Collision
   resistance is `2^80` random bits + monotonic-within-millisecond
   ordering; debug-friendly (Crockford Base32, 26 chars, sortable);
   uses the existing `system.GenerateID()` helper. No schema change,
   no migration, smallest diff.

**Chosen: option 3.** It is the smallest change, removes the time-based
fragility entirely, and matches how every other entity in the codebase
already generates ids — `system.GenerateID()` (lowercased ULID) is the
canonical id generator for sessions, apps, tools, sandboxes, etc.
(`api/pkg/system/uuid.go:61-67`).

## Proposed change

### `generateRepositoryID`

```go
func (s *GitRepositoryService) generateRepositoryID(repoType types.GitRepositoryType, name string) string {
    sanitizedName := strings.ReplaceAll(strings.ToLower(name), " ", "-")
    sanitizedName = strings.ReplaceAll(sanitizedName, "_", "-")

    return fmt.Sprintf("%s-%s-%s", repoType, sanitizedName, system.GenerateID())
}
```

That's the whole behaviour change. `time` import becomes unused in this
function but is still used elsewhere in the file (`LastActivity`,
`CreatedAt`, etc.) — no import change needed.

### Format example

Before: `code-w-mt-1781019169` (19 chars after the name)
After:  `code-w-mt-01jx3vqz2j4n8m9p0r5t6w7x8y` (26 char ULID)

The id stays human-readable, still starts with `code-w-mt-`, and is now
sortable by creation time via ULID's leading millisecond timestamp — a
nice side effect of dropping `time.Now().Unix()` for `ulid.Make()`.

### Why not also drop the name-suffix retry loop

`CreateSampleRepository` (lines 1047-1105) already auto-increments the
**user-facing repo name** (`helix` → `helix-2` → `helix-3`) on conflict.
That logic is about display name uniqueness within an `(org, owner)` pair,
not internal id uniqueness, so it stays.

Per-Worker repos created by the spawner do not go through that retry loop
(they call `CreateRepository` directly, not `CreateSampleRepository`),
which is why this collision is fatal in that path — there is no second
attempt to mask it. After the ULID change, the spawner path no longer
needs a retry loop because collision probability is effectively zero.

## Test strategy

Add a small, deterministic test alongside the other `generateRepositoryID`
behaviour in `api/pkg/services/git_repository_service_test.go`:

```go
func TestGenerateRepositoryID_NoCollisionUnderLoad(t *testing.T) {
    s := &GitRepositoryService{}
    seen := make(map[string]struct{}, 10000)
    for i := 0; i < 10000; i++ {
        id := s.generateRepositoryID(types.GitRepositoryTypeCode, "w-mt")
        if _, dup := seen[id]; dup {
            t.Fatalf("duplicate id minted at iteration %d: %s", i, id)
        }
        seen[id] = struct{}{}
    }
}
```

This is the regression test that would have caught the original bug: with
the old `time.Now().Unix()` implementation it fails immediately (10,000
calls in <1s all share one timestamp); with the ULID implementation it
passes.

Higher-up integration is covered by the existing
`api/pkg/org/infrastructure/runtime/helix/project_test.go` Ensure path —
no new wiring needed there because the bug is purely in the id-mint
function and the existing tests already exercise the full apply flow.

## Migration and rollout

- **No DB migration.** Existing repos keep their `-<unixSeconds>` ids;
  they are already in the table and continue to work. The only externally
  observable change is the format of *new* ids minted after deploy.
- **No coordination across nodes.** ULID generation is purely local —
  even if two API replicas mint a repo id in the same millisecond, the
  80-bit random component makes a collision astronomically unlikely.
- **No client-side code reads the id format.** Frontend and Zed treat
  repo ids as opaque strings, so widening from `-\d+` to `-[0-9a-z]{26}`
  is invisible upstream.

## Risks

- **Sorting by id.** Any code path that implicitly sorted repos by id
  (alphabetic = chronological under the old format) keeps working because
  ULIDs are also sortable, just on a different alphabet. No code in
  `api/pkg/services/` or `api/pkg/store/` was found relying on this.
- **Log grepping.** Operators who memorised the `code-<name>-<digits>`
  shape will see `code-<name>-<ulid>` going forward. The `code-` prefix
  and worker name are unchanged, so `grep code-w-mt` still works.

## Related

- #2570 — same id-collision class, fixed at the spawner / activation
  queue / mirror / streamhub layer. This task closes the equivalent
  defect in the git-repo service.
