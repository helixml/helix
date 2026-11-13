# Drone: main builds cancelled by newer pushes, and runner hygiene

## 1. Main SHAs without images

**Cause: Drone repo settings, not `.drone.yml`.** `GET /api/repos/helixml/helix` returns
`auto_cancel_pushes: true` and `auto_cancel_running: true` (server 2.12.1). `.drone.yml`
has no cancel step or plugin.

In Drone 2.12.1 (`trigger/trigger.go`, `service/canceler/match.go`), every new push build
calls `CancelPending`. That marks as `skipped` every older build of the same repo, event and
ref that is `pending`, or `running` when `auto_cancel_running` is set. The scope is the whole
build, so `.drone.yml` can't exempt individual pipelines (image builds) from it.

Evidence: of the last 100 `main` builds, 12 are `skipped`, and each one finished exactly when
the next main build was created (for example 5715 for `7e970cc`: `build-sandbox-arm64` was
`killed` and `manifest-sandbox` was `skipped`). **All 12 had already started**, so turning off
`auto_cancel_running` alone would have saved every one of them.

**Admin action (applied 2026-11-13 17:55 UTC; `auto_cancel_running` is now false):**

```bash
drone repo update helixml/helix --auto-cancel-running=false
# or
curl -X PATCH -H "Authorization: Bearer $DRONE_TOKEN" -H 'Content-Type: application/json' \
  -d '{"auto_cancel_running": false}' "$DRONE_SERVER/api/repos/helixml/helix"
```

Keep `auto_cancel_pushes` on. It still drops a queued build that hasn't started any stage
yet, so branch pushes stay cheap. Turning it off as well would also build every
superseded feature-branch push.

To backfill an image for a skipped SHA: `drone build restart helixml/helix <number>`.

## 2. Runner hygiene

Runners: node03 runs a plain `docker run --restart always` `drone-runner` container, and
node04 runs a Synpse-managed one (both `drone-runner-docker`, capacity 16).

**Stale builds.** When a runner dies, Drone keeps its builds `running`.
`scripts/drone-reap-stale-builds.sh` lists builds from `/api/builds/incomplete` (admin token)
that are older than `--max-age-minutes` (default 240; the helix repo timeout is 90).
`--apply` cancels them. Run it by hand after a runner reboot, or from cron on a host that
has the admin token.

**Leaked networks.** Each running stage creates a Docker network. Docker's default address
pools hold about 31 networks (172.17–31.0.0/16, plus 192.168.0.0/16 split into /20), so
32 leftover networks fill them: `all predefined address pools have been fully subnetted`.

- Prune stopped-build networks before the runner starts. On node03:
  `docker network prune -f --filter label=io.drone=true` (every stage network carries that label;
  prune only removes unused ones), in
  the unit or script that starts `drone-runner`. On node04, do the same in the Synpse
  application spec (a pre-start step).
- Enlarge the pool in `/etc/docker/daemon.json` so the runner's concurrency can never use it up:
  `"default-address-pools": [{"base": "10.200.0.0/16", "size": 24}]` (256 networks; restart
  dockerd in a maintenance window, and check that 10.200/16 is free on that LAN).
- Keep `DRONE_RUNNER_CAPACITY` well below the number of free networks. Without the pool
  change, 16 stages plus the default bridge and any compose networks on the host is
  already near 31.
