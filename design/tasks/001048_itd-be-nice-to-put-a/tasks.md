# Implementation Tasks

## Setup

- [ ] Export `useSandboxState` hook from `ExternalAgentDesktopViewer.tsx`

## SpecTaskDetailContent.tsx Changes

- [ ] Import `useSandboxState` from `ExternalAgentDesktopViewer`
- [ ] Import `StopIcon` and `PlayArrow` icons from MUI
- [ ] Call `useSandboxState(activeSessionId)` to get `isRunning`, `isPaused`, `isStarting`
- [ ] Add `isStopping` state variable and `stopConfirmOpen` state for confirmation dialog
- [ ] Add `handleStopSession` function that calls `v1SessionsStopExternalAgentDelete`
- [ ] Add stop confirmation dialog (similar to restart dialog, warn about unsaved IDE buffers)
- [ ] Add `handleStartSession` function that calls `v1SessionsResumeCreate`
- [ ] Update big screen toolbar (around L1373): conditionally show Stop/Start/Restart based on state
- [ ] Update small screen toolbar (around L1667): same conditional logic
- [ ] Hide Restart button when desktop is stopped (restart doesn't make sense for stopped VM)
- [ ] Hide Upload button when desktop is stopped (can't upload to stopped container)
- [ ] Show Start button (with PlayArrow icon) when desktop is stopped
- [ ] Show Stop button (with StopIcon) when desktop is running

## Testing

- [ ] Verify stop button stops desktop and shows paused state
- [ ] Verify start button resumes desktop from paused state
- [ ] Verify restart button still works when running
- [ ] Verify chat panel remains functional throughout stop/start cycle
- [ ] Verify button states update correctly during transitions