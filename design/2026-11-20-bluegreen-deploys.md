# Blue/green API deploys (London and Meta)

Task 004220. Follows the 2026-11-18 outage: a `pg_dump` held locks, the new
API's boot-time `ALTER` on `prompt_history_entries.interrupt` queued behind it,
and every query queued behind the `ALTER`. A deploy stopped the only API
process and started the new one, so any slow boot was downtime.

## What runs

```
              public ports (80, 443, 2224, 8080, 9110)
                              │
                         host nginx  ── /etc/nginx/helix-active/{stream,http}.conf
                         │        │       (written only by the deploy)
       127.0.0.1:1xxxx  ▼        ▼  127.0.0.1:2xxxx
                   api-blue    api-green          one is active; the other
                        │  ╲    ╱  │              exists only during a deploy
                        │   nats   │              (external, shared JetStream)
                        └─ postgres┘   leader lease: pg advisory lock
```

- `docker-compose.bluegreen.yaml` layers on the host's compose files (via
  `COMPOSE_FILE` in `.env`). The colours `extend` the host's `api`
  (`HELIX_COMPOSE_BASE`), run the release image
  `${HELIX_CONTROLPLANE_IMAGE}:${HELIX_VERSION_<COLOUR>}`, publish their ports
  on 127.0.0.1 at +10000 (blue) / +20000 (green), and are in the `bluegreen`
  profile, which `COMPOSE_PROFILES` never enables: only the deploy starts them.
  The single `api` moves to the `single` profile.
- NATS is a container (`nats`), not embedded, so both colours share pub/sub.
  Same token (`RUNNER_TOKEN`) and the same JetStream directory
  (`/filestore/nats`) the embedded server used, so streams carry over.
- `HELIX_SKIP_AUTOMIGRATE=1` on the colours: migrations run once, before the
  new colour starts, in a one-off `--migrate-only` container.
- Singleton work (crons, bots, orchestrator, reapers, Kodit workers, startup
  resets, token refresh) runs only in the process holding the leader lease
  (`api/pkg/leader`). The new colour gets it ~6s after the old one exits.
- SIGTERM drains (`api/pkg/server/graceful.go`): readyz turns 503, hijacked
  connections (websockets, RevDial, NATS proxy) close at once so clients
  reconnect through nginx to the new colour, requests get 45s.
- `/api/v1/readyz` answers `X-Helix-Instance: <colour>`, which is how the
  deploy proves which colour nginx reaches.

## A deploy (`scripts/lib/bluegreen.sh`)

1. Preflight (read-only): `.env`'s `HELIX_ACTIVE_COLOUR` must match the
   colour nginx's `stream.conf` says it points at.
2. Set `HELIX_VERSION_<idle>`, pull, run `--migrate-only` with the new image.
   Migrations are expand-only and lock-bounded (`api/pkg/store/migrations/README.md`,
   CI's `schema-compat` step), so the active colour keeps working.
3. Start the idle colour; wait until readyz passes 3× in a row over its own
   HTTP port and, with embedded TLS, its own TLS port with the public name.
4. Render nginx's files for the idle colour, `nginx -t`, reload. Watch the
   public edge for 30s: it must answer with the new colour.
5. `docker compose stop -t 60` the old colour (drain), then remove its
   container (a stopped `restart: always` container returns when dockerd
   restarts).

Any failure rolls back what ran: nginx back to the old colour, the new
colour removed, `.env` restored. The old colour is never touched before the
switch, so a failed deploy costs no requests.

London: `deploy-controlplane-remote.sh` does its usual preflight (compose
drift, image pins, PG major, pullability), ZFS snapshot and converge of the
other services, then `bg_deploy` instead of the in-place restart. Meta:
`deploy-meta.sh` deploys CI's `controlplane:<sha7>-linux-amd64` image of the
commit (the `deploy-meta` pipeline now waits for `build-controlplane-amd64`);
the checkout still drives the sandbox and Zed builds. Both keep the old
in-place path until the host is converted.

## Converting a host (once)

`scripts/bluegreen-cutover.sh`, run on the host after the release containing
this is deployed in place. It prints the resolved config diff between `api`
and `api-blue`, migrates, then stops `api` (the embedded NATS must stop before
the external one opens its JetStream directory), starts `nats` and
`api-blue`, installs the host's nginx.conf from helixml/infra and reloads.
That is the only downtime: 7-27s across rehearsal runs (mostly the new container's boot). On failure it
restores `.env`, nginx.conf and `api`. Runbook: helixml/infra
`docs/helix-bluegreen-cutover.md`.

## Verified

`scripts/rehearse-bluegreen.sh` runs all of it in docker-in-docker with real
compose, nginx, NATS, Postgres and this checkout's API binary, probing
`/api/v1/config` through nginx every 100ms:

| step | failed requests |
|---|---|
| cutover from single `api` | 65-237 (7-27s, expected) |
| deploy v2 → green, broken build → rollback, v1 → blue | 0, 0, 0 |
| London's `deploy-controlplane-remote.sh`: v2, broken → rollback | 0, 0 |
| nginx/.env disagree | deploy refused before any change |

Not covered there: the TLS readiness checks (the rehearsal has no
certificates) and real clients reconnecting. Meta's first blue/green deploys
exercise both before London's cutover.

## Known gaps

- Through a TCP proxy every client is 127.0.0.1: per-IP login throttling
  degrades to per-account (London already ran this way), and audit logs lose
  client IPs on Meta. PROXY protocol support in the API would fix both.
- No automatic reconnect: the session terminal pane, admin runner logs (one
  retry) and the evaluation SSE stream. Same as any restart today.
- Meta's `:30000-30100` expose range is not carried over: no code serves it
  any more (only stale swagger definitions).
