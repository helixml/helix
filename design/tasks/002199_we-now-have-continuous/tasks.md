# Implementation Tasks: Reconcile Stale Dev Container State After Sandbox Restart

- [ ] Add a store query to list sessions for a given sandbox currently marked `ExternalAgentStatus = "running"` with a non-empty `ContainerName` (needed for the subtract pass).
- [ ] In `DiscoverContainersFromSandbox` (`api/pkg/external-agent/hydra_executor.go:1415`), build the set of session IDs hydra reports as live.
- [ ] Add the "subtract" branch: for DB-running sessions on that sandbox NOT in hydra's live set, set `ExternalAgentStatus = "stopped"`, clear container metadata, and evict the stale `h.sessions` entry (mirror `OnSandboxDisconnected` per-session logic).
- [ ] Ensure the subtract pass also runs when hydra reports zero containers (fix the `len(containers) == 0` early-return so it doesn't skip reconciliation).
- [ ] (Optional / measure first) In `listTasks` (`api/pkg/server/spec_driven_task_handlers.go:309-351`), replace the in-memory `GetSession` live-check with a real RevDial liveness probe (`HasRunningContainer`), keeping downgrade-only + skip-`starting` rules; probe at most once per sandbox to bound cost. If latency is unacceptable, keep the cheap check and document reliance on the reconnect reconciler.
- [ ] Add a Go unit test (gomock store) covering: hydra reports a subset → missing sessions become `stopped` with cleared metadata; hydra reports zero → all become `stopped`; surviving sessions stay `running`.
- [ ] Build check: `go build ./api/pkg/external-agent/ ./api/pkg/server/ ./api/pkg/store/ ./api/pkg/types/`.
- [ ] E2E in inner Helix: create spec task → live container shows `running` → simulate sandbox restart (kill container / restart hydra) → force reconnect → confirm Kanban card flips to `absent` and desktop viewer no longer errors; then confirm the task can be re-started cleanly.
- [ ] Verify no false downgrade for a re-adopted container after restart.
- [ ] Redeploy hydra into the sandbox container if hydra code changed (per CLAUDE.md hot-rebuild loop) and confirm reconnect logs.
