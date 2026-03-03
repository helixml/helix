# Implementation Tasks

## Backend Changes

- [x] Add `OnGracePeriodExpired` callback field to `ConnectionManager` struct in `api/pkg/connman/connman.go`
- [x] Add `SetOnGracePeriodExpired(fn func(key string))` method to `ConnectionManager`
- [x] Call the callback in `cleanupExpired()` when grace period expires for a key
- [x] Add `ListSessionsBySandbox(ctx, sandboxID)` method to store interface and PostgresStore
- [x] Add `OnSandboxDisconnected(sandboxID string)` method to `HydraExecutor` that clears session metadata
- [x] Add `clearSessionsBySandbox(sandboxID string)` method to clear in-memory sessions map in `HydraExecutor`
- [x] Wire up the callback in `server.go` initialization to call `executor.OnSandboxDisconnected()`

## Testing

- [x] Add unit test for `ConnectionManager` callback invocation on grace period expiry
- [ ] Add unit test for `HydraExecutor.OnSandboxDisconnected` clearing session metadata
- [ ] Manual test: Start session → restart sandbox → verify UI shows "Paused" not spinner
- [ ] Manual test: After sandbox restart → click Resume → verify session restarts

## Documentation

- [ ] Update CLAUDE.md if any new patterns are introduced