# Requirements

## Problem

`GCOrphanedZvols` fails to destroy session zvols when those zvols have child snapshots (e.g. snapshots manually created by a user via `zfs snapshot`). ZFS refuses `zfs destroy` on a dataset with children unless `-r` is passed.

Observed log:
```
Failed to GC orphaned zvol error="failed to destroy clone prod/helix-zvols/ses-...:
  zfs destroy ...: exit status 1 (output: cannot destroy '...': volume has children
  use '-r' to destroy the following datasets:
  prod/helix-zvols/ses-...@pre-repo-cleanup-2026-03-31)"
```

The orphaned zvols accumulate forever, wasting disk and polluting logs every GC cycle.

## User Story

**As a** Helix operator,
**I want** orphaned session zvols to be garbage-collected even when they have user-created child snapshots,
**So that** stale data doesn't accumulate on disk and GC logs stay clean.

## Acceptance Criteria

- [ ] `CleanupSessionZvol` successfully destroys a session zvol that has one or more child snapshots, removing the snapshots along with the zvol.
- [ ] `GCOrphanedZvols` no longer logs "Failed to GC orphaned zvol ... volume has children" warnings for the affected zvols.
- [ ] The behaviour is unchanged for session zvols with no child snapshots (no regression).
- [ ] If destruction still fails for a different reason (e.g. a child snapshot has its own dependent clone, mount busy), GC logs the failure and continues to the next zvol — it does not abort the GC pass.
- [ ] A unit test in `golden_zvol_test.go` exercises the "zvol has child snapshot" path and asserts cleanup succeeds.

## Out of Scope

- Preventing users from creating manual snapshots in the first place.
- Recovering data from manually-created snapshots before destruction (the user-request explicitly says: when GC has decided to GC a zvol, we really do mean it).
- Changing the 7-day staleness threshold or the activity-marker scheme.
- Garbage-collecting golden snapshots (separate `GCStaleSnapshots` path, already handles this gracefully).
