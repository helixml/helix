# Upgrade Guide

## Upgrading to 0.4.0

Chart version 0.4.0 replaces the Bitnami PostgreSQL subchart with the official `postgres:17-alpine` image. This is a **breaking change** for existing installations.

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

**Before (0.3.x):**
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

**After (0.4.0):**
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
