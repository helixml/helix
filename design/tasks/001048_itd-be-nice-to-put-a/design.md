# Design: Stop/Start Buttons for Agent Spec Task Page

## Context

The `SpecTaskDetailContent.tsx` component currently has a restart button in the toolbar (appears in two places for big screen and small screen layouts). The restart button stops then immediately starts the session.

Users need the ability to just stop without restarting (to free resources), and to start again later.

## Existing Patterns

### API Endpoints (already exist)
- `v1SessionsStopExternalAgentDelete(sessionId)` - stops the desktop container (same endpoint used by the X button in `AgentSandboxes.tsx` admin dashboard)
- `v1SessionsResumeCreate(sessionId)` - starts/resumes the desktop container

### Session State Hook
`ExternalAgentDesktopViewer.tsx` has `useSandboxState(sessionId)` which returns:
- `isRunning` - container is running
- `isPaused` - container is stopped/absent
- `isStarting` - container is starting up

### Existing UI Patterns
- `SpecTasksPage.tsx` shows Stop/Resume buttons for exploratory sessions
- `ExternalAgentDesktopViewer.tsx` shows a "Start Desktop" button overlay when paused
- Icons used: `StopIcon`, `PlayArrow`, `RestartAltIcon`

## Design Decisions

### 1. Reuse `useSandboxState` hook
Import and use the existing `useSandboxState` hook in `SpecTaskDetailContent.tsx` to track desktop state. This is the same hook the `ExternalAgentDesktopViewer` uses internally.

### 2. Button visibility logic
| Desktop State | Stop Button | Start Button | Restart Button | Upload Button |
|---------------|-------------|--------------|----------------|---------------|
| Running       | ✓ Visible   | Hidden       | ✓ Visible      | ✓ Visible     |
| Stopped       | Hidden      | ✓ Visible    | Hidden         | Hidden        |
| Starting      | Hidden      | Disabled     | Hidden         | Hidden        |

### 3. Confirmation for stop
Stop requires confirmation because unsaved files in memory (e.g., IDE buffers) will be lost. Use a similar dialog to the restart confirmation but with appropriate messaging for stop.

### 4. Placement
Stop button appears immediately before the restart button (when both visible). Start button appears in the same position when stopped.

## Component Changes

### SpecTaskDetailContent.tsx

1. Import `useSandboxState` from `ExternalAgentDesktopViewer.tsx` (need to export it first)
2. Import `StopIcon` and `PlayArrow` icons
3. Add state for `isStopping` and `isStarting`
4. Add `handleStopSession` function (calls `v1SessionsStopExternalAgentDelete`)
5. Add `handleStartSession` function (calls `v1SessionsResumeCreate`)
6. Conditionally render Stop/Start/Restart buttons based on desktop state

### ExternalAgentDesktopViewer.tsx

Export `useSandboxState` hook so it can be reused.

## UI Mockup (text)

**When Running:**
```
[Clone] [Stop ⏹] [Restart ↻] [Upload ⬆]
```

**When Stopped:**
```
[Clone] [Start ▶]
```

## Error Handling

- Stop failures: Show snackbar error, reset button state
- Start failures: Show snackbar error, reset button state
- Both operations invalidate session query to refresh state