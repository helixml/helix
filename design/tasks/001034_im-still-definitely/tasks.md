# Implementation Tasks

## Investigation Phase

- [ ] Add console logging to `DesktopStreamViewer.connect()` to track all connection attempts with timestamps
- [ ] Add console logging to `WebSocketStream` constructor to log instance creation with unique IDs
- [ ] Add browser DevTools check: monitor Network tab for multiple `/ws/stream` WebSocket connections during normal use
- [ ] Test reconnection scenarios (tab hide/show, network disconnect, bitrate change) to reproduce duplicate connections

## Fix: Connection Singleton Guard

- [ ] Create `frontend/src/lib/helix-stream/connectionGuard.ts` with `acquireConnection()` and `releaseConnection()` functions
- [ ] Track active connections by sessionId with component instance IDs
- [ ] Log warnings when duplicate connection attempts are detected
- [ ] Integrate guard into `DesktopStreamViewer.connect()` - call `acquireConnection()` before creating WebSocketStream
- [ ] Integrate guard into `DesktopStreamViewer.disconnect()` - call `releaseConnection()` on cleanup

## Fix: Disable WebSocketStream Internal Auto-Reconnect

- [ ] Add `disableAutoReconnect` option to `WebSocketStream` constructor
- [ ] Pass `disableAutoReconnect: true` from `DesktopStreamViewer`
- [ ] Ensure `DesktopStreamViewer` reconnect logic handles all reconnection scenarios

## Fix: Close Stale WebSocket Before Creating New One

- [ ] In `WebSocketStream.connect()`, ensure old `this.ws` is fully closed before creating new WebSocket
- [ ] Add guard to prevent `connect()` being called while already connecting (use `connecting` state flag)
- [ ] Clear any pending reconnect timeouts when `close()` is called explicitly

## Verification

- [ ] Manual test: Open stream, verify single WebSocket in DevTools
- [ ] Manual test: Trigger reconnect (disconnect network), verify old WS closes before new one opens
- [ ] Manual test: Switch video/screenshot mode rapidly, verify no duplicate connections
- [ ] Manual test: Change bitrate setting, verify clean reconnect with single connection
- [ ] Check console logs for any "Duplicate connection attempt" warnings during all scenarios