# Implementation Tasks

- [ ] Add `onSwitchToBoard?: () => void` prop to `TabsViewProps` interface in `TabsView.tsx`
- [ ] Update `TabsView` component to render a "Board View" button when `onSwitchToBoard` is provided
- [ ] Position button in the left side of the first TaskPanel's tab bar area
- [ ] Pass `() => setViewMode("kanban")` callback to `TabsView` in `SpecTasksPage.tsx`
- [ ] Test: Verify clicking button switches from Split Screen to Board view
- [ ] Test: Verify button is visible on desktop and tablet breakpoints