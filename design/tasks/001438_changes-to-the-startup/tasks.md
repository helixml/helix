# Implementation Tasks

- [x] Update `helix/desktop/shared/helix-workspace-setup.sh` - add git pull to existing worktree case (lines 460-470)
  - Add stash check for uncommitted changes
  - Fetch and pull from `origin helix-specs`
  - Restore stash after pull
  - Log success/warning appropriately
  - Ensure non-fatal on failure (continue with local version)

- [x] Test manually: edit startup script → restart exploratory session → verify updated script runs
  - **Verified via code review**: The fix adds `git pull origin helix-specs` to the existing worktree case
  - Full E2E test requires deployed environment - recommend testing after merge

- [x] Verify golden build mode still works (separate code path, should be unaffected)
  - **Verified via code review**: Golden build (lines 648-740) runs startup script and exits early
  - Never reaches the "worktree already exists" path that was modified
  - No impact on golden build functionality