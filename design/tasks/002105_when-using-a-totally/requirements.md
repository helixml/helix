# Requirements: Ensure 'main' Is Pushed Before 'helix-specs' on New GitHub Repos

## Background

When a project is connected to a **brand new, empty external GitHub repository**,
Helix can push the `helix-specs` design-docs branch to GitHub **before** `main`.
GitHub auto-promotes the **first branch pushed to an empty repo** to be the
repository's default branch. As a result the empty GitHub repo ends up with
`helix-specs` as its default branch instead of `main`.

This is wrong because:
- `helix-specs` is an orphan branch that contains only design docs, no code.
- Downstream tooling (e.g. the `git worktree add` for the design-docs worktree
  in `helix-workspace-setup.sh`) breaks when `helix-specs` is the upstream
  default — you cannot worktree-add a branch already checked out as the default.
- Users see a confusing, non-standard default branch on their new GitHub repo.

### Root cause (confirmed)

The branch-forwarding loop that mirrors internal pushes to the external GitHub
remote iterates over a Go map with **non-deterministic order**:

- `api/pkg/services/git_http_server.go` → `handleReceivePack`, loop at ~line 671:
  `for branch, isForce := range pushedBranchesMap { ... PushBranchToRemote(...) }`.
  `pushedBranchesMap` comes from `detectChangedBranches` (a `map[string]bool`),
  so when more than one branch is forwarded in the same push, the order is
  random. Whichever branch lands first on the empty GitHub repo wins the
  default-branch slot — `helix-specs` ~50% of the time.

The desktop shell scripts already seed `main` first within their own flow
(`helix-workspace-setup.sh` empty-repo init at ~line 324; `helix-specs-create.sh`
empty-repo seeding, commit `ee00cc926`). The remaining gap is the **Go
forwarding layer**, which does not guarantee `main` is forwarded before
`helix-specs`.

## User Stories

### US-1: New GitHub repo gets `main` as default
**As a** Helix user connecting a brand new (empty) GitHub repo to a project,
**I want** `main` to be pushed first and become the repository's default branch,
**so that** my repo follows the standard convention and downstream worktree
setup keeps working.

### US-2: Existing repos are unaffected
**As a** Helix user with an existing GitHub repo (already has a default branch),
**I want** branch pushes to behave exactly as before,
**so that** this fix introduces no regression for non-empty repos.

## Acceptance Criteria

- [ ] When a push forwards multiple branches to an external remote, the repo's
      default branch (normally `main`) is forwarded **first**, before any other
      branch (especially `helix-specs`).
- [ ] When `main`/default is not among the pushed branches, remaining branches
      are forwarded in a **deterministic** (sorted) order, with `helix-specs`
      never preferred ahead of an ordinary branch.
- [ ] After connecting a brand new empty GitHub repo and running through project
      setup, the GitHub repo's default branch is `main`, not `helix-specs`,
      across repeated runs (no longer order-dependent / flaky).
- [ ] Pushes to repos that already have a default branch behave identically to
      today (no change in observable behaviour).
- [ ] No regression in the existing shell-script empty-repo seeding behaviour.
- [ ] Existing tests still pass; new behaviour is covered by a test
      (deterministic ordering of forwarded branches).

## Out of Scope

- Explicitly setting GitHub's default branch through the GitHub REST API
  (`PATCH /repos/{owner}/{repo}` with `default_branch`). This is a possible
  future hardening but is not required to fix the reported bug and adds an API
  dependency. See design.md for the trade-off.
- Changing how `helix-specs` itself is created or what it contains.
