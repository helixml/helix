# Requirements: Fix Project Startup Script

## Context

The Helix-in-Helix development project has a startup script at `.helix/startup.sh` in the helix-specs branch. This script builds and starts the inner Helix development stack. The script lives in a git worktree (`~/work/helix-specs`) separate from the main code (`~/work/helix-4`).

## Problems

1. **Docker compose commands fail**: The docker-shim wrapper passes "compose" twice to the real plugin, causing "unknown docker command: compose compose" errors.

2. **helix-specs not accessible to tools**: The helix-specs worktree is created at `~/work/helix-specs` but is not included as a project root directory for Zed/IDE, so AI tools cannot read or write files in it.

3. **Startup script fragility**: The script doesn't verify the repo is on the correct branch before building, and doesn't handle re-runs gracefully.

## Acceptance Criteria

- [ ] `docker compose version` works without errors
- [ ] `docker compose -f docker-compose.dev.yaml config` works
- [ ] The helix-specs directory is visible as a project root in the IDE
- [ ] AI tools can read and write files in helix-specs
- [ ] The startup script completes successfully
- [ ] The startup script can be run multiple times without errors (idempotent)
- [ ] The Helix stack builds and starts in tmux

## Notes

- The startup script creates symlinks from numbered directories (helix-4, zed-4, qwen-code-4) to canonical names (helix, zed, qwen-code) because the `./stack` script expects `../zed` and `../qwen-code` relative paths.
- Docker-shim fix goes to main branch
- Project roots fix goes to main branch  
- Startup script improvements go to helix-specs branch