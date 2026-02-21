# Requirements: Auto-Initialize Empty Git Repositories

## Problem Statement

When a user creates a new repository in GitHub and points Helix at it, the session fails to start because the repo has no branches. The workspace setup script tries to checkout `main` or `master` but neither exists on an empty repo.

### Current Error Flow
1. User creates new repo on GitHub (empty, no README)
2. Helix clones it via git proxy (`WithExternalRepoRead`, `giteagit.Clone` with `Mirror: true`) - **this succeeds** (git proxy handles empty repos correctly)
3. Desktop container receives the cloned repo (warning: "remote HEAD refers to nonexistent ref")
4. `helix-workspace-setup.sh` enters "new branch" mode and tries to find base branch
5. Fails with: "FATAL: Base branch not found on remote: main"
6. Zed never starts - user must manually initialize the repo from terminal

### Git Proxy Verification

The Helix git proxy layer (`api/pkg/services/git_external_sync.go`, `git_repository_service.go`) already handles empty repos correctly:
- `SyncAllBranches` has explicit test coverage for empty repos (`TestSyncAllBranches_EmptyRepo`)
- `giteagit.Clone` with `Mirror: true` succeeds on empty repos
- `WithExternalRepoRead` / `WithExternalRepoWrite` work with empty repos

**The issue is in the desktop shell script**, not the git proxy.

## User Stories

### US-1: Auto-init empty repositories
**As a** user creating a new project  
**I want** Helix to automatically initialize empty repositories  
**So that** I can start coding immediately without manual git setup

## Acceptance Criteria

### AC-1: Empty repo detection and initialization
- [ ] Detect when a cloned repo is empty (no commits, no branches)
- [ ] Create an initial commit with a basic README.md
- [ ] Push the initial commit to establish the `main` branch
- [ ] Continue with normal branch setup flow

### AC-2: Graceful handling
- [ ] Show clear message: "Initializing empty repository..."
- [ ] Don't fail on the "nonexistent ref" warning from git clone
- [ ] Work with both "new branch" and "existing branch" modes

### AC-3: No regression
- [ ] Normal repos (with existing branches) work exactly as before
- [ ] Empty repos that the user intentionally wants empty should get minimal init (just enough to work)

## Out of Scope
- Custom initial commit content
- Template repositories
- Choosing initial branch name (will use `main`)