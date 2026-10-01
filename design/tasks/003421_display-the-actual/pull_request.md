# Show the actual pull request number in task actions

## Summary
Display the pull request's GitHub number in the task action toolbar instead of the repository name. A single pull request such as `pull/19` now appears as `PR: #19`, so the label matches the destination it opens.

## Testing
- Added a regression test covering repository `birding-3` with pull request number 19.
- Ran the complete frontend test suite: 1,357 tests passed and 1 was skipped.
- Ran the production frontend build successfully.
