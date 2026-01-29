# Requirements: Fix Project Startup Script

## Context

The startup script lives at `/home/retro/work/helix-specs/.helix/startup.sh` in the helix-specs branch (a git worktree). The main Helix codebase is at `/home/retro/work/helix` on the main branch. When the API clones repos for a project, it creates numbered directories (helix-4, zed-4, qwen-code-4) which the startup script renames to canonical names.

## User Story

As a developer working on Helix-in-Helix development, I want the startup script to run successfully and idempotently so that I can build and start the Helix stack without manual intervention.

## Current Issues Identified

1. **Docker Compose Shim Bug**: The docker-shim wrapper at `/home/retro/work/helix/desktop/docker-shim/compose.go` incorrectly passes "compose" as the first argument when calling `docker-compose.real`, causing commands to fail with "unknown docker command: 'compose compose'".

2. **Script Assumptions**: The startup script assumes it's running in a context where it can find the Helix repo at `~/work/helix`, but needs clarity on:
   - Whether the repo should be on main or helix-specs branch when the script runs
   - How the script gets executed (from where, by what process)
   - What the exact directory structure should be

3. **Missing Dependencies**: The script tries to install yarn globally but this may fail or take time.

## Acceptance Criteria

- [ ] The startup script runs successfully without errors
- [ ] The script is idempotent (can be run multiple times safely)
- [ ] The docker compose commands work correctly
- [ ] The Helix stack builds and starts in tmux
- [ ] The script handles both fresh setup and restart scenarios
- [ ] Dependencies (tmux, yarn) are installed correctly
- [ ] The script provides clear feedback about what it's doing and any issues

## Notes

- The startup script is stored in the helix-specs branch for version control of project configuration
- Changes must be committed to the helix-specs branch, not main
- The script may need to handle switching between branches or using git worktrees
