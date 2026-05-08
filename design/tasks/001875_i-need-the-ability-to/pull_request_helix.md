# Add pin/unpin button to project detail page

## Summary
Users can now pin/unpin a project directly from the project detail page (SpecTasksPage), without navigating back to the projects list.

## Changes
- Added pin/unpin `IconButton` to the project detail page toolbar, positioned between the members bar and action buttons
- Wired to existing `usePinProject` / `useUnpinProject` hooks — no backend changes needed
- Uses the same purple pin icon style (`#a78bfa`) as the projects list for visual consistency
- Hidden on mobile viewports, consistent with other toolbar actions

## Test plan
- [ ] Navigate to a project's detail page and verify the pin icon appears in the toolbar
- [ ] Click the pin icon — it should fill in and turn purple
- [ ] Navigate back to projects list — the project should appear in the "Pinned" section
- [ ] Return to the project and click to unpin — icon should revert to outline
- [ ] Verify the projects list no longer shows it as pinned
