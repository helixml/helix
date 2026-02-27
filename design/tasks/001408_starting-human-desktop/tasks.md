# Implementation Tasks

## Backend (api/pkg/external-agent/hydra_executor.go)

- [ ] Add status message "Creating container..." before calling `hydraClient.CreateDevContainer()`
- [ ] Add status message "Starting desktop environment..." after container created, before `waitForDesktopBridge()`
- [ ] Add status message "Connecting to desktop..." inside `waitForDesktopBridge()` loop (every 10 seconds)
- [ ] Ensure status message is cleared on both success and error paths

## Frontend (frontend/src/components/external-agent/ExternalAgentDesktopViewer.tsx)

- [ ] Add `startTime` state to track when startup began
- [ ] Set `startTime` when `isStarting` becomes true, clear when false
- [ ] Add `elapsedSeconds` calculation with 1-second interval update
- [ ] Display elapsed time suffix "(Xs)" after 5 seconds of waiting
- [ ] Apply elapsed time display to both screenshot mode and stream mode UI

## Testing

- [ ] Verify "Creating container..." appears immediately on start
- [ ] Verify "Unpacking build cache (X/Y GB)" shows during golden cache copy (if applicable)
- [ ] Verify "Starting desktop environment..." appears after container creation
- [ ] Verify "Connecting to desktop..." appears during long bridge waits
- [ ] Verify elapsed time appears after 5 seconds
- [ ] Verify messages clear when desktop becomes running
- [ ] Test both Kanban card (screenshot) and floating window (stream) modes