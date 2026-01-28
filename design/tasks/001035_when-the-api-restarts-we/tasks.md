# Implementation Tasks

## Core Implementation

- [ ] Modify `websocket_server_user.go` to fetch session state after WebSocket upgrade
- [ ] Include session interactions in the initial state payload
- [ ] Send `session_update` event immediately after NATS subscription is established
- [ ] Add error handling for session lookup failures (log warning, don't block connection)

## Testing

- [ ] Manual test: Connect to session, restart API, verify UI receives current state
- [ ] Test with session that has active streaming interaction
- [ ] Test with non-existent session ID (should not crash)
- [ ] Verify no duplicate scroll/flicker issues in frontend on reconnect

## Documentation

- [ ] Add comment in code explaining why initial state sync is needed