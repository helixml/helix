# Implementation Tasks

- [x] In `api/pkg/hydra/golden_zvol.go`, change `runCmd("zfs", "destroy", cloneName)` at line 510 to `runCmd("zfs", "destroy", "-r", cloneName)`.
- [x] Update the doc-comment on `CleanupSessionZvol` (line 490) to note the destroy is recursive (cleans up any child snapshots).
- [x] Add `TestCleanupSessionZvol_WithChildSnapshot` to `api/pkg/hydra/golden_zvol_test.go` that asserts `zfs destroy` is invoked with the `-r` flag.
- [x] Update existing `TestCleanupSessionZvol_*`, `TestGCOrphanedZvols_*`, and `TestCleanup_ZvolSessionUsesZvolPath` assertions to expect `zfs destroy -r ...` (5 sites). Two destroy assertions for non-CleanupSessionZvol code paths (mount-fail at line 470, mkfs.xfs-fail at line 730-ish) intentionally left as `zfs destroy ...`.
- [~] Build: `cd api && go build ./pkg/hydra/...`.
- [ ] Run unit tests: `cd api && CGO_ENABLED=1 go test -v -run GoldenZvolSuite ./pkg/hydra/ -count=1`.
- [ ] Push feature branch.
- [ ] Write per-repo PR description (`pull_request_helix.md`) referencing the original log snippet.
