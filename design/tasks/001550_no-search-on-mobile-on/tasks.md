# Implementation Tasks

- [ ] In `frontend/src/components/tasks/BacklogFilterBar.tsx`, change the root `Box` `sx` prop to add `display: { xs: "none", md: "flex" }` so the entire filter bar (search + priority filter) is hidden on mobile screens
- [ ] Verify the backlog table still shows all tasks (unfiltered) on mobile when the backlog is expanded
- [ ] Verify on desktop the search and priority filter still work correctly (no regression)
