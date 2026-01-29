# Implementation Tasks

## Fix Docker Compose Shim (Main Branch)

- [ ] Edit `helix-4/desktop/docker-shim/compose.go`
- [ ] Remove the `pluginName` from `finalArgs` (around line 35-40)
- [ ] Test: `docker compose version` should work
- [ ] Test: `docker compose -f docker-compose.dev.yaml config --services` should work
- [ ] Rebuild shim if needed: `cd desktop/docker-shim && go build -o /usr/local/bin/docker-shim`
- [ ] Commit to main branch

## Fix helix-specs Branch Handling (Main Branch)

- [ ] Edit `helix-4/desktop/shared/helix-workspace-setup.sh`
- [ ] In the branch checkout section (around line 268), add special handling for `helix-specs` branch
- [ ] When `HELIX_WORKING_BRANCH=helix-specs`, skip the direct checkout and let worktree handle it
- [ ] Keep the main repo on `main` branch (or base branch)
- [ ] Test: Verify helix-specs worktree is created at `~/work/helix-specs`
- [ ] Test: Verify helix-specs appears in `~/.helix-zed-folders`
- [ ] Test: Verify AI tools can read/write files in helix-specs directory
- [ ] Commit to main branch

## Fix Startup Script (helix-specs Branch)

- [ ] Edit `helix-specs/.helix/startup.sh`
- [ ] Add check that helix directory is on `main` branch before building
- [ ] Handle existing tmux sessions gracefully (check before starting)
- [ ] Improve error messages for common failure cases
- [ ] Test the script runs successfully
- [ ] Test the script is idempotent (runs twice without errors)
- [ ] Commit to helix-specs branch

## Testing

- [ ] Full flow test: Start fresh Helix project, verify startup script works
- [ ] Verify docker compose commands work after shim fix
- [ ] Verify helix-specs directory is accessible to tools
- [ ] Verify Helix stack builds and starts in tmux

## Git Commits

- [ ] Push docker-shim fix to main branch
- [ ] Push project roots fix to main branch  
- [ ] Push startup script improvements to helix-specs branch