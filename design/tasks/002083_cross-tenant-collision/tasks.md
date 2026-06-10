# Implementation Tasks: Make per-Worker Git Repo ID Collision-Proof Across Orgs

- [x] In `api/pkg/services/git_repository_service.go`, change `generateRepositoryID` to use `system.GenerateID()` as the suffix instead of `time.Now().Unix()`. Add the `github.com/helixml/helix/api/pkg/system` import if not already present in that file.
- [x] Add `TestGenerateRepositoryID_NoCollisionUnderLoad` to `api/pkg/services/git_repository_service_test.go`: mint 10,000 ids with the same `(repoType, name)` and assert all distinct. Confirm it fails on the old implementation and passes on the new one.
- [~] Run `go build ./api/pkg/services/...` and `go test -run TestGenerateRepositoryID -count=1 ./api/pkg/services/...` locally.
- [ ] Reproduce the bug end-to-end before the fix (two orgs hire `w-mt` back-to-back via MCP `hire_worker`, observe `git_repositories_pkey` failure in API logs), then re-run after the fix and confirm both activations succeed.
- [ ] Open the PR against `helixml/helix`, link to this design doc, and reference issue #2570 as the related id-collision-class fix.
- [ ] Check Drone CI after pushing — the `api` test step exercises the services package.
