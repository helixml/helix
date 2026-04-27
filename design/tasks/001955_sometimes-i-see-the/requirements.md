# Requirements: Stop git from opening vim during agent session startup

## Problem

When an agent session starts, the workspace setup script sometimes opens `vim` (or whatever `$EDITOR` is) to ask the user to confirm a merge commit message. The user must press `:wq` to continue, blocking the entire session startup on a git incantation.

This happens *intermittently* — only when the local base branch (or `helix-specs` worktree) has diverged from its remote, forcing `git pull` to create a non-fast-forward merge commit. By default, git invokes `$EDITOR` to author that commit message.

## Root Cause

`desktop/shared/helix-workspace-setup.sh` runs at container startup and contains two unguarded `git pull` calls:

- **Line 382** (`git pull origin "$BASE_BRANCH"`) — pulls the primary repo's base branch (e.g., `main`) before branching off it.
- **Line 473** (`git -C "$WORKTREE_PATH" pull origin helix-specs`) — pulls the design-docs worktree to pick up startup script updates.

The script also explicitly sets `git config --global pull.rebase false` (line 157), so any divergence triggers a merge commit, which opens `$EDITOR`.

## User Story

**As an agent session user, I want session startup to never block on a git editor prompt** so I can use the session immediately without remembering vim keybindings.

## Acceptance Criteria

- Starting an agent session never opens vim (or any editor) for a git commit message.
- If the base branch has diverged from origin, startup logs a clear warning and continues — it does not silently create surprise merge commits or hang waiting for input.
- If the `helix-specs` worktree has diverged from origin, startup logs a warning and continues with the local version (current fallback behaviour preserved).
- The fix applies to both rebuilt container images (Dockerfile-baked script) and any running session that sources the script fresh.

## Out of Scope

- Changing the global `pull.rebase` policy for repos that the user/agent manipulates after startup. This task only hardens the *startup script's own* git invocations.
- Other `git pull` call sites outside the startup path (e.g., `for-mac/scripts/provision-vm.sh`, `sample_project_code_service.go`) — those run in different contexts and are not part of agent session startup.
