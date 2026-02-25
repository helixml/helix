# Implementation Tasks

## Backend Fix

- [ ] In `api/pkg/server/project_handlers.go`, modify `getProjectRepositories` to sort repos with primary repository first
  - Fetch `project.DefaultRepoID` (already available from auth check)
  - After `ListGitRepositories` call, reorder slice: primary repo at index 0, others maintain `created_at DESC` order
  - Use `sort.SliceStable` to preserve relative ordering of non-primary repos

## Testing

- [ ] Manual test: Create project with 3 repos (A, B, C in attachment order), set B as primary → verify API returns B first
- [ ] Manual test: Verify frontend project settings page shows primary repo at top
- [ ] Manual test: Start a session and verify startup script runs in primary repo directory (check terminal output for "Working directory: <primary-repo-name>")
- [ ] Verify behavior when no primary is set (should fall back to existing ordering)

## Optional Enhancements

- [ ] Add unit test for `getProjectRepositories` sorting behavior
- [ ] Consider adding `is_primary` field to API response for each repo (frontend already marks this with a chip, but the data comes from a separate field)