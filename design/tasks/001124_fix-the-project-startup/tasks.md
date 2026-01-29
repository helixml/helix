# Implementation Tasks

## Fix Docker Compose Shim (Feature Branch)

- [x] Edit `helix-4/desktop/docker-shim/compose.go`
- [x] Remove `pluginName` from `finalArgs` (around line 35-40)
- [x] Test: `docker compose version` should work
- [x] Test: `docker compose -f docker-compose.dev.yaml config --services` should work
- [ ] Commit to feature branch

## Fix Task Branch Configuration (Feature Branch)

- [ ] Find where "let AI fix your startup script" button sets branch configuration
- [ ] Ensure `helix-specs` is never used as a feature branch name (it's reserved for the spec documents worktree)
- [ ] Tasks should get proper feature branch names like `feature/001124-fix-the-project-startup`
- [ ] Test: Click "let AI fix your startup script" and verify task gets a feature branch name, not `helix-specs`
- [ ] Commit to feature branch

## Fix Workspace Setup (Feature Branch)

- [ ] Edit `helix-4/desktop/shared/helix-workspace-setup.sh`
- [ ] Add defensive handling: if `HELIX_WORKING_BRANCH=helix-specs`, don't checkout directly
- [ ] Keep main repo on default branch and let worktree creation handle helix-specs
- [ ] Test: Verify helix-specs worktree is created at `~/work/helix-specs`
- [ ] Test: Verify both helix-4 and helix-specs appear in `~/.helix-zed-folders`
- [ ] Commit to main branch

## Fix Startup Script (helix-specs Worktree)

- [~] Edit `~/work/helix-specs/.helix/startup.sh` directly in the worktree
- [ ] Add check that `./stack` script exists before trying to run it
- [ ] Handle existing tmux sessions gracefully
- [ ] Improve error messages
- [ ] Test the script runs successfully
- [ ] Test the script is idempotent
- [ ] Commit to helix-specs branch

## Testing

- [ ] Full flow test: Create new task, verify correct branch setup
- [ ] Verify docker compose commands work after shim fix
- [ ] Verify both helix-4 and helix-specs directories are accessible to tools
- [ ] Verify startup script completes successfully

## Git Commits

- [ ] Push docker-shim fix to feature branch
- [ ] Push task creation fix to feature branch
- [ ] Push workspace setup fix to feature branch
- [ ] Push startup script improvements directly from `~/work/helix-specs` worktree to helix-specs branch