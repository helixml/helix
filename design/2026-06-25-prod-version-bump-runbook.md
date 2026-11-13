# Prod version-bump runbook (generic)

Reusable procedure for upgrading the **helix.ml production deployment** to a new release.
Version-agnostic — substitute `<NEW_VERSION>` (e.g. `2.11.25`) and `<OLD_VERSION>` throughout.
For the web-service hosting feature switch, see
`design/2026-06-25-london-web-service-rollout.md`, which references this runbook for the
upgrade step.

## Prod topology (verified 2026-06-25)
- **Controlplane**: GCP `helix-cloud-london` (europe-west2-a). Container `helix-api-1` =
  `ghcr.io/helixml/controlplane:<VERSION>`. This **is** app.helix.ml / app.tryhelix.ai — live
  customers. Stack at `/data/helix-app`. Shares the box with Harbor (registry.helix.ml),
  Keycloak, demos. DB = `helix-postgres-1`.
  Access: `gcloud compute ssh --zone europe-west2-a helix-cloud-london --project helixml`.
- **Runner / sandbox host**: `root@code.helix.ml` (SSH key-based, Hetzner). One sandbox host
  `helix-sandbox-app` = `ghcr.io/helixml/helix-sandbox:<VERSION>`,
  `SANDBOX_INSTANCE_ID=code-for-app`, points at https://app.helix.ml. Upgraded by editing
  `/opt/HelixML/upgrade-sandbox-app.helix.ml.sh` (a `docker run` wrapper, **not** compose).
- The two move **together**: the runner pulls desktop images by heartbeat version, so a version
  mismatch between controlplane and runner means stale desktop/sandbox images. Always bump both.

## Automated path: `scripts/deploy-prod.sh`
The release tag's `deploy-prod` pipeline runs this runbook. It can also be run by hand from a
checkout of the version being deployed, which may be a release tag or the 7-character SHA of
a green `main` build. Principles:
- **The compose file is part of the release.** The deploy diffs the host's
  `docker-compose.yaml` against the release's and prints each service's image as running →
  deploying; it reads the running containers, not a re-rendering of `.env`, which may have
  been edited since. Drift fails the deploy. `COMPOSE_SYNC=true` installs the release's file
  after a timestamped backup; the host's `docker-compose.override.yaml` and `.env` stay.
- **Nothing changes until everything checks out.** Every image the deploy will pull must be
  pullable on the host first, so a private ghcr package fails here and names the package.
- **The stack converges, the databases don't.** Every non-database service is brought to the
  new config, so `api` and `chrome` both follow `HELIX_VERSION`. Postgres moves only with
  `RECREATE_POSTGRES=true`, after the ZFS snapshot. A database's target image must match the
  major of its data (`PG_VERSION` in the running container), with or without the flag; a
  major change is a dump/restore (`docs/postgres-upgrade.md`). With the flag, a defined but
  stopped database fails the deploy, since its data major can't be read.
- Secrets the new compose needs and only this host uses, such as `CHROME_LAUNCHER_TOKEN`, are
  generated on the host into `.env`. They never pass through CI.
- Any failure after the snapshot (pull, recreate or health-check) restores the exact previous
  `.env` and compose file and brings the previous services back.
- Database services (by name or PostgreSQL-family image) are never created or recreated
  without `RECREATE_POSTGRES=true`.

To deploy a tag with an opt-in from CI, restart its build with the param; the
`prod control plane` step passes only `COMPOSE_SYNC` and `RECREATE_POSTGRES` through:
```bash
drone build restart --param COMPOSE_SYNC=true helixml/helix <tag build number>
```

The manual steps below remain the reference and the fallback when CI is unavailable.

## Step 1 — build & publish images (no prod impact)
1. Land all PRs for the release on `main`.
2. Cut release tag **`<NEW_VERSION>`** → Drone builds and pushes `controlplane:<NEW_VERSION>`,
   `helix-sandbox:<NEW_VERSION>`, and desktop images to ghcr.
3. Confirm the build is green and images exist on ghcr before touching prod.

## Step 2 — back up the controlplane DB (do this every time)
```bash
gcloud compute ssh --zone europe-west2-a helix-cloud-london --project helixml --command \
  'docker exec helix-postgres-1 pg_dumpall -U postgres | gzip > /data/helix-app/backup-<OLD_VERSION>-$(date +%F).sql.gz && ls -la /data/helix-app/backup-*.sql.gz'
```

## Step 3 — upgrade the controlplane (london) — HIGHEST RISK (customer-facing)
1. SSH to london. The version pin is `HELIX_VERSION` in `/data/helix-app/helix/.env`. Diff
   `/data/helix-app/helix/docker-compose.yaml` against the release's first: compose-level fixes
   (image pins, new env, log rotation) only reach prod if that file is current.
2. Bump `HELIX_VERSION` → `<NEW_VERSION>`, then pull and recreate every service whose image
   follows it (never postgres):
   ```bash
   cd /data/helix-app/helix
   docker compose pull api chrome
   docker compose up -d --no-deps api chrome
   ```
3. **Watch GORM AutoMigrate** on start: `docker logs -f helix-api-1` (look for migrate
   completion, no panics).
4. Verify app.helix.ml is healthy: login + one chat round-trip. Check
   `docker ps` shows `controlplane:<NEW_VERSION>` Up.
5. **Rollback**: set the tag back to `<OLD_VERSION>`, `docker compose up -d api`. If a migration
   ran and the new schema is incompatible, restore the Step-2 dump first.

## Step 4 — upgrade the runner (code.helix.ml)
1. SSH `root@code.helix.ml`. Edit the version in the upgrade script and re-run it:
   ```bash
   sed -i 's/^SANDBOX_TAG=.*/SANDBOX_TAG="<NEW_VERSION>"/' /opt/HelixML/upgrade-sandbox-app.helix.ml.sh
   /opt/HelixML/upgrade-sandbox-app.helix.ml.sh
   ```
   The script stop/rm/runs `helix-sandbox-app` on the new tag (auto-pulls), then prunes old
   sandbox images. Named volumes `sandbox-storage-app:/var/lib/docker`, `sandbox-data-app:/data`,
   `hydra-storage-app:/hydra-data` persist across the swap. **~seconds offline** during the swap
   (and a brief blip for any live sandbox/web-service while its container restarts; `/data`
   survives).
2. Verify re-registration on london:
   ```bash
   docker exec helix-postgres-1 psql -U postgres -d postgres -x -c \
     "SELECT id,status,desktop_versions,last_seen FROM sandbox_instances WHERE id='code-for-app';"
   ```
   Expect `status=online`, `desktop_versions` showing `<NEW_VERSION>`, recent `last_seen`.
3. **Rollback**: set `SANDBOX_TAG` back to `<OLD_VERSION>`, re-run the script.

## Order & notes
- Order: Step 1 → 2 → 3 (controlplane) → 4 (runner). Controlplane is the schema owner; bring it
  up first, verify, then the runner.
- Do customer-facing steps (3, 4) in a low-traffic window.
- Both controlplane and runner must end on the **same** `<NEW_VERSION>`.
