# Design: Desktop Auto-Shutdown After Idle Timeout

## Architecture

The feature adds a periodic background goroutine started in `api/cmd/helix/serve.go`, alongside existing background workers (knowledge reconciler, trigger manager, etc.).

### Key Files

| File | Role |
|------|------|
| `api/pkg/external-agent/hydra_executor.go` | `CleanupExpiredSessions()` already defined (line ~605) but never called. Extend or replace it to use DB queries. |
| `api/pkg/store/store_sessions.go` | Add `ListIdleDesktops(ctx, idleThreshold time.Time)` query |
| `api/cmd/helix/serve.go` | Wire up the periodic goroutine |

## Data Flow

```
serve.go startup
  └─ go runDesktopIdleChecker(ctx, hydraExecutor, store)
        every 5 minutes:
          store.ListIdleDesktops(ctx, time.Now().Add(-1h))
            SQL: SELECT DISTINCT ON (s.config->>'external_agent_id') s.id, s.config
                 FROM sessions s
                 LEFT JOIN interactions i ON i.session_id = s.id
                 WHERE s.config->>'external_agent_status' = 'running'
                   AND s.deleted_at IS NULL
                 GROUP BY s.id, s.config
                 HAVING MAX(i.updated) < $1
                    OR (COUNT(i.id) = 0 AND s.updated < $1)
          for each idle session:
            hydraExecutor.StopDesktop(ctx, session.ID)
            store.UpdateSessionMetadata(ctx, session.ID, {ExternalAgentStatus: "terminated_idle"})
```

## Key Decisions

**Use the store query approach instead of in-memory**: `CleanupExpiredSessions()` currently works on in-memory session state tracked by `HydraExecutor`. This misses sessions from a previous API restart. Using a DB query is authoritative and resilient to restarts.

**Group by `external_agent_id`**: A single Zed instance (`ExternalAgentID` in `SessionMetadata`) may have multiple associated sessions (planning + implementation). The idle check must look at interactions across all sessions sharing the same `external_agent_id`. The stop call uses the session's `SandboxID` and `ExternalAgentID` to identify the container.

**Fall back to `session.Updated` when no interactions exist**: A freshly created desktop with no interactions yet should not be immediately eligible for shutdown. Use `session.Updated` as the activity timestamp when `COUNT(interactions) = 0`.

**Check interval: 5 minutes**: Balances responsiveness (idle for up to 65 minutes max) against DB query overhead.

**Idle threshold: 1 hour**: Configurable via a constant; exposes as an optional env var in the future if needed.

## Codebase Patterns

- Background goroutines are started in `serve.go` with `go someService.Start(ctx)` pattern
- `store.UpdateSessionMetadata()` is the correct way to update `SessionMetadata` fields
- `HydraExecutor.StopDesktop(ctx, sessionID)` is the correct shutdown call
- `ExternalAgentStatus` values: `"running"`, `"stopped"`, `"terminated_idle"` (already used in `ClearStaleStartingSessions`)
- GORM soft-delete: filter `deleted_at IS NULL` is applied automatically by GORM scopes; raw SQL in store methods must add it explicitly
