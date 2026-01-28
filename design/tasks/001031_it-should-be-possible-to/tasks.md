# Implementation Tasks

- [~] Make description text clickable to enter edit mode in `SpecTaskDetailContent.tsx`
  - Wrap the description Typography in a Box with `onClick={handleEditToggle}`
  - Only enable click when `task.status === 'backlog'`

- [ ] Add hover styles to indicate editability
  - Cursor: `pointer` (backlog only)
  - Background: subtle highlight on hover using `action.hover`
  - Add slight padding/margin adjustment for natural hover area

- [ ] Test the feature:
  - Create a new SpecTask (stays in backlog)
  - Hover over description text - should see cursor change and highlight
  - Click description - should enter edit mode
  - Verify Save/Cancel still work
  - Start planning on a task - verify hover effect is gone for non-backlog tasks