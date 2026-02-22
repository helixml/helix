# Implementation Tasks

## Pre-Implementation
- [~] Verify understanding by reading `helix-workspace-setup.sh` clone completion section (lines 230-260)
- [ ] Review existing empty repo handling in `helix-specs-create.sh` for patterns to reuse

## Core Implementation
- [ ] Add empty repo detection function in `helix-workspace-setup.sh`
  - Use `git rev-parse HEAD` to check for commits
  - Return true if repo is empty (exit code 128)
- [ ] Add initialization section after clone completion loop (after line ~250)
  - Loop through `CLONE_DIRS` array
  - Check each for empty state
  - Create README.md with repo name
  - Configure git user if not set (email/name)
  - Commit with message "Initial commit"
  - Push to origin main with `-u` flag
  - Print success message with ✅

## Error Handling
- [ ] Handle push failure gracefully (show clear error, don't mask the issue)
- [ ] Ensure git user.email and user.name are configured before commit (reuse existing config from script)

## Testing
- [ ] Manual test: Create empty GitHub repo → Start Helix session → Verify Zed opens
- [ ] Manual test: Existing repo with branches still works normally
- [ ] Manual test: Empty repo in "existing branch" mode handles gracefully