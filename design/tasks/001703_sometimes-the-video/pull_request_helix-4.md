# Fix video streaming reconnect loop

## Summary

Video streaming sometimes gets stuck in a reconnect loop where it reconnects successfully but then almost immediately reconnects again. This PR fixes the issue by adding connection stability tracking and preventing duplicate reconnection attempts.

## Root Cause

The `reconnectAttempts` counter was being reset immediately on connection open (`onOpen()`). If a connection opened briefly then closed again (e.g., due to proxy issues), the counter would reset, and the cycle would repeat indefinitely. Each reconnect would "succeed" briefly, reset the counter, then fail.

## Changes

- **Connection stability timer**: Only reset `reconnectAttempts` after the connection has been stable for 2 seconds. This prevents rapid connect/disconnect loops from resetting the counter.
- **Reconnection guard**: If a reconnection timeout is already pending, don't schedule another one. This prevents duplicate reconnection attempts.
- **Enhanced logging**: Added detailed close event logging including code, reason, wasClean, connection duration, and whether the connection had stabilized.
- **Type update**: Added optional `code` property to the `disconnected` event type for frontend diagnostics.

## Testing

- Connection should stabilize after a brief disconnect (1-2 reconnect attempts)
- Rapid network toggling should not create infinite reconnect loops
- Stats for Nerds connection log now shows more diagnostic information
