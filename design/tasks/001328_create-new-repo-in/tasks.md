# Implementation Tasks

## Pre-Implementation
- [x] Verify understanding by reading `helix-workspace-setup.sh` clone completion section (lines 230-260)
- [x] Review existing empty repo handling in `helix-specs-create.sh` for patterns to reuse

## Core Implementation
- [x] Add empty repo detection and initialization section after clone completion loop (after line ~256)
  - Loop through `CLONE_DIRS` array
  - Check each for empty state using `git rev-parse HEAD`
  - Create README.md with repo name
  - Configure git user (reuse existing config from script)
  - Commit with message "Initial commit"
  - Push to origin main with `-u` flag
  - Print success message with ✅

## Error Handling
- [x] Handle push failure gracefully (show clear error, don't mask the issue)

## Testing
- [ ] Manual test: Create empty GitHub repo → Start Helix session → Verify Zed opens
- [ ] Manual test: Existing repo with branches still works normally