# Requirements: Fix Project Startup Script

## Context

This task fixes issues with the Helix-in-Helix development project startup. The correct workspace setup should be:

1. `~/work/helix-4` on a feature branch (e.g., `feature/001124-fix-the-project-startup`) for code changes
2. `~/work/helix-specs` as a git worktree on the `helix-specs` branch for design docs and startup script

Both should exist simultaneously. Changes to code (like docker-shim) go to the feature branch; changes to design docs and `.helix/startup.sh` go directly to the helix-specs branch.

## Problems

1. **Docker compose commands fail**: The docker-shim wrapper passes "compose" twice to the real plugin, causing "unknown docker command: compose compose" errors.

2. **Wrong branch configuration**: Tasks started by the "let AI fix your startup script" button are incorrectly created with `HELIX_WORKING_BRANCH=helix-specs` instead of a feature branch. This causes the workspace setup to checkout helix-specs directly on helix-4 (corrupting it) instead of creating a worktree.

3. **Startup script fragility**: The script doesn't verify prerequisites before building, and doesn't handle re-runs gracefully.

## Acceptance Criteria

- [ ] `docker compose version` works without errors
- [ ] `docker compose -f docker-compose.dev.yaml config` works
- [ ] Tasks get feature branch names, not `helix-specs` as working branch
- [ ] The helix-specs worktree is created at `~/work/helix-specs`
- [ ] The main repo stays on its feature branch (not helix-specs)
- [ ] Both helix-4 and helix-specs appear in `~/.helix-zed-folders`
- [ ] The startup script completes successfully
- [ ] The startup script can be run multiple times without errors (idempotent)

## Notes

- Docker-shim fix goes to main branch
- Task creation fix goes to main branch
- Workspace setup fix goes to main branch
- Startup script improvements go to helix-specs branch