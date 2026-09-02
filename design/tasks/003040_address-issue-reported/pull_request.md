# Prevent obsolete desktop images from exhausting Hydra storage

## Summary
Hydra sandbox startup now garbage-collects obsolete Helix desktop image tags from local, GHCR, and custom registry repositories, including registries with explicit ports. Cleanup runs conservatively before the current image pull so critically full Docker PVCs can recover during upgrade, and again after a successful pull. Images referenced by containers remain protected, while `HELIX_DESKTOP_IMAGE_RETENTION` controls how many previous versions are retained for rollback and defaults to one.

Disk-pressure monitoring now emits an operator-visible warning through the Hydra Runner Logs stream before admission is blocked. The warning threshold is configurable with `HELIX_DISK_PRESSURE_WARN_FREE_PCT`. Storage-capacity admission failures return HTTP 507 instead of a generic 500 and include the affected Hydra hostname, constraining storage path, and recommended remediation.

## Testing
- Ran targeted Hydra tests covering registry-aware image selection, duplicate tags, configurable retention, HTTP 507 mapping, capacity-error details, and disk-warning transitions; all passed.
- Validated both sandbox shell scripts with `bash -n`.
- Ran `git diff --check`; no whitespace errors were found.
- The broader Hydra package test run was also inspected; unrelated existing DNS-address and ZFS-fallback assertions remain outside this change.
