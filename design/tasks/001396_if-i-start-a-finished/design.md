# Design: Show Desktop for Finished Tasks When Container Running

## Current Behavior

In `SpecTaskDetailContent.tsx`, the rendering logic for the desktop view checks `isTaskCompleted` first:

```tsx
const isTaskCompleted = task?.status === "done" || task?.merged_to_main;

// Later in render...
{currentView === "desktop" &&
  (isTaskCompleted ? (
    <Alert>Task finished...</Alert>
  ) : (
    <ExternalAgentDesktopViewer ... />
  ))}
```

This means **any** completed task shows the "Task finished" message, regardless of whether the container is still running. Users can't see or restart the desktop.

## Solution

Change the condition to show:
1. **Desktop viewer** when container is running or starting (even if task is completed)
2. **"Task finished" message with play button** when container is stopped and task is completed

```tsx
{currentView === "desktop" &&
  (isTaskCompleted && isDesktopPaused ? (
    <Box>
      <Alert>Task finished...</Alert>
      <Button onClick={handleStartDesktop}>Start Desktop</Button>
    </Box>
  ) : isTaskArchived ? (
    <Alert>Task rejected...</Alert>
  ) : (
    <ExternalAgentDesktopViewer ... />
  ))}
```

The start button handler already exists (`handleStartDesktop` or similar) - it's used elsewhere in the component for the play button in the header.

## Files to Modify

- `helix/frontend/src/components/tasks/SpecTaskDetailContent.tsx`
  - Two locations: desktop view (big screen) around L1886, and mobile view around L2352
  - Add a "Start Desktop" button to the "Task finished" alert

## Edge Cases

- **Container starting**: Show desktop viewer (it handles the loading state)
- **Container running**: Show desktop viewer
- **Container stopped + task completed**: Show "Task finished" message with play button to restart
- **Container stopped + task not completed**: Desktop viewer shows "paused" state (existing behavior)
- **Archived tasks**: Keep existing behavior (always show archived message, no restart option)