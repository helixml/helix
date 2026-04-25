# Implementation Tasks

- [ ] In `api/pkg/hydra/golden_zvol.go`, change `runCmd("zfs", "destroy", cloneName)` at line 510 to `runCmd("zfs", "destroy", "-r", cloneName)`.
- [ ] Update the doc-comment on `CleanupSessionZvol` (line 490) to note the destroy is recursive (cleans up any child snapshots).
- [ ] Add `TestCleanupSessionZvol_WithChildSnapshot` to `api/pkg/hydra/golden_zvol_test.go` that asserts `zfs destroy` is invoked with the `-r` flag.
- [ ] Verify existing `TestCleanupSessionZvol_*` tests still pass (the no-children path is unchanged in observable behaviour, but the command-line mock may need its expected args updated).
- [ ] Build: `cd api && go build ./pkg/hydra/...`.
- [ ] Run unit tests: `cd api && CGO_ENABLED=1 go test -v -run GoldenZvolSuite ./pkg/hydra/ -count=1`.
- [ ] Deploy to the helix-in-helix sandbox via `./stack build-sandbox` and confirm the warning stops appearing in `docker compose logs sandbox-nvidia` for the affected session IDs.
- [ ] Open the PR; reference the original log snippet in the description so reviewers can match it to the fix.
