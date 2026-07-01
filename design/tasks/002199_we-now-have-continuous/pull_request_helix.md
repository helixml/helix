# Reconcile stale dev container state after sandbox restart

## Summary
After a continuous-delivery redeploy stops and restarts the sandbox, its dev
containers are destroyed but the control-plane kept reporting them as **running**.
The spec-task details page / Kanban then showed live cards whose desktop viewer
503s when opened ("Sandbox not connected"). The reconnect-time discovery was
**additive only** — it marked discovered containers running and fixed the counter,
but never marked now-missing sessions stopped. The only downgrade path
(`OnSandboxDisconnected`) requires the disconnect grace period to expire, which is
unreliable on a fast redeploy.

This makes the reconnect discovery symmetric: it now also downgrades sessions the
control-plane still believes are running on that sandbox but that hydra no longer
reports.

## Changes
- `api/pkg/external-agent/hydra_executor.go`
  - New `markMissingSessionsStopped(...)`, called from
    `DiscoverContainersFromSandbox` right after the container-count resync and
    **before** the empty-list early return (so the full-wipe case is covered).
  - For each DB session marked `running` on the sandbox that is absent from hydra's
    live snapshot, it takes the per-session **creation lock** (same lock
    `StartDesktop` uses), re-reads the session, and does an authoritative
    `GetDevContainer` probe. Only if the probe confirms the container is gone does
    it clear container metadata, set `ExternalAgentStatus = "stopped"`, and evict
    the stale in-memory entry. This avoids racing a concurrent `StartDesktop` that
    re-created the container after hydra's snapshot.
  - `starting` sessions are never touched; `DesiredState`/`SandboxID` are preserved
    so a reconciler can restart the session.
  - Applies to the single-node `local` sandbox too (only the ambiguous empty id is
    skipped) — a self-hosted redeploy wipes its containers the same way.
- `api/pkg/external-agent/reconcile_missing_sessions_test.go` — new gomock unit
  tests: subset → only missing session downgraded (+ in-memory eviction), live left
  alone, `starting` skipped; zero containers → all downgraded; `local` processed;
  empty id skipped.

## Testing
- Unit tests pass (`go test ./pkg/external-agent/ -run TestMarkMissingSessionsStopped`).
- `go build ./pkg/external-agent/ ./pkg/server/ ./pkg/store/ ./pkg/types/` — OK.
- E2E in inner Helix (sandbox_id=local): created a spec task with a live dev
  container, then killed the container + restarted hydra to force a reconnect with
  the container gone. The reconcile logged *"Marked stale dev container session
  stopped (not reported by hydra after reconnect)"*, the DB session flipped to
  `stopped` with container metadata cleared, and the Kanban card stopped showing a
  running/viewable desktop. The immediately-preceding healthy reconnect (container
  alive) did **not** downgrade — no false positives.

## Screenshots
![Kanban after reconcile](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002199_we-now-have-continuous/screenshots/01-kanban-after-reconcile.png)
