# Upgrade Guide

## Crawler browser image

The crawler browser moves from `ghcr.io/go-rod/rod:v0.116.2` to `ghcr.io/helixml/chrome`, built with each release and tagged with the chart `appVersion`.

- **Mirrored or private registries.** If your values override only `chrome.image.repository` (for example to a mirror of `ghcr.io/go-rod/rod`), the chart now renders `<your repository>:<appVersion>`, which your mirror probably does not have. Mirror `ghcr.io/helixml/chrome` at the release tag, or set `chrome.image.tag` as well.
- **Launch token.** The controlplane now authenticates to the browser manager with a shared secret (`RAG_CRAWLER_LAUNCHER_TOKEN` / `CHROME_LAUNCHER_TOKEN`). The chart generates it into the `<release>-chrome-launcher` Secret and reads it back on upgrade. If you render with `helm template` or a GitOps tool, `lookup` cannot see the cluster and a new value is generated on every render (both pods roll together, so nothing breaks, but they restart each sync): set `chrome.launcherToken` or `chrome.launcherTokenExistingSecret` instead.
- **NetworkPolicy.** The chart adds a NetworkPolicy, `chrome.networkPolicy.enabled` (default `true`), that only admits the controlplane pods to the chrome pod on port 7317. Add other sources with `chrome.networkPolicy.extraFrom`.

## JWT signing key

Applies to the first release that no longer has a built-in `REGULAR_AUTH_JWT_SECRET` default (any install method: Helm, `install.sh`, docker compose).

- **One-time sign-out.** Unless you already set `REGULAR_AUTH_JWT_SECRET`, sessions and password-reset links issued before the upgrade stop working and users sign in again. Rolling back to an earlier release signs everyone out again, because the earlier release uses its built-in key. Reset links issued by earlier releases also stop working; request a new one.
- **Where the key lives.** With `REGULAR_AUTH_JWT_SECRET` unset, the API generates a random key on first boot and stores it in `system_settings.jwt_signing_secret`, shared by all replicas. Database backups therefore contain the key: anyone holding a backup can mint sessions for that deployment. To keep it out of the database, set `controlplane.regularAuthJWTSecret` / `controlplane.regularAuthJWTSecretExistingSecret` (Helm) or `REGULAR_AUTH_JWT_SECRET` (compose `.env`). `install.sh` now writes one into `.env`.
- **`DATABASE_AUTO_MIGRATE=false`.** Add the column before upgrading (it is additive and ignored by earlier releases):

  ```sql
  ALTER TABLE system_settings ADD COLUMN IF NOT EXISTS jwt_signing_secret text;
  ```

- **Password-reset links** are now valid for one hour and for one use only, and stop working after any password change.

## Upgrading to 2.11.0-rc2

`helm upgrade` removes the orphaned Bitnami `postgresql` StatefulSet left behind by earlier releases. No data risk. To reclaim storage, manually delete the orphaned PVC after upgrade:

```bash
kubectl delete pvc data-<release>-postgresql-0 -n <namespace>
```

## Upgrading to 2.9.0

Chart version 2.9.0 replaces the Bitnami PostgreSQL subchart with the official `postgres:17-alpine` image. This is a **breaking change** for existing installations.

### What changed

- The Bitnami `postgresql` and `common` chart dependencies have been removed
- PostgreSQL is now deployed as a simple Deployment + Service using the official Docker image
- The service name changed from `{release}-postgresql` to `{release}-helix-controlplane-postgres`
- The PVC name changed from `data-{release}-postgresql-0` to `{release}-helix-controlplane-postgres-pvc`
- Several `values.yaml` fields were removed (see below)

### Removed values.yaml fields

The following fields no longer have any effect and can be removed from your values overrides:

| Removed field | Notes |
|---|---|
| `postgresql.image.registry` | Use `global.imageRegistry` for private registries |
| `postgresql.auth.postgresPassword` | No separate superuser password — `auth.password` is used |
| `postgresql.auth.postgresPasswordKey` | Removed with `postgresPassword` |
| `postgresql.architecture` | Always `standalone` (single replica) |

### New values.yaml fields

| New field | Default | Notes |
|---|---|---|
| `postgresql.persistence.enabled` | `true` | Previously managed internally by Bitnami |
| `postgresql.persistence.size` | `8Gi` | |
| `postgresql.persistence.storageClass` | `""` | |
| `postgresql.persistence.accessModes` | `[ReadWriteOnce]` | |
| `postgresql.persistence.existingClaim` | `""` | Use to attach a pre-existing PVC |

### Option A: Fresh install (recommended for most users)

If you can afford to lose your database and start fresh:

```bash
# 1. Delete the old Bitnami PostgreSQL PVC
kubectl delete pvc data-{release}-postgresql-0 -n {namespace}

# 2. Upgrade the chart
helm upgrade {release} helix/helix-controlplane \
  -n {namespace} \
  -f your-values.yaml

# 3. The controlplane will create a fresh database on startup
```

### Option B: Preserve existing data

If you need to keep your data, use the migration script included with the chart.

**Prerequisites:**
- `kubectl` configured with access to your cluster
- The old chart version still running (do **not** upgrade first)

**Steps:**

```bash
# 1. Download the migration script
curl -O https://raw.githubusercontent.com/helixml/helix/main/charts/helix-controlplane/scripts/migrate-from-bitnami.sh
chmod +x migrate-from-bitnami.sh

# 2. Run the script — it will:
#    - Dump your database from the old Bitnami pod
#    - Pause and prompt you to run helm upgrade
#    - Restore the dump into the new postgres pod
#    - Verify the migration
./migrate-from-bitnami.sh --release {release} --namespace {namespace}
```

The script is interactive — it will pause after the dump and tell you when to run `helm upgrade`. Follow the on-screen instructions.

**After successful migration:**

```bash
# Delete the old Bitnami PVC (the data is now in the new PVC)
kubectl delete pvc data-{release}-postgresql-0 -n {namespace}
```

The local dump file is preserved at `/tmp/helix-pg-migration-{release}.sql` as a backup.

### Updating your values.yaml

If your values file referenced Bitnami-specific fields, update it:

**Before (2.8.x):**
```yaml
postgresql:
  enabled: true
  image:
    registry: docker.io
    repository: bitnamilegacy/postgresql
    tag: "17"
  auth:
    postgresPassword: "admin-password"
    username: helix
    password: "my-password"
    database: helix
  architecture: standalone
```

**After (2.9.0):**
```yaml
postgresql:
  enabled: true
  image:
    repository: postgres
    tag: "17-alpine"
  auth:
    username: helix
    password: "my-password"
    database: helix
  persistence:
    enabled: true
    size: 8Gi
```

### Using a private registry

If you were using `postgresql.image.registry` to pull from a private registry, use `global.imageRegistry` instead:

**Before:**
```yaml
postgresql:
  image:
    registry: my-registry.example.com
    repository: bitnamilegacy/postgresql
```

**After:**
```yaml
global:
  imageRegistry: my-registry.example.com
postgresql:
  image:
    repository: postgres
    tag: "17-alpine"
```

### External PostgreSQL (no change)

If you use an external PostgreSQL instance (`postgresql.enabled: false`), no action is required. The `postgresql.external.*` configuration is unchanged.
