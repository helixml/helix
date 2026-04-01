# Auto-shutdown idle desktops after configurable timeout

## Summary

Desktops now automatically shut down when they have had no interaction activity for longer than the configured idle timeout (default 1 hour). A background goroutine checks every 5 minutes and calls `StopDesktop` + sets `ExternalAgentStatus = "terminated_idle"` for any desktop past the threshold.

## Changes

- `api/pkg/store/store.go` — add `ListIdleDesktops(ctx, idleSince)` to the Store interface
- `api/pkg/store/store_sessions.go` — implement `ListIdleDesktops` with a CTE-based SQL query; groups sessions by `external_agent_id` and uses `COALESCE(MAX(interactions.updated), MAX(sessions.updated))` as the activity marker
- `api/pkg/store/store_mocks.go` — add mock for `ListIdleDesktops`
- `api/pkg/store/store_desktop_idle_test.go` — Postgres store tests for all four cases (idle returned, recent interaction skipped, stopped desktop skipped, new desktop with no interactions skipped)
- `api/pkg/external-agent/idle_checker.go` — `RunDesktopIdleChecker` goroutine (5-minute tick)
- `api/pkg/config/config.go` — `DesktopIdleTimeout` (`HELIX_DESKTOP_IDLE_TIMEOUT`, default `1h`) and `DesktopIdleCheckInterval` (`HELIX_DESKTOP_IDLE_CHECK_INTERVAL`, default `5m`) env vars
- `api/pkg/server/server.go` — start the idle checker in `ListenAndServe`
