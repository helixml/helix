# Failed desktop start left spec task stuck on "booting virtual desktop environment..."

Incident on meta (spt_01m5cbt8e387tk5fhmcpv78zdp): a deploy recreated the sandbox, the
disconnect handler set the session `stopped`, then a StartDesktop that failed with
`failed to dial Hydra via RevDial: no connection` reset `external_agent_status` to `""`.
`getSpecTaskLaunchPhase` reads `implementation` + no lifecycle state as "first launch not
started yet", so the page showed the boot screen forever with no Start button.

## Fix
- `HydraExecutor.StartDesktop` error path writes `stopped` + `status_message="Desktop failed
  to start: <err>"`; the next attempt clears the message when it writes `starting`.
- Every other writer of `""` now writes `stopped`: `StopDesktop`, `ClearSessionStartingStatus`
  (auto-wake exhaustion, restart error paths) and `ClearStaleStartingSessions` (API boot).
  No reader distinguishes `""` from `stopped` except the frontend's "never launched" check.
- StartDesktop waits for its golden-cache progress poller (now on a cancellable context)
  before recording the outcome, so a late full-row write can't put `starting` back.
- Frontend: the paused desktop shows `status_message`; a session with `dev_container_id`
  (written on every successful start, never cleared) always has lifecycle state, so it can't
  read as a first launch — this also unsticks rows already wiped in production.

## Live repro (inner Helix)
Stop `sandbox-nvidia`, immediately `helix spectask resume <ses>` (fails with "no connection"
while the runner heartbeat is still fresh), start the sandbox again. Note: triggering via a
queued message reproduces it only if the start runs inside the heartbeat window; auto-wake's
later retries fail earlier with "no eligible sandbox runner", which never touched the status.

| | DB after failed resume | Page |
|---|---|---|
| before | `external_agent_status=""`, no container | `stuck-booting-screenshots/2026-10-20-stuck-booting-before-fix.png` |
| after | `stopped`, `status_message="Desktop failed to start: …no connection"` | `…-after-fix-reload.png` |

Start → desktop running, message cleared → follow-up message answered
(`…-after-fix-restarted.png`). Old wiped row with fixed frontend: `…-legacy-row-after-fix.png`.
