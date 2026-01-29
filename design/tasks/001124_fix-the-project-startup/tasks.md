# Implementation Tasks

## Investigation: Find Where Project Roots Are Configured

- [ ] Search for where Zed/IDE project roots are configured in Hydra Executor
- [ ] Check DesktopAgent structure and how RepositoryIDs are used
- [ ] Find where workspace directories are passed to Zed
- [ ] Check Zed workspace configuration files (if any)
- [ ] Determine if project roots come from environment variables or command-line args
- [ ] Document the current flow: repos → project roots → Zed

## Fix Docker Compose Shim (Main Branch)

- [ ] Edit `/home/retro/work/helix-4/desktop/docker-shim/compose.go`
- [ ] Remove pluginName from finalArgs around line 40
- [ ] Test: `docker compose -f docker-compose.dev.yaml config --services`
- [ ] Rebuild docker-shim: `cd desktop/docker-shim && go build -o /usr/local/bin/docker-shim`
- [ ] Test docker compose commands work correctly
- [ ] Commit to main branch with message: "Fix docker-shim double-compose bug"

## Add helix-specs to Project Roots (Main Branch)

- [ ] Based on investigation, identify where to add helix-specs path
- [ ] Add logic to include `~/work/helix-specs` in project roots when primary repo exists
- [ ] Ensure helix-specs is added AFTER the worktree is created in workspace setup
- [ ] Consider: Should this be automatic for all projects with primary repos?
- [ ] Test: Verify helix-specs directory is accessible from tools/IDE
- [ ] Test: Verify files in helix-specs can be read/written by AI tools
- [ ] Commit to main branch with message: "Add helix-specs worktree to project roots"

## Fix Startup Script (helix-specs Branch)

- [ ] Edit `/home/retro/work/helix-specs/.helix/startup.sh`
- [ ] Update directory handling to work with numbered names (helix-4 not helix)
- [ ] Add check to verify helix-specs worktree exists at ~/work/helix-specs
- [ ] Add check to ensure main repo (helix-4) is on main branch before building
- [ ] Improve yarn installation handling (check if already installed/installing)
- [ ] Add better error messages for each failure point
- [ ] Make tmux session creation idempotent (check if already exists)
- [ ] Handle case where stack script doesn't exist (not on main branch)
- [ ] Test the script end-to-end

## Testing

- [ ] Test full flow: create new project with Helix repo
- [ ] Verify helix-specs worktree is created automatically
- [ ] Verify helix-specs appears as a project root in IDE
- [ ] Verify AI tools can read files from helix-specs directory
- [ ] Verify AI tools can write files to helix-specs directory
- [ ] Run startup script and verify it completes successfully
- [ ] Run startup script again to verify idempotency
- [ ] Verify docker compose commands work
- [ ] Verify Helix stack builds and starts
- [ ] Test with and without privileged mode

## Documentation

- [ ] Add comments explaining where project roots are configured
- [ ] Add comments to startup script explaining the expected directory structure
- [ ] Document the helix-specs worktree setup in workspace-setup.sh
- [ ] Update project setup documentation if needed

## Git Commits

- [ ] Commit docker-shim fix to main branch
- [ ] Commit project roots fix to main branch
- [ ] Commit startup script updates to helix-specs branch
- [ ] Push all changes to origin