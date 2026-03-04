# Implementation Tasks

- [ ] Update `helix/desktop/shared/helix-workspace-setup.sh` - add git pull to existing worktree case (lines 460-470)
  - Add stash check for uncommitted changes
  - Fetch and pull from `origin helix-specs`
  - Restore stash after pull
  - Log success/warning appropriately
  - Ensure non-fatal on failure (continue with local version)

- [ ] Test manually: edit startup script → restart exploratory session → verify updated script runs

- [ ] Verify golden build mode still works (separate code path, should be unaffected)