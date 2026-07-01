# Design: Reconcile Stale Dev Container State After Sandbox Restart

## Summary

Make control-plane container state converge to hydra's ground truth after a sandbox
restart. Two complementary layers of defence:

1. **Reconcile on reconnect (primary fix):** when a sandbox reconnects and we
   discover its live containers, also **downgrade to `stopped`** any DB session that
   the control-plane still thinks is `running` on that sandbox but hydra no longer
   reports.
2. **Trustworthy Kanban live-check (defence in depth):** make `listTasks` verify
   liveness against the real container (via hydra RevDial) instead of the stale
   in-memory `h.sessions` map.

Either layer alone would fix the reported symptom; doing both closes the window
between restart and reconnect and removes the stale-map trap.

## Key facts (from investigation)

- Container liveness for the UI lives on `session.Metadata.ExternalAgentStatus`
  (`starting` / `running` / `stopped` / `terminated_idle` / `""`), plus
  `ContainerName`. SpecTask links to it via `PlanningSessionID`
  (`api/pkg/types/simple_spec_task.go`).
- `SandboxState` (`absent` / `starting` / `running`) is derived, not stored
  (`simple_spec_task.go`, gorm:"-"), computed in
  `spec_driven_task_handlers.go:309-351`.
- Hydra is ground truth: `ListDevContainers` returns only truly-alive containers
  after `RecoverDevContainersFromDocker` (`api/pkg/hydra/devcontainer.go`).
- Reconcile-on-reconnect entry point already exists:
  `DiscoverContainersFromSandbox` (`hydra_executor.go:1415`), triggered on RevDial
  connect (`server.go:~2690`) and sandbox register (`sandbox_handlers.go:~59`). It
  already `SetSandboxContainerCount` from hydra's count and adds discovered
  containers — it just never subtracts.
- `HasRunningContainer` (`hydra_executor.go:832`) already does a real RevDial
  liveness probe and evicts stale `h.sessions` entries — the Kanban path just
  doesn't use it.

## Design Decisions

### D1 — Add a "mark missing sessions stopped" branch to `DiscoverContainersFromSandbox`
After building the set of container/session IDs hydra reports as running for the
sandbox, query the store for sessions on that sandbox currently marked `running`
(and with a `ContainerName`). For any such session NOT in hydra's live set, set
`ExternalAgentStatus = "stopped"` and clear the container metadata (mirroring what
`OnSandboxDisconnected` does per-session), and evict the stale `h.sessions` entry.

- Scope the DB query by sandbox so we never touch sessions belonging to a different,
  healthy sandbox.
- Must handle the `len(containers) == 0` case too: currently that early-returns
  after setting the counter. The downgrade pass must run even when hydra reports
  zero containers (that's the full-wipe case). Move/extend the reconcile so the
  subtract step is not skipped by the empty early-return.

**Why here:** this is the single choke point that already runs on every reconnect
and already owns the additive half of the sync. Making it symmetric (add + subtract)
is the correct, non-hacky fix and removes reliance on the flaky grace-period path.

### D2 — Make the Kanban live-check trustworthy
In `spec_driven_task_handlers.go`, replace the in-memory `GetSession` liveness check
with a real liveness probe (`HasRunningContainer`, which uses hydra RevDial and
self-heals the stale map). Preserve existing safety rules:
- Only ever downgrade (`running`/`""` → `stopped`); never upgrade `starting` →
  `running` (avoids ScreenshotViewer 503s before RevDial is ready).
- On probe error treat as `stopped`/`absent`.

**Cost/consideration:** `listTasks` iterates all tasks and polls ~3.1s from the
frontend. A per-task RevDial round-trip could be expensive with many tasks. Mitigate
by probing once per sandbox (or per unique container) and/or only for sessions still
marked `running`. If per-task probing proves too costly, rely on D1 as the primary
fix and keep the cheap in-memory check — but D1 must then be solid because the
Kanban won't self-correct between reconnect events. **Recommendation:** implement D1
fully; add D2 only if measured latency is acceptable, otherwise document the
reliance on D1.

### D3 — No schema changes
`ExternalAgentStatus` already has a `stopped` value and `SandboxState` already maps
it to `absent`. No migration needed.

## Data / control flow (after fix)

```
CD redeploy → sandbox restart → dev containers destroyed
        │
sandbox reconnects (RevDial) ──▶ DiscoverContainersFromSandbox
        │                              ├─ SetSandboxContainerCount(hydra count)
        │                              ├─ ADD: mark discovered sessions "running"
        │                              └─ SUBTRACT (NEW): mark DB "running" sessions
        │                                  not in hydra's live set → "stopped",
        │                                  clear metadata, evict h.sessions entry
        ▼
listTasks derives SandboxState = "absent" → Kanban shows stopped, no dead viewer
```

## Implementation Notes (as built)

- **Reused `store.ListSessionsBySandbox`** — no new store query needed; the same
  method `OnSandboxDisconnected` uses.
- **New method `HydraExecutor.markMissingSessionsStopped`** (in
  `api/pkg/external-agent/hydra_executor.go`), called from
  `DiscoverContainersFromSandbox` right after the `SetSandboxContainerCount` block
  and **before** the `len(containers) == 0` early return (so the full-wipe case is
  covered).
- **Race avoidance:** for each DB session marked `running` on the sandbox that is
  NOT in hydra's live snapshot, we take the per-session **creation lock** (same lock
  `StartDesktop` uses), re-read the session, and do an authoritative
  `hydraClient.GetDevContainer(sessionID)` probe. Only if that probe fails do we
  downgrade. This prevents tearing down a container that `StartDesktop` (re)created
  after hydra's list snapshot was taken. We skip `starting` (only act on `running`
  with a `ContainerName`).
- **On downgrade:** clear `ContainerName/ContainerID/ContainerIP`, set
  `ExternalAgentStatus = "stopped"`, keep `DesiredState`/`SandboxID` (so a reconciler
  can restart it), persist, then `delete(h.sessions, id)` to evict the stale
  in-memory entry.

### Gotcha: `sandbox_id == "local"` must NOT be skipped
Initial implementation copied the `SetSandboxContainerCount` guard that skips
`sandboxID == "" || "local"`. That was wrong for status reconcile: the single-node
dev/self-hosted deployment registers its hydra as `sandbox_id = "local"` (confirmed
in api logs: `Auto-registered sandbox … sandbox_id=local`), and a self-hosted CD
redeploy wipes its containers the same way. The `"local"` skip only makes sense for
the multi-tenant autoscaler counter. Fixed: `markMissingSessionsStopped` skips only
the ambiguous empty id.

### E2E result (inner Helix, sandbox_id=local)
Reproduced the exact bug and confirmed the fix:
- Created a spec task → dev container running, session `external_agent_status=running`.
- Killed the container inside DinD + restarted hydra → RevDial reconnect →
  `DiscoverContainersFromSandbox("local")` → `ListDevContainers` no longer reports it.
- Reconcile downgraded the session to `stopped`, cleared container metadata, evicted
  the in-memory entry. Kanban card stopped showing a running/viewable desktop.
- The immediately-preceding healthy reconnect (container still alive) did NOT
  downgrade — no false positives.

### D2 decision (Kanban live-check): NOT changed
The primary reconcile sets DB → `stopped` and evicts the in-memory entry on
reconnect, so the existing cheap `GetSession` check in `listTasks` already resolves
to `absent` afterwards. Adding a per-task RevDial probe on the ~3.1s Kanban poll
would cost a hydra round-trip per running task per poll to cover only the brief
window before reconnect fires — a window already covered by `OnSandboxDisconnected`
on definitive disconnect. Kept the cheap check; rely on the reconnect reconciler.

## Testing Strategy

- **Go unit test** (gomock store, per repo pattern): drive
  `DiscoverContainersFromSandbox` with (a) hydra reporting a subset of DB-running
  sessions and (b) hydra reporting zero, and assert the missing sessions are set to
  `stopped` with metadata cleared, while surviving ones stay `running`.
- **E2E in inner Helix (mandatory for lifecycle change):** create a spec task, get a
  live dev container (verify `sandbox_state=running` on the Kanban), simulate the
  restart by killing the container in the sandbox / restarting hydra, force a
  reconnect, then confirm the card flips to absent and the desktop viewer no longer
  360s-503s. Follow CLAUDE.md's "test the next operation" rule: after it goes absent,
  confirm the task can be re-started cleanly.
- Verify no false downgrade for a container that is re-adopted on restart.
