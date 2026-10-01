# Golden builds survive CD

**Problem.** Every merge to main triggers a golden build (25–90 min) *and* a
`deploy-meta` that often recreates the sandbox and always restarts the API (Air).
The deploy killed the build it triggered; the API lost the in-memory monitor,
`pendingRebuild` and debounce maps; Hydra lost its `monitorGoldenBuild` goroutine.
Interrupted builds were reset to `none` and never retried.

**Decision (product).** Deploys do not wait for or coordinate with golden builds.
Golden builds self-heal instead.

## Design

### State: `golden_builds` table (one row per project × sandbox)
`types.SandboxCacheState` is now the row (`TableName() = golden_builds`), and it is
the single source of truth for the API, UI and reconciliation. Fields added:
`attempt`, `triggered_at`, `pending_rebuild`, `interrupt_reason`, `next_retry_at`.

Why not `project.Metadata.DockerCacheStatus`: `UpdateProject` is a GORM `Save` of
the whole metadata JSON. Any project save (board settings, auto-warm toggle,
the old prune-on-read) raced with the build service and could write back a stale
copy, silently losing a pending rebuild or attempt count. A row updated with
`SELECT … FOR UPDATE` (`store.UpdateGoldenBuild`) makes every transition atomic and
fenceable. The project GET handler fills `metadata.docker_cache_status` from the
table, so the API shape is unchanged. A one-time migration copies existing
metadata entries into the table and drops the metadata key. Rows cascade-delete
with the project.

### State machine (`GoldenBuildService`)
- `claimBuild` (atomic): `building` already → a new trigger sets `pending_rebuild`;
  otherwise → `building`, `attempt=1` for a new trigger, `attempt+1` for a retry.
- Every later transition is fenced on `build_session_id`, so a cancelled or
  superseded build's monitor can't overwrite newer state.
- The monitor polls Hydra with `GoldenBuildContainerRunning(sandbox, session)`,
  which tells *gone* (404) apart from *unreachable* (error) and doesn't depend on
  the executor's in-memory session map (empty after an API restart).
  While Hydra is unreachable the monitor just waits.
- Outcomes:
  | Result | Status |
  |---|---|
  | success | `ready` |
  | script exit ≠ 0, timeout, promotion failure | `failed`, not retried |
  | container gone + no result (sandbox recreate / Hydra lost it), container exited without writing a result, StartDesktop failed, API died before the session existed | interruption |
- Interruption: auto-warm projects → `retrying` with `next_retry_at` (1m, 2m, …),
  up to `GoldenBuildMaxAttempts` (3) per trigger, then `failed: Interrupted N
  times, giving up: <reason>`. Non-auto-warm (manual) → `failed`. A pending
  rebuild supersedes the retry (a fresh trigger).
- `ReconcileSandbox(sandboxID)` is the single "what next" decision: resume a monitor
  for a `building` row no goroutine in this process owns, start due retries,
  start pending rebuilds. It runs on Hydra RevDial connect (after container
  discovery), on the existing 30s sandbox reconcile sweep, and after every
  terminal transition. `RecoverStaleBuilds` and the lazy recovery in `getProject`
  are gone.
- The only process memory left is `monitoring` (session IDs this process is
  polling), which exists purely to avoid starting duplicate goroutines.

### Hydra
Golden containers get labels `helix.golden_build`, `helix.project_id`,
`helix.golden_build_deadline`. `RecoverDevContainersFromDocker` →
`adoptRecoveredContainer` restores `IsGoldenBuild`/`ProjectID`/deadline and
restarts `monitorGoldenBuild`. A build that survives a Hydra restart then
completes and promotes normally. Startup GC doesn't reap the running build's
data, because session data is protected by `.last-active` markers.

## Known limits
- Hydra keeps build results in memory. If Hydra restarts in the ~15s between
  promoting a build and the API reading the result, the API sees "no result" and
  retries. That costs one extra build; it doesn't lose correctness.
- Containers created before this change have no golden labels. A Hydra restart
  during such a build is handled as an interruption (retried), not resumed.
