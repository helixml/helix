# Implementation Tasks

## Fix BacklogTableView Field References

- [ ] Update `handlePromptClick` to load from `description` with fallback to `original_prompt`
- [ ] Update display text to show `description` with fallback to `original_prompt`
- [ ] Update search filter to search `description` with fallback to `original_prompt`

## Verification

- [ ] Test editing a task description in BacklogTableView and verify it persists
- [ ] Test that starting planning uses the edited description
- [ ] Run `cd frontend && yarn build` to verify no TypeScript errors