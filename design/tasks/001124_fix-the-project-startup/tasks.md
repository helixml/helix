# Implementation Tasks

## Investigation & Debugging

- [ ] Test worktree creation manually: run helix-workspace-setup.sh and check logs
- [ ] Verify HELIX_PRIMARY_REPO_NAME is set correctly in the desktop container
- [ ] Verify helix-specs-create.sh is being sourced successfully
- [ ] Check if create_helix_specs_branch() runs and what it returns
- [ ] Add debug logging to helix-workspace-setup.sh worktree creation section
- [ ] Identify exact failure point in worktree creation logic

## Fix Docker Compose Shim (Main Branch)

- [ ] Edit `/home/retro/work/helix-4/desktop/docker-shim/compose.go`
- [ ] Remove pluginName from finalArgs around line 40
- [ ] Test: `docker compose -f docker-compose.dev.yaml config --services`
- [ ] Rebuild docker-shim: `cd desktop/docker-shim && go build -o /usr/local/bin/docker-shim`
- [ ] Test docker compose commands work correctly
- [ ] Commit to main branch with message: "Fix docker-shim double-compose bug"

## Fix helix-specs Worktree Creation (Main Branch)

- [ ] Based on investigation, fix the worktree creation logic in helix-workspace-setup.sh
- [ ] Add better error messages and logging for debugging
- [ ] Handle edge cases (repo not cloned, branch doesn't exist, etc.)
- [ ] Test worktree creation on fresh project setup
- [ ] Test worktree creation on project restart (should skip if exists)
- [ ] Commit to main branch with message: "Fix helix-specs worktree creation in workspace setup"

## Fix Startup Script (helix-specs Branch)

- [ ] Edit `/home/retro/work/helix-specs/.helix/startup.sh`
- [ ] Remove or update the directory renaming logic (helix-4 vs helix)
- [ ] Add check to verify helix-specs worktree exists at ~/work/helix-specs
- [ ] Add check to ensure main repo (helix-4) is on main branch before building
- [ ] Improve yarn installation handling (check if already installed/installing)
- [ ] Add better error messages for each failure point  
- [ ] Make tmux session creation idempotent (check if already exists)
- [ ] Test the script end-to-end with the worktree fix

## Testing

- [ ] Test full flow: create new project with Helix repo
- [ ] Verify helix-specs worktree is created automatically
- [ ] Verify startup script finds the worktree correctly
- [ ] Run startup script and verify it completes successfully
- [ ] Run startup script again to verify idempotency
- [ ] Verify docker compose commands work
- [ ] Verify Helix stack builds and starts
- [ ] Test with and without privileged mode

## Documentation

- [ ] Add comments to helix-workspace-setup.sh explaining worktree creation
- [ ] Add comments to startup script explaining the expected directory structure
- [ ] Document any remaining manual steps
- [ ] Update project setup documentation if needed

## Git Commits

- [ ] Commit docker-shim fix to main branch
- [ ] Commit worktree creation fix to main branch  
- [ ] Commit startup script updates to helix-specs branch
- [ ] Push all changes to origin
