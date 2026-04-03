# Design: Enter Key + Plus Button on Spec Task Details Page

## Overview

Add the "Enter to create new task" shortcut and a plus icon button to `SpecTaskDetailPage.tsx`. This mirrors existing functionality in `SpecTasksPage.tsx` — copy the same pattern directly.

## Key Files

- **Target**: `/frontend/src/pages/SpecTaskDetailPage.tsx` — only file that needs changes
- **Reference**: `/frontend/src/pages/SpecTasksPage.tsx` — has the working Enter key + plus button + NewSpecTaskForm panel pattern
- **Component**: `/frontend/src/components/tasks/NewSpecTaskForm.tsx` — already used in SpecTasksPage, import as-is

## Implementation Pattern

### State
Add `const [createDialogOpen, setCreateDialogOpen] = useState(false)` to the component.

### Enter key handler
Copy the `useEffect` from `SpecTasksPage` (lines ~380-413). Guards:
- Ignore if `e.ctrlKey || e.metaKey || e.altKey || e.shiftKey`
- Ignore if target is INPUT, TEXTAREA, contentEditable, or has tabindex attribute
- Toggle `createDialogOpen` on bare Enter; close on Escape

### Plus button
Add an `<IconButton>` with a `<Add>` (MUI) or `<Plus>` (lucide-react) icon to `topbarContent`, alongside the existing "Open in Split Screen" button. Tooltip: "Create New Task".

### NewSpecTaskForm panel
Render a slide-in right panel (same `Box` sx pattern as SpecTasksPage: `width: createDialogOpen ? "450px" : 0`, transition, borderLeft). Inside: `<NewSpecTaskForm projectId={projectId} onTaskCreated={handleTaskCreated} onClose={...} showHeader={true} embedded={false} />`.

The `topbarContent` in `Page` likely sits above the main content area, so the form panel should be rendered as a sibling of the main `<Box>` inside the `<Page>` — wrap both in a flex row container.

### handleTaskCreated
On task created, just close the panel (`setCreateDialogOpen(false)`). No navigation — user stays on the current detail page.

## Decisions

- **No new component**: all changes in `SpecTaskDetailPage.tsx` alone
- **Plus icon**: use `Add` from `@mui/icons-material` (already imported in the file as `ViewModule`) to stay consistent with MUI imports in this file; or use `Plus` from `lucide-react` if SpecTasksPage uses it (it does — line 29)
- **Panel layout**: need to change the Page's children from a single `<Box>` to a flex row `<Box>` containing both the detail content and the form panel (same as SpecTasksPage's layout approach)

## Codebase Patterns

- `SpecTasksPage` uses `Plus` from `lucide-react` for its create button (line 29, 921)
- The Enter key `useEffect` in `SpecTasksPage` cleans up the listener in the return — copy exactly
- `NewSpecTaskForm` receives `projectId` as a prop — this is already available in `SpecTaskDetailPage` via `route.params.id`
