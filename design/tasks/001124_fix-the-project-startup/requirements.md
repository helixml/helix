# Requirements: Fix Project Startup Script

## Context

The startup script lives at `/home/retro/work/helix-specs/.helix/startup.sh` in the helix-specs branch (a git worktree). The main Helix codebase is at `/home/retro/work/helix-4` on the main branch. When the API clones repos for a project, it creates numbered directories (helix-4, zed-4, qwen-code-4) which the startup script renames to canonical names and creates symlinks back to the numbered names.

**Important**: The helix-specs worktree IS being created correctly at `/home/retro/work/helix-specs` by the workspace setup. However, it's NOT being included as a project root directory in the Zed/IDE configuration, so tools can't access files in it.

## User Stories

1. As a developer working on Helix-in-Helix development, I want the startup script to run successfully and idempotently so that I can build and start the Helix stack without manual intervention.

2. As a developer using Zed/AI tools in a Helix project, I want the helix-specs worktree to be included as a project root so I can read/edit design documents and the startup script.

## Current Issues Identified

1. **Docker Compose Shim Bug**: The docker-shim wrapper at `/home/retro/work/helix/desktop/docker-shim/compose.go` incorrectly passes "compose" as the first argument when calling `docker-compose.real`, causing commands to fail with "unknown docker command: 'compose compose'".

2. **helix-specs Not in Project Roots**: The helix-specs worktree is created at `~/work/helix-specs` by workspace setup, but it's NOT included in the list of project root directories given to Zed/the IDE. This means:
   - Tools can't see files in the helix-specs directory
   - The `.helix/startup.sh` file isn't accessible through the IDE's file system
   - Design documents in `design/tasks/` aren't visible as project files

3. **Script Assumptions**: The startup script assumes it's running in a context where it can find the Helix repo, but needs to handle:
   - The main repo being at `~/work/helix-4` (numbered directory)  
   - The helix-specs worktree being at `~/work/helix-specs`
   - Ensuring the main repo is on the main branch before building

4. **Missing Dependencies**: The script tries to install yarn globally but this may fail or take time.

## Acceptance Criteria

- [ ] The helix-specs worktree is included as a project root directory in Zed/IDE
- [ ] Tools can read and write files in the helix-specs directory
- [ ] The startup script runs successfully without errors
- [ ] The script is idempotent (can be run multiple times safely)
- [ ] The docker compose commands work correctly
- [ ] The Helix stack builds and starts in tmux
- [ ] Dependencies (tmux, yarn) are installed correctly
- [ ] The script provides clear feedback about what it's doing and any issues

## Notes

- The startup script is stored in the helix-specs branch for version control of project configuration
- Changes to the startup script must be committed to the helix-specs branch, not main
- Changes to docker-shim must be committed to the main branch
- The project root configuration needs to be updated to include the helix-specs worktree path
