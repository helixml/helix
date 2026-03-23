fix(websocket_sync): preserve streaming content in Go accumulator + update ZED_COMMIT

## Summary

Two related fixes to the Zed WebSocket sync pipeline:

1. **Go-side**: Skip empty-content DB writes in `handleMessageAdded` so that
   the initial ACP `content_block_start` event (which carries empty text)
   doesn't overwrite `ResponseMessage=""` to the DB, leaving `dirty=false`
   for subsequent writes.

2. **ZED_COMMIT update**: Pins `sandbox-versions.txt` to the new zed-4 `main`
   HEAD after the upstream merge + streaming content fix PR is merged.

## Changes

- `api/pkg/server/websocket_external_agent_sync.go`: Skip DB write in
  `handleMessageAdded` when `acc.Content == ""`. Keeps `lastDBWrite` at zero
  so the next non-empty write fires immediately instead of being throttled.
- `api/pkg/server/websocket_external_agent_sync_test.go`: New test
  `TestStreamingThrottle_EmptyContentSkipsDBWrite` verifying the guard.
- `api/pkg/server/test_helpers.go`: Added `StreamingContextInteractionID()`
  helper for E2E test observability.
- `sandbox-versions.txt`: Update `ZED_COMMIT` to new SHA.

## Verification

All 47 Go unit tests pass:
```
go test -v -run TestWebSocketSyncSuite ./pkg/server/ -count=1
```

E2E test (`run_docker_e2e.sh`) passes with both agents — store validation
confirms non-empty `ResponseMessage` and `ResponseEntries` for all phases.

Release Notes:

- N/A
