# Changelog

## [Unreleased]

### Changed
- The crawler browser now uses `ghcr.io/helixml/chrome`, tagged with the chart `appVersion`: go-rod's launcher manager with a current Chromium from Debian, running as a non-root user. It replaces `ghcr.io/go-rod/rod:v0.116.2`, whose Chromium dated from 2024. Set `chrome.image.repository`/`chrome.image.tag` to keep a different image.

### Added
- The controlplane sends a shared launch token to the crawler browser, which refuses launches without it. The chart generates it (Secret `<release>-chrome-launcher`) unless `chrome.launcherToken` or `chrome.launcherTokenExistingSecret` is set. See [UPGRADE.md](UPGRADE.md#crawler-browser-image).
- `chrome.networkPolicy` (enabled by default): only the controlplane pods may reach the chrome pod on port 7317. See [UPGRADE.md](UPGRADE.md#crawler-browser-image).
- Optional `controlplane.regularAuthJWTSecret` (or `controlplane.regularAuthJWTSecretExistingSecret` + `controlplane.regularAuthJWTSecretExistingSecretKey`, default key `jwt-secret`) sets `REGULAR_AUTH_JWT_SECRET`, the signing key for login sessions and password-reset links. When neither is set the chart sets nothing and the API generates a random key on first boot and stores it in its database. See [UPGRADE.md](UPGRADE.md#jwt-signing-key).
- Install notes warn when `controlplane.encryptionKey` / `controlplane.encryptionKeyExistingSecret` are unset.

## [2.11.0-rc2]

### Fixed
- Removed the Bitnami PostgreSQL subchart dependency that was inadvertently still being rendered. `helm upgrade` deletes the orphaned Bitnami `postgresql` StatefulSet. See [UPGRADE.md](UPGRADE.md) for the optional PVC cleanup.

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
