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
1. SSH to london. Confirm the version-pin location (compose vs. install script) — typically the
   `image: ghcr.io/helixml/controlplane:<OLD_VERSION>` line in `/data/helix-app/docker-compose.yaml`.
2. Bump the tag → `<NEW_VERSION>`, then:
   ```bash
   cd /data/helix-app
   docker compose pull api
   docker compose up -d api
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
