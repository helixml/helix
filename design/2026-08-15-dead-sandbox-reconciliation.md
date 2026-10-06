# Dead sandboxes reported as running

## Symptom

Task `spt_01m020k5y9z6xe800r9vz70yhr` (headless, keel project) showed, all at once:

- a green "Sandbox running" dot in the chat header,
- a ticking `Working for 1m 8s` spinner,
- an enabled `PR: keel` action,
- `Desktop not running` in the right-hand panel and `failed to create exec` in the terminal drawer,
- an errored interaction: `Agent never connected after auto-wake cold-start retries (no WebSocket — see helixml/helix#2397)`.

The container had been `Exited (1)` for over 20 minutes.

## What actually happened

Reconstructed from the API log, hydra, and the DB (all times UTC, 2026-08-15):

| Time | Event |
|---|---|
| 08:12:54 | Implementation approved. `ensurePullRequest` takes the **keel repo mutex** (`spec_task_workflow_handlers.go`, `WithRepoLock(... PushBranchToRemote ...)`) and starts a native `git push` to `github.com/keel-hq/keel`. The same click queues the approval prompt and cold-starts the headless container. |
| 08:12:56 | Container boots; `helix-workspace-setup.sh` runs `git pull origin helix-specs`. `handleUploadPack` blocks on `lock.Lock()` — the push holds it. (`Handling upload-pack request` is logged; `Syncing from upstream before serving pull` is not.) |
| 08:15:27–08:20:27 | GitHub is unreachable in bursts (`fatal: unable to access …`, `context deadline exceeded`). The push burns 5 minutes and fails. Lock still held. |
| 08:17:58 | Inside the container `wait_for_setup_complete` hits its hard 300 s cap (`desktop/shared/start-zed-core.sh`): `FATAL: Workspace setup did not complete after 300s` → PID 1 exits → **container exits 1**. Zed never launched, so no external-agent WebSocket. |
| 08:18:20 | The auto-wake worker's 5-minute cold-start grace expired at 08:17:54, four seconds before the container gave up. It burns both retries in two ticks and marks the interaction `state=error`. |
| 08:20:27 | Lock released; the API serves the upload-pack to a client that died 2.5 minutes earlier. |
| 08:26:40 | API restarts. On RevDial reconnect, `DiscoverContainersFromSandbox` **resurrects the dead session**: hydra still lists the exited container, so the session is re-added to the in-memory map and force-written back to `external_agent_status = "running"`. |
| 08:35:19 | A CI-passed webhook injects a new prompt into the dead session → `state=waiting` → the `Working for …` spinner. |

Corroboration: `FETCH_HEAD` in `keel/.git/worktrees/helix-specs` is 0 bytes, stamped 08:12 — the fetch started and never returned. The container's setup log ends mid-line at `Pulling latest changes from remote...`.

## Why the status stayed green

Three independent "exists ⇒ alive" bugs, none of which any single reconnect could correct:

1. **`HydraExecutor.GetSession` is an in-memory map lookup.** Both `getSession` (`session_handlers.go`) and `populateSessionState` (`spec_driven_task_handlers.go`) call it as a "live check against the executor". It never asks hydra or Docker anything. `StartDesktop` inserts the entry; only `StopDesktop`, the idle checker, and sandbox-disconnect remove it. A container whose entrypoint exits on its own removes nothing.

2. **`hydra.ListDevContainers` returned a cached status.** `dc.Status` was only written when hydra itself stopped a container, or lazily by `GetDevContainer`. There is no container-exit monitor, so a self-exited container kept `Status: "running"` indefinitely. That list is the control plane's live-set.

3. **`markMissingSessionsStopped`'s "authoritative probe" only checked `err == nil`.** `GetDevContainer` returns a container hydra still tracks even when Docker reports it exited, so the probe read a dead container as alive and skipped the downgrade — every time.

And the reconciliation that would have caught it **only ran on sandbox lifecycle events** (`/sandboxes/register`, RevDial connect). A dev container dying while its sandbox stays happily connected fires neither.

## The fix

- `hydra.ListDevContainers(ctx)` refreshes each container's status from Docker. Status and IP handling is now shared with `GetDevContainer` via `inspectContainerState`, which reports **stopped** whenever Docker cannot confirm the container is running, and writes that back to the cache.
- `DiscoverContainersFromSandbox` filters hydra's list to running containers (`filterRunningContainers`) before using it as the live-set, so a reconnect can no longer resurrect an exited container.
- `markMissingSessionsStopped`'s probe requires `Status == running`. The parameter is narrowed to a `devContainerProber` interface so the "tracked but exited" case is unit-testable.
- New `startSandboxContainerReconciler` sweeps every online sandbox on a timer (`HELIX_SANDBOX_CONTAINER_RECONCILE_INTERVAL`, default 30s), making reconciliation time-driven rather than connection-driven. It reuses `DiscoverContainersFromSandbox`, which is already idempotent and correctly locked.
- Frontend: `deriveSandboxState` / `isSandboxOffline` extracted from `useSandboxState` as pure helpers. `InteractionLiveStream` takes `agentOffline` and renders `AgentOfflineNotice` instead of the ticking timer. `SpecTaskActionButtons` disables Open PR unless `sandbox_state === "running"`.

## Deliberately not fixed here

The **repo-lock-across-a-network-push** in `ensurePullRequest` is the trigger, and it will re-trigger. It is not fixed in this change because the agent pushes its branch from inside the sandbox: the sandbox owns the working copy and may not be on a filesystem the control plane can reach, so the push cannot simply be moved out of the sandbox's path. The UI now refuses to offer the action when the sandbox is gone rather than pretending it will work.

Two smaller ones, also untouched:

- `helix-workspace-setup.sh` has **no timeout on any git operation**, so a hung fetch consumes the whole 300 s setup budget. The `helix-specs` pull that hung is a nice-to-have (refresh the startup script); it should not be able to kill the session.
- The container's 300 s setup timeout and the auto-wake 5-minute cold-start grace are the same duration, so auto-wake exhausts its retries the instant the container gives up — zero margin.
- Headless's FATAL message points at `/tmp/helix-workspace-setup.log`, but headless `launch_terminal` writes to `/tmp/helix-Helix-Setup.log`. The advertised path does not exist.

## Deploying

The hydra changes run **inside** the sandbox container and are not picked up by Air:

```bash
./stack build-sandbox
# or, for a dev hot-swap:
cd api && CGO_ENABLED=0 GOOS=linux go build -o /tmp/hydra-linux ./cmd/hydra
docker cp /tmp/hydra-linux helix-sandbox-nvidia-1:/usr/local/bin/hydra
docker compose -f docker-compose.dev.yaml exec -T sandbox-nvidia pkill -TERM hydra
```

The API-side changes hot-reload. They are independently sufficient for the downgrade path: the per-session probe reaches Docker through `GetDevContainer`, which refreshed status even before this change.
