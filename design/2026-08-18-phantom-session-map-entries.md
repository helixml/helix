# Phantom in-memory session entries make "Start desktop" a silent no-op

## Symptom

A spec task whose sandbox had been idle-stopped could not be restarted. Clicking
**Start** produced a success toast and nothing else — no container, no desktop,
and the stream WebSocket retrying `no connection` forever.

`POST /api/v1/sessions/{id}/resume` returned **HTTP 200**:

```
session_handlers.go:2327  Resuming external agent session
hydra_executor.go:180     Starting dev container via Hydra
hydra_executor.go:196     Dev container already running, returning existing session   <- false
session_handlers.go:2364  External agent session resumed successfully  dev_container_id=
```

…while the sandbox had no such container at all:

```
$ docker exec helix-sandbox-nvidia-1 docker ps -a | grep ubuntu-external-<sid>
(nothing)
```

The empty `dev_container_id` in the success line is the tell: that field is only
unset on the early-return branch, which never touches a container.

## How the state arises

`HydraExecutor` keeps two records of whether a session is running: the
`sessions` map in memory and `external_agent_status` on the session row. An
idle shutdown racing a discovery sweep desynchronises them:

| time | actor | effect |
|---|---|---|
| 22:06:56 | `idle_checker` → `StopDesktop` | takes the session lock, deletes the map entry, begins stopping the container (~5s) |
| 22:07:01 | discovery sweep | container is still in hydra's snapshot; entry is untracked, so it is **re-added** to the map as `running` (`hydra_executor.go` "Recovered container from sandbox discovery") and the DB row is written back to `running` |
| 22:07:01 | `idle_checker` (post-stop, outside the lock) | re-reads the session and writes `external_agent_status = "terminated_idle"` — landing **after** discovery's write |

Final state: DB says `terminated_idle`, the map says `running`. The container is
gone.

## Why it never self-healed

The two halves each prevented the other from being repaired:

- **`StartDevContainer`** trusted `h.sessions[id].Status == "running"` with no
  liveness check, so every resume short-circuited.
- **`markMissingSessionsStopped`** — which owns the `delete(h.sessions, id)`
  eviction — built its candidate list purely from the DB and skipped anything
  not marked `running`:

  ```go
  if session.Metadata.ExternalAgentStatus != "running" || session.Metadata.ContainerName == "" {
      continue
  }
  ```

  The row said `terminated_idle`, so the session was skipped on every sweep and
  the eviction line was unreachable.

The session was therefore stuck until the API process restarted. `sandbox_state`
correctly derived `absent`, so the UI kept offering a Start button that could
never work.

## Fix

1. **`markMissingSessionsStopped` reconciles both stores.** The candidate set
   (`staleReconcileCandidates`) is now the union of DB rows that claim to be
   running and in-memory entries pinned to the sandbox. The DB side keeps its
   `running` filter — scanning every historic row would mean a hydra probe per
   row — while the map side is naturally bounded by live containers plus
   phantoms.

   The two remediations are decoupled. Map eviction is unconditional once the
   probe confirms the container is dead, because the entry alone is enough to
   block `StartDesktop`. The DB downgrade still only applies to rows that claim
   to be running, so `terminated_idle` is not overwritten with `stopped` and the
   reason the desktop went away is preserved.

2. **`StartDevContainer` verifies before short-circuiting.** It now calls the
   existing `HasRunningContainer`, which probes hydra and evicts a stale entry on
   the spot, so a phantom degrades into a real create instead of a false success.
   This makes resume correct immediately rather than only after the next
   reconcile sweep. The early return also reports the reused `DevContainerID`,
   so a genuine reuse is distinguishable from the no-op in the logs.

The authoritative per-session `GetDevContainer` probe, taken under the session's
creation lock, remains the guard against tearing down a container a concurrent
`StartDesktop` just created.

## Tests

`api/pkg/external-agent/reconcile_phantom_sessions_test.go`:

- phantom entry + terminal DB row → evicted, and `UpdateSession` is **not**
  called (verified to fail before the fix)
- phantom whose probe reports the container running → kept
- phantom pinned to another sandbox → untouched by this sandbox's sweep
- phantom + still-`running` DB row → both evicted and downgraded, probed once
- `staleReconcileCandidates` dedup, `starting`/terminal filtering, and the
  unset-`SandboxID`-means-`local` convention
