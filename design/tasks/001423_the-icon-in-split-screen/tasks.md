# Implementation Tasks

- [~] Add `taskSearchQuery` state variable to `TaskPanel` component
- [~] Add `filteredTasks` useMemo that filters `unopenedTasks` by search query
- [~] Import `SearchIcon` and `InputAdornment` from MUI (if not already imported)
- [~] Add search TextField at top of Menu, after `anchorEl` and before "Create New Task"
- [~] Set `autoFocus` on TextField for immediate typing
- [~] Reset `taskSearchQuery` to empty string when menu closes (`onClose` handler)
- [~] Update task list rendering to use `filteredTasks` instead of `unopenedTasks`
- [~] Add "No matching tasks" message when `filteredTasks` is empty but `unopenedTasks` is not
- [ ] Test: Verify search filters task list in real-time
- [ ] Test: Verify Human Desktop option remains visible regardless of search query
- [ ] Test: Verify search field is focused when menu opens
- [ ] Test: Verify search resets when menu closes and reopens