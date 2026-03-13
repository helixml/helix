# Changelog

## [2.9.0] - 2026-03-13

### Changed
- **BREAKING: Replaced Bitnami PostgreSQL with official postgres image**. See [UPGRADE.md](UPGRADE.md) for migration instructions.
  - Removed Bitnami `postgresql` and `common` chart dependencies
  - PostgreSQL now uses `postgres:17-alpine` via a self-managed Deployment
  - Service name changed from `{release}-postgresql` to `{release}-helix-controlplane-postgres`
  - PVC name changed from `data-{release}-postgresql-0` to `{release}-helix-controlplane-postgres-pvc`
  - Removed `postgresql.image.registry`, `postgresql.auth.postgresPassword`, `postgresql.architecture` values
  - Added `postgresql.persistence.*` values (previously managed by Bitnami subchart)
- Changed pgvector deployment strategy from `RollingUpdate` to `Recreate` to prevent deadlocks with ReadWriteOnce PVCs

### Added
- Migration script (`scripts/migrate-from-bitnami.sh`) for preserving data during upgrade
- PostgreSQL readiness probe using `pg_isready`
- `helix-controlplane.tplvalues.render` and `helix-controlplane.storage.class` template helpers (replacing Bitnami common library)

### Removed
- Keycloak installation from `kind_helm_install.sh`

## [Unreleased]

### Added
- **Structured License Key Configuration**: Added explicit structured configuration parameters for the Helix license key in the controlplane helm chart
  - `controlplane.licenseKey`: Direct license key value
  - `controlplane.licenseKeyExistingSecret`: Reference to existing Kubernetes secret containing the license key
  - `controlplane.licenseKeyExistingSecretKey`: Key within the secret (defaults to "license")
  - This replaces the need to use `secretEnvVars` for license key configuration
  - Follows the same pattern as other credentials (runner token, Keycloak, provider API keys)

### Changed
- Updated `values-example.yaml` to document the new structured license key configuration
- Deprecated using `secretEnvVars` for common configurations like LICENSE_KEY in favor of structured configuration 