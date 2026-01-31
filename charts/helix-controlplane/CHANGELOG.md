# Changelog

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