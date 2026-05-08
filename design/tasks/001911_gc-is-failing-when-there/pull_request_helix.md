# fix(hydra): use `zfs destroy -r` in CleanupSessionZvol so manual snapshots don't block GC

## Summary

`GCOrphanedZvols` was failing to clean up session zvols whenever an operator had created a snapshot on the zvol out of band (e.g. `@pre-repo-cleanup-2026-03-31`). ZFS refuses plain `zfs destroy` on a dataset with children, so those orphans accumulated forever and the warning repeated every GC cycle:

```
sandbox-nvidia-1  | [HYDRA] 8:14AM WRN Failed to GC orphaned zvol
  error="failed to destroy clone prod/helix-zvols/ses-ses_01kmqtedcrk20w85qqfnwbqvj9:
  zfs destroy ...: exit status 1
  (output: cannot destroy '...': volume has children
   use '-r' to destroy the following datasets:
   prod/helix-zvols/ses-ses_01kmqtedcrk20w85qqfnwbqvj9@pre-repo-cleanup-2026-03-31)"
  session_id=ses_01kmqtedcrk20w85qqfnwbqvj9
```

Fix: pass `-r` to `zfs destroy` in `CleanupSessionZvol`.

## Changes

- `api/pkg/hydra/golden_zvol.go` — one-line change: `runCmd("zfs", "destroy", cloneName)` → `runCmd("zfs", "destroy", "-r", cloneName)`. Doc-comment expanded to explain the `-r` choice and the `-R` alternative we explicitly rejected.
- `api/pkg/hydra/golden_zvol_test.go` — new `TestCleanupSessionZvol_WithChildSnapshot` covers the bug; existing assertions updated to expect the `-r` form (5 sites).

## Why `-r` and not `-R`

| Flag | Behaviour |
|------|-----------|
| `-r` | Destroy + descendant snapshots. Fails if a snapshot has a *clone* outside the dataset. |
| `-R` | Also destroys those clones. Could silently nuke an unrelated session zvol that happens to be cloned from one of these snapshots. |

By the time `CleanupSessionZvol` runs the session is already known to be gone (GC: >7 days inactive; failed-build path: build just failed; golden deletion: project is being torn down), so taking incidental snapshots with it is correct. But silently nuking *clones* of those snapshots would be a footgun — if `-r` fails because of a dependent clone, the existing fail-soft behaviour in `GCOrphanedZvols` logs and moves on, and the operator deals with the dependent clone explicitly. Same pattern already used at `golden_zvol.go:593` for the post-promote old-golden cleanup.

## Test plan

- [x] `cd api && go build ./pkg/hydra/ ./pkg/store/ ./pkg/types/` — clean.
- [x] `cd api && go test -v -run TestGoldenZvolSuite ./pkg/hydra/ -count=1` — 53/53 pass, including the new `TestCleanupSessionZvol_WithChildSnapshot`.
- [ ] After deploy: confirm "Failed to GC orphaned zvol ... volume has children" warnings stop appearing in `docker compose logs sandbox-nvidia` for the listed `ses_01kmqtedcrk2...`, `ses_01kmyhx3xw5kx...`, etc.
