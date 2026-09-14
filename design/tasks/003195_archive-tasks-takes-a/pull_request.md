# fix(api): archive spec tasks without blocking on desktop teardown

## Summary

Archiving a spec task blocked the HTTP response until desktop teardown finished. `archiveSpecTask` ran `StopDesktop` synchronously before responding — and `StopDesktop` is heavyweight: paused-screenshot capture, RevDial dev-container delete, billing-row settle, and session API-key revocation, with a 5-minute internal timeout. It also fired for every archived task carrying a stale `planning_session_id`, even long-finished ones, which is why archiving routinely felt extremely slow.

The agent-stopping block now runs in a background goroutine with its own detached context (`detachContext(r.Context(), 5*time.Minute)`). The handler marks the task archived and responds immediately; the desktop stops run afterwards and remain best-effort (errors are logged and never fail the archive, same as before). The goroutine only reads values captured before it starts (`planningSessionID`, `taskID`), so it does not race with the `task.Archived` write. No API contract changes — same endpoint, same request/response shape; the CLI archive path gets the same speedup automatically.

## Testing

- `CGO_ENABLED=0 go test -v -run TestSpecTaskKeepAliveSuite ./pkg/server/ -count=1` — all 6 tests pass, including the new `TestArchiveTask_ReturnsBeforeDesktopStopCompletes`, which blocks `StopDesktop` on a channel and asserts the archive handler still returns 200 while the stop is pending (fails deterministically if the stop ever blocks the response again)
- `go build ./pkg/server/` and `go build ./...` — clean
- `CGO_ENABLED=0 go test ./pkg/server/ -count=1` — the 2 failures (`TestInProcClient_DeleteLinkedAgentPreservesConfiguredProjectAndUnsetsAgentID`, `TestInProcClient_DeleteLinkedAgentContinuesWhenSessionStopFails`) fail identically on the base commit without my change (pre-existing, unrelated)
- End-to-end against the local dev stack: created a fixture task in `done` status with a fake `zed_external` planning session and archived it via `PATCH /api/v1/spec-tasks/{id}/archive` — response returned in **16ms**, and API logs show `SpecTask archived` (response path, handler line 1565) before `Stopped agent session when archiving` (background goroutine, handler line 1520), proving the stop runs after the response
- Race safety reviewed by inspection (no C compiler in sandbox for `-race`): the goroutine touches no shared mutable state
