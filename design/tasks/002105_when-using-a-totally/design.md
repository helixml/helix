# Design: Ensure 'main' Is Pushed Before 'helix-specs' on New GitHub Repos

## Summary

Make the branch-forwarding order deterministic when Helix mirrors internal
pushes to an external GitHub remote, so the repository's default branch
(normally `main`) is always pushed **first**. This prevents an empty GitHub repo
from adopting `helix-specs` as its default branch.

## Where the fix goes

**Primary fix — Go backend (the actual root cause):**

`api/pkg/services/git_http_server.go`, function `handleReceivePack`, the
external-push loop at ~line 671:

```go
upstreamPushFailed := false
for branch, isForce := range pushedBranchesMap {   // <-- random map order
    ...
    err := s.gitRepoService.PushBranchToRemote(branchCtx, repoID, branch, isForce, pushUserID)
    ...
}
```

Replace the random map iteration with an **ordered slice** of branch names:

1. Build a slice of the branch names from `pushedBranchesMap`.
2. Sort it so that:
   - `repo.DefaultBranch` (already available in this function, e.g. used at
     ~line 621) comes **first** if present;
   - otherwise, if `main` is present it comes first, then `master`;
   - all remaining branches follow in stable alphabetical order;
   - `helix-specs` is never ordered ahead of the default/ordinary branches
     (alphabetically it sorts before `main`, which is exactly the trap — so the
     default-branch-first rule must take precedence over plain alpha sort).
3. Iterate the sorted slice, looking up `isForce` from the map per branch.

This guarantees that on a first push to an empty external repo, the default
branch reaches GitHub first and becomes the default.

### Suggested helper

Add a small, pure, testable helper (same file or a sibling) e.g.:

```go
// orderBranchesForUpstream returns branch names ordered so the default branch
// is pushed first to an external remote. This ensures a brand-new empty GitHub
// repo adopts the default branch (e.g. "main") rather than whichever branch
// happened to be iterated first.
func orderBranchesForUpstream(branches map[string]bool, defaultBranch string) []string
```

Order rule: `defaultBranch` first (if non-empty and present) → `main` → `master`
→ remaining names sorted ascending. Keep it deterministic and side-effect free
so it can be unit-tested without git.

## Why ordering (not GitHub API) is the chosen approach

- **Minimal & dependency-free.** It mirrors the approach the shell scripts
  already take (seed `main` before `helix-specs`) and needs no GitHub token
  scope or API client.
- **Matches GitHub's documented behaviour** — the first branch pushed to an
  empty repo becomes the default. Controlling order is sufficient.
- **Lower blast radius** — a pure ordering change in one loop, easy to test.

A GitHub-API approach (`PATCH /repos/{owner}/{repo}` `default_branch=main`) would
be more bulletproof (order-independent) but introduces an external API call,
auth-scope requirements, and provider-specific code paths. Documented as
out-of-scope future hardening in requirements.md.

## Interaction with existing shell-script handling

The desktop scripts already seed the default branch before `helix-specs`:

- `desktop/shared/helix-workspace-setup.sh` — empty-repo init (~line 324):
  `git checkout -b main` + `git push -u origin main` before the helix-specs
  worktree step (~line 488).
- `desktop/shared/helix-specs-create.sh` — empty-repo seeding pushes
  `RETURN_BRANCH` first (commit `ee00cc926`).

These operate against the **internal** Helix git server. The gap they don't
cover is the **forwarding to the external GitHub remote**, where a single push
event can carry multiple branches and the Go loop forwards them in random order.
The primary fix closes that gap. No shell changes are required, but the scripts
should be re-verified after the Go fix to confirm end-to-end ordering.

## Key decisions

- **Decision:** Fix ordering in the Go forwarding loop rather than (or before)
  adding a GitHub API call. **Rationale:** smallest correct change; removes the
  non-determinism that is the literal root cause.
- **Decision:** Prefer `repo.DefaultBranch` over hardcoding `main`.
  **Rationale:** repos may legitimately use `master`; the stored default branch
  is the source of truth and is already in scope in `handleReceivePack`.
- **Decision:** Keep the change to a pure, unit-testable helper.
  **Rationale:** map-iteration order can't be asserted directly; extracting the
  ordering makes the fix verifiable without spinning up git/GitHub.

## Risks / gotchas

- The forwarding loop breaks on the first failed branch and rolls back
  (`upstreamPushFailed`). Ordering must not change that early-exit semantics —
  only the order of iteration.
- `isForce` must still be read per-branch from the map after sorting.
- Confirm `repo.DefaultBranch` is populated for external repos at this point
  (it is set when the external repo is connected — see
  `api/pkg/server/project_handlers.go` external-repo creation). If empty, fall
  back to the `main` → `master` rule.

## Test plan

- Unit test `orderBranchesForUpstream` (or equivalent):
  - `{helix-specs, main}` with default `main` → `[main, helix-specs]`.
  - `{helix-specs, master}` with default `master` → `[master, helix-specs]`.
  - default branch absent → `main`/`master` precedence then alpha.
  - single branch → unchanged.
- Manual / integration: connect a fresh empty GitHub repo, run project setup
  repeatedly, confirm GitHub default branch is always `main`.
