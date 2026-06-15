# Implementation Tasks: Ensure 'main' Is Pushed Before 'helix-specs' on New GitHub Repos

- [ ] Add a pure helper `orderBranchesForUpstream(branches map[string]bool, defaultBranch string) []string` in `api/pkg/services/git_http_server.go` that orders branch names: `defaultBranch` first (if present), then `main`, then `master`, then remaining names sorted ascending.
- [ ] In `handleReceivePack` (~line 671), replace `for branch, isForce := range pushedBranchesMap` with iteration over `orderBranchesForUpstream(pushedBranchesMap, repo.DefaultBranch)`, reading `isForce` from the map per branch.
- [ ] Preserve existing semantics: per-branch timeout, the `upstreamPushFailed` early-break, and the rollback path must be unchanged — only the iteration order changes.
- [ ] Handle empty/missing `repo.DefaultBranch` by falling back to the `main` → `master` precedence rule.
- [ ] Add unit tests for `orderBranchesForUpstream` covering: `{helix-specs, main}`→`[main, helix-specs]`, `{helix-specs, master}` with default `master`, default-branch-absent fallback, and single-branch input.
- [ ] Re-verify the desktop shell scripts (`helix-workspace-setup.sh` empty-repo init, `helix-specs-create.sh` seeding) still order `main` before `helix-specs` end-to-end after the Go change.
- [ ] Manually verify: connect a brand new empty GitHub repo, run project setup multiple times, confirm GitHub's default branch is `main` every time (no flakiness).
- [ ] Run existing Go test suite to confirm no regressions.
