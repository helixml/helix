# Design

## Root Cause

`CleanupSessionZvol` at `api/pkg/hydra/golden_zvol.go:510` runs:

```go
if err := runCmd("zfs", "destroy", cloneName); err != nil {
    return fmt.Errorf("failed to destroy clone %s: %w", cloneName, err)
}
```

`zfs destroy` (no flags) refuses to destroy a dataset that has children. When an operator (or a debugging script) ran `zfs snapshot prod/helix-zvols/ses-...@pre-repo-cleanup-2026-03-31` against a session zvol, that snapshot becomes a permanent blocker for GC — every cleanup pass fails the same way.

The user's intent is explicit: when GC has determined a session is orphaned (>7 days inactive, or pre-marker session), the operator wants the cleanup to win regardless of incidental snapshots.

## Fix

Pass `-r` (recursive) to `zfs destroy` in `CleanupSessionZvol`:

```go
if err := runCmd("zfs", "destroy", "-r", cloneName); err != nil {
    return fmt.Errorf("failed to destroy clone %s: %w", cloneName, err)
}
```

`-r` destroys the dataset together with its descendant snapshots. This is the documented ZFS-recommended remediation for the exact error the logs show.

### Why `-r` and not `-R`

| Flag | Behaviour |
|------|-----------|
| `-r` | Destroy the dataset + its descendant snapshots. Fails if a snapshot has a *clone* outside the dataset. |
| `-R` | Also destroys clones of those snapshots, recursively. Much more dangerous — could nuke an unrelated session zvol that happens to be cloned from one of these snapshots. |

We pick `-r`. Session zvols are themselves clones of golden snapshots; nobody clones *from* a session zvol's snapshot in normal operation. If someone has manually done so, failing loudly is the right answer — `-R` would silently destroy their work.

## Callers — All Three Want This Behaviour

`CleanupSessionZvol` has three call sites; recursive destroy is the right call for all of them:

| Caller | File:Line | Context |
|--------|-----------|---------|
| `GCOrphanedZvols` | `golden_zvol.go:789` | Session is orphaned/stale. Definitely want it gone. |
| Golden-build failure cleanup | `devcontainer.go:1800` | Failed build — clean up the session's docker data zvol. Same intent. |
| Pre-golden-deletion cleanup | `golden.go:486` | Operator deleted the project's golden cache; we destroy stopped session clones first. Same intent. |

So we change the function itself, not each caller. No need for a new flag/parameter.

## Existing Pattern Match

`PromoteSessionToGoldenZvol` at `golden_zvol.go:583` already uses `zfs destroy -r` for the analogous cleanup of an old golden:

```go
if err := runCmd("zfs", "destroy", "-r", goldenName); err != nil { ... }
```

So this is consistent with how the codebase already handles the "destroy zvol that may have inherited snapshots" case.

## Edge Case — Snapshot Has a Dependent Clone

If a user manually created `prod/helix-zvols/ses-X@manual-snap` *and then* `zfs clone`d it into another dataset, `-r` will still fail with `dataset is busy`. Behaviour in that case:

1. `CleanupSessionZvol` returns the wrapped error.
2. `GCOrphanedZvols` (line 790) logs the warning and `continue`s — does not abort the GC pass. This is the existing fail-soft behaviour and is correct.
3. The zvol remains until the operator manually removes the dependent clone.

This is the right outcome — we don't want to silently destroy unrelated zvols. The error message is informative (it lists the busy dataset).

## Logging

Update the success log in `CleanupSessionZvol` (currently line 517–520) to keep its current shape — it already says "Cleaned up session ZFS clone" which is still accurate. No log-format change needed.

The `-r` path destroys snapshots silently as part of the same `zfs destroy` call, so we don't get per-snapshot log lines, which is fine — the manually-created snapshots being destroyed are by definition out-of-band artifacts the system didn't track in the first place.

## Test Strategy

Add a test case in `api/pkg/hydra/golden_zvol_test.go` alongside the existing `TestCleanupSessionZvol_*` tests:

```go
func (s *GoldenZvolSuite) TestCleanupSessionZvol_WithChildSnapshot() {
    // Arrange: mock zfs to report the clone exists, mock the destroy command
    // and assert it was called with the "-r" flag.
    // The mock's `zfs destroy` (no -r) should NOT be invoked — only the -r form.
}
```

The existing mock harness (`mockZFS` in `golden_zvol_test.go`) overrides `execCmdRun` / `execCmdCombinedOutput`, so we can capture the exact command line and assert `-r` was passed.

## Files Touched

- `api/pkg/hydra/golden_zvol.go` — one-line change at line 510 (add `"-r"` arg).
- `api/pkg/hydra/golden_zvol_test.go` — add one test case.
- Update doc-comment on `CleanupSessionZvol` to mention recursive destroy.

That's it. No new helpers, no new flags, no API changes.

## Notes for Implementers

- Don't refactor surrounding code — this is a one-arg fix.
- Don't change the other `zfs destroy` call sites unless their behaviour is part of the bug; they aren't.
- After committing, verify in production logs that the "Failed to GC orphaned zvol" warnings stop appearing for the listed session IDs.
- The `runCmd` wrapper at `golden_zvol.go:1104` produces the error string seen in the logs (`"%s %s: %w (output: ...)"`); confirm test mocks honour the same `combined output` contract so error matching still works.
