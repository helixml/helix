# Implementation Tasks

- [~] Add `ListIdleDesktops(ctx context.Context, idleSince time.Time) ([]*types.Session, error)` to the store interface (`api/pkg/store/store.go`) and implement in `api/pkg/store/store_sessions.go`: query sessions where `external_agent_status = 'running'`, grouped by `external_agent_id`, where max interaction `updated` < `idleSince` (or no interactions and `session.updated` < `idleSince`)
- [ ] Add Postgres store tests for `ListIdleDesktops` in `api/pkg/store/` covering: returns idle desktop, skips desktop with recent interaction, skips already-stopped desktop, skips desktop with no interactions but recently updated session
- [ ] Add `runDesktopIdleChecker(ctx context.Context, executor ExternalAgentExecutor, store store.Store)` function in `api/pkg/external-agent/` that loops every 5 minutes, calls `ListIdleDesktops`, and for each result calls `StopDesktop` + `UpdateSessionMetadata` with `ExternalAgentStatus: "terminated_idle"`; log each shutdown at INFO level
- [ ] Add `DesktopIdleTimeout time.Duration` to the server config struct, populated from `HELIX_DESKTOP_IDLE_TIMEOUT` env var (default `1h`)
- [ ] Wire up `runDesktopIdleChecker` in `api/cmd/helix/serve.go` as a background goroutine (alongside existing background workers), passing the configured idle timeout
