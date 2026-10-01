# Sandbox restart: graceful shutdown and no sandbox-less window

## Incident

Meta deploy (Drone build 4774, d83767c25): `./stack build-sandbox` recreated
`helix-sandbox-nvidia-1`. `docker compose up` failed twice with `cannot stop container: … tried to kill
container, but did not receive an exit event`. The old container needed ~10 min to exit (Exited 137) and left
`faa42461411a_helix-sandbox-nvidia-1` in `Created`. Meta had no sandbox for ~30 min.

## Root cause (ours)

The sandbox's PID 1 was `tail -f /dev/null` (`exec gosu … startup-app.sh`). The kernel drops signals PID 1
has no handler for, so `docker stop`'s SIGTERM did nothing. After compose's default 10s grace, Docker
SIGKILLed the whole tree at once: the sandbox dockerd, every desktop and its nested dockerd/BuildKit, and the
XFS-on-zvol and loop mounts under them. The kernel then spent minutes in uninterruptible teardown flushing those
filesystems, longer than dockerd waits for the exit event after SIGKILL. Compose gave up, and the retry raced
the same still-dying container.

Reproduced in the inner stack on the old image: `docker events` shows `kill 15`, then `kill 9` 10s later, then
exit 137.

## Fix

- **Sandbox** (`sandbox/overlay/entrypoint.sh`, `sandbox/shutdown.sh`): the entrypoint stays PID 1 and traps
  SIGTERM to run `helix-sandbox-shutdown`, which shuts down top-down with a bound on each step:
  1. Stop Hydra.
  2. Stop each desktop's inner dockerd (the `/tmp/.dockerd-stop` protocol golden builds already use).
  3. `docker stop` the desktops.
  4. Stop the sandbox dockerd.
  5. `sync`, then unmount the zvol/loop mounts.

  The dockerd and Hydra supervisor loops stop restarting their daemon once `/run/helix-sandbox-stopping`
  exists. The entrypoint clears that file at boot. Compose `stop_grace_period: 5m`.
- **`./stack restart-sandbox`** (used by `build-sandbox`):
  1. Tag the running image `helix-sandbox:previous`.
  2. `stop_sandbox`: `docker stop -t 300`, then poll `docker inspect` until every container for the service,
     including hash-prefixed ones, has exited. Give up after 15 min and never recreate over a running
     container. Then remove the containers.
  3. `start_sandbox`: `up -d --no-deps`, wait for `healthy`, up to 3 bring-up attempts.
  4. On failure, `ensure-sandbox helix-sandbox:previous`, then fail.
- **`./stack ensure-sandbox`**: if no sandbox is serving (running, not unhealthy, not mid-shutdown), wait for
  any old one to exit, then bring up `helix-sandbox:latest`, else `helix-sandbox:previous`.
- **`scripts/deploy-meta.sh`**: if the deploy fails after the sandbox restart began, the EXIT trap runs
  `./stack ensure-sandbox` before reporting the failure.
- All compose calls on this path go through `sandbox_compose`, which uses the same TLS overlay detection as
  the rest of `./stack`.

## Known remaining

The desktop image's PID 1 (`startup.sh`) also ignores SIGTERM. The sandbox shutdown works around this by
stopping the desktop's inner dockerd explicitly before `docker stop`. Fixing it in the desktop entrypoint is a
separate change that needs a `build-ubuntu`.
