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

This means **any** completed task shows the "Task finished" message, regardless of whether the container is still running.

## Solution

Change the condition to also check `isDesktopRunning` (already available from `useSandboxState`):

```tsx
const shouldShowDesktopViewer = !isDesktopPaused || isDesktopRunning || isDesktopStarting;

// In render...
{currentView === "desktop" &&
  (isTaskCompleted && !shouldShowDesktopViewer ? (
    <Alert>Task finished...</Alert>
  ) : (
    <ExternalAgentDesktopViewer ... />
  ))}
```

The `ExternalAgentDesktopViewer` already handles showing appropriate UI for paused/stopped states, so we can just render it and let it show its own "desktop is paused" state if needed.

## Files to Modify

- `helix/frontend/src/components/tasks/SpecTaskDetailContent.tsx`
  - Two locations: desktop view (big screen) around L1886, and mobile view around L2352

## Edge Cases

- **Container starting**: Show desktop viewer (it handles the loading state)
- **Container running**: Show desktop viewer
- **Container stopped + task completed**: Show desktop viewer in "paused" state with play button to restart
- **Container stopped + task not completed**: Desktop viewer shows "paused" state (existing behavior)
- **Archived tasks**: Keep existing behavior (always show archived message)

## Key Insight

The `ExternalAgentDesktopViewer` already handles showing a "paused" state with a play button when the container is stopped. So for completed tasks, we should just render the desktop viewer unconditionally (same as non-completed tasks). This gives the user:
1. The desktop view when running
2. A play button to restart when stopped

The "Task finished" alert in the details panel is sufficient to inform the user the task is done - we don't need to block the desktop view.