# Implementation Tasks

- [x] Add `taskSearchQuery` state variable to `TaskPanel` component
- [x] Add `filteredTasks` useMemo that filters `unopenedTasks` by search query
- [x] Import `SearchIcon` and `InputAdornment` from MUI (if not already imported)
- [x] Add search TextField at top of Menu, after `anchorEl` and before "Create New Task"
- [x] Set `autoFocus` on TextField for immediate typing
- [x] Reset `taskSearchQuery` to empty string when menu closes (`onClose` handler)
- [x] Update task list rendering to use `filteredTasks` instead of `unopenedTasks`
- [x] Add "No matching tasks" message when `filteredTasks` is empty but `unopenedTasks` is not
- [ ] Test: Verify search filters task list in real-time
- [ ] Test: Verify Human Desktop option remains visible regardless of search query
- [ ] Test: Verify search field is focused when menu opens
- [ ] Test: Verify search resets when menu closes and reopens