# Changelog

## [Unreleased]

### Fixed
- Completed Bitnami PostgreSQL removal. The 2.9.0 change removed the subchart from `values.yaml` and added the self-managed Deployment, but left the `dependencies:` block in `Chart.yaml.tmpl`. Every published chart from 2.9.0 through 2.11.0-rc1 still vendored and rendered the Bitnami `postgresql` StatefulSet, `secrets`, `networkpolicy`, `serviceaccount`, and two `Service`s alongside the new self-managed Postgres. The controlplane was always pointing at the new self-managed Postgres (`{release}-helix-controlplane-postgres`), so the Bitnami StatefulSet was dead weight - no data risk on upgrade, but wasted PVC and memory. `helm upgrade` to a release containing this fix removes the orphaned Bitnami resources.

## [2.9.0] - 2026-03-13

### Changed
- **BREAKING: Replaced Bitnami PostgreSQL with official postgres image**. See [UPGRADE.md](UPGRADE.md) for migration instructions.
  - Removed Bitnami `postgresql` and `common` chart dependencies
  - PostgreSQL now uses `postgres:17-alpine` via a self-managed Deployment
  - Service name changed from `{release}-postgresql` to `{release}-helix-controlplane-postgres`
  - PVC name changed from `data-{release}-postgresql-0` to `{release}-helix-controlplane-postgres-pvc`
  - Removed `postgresql.image.registry`, `postgresql.auth.postgresPassword`, `postgresql.architecture` values
  - Added `postgresql.persistence.*` values (previously managed by Bitnami subchart)
  - Added `postgresql.resources` to configure requests/limits on the bundled PostgreSQL container
- Updated `values-example.yaml` to document the new structured license key configuration
- Deprecated using `secretEnvVars` for common configurations like LICENSE_KEY in favor of structured configuration

### Added
- Migration script (`scripts/migrate-from-bitnami.sh`) for preserving data during upgrade
- PostgreSQL readiness probe using `pg_isready`
- `helix-controlplane.tplvalues.render`, `helix-controlplane.storage.class`, and `helix-controlplane.postgres-image` template helpers (replacing Bitnami common library)
- **Structured License Key Configuration**: Added explicit structured configuration parameters for the Helix license key in the controlplane helm chart
  - `controlplane.licenseKey`: Direct license key value
  - `controlplane.licenseKeyExistingSecret`: Reference to existing Kubernetes secret containing the license key
  - `controlplane.licenseKeyExistingSecretKey`: Key within the secret (defaults to "license")
  - This replaces the need to use `secretEnvVars` for license key configuration
  - Follows the same pattern as other credentials (runner token, Keycloak, provider API keys)

### Removed
- Keycloak installation from `kind_helm_install.sh`
