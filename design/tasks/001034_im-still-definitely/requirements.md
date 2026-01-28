# Requirements: Video Frame Ordering Investigation

## Problem Statement

User reports seeing out-of-order video frames during desktop streaming. The hypothesis is that the frontend may have multiple WebSocket connections open simultaneously for video, causing interleaved frame delivery.

## User Stories

1. **As a user**, I want video frames to always display in the correct order so that the remote desktop appears smooth and coherent.

2. **As a developer**, I want clear logging/diagnostics when multiple video connections exist so I can debug connection lifecycle issues.

## Acceptance Criteria

### Investigation

- [ ] Confirm or rule out multiple simultaneous WebSocket video connections
- [ ] Identify the code path(s) that could create duplicate connections
- [ ] Document any race conditions in connection lifecycle (mount/unmount, reconnect, mode-switch)

### Fix (if multiple connections confirmed)

- [ ] Only one WebSocket video stream active per `DesktopStreamViewer` instance at any time
- [ ] Clean up previous connection before establishing new one (no race window)
- [ ] Log warnings when duplicate connection attempts are detected

### Verification

- [ ] No frame ordering issues during normal streaming
- [ ] No frame ordering issues during reconnection scenarios
- [ ] No frame ordering issues during video/screenshot mode switching
- [ ] No duplicate WebSocket connections visible in browser DevTools during any workflow

## Out of Scope

- Backend frame ordering (already uses single-threaded appsink → channel → WebSocket)
- Network-level packet reordering (WebSocket over TCP guarantees order)
- Decoder-level frame reordering (PTS-based, already handled)