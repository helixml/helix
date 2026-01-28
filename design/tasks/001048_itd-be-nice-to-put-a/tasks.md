# Implementation Tasks

## Setup

- [x] Export `useSandboxState` hook from `ExternalAgentDesktopViewer.tsx`

## SpecTaskDetailContent.tsx Changes

- [x] Import `useSandboxState` from `ExternalAgentDesktopViewer`
- [x] Import `StopIcon` and `PlayArrow` icons from MUI
- [x] Call `useSandboxState(activeSessionId)` to get `isRunning`, `isPaused`, `isStarting`
- [x] Add `isStopping` state variable and `stopConfirmOpen` state for confirmation dialog
- [x] Add `handleStopSession` function that calls `v1SessionsStopExternalAgentDelete`
- [x] Add stop confirmation dialog (similar to restart dialog, warn about unsaved IDE buffers)
- [x] Add `handleStartSession` function that calls `v1SessionsResumeCreate`
- [x] Update big screen toolbar (around L1373): conditionally show Stop/Start/Restart based on state
- [x] Update small screen toolbar (around L1667): same conditional logic
- [x] Hide Restart button when desktop is stopped (restart doesn't make sense for stopped VM)
- [x] Hide Upload button when desktop is stopped (can't upload to stopped container)
- [x] Show Start button (with PlayArrow icon) when desktop is stopped
- [x] Show Stop button (with StopIcon) when desktop is running

## Testing

- [~] Verify stop button stops desktop and shows paused state
- [~] Verify start button resumes desktop from paused state
- [~] Verify restart button still works when running
- [~] Verify chat panel remains functional throughout stop/start cycle
- [~] Verify button states update correctly during transitions