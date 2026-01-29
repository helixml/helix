# Requirements: Fix Project Startup Script

**Note:** This task is meta - it is both running inside a Helix spec task session AND working on fixing the Helix spec task setup itself. The issues we're experiencing are the very issues we're trying to fix.

## Context

This task fixes issues with the Helix-in-Helix development project startup. The correct workspace setup should be:

1. `~/work/helix-4` on a feature branch (e.g., `feature/001124-fix-the-project-startup`) for code changes
2. `~/work/helix-specs` as a git worktree on the `helix-specs` branch for design docs and startup script

Both should exist simultaneously. Changes to code (like docker-shim) go to the feature branch; changes to design docs and `.helix/startup.sh` go directly to the helix-specs branch.

## Problems

1. **Docker compose commands fail**: The docker-shim wrapper passes "compose" twice to the real plugin, causing "unknown docker command: compose compose" errors.

2. **Wrong branch configuration**: Tasks started by the "let AI fix your startup script" button were incorrectly created with `branch_name=helix-specs` instead of a feature branch. The `helix-specs` branch is reserved for storing spec documents (via a worktree), not for feature development. This caused the workspace setup to checkout helix-specs directly on helix-4 (corrupting it) instead of creating the worktree. **This specific task's configuration has been fixed in the database** (branch_name → `feature/001124-fix-the-project-startup`, base_branch → `main`, branch_mode → `new`), but the underlying bug in task creation needs to be fixed to prevent this from happening to future tasks.

3. **Startup script fragility**: The script doesn't verify prerequisites before building, and doesn't handle re-runs gracefully.

## Acceptance Criteria

- [ ] `docker compose version` works without errors
- [ ] `docker compose -f docker-compose.dev.yaml config` works
- [ ] Task creation code prevents `helix-specs` from being used as a feature branch name
- [ ] The helix-specs worktree is created at `~/work/helix-specs`
- [ ] The main repo stays on its feature branch (not helix-specs)
- [ ] Both helix-4 and helix-specs appear in `~/.helix-zed-folders`
- [ ] The startup script completes successfully
- [ ] The startup script can be run multiple times without errors (idempotent)

## Notes

- Docker-shim fix goes to a feature branch (then merged to main)
- Task creation fix goes to a feature branch (then merged to main)
- Workspace setup fix goes to a feature branch (then merged to main)
- Startup script improvements go directly to helix-specs branch