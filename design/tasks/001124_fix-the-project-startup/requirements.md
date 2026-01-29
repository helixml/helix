# Requirements: Fix Project Startup Script

## Context

The Helix-in-Helix development project has a startup script at `.helix/startup.sh` in the helix-specs branch. This script builds and starts the inner Helix development stack. The script should live in a git worktree (`~/work/helix-specs`) separate from the main code (`~/work/helix-4`).

## Problems

1. **Docker compose commands fail**: The docker-shim wrapper passes "compose" twice to the real plugin, causing "unknown docker command: compose compose" errors.

2. **helix-specs worktree not created**: When `HELIX_WORKING_BRANCH=helix-specs`, the workspace setup script checks out the helix-specs branch directly on the main repo instead of creating a worktree. This leaves the main repo on the wrong branch (no code, no `./stack` script) and the `~/work/helix-specs` directory is never created.

3. **Startup script fragility**: The script doesn't verify the repo is on the correct branch before building, and doesn't handle re-runs gracefully.

## Acceptance Criteria

- [ ] `docker compose version` works without errors
- [ ] `docker compose -f docker-compose.dev.yaml config` works
- [ ] The helix-specs worktree is created at `~/work/helix-specs`
- [ ] The main repo (helix-4) stays on `main` branch
- [ ] The helix-specs directory appears in `~/.helix-zed-folders` and is visible in Zed
- [ ] AI tools can read and write files in helix-specs
- [ ] The startup script completes successfully
- [ ] The startup script can be run multiple times without errors (idempotent)
- [ ] The Helix stack builds and starts in tmux

## Notes

- The startup script creates symlinks from numbered directories (helix-4, zed-4, qwen-code-4) to canonical names (helix, zed, qwen-code) because the `./stack` script expects `../zed` and `../qwen-code` relative paths.
- Docker-shim fix goes to main branch
- Workspace setup fix goes to main branch  
- Startup script improvements go to helix-specs branch