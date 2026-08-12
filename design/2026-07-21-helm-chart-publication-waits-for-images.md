# Helm Chart Publication After Release Manifests

## Root Cause

The tag-triggered Cloud Build chart publisher raced the independent Drone image pipelines. It could expose a chart version before the matching multi-architecture images existed.

Cloud Build is now neutralized: `cloudbuild.yaml` only reports that Drone owns Helm publication and writes nothing to the chart bucket.

## Drone Release Order

The tag-only `publish-helm` pipeline depends on:

```text
default
manifest-controlplane
manifest-sandbox
```

`manifest-controlplane` completes the controlplane release manifest. `manifest-sandbox` completes the `helix-sandbox`, `helix-sway`, and `helix-ubuntu` release manifests. Drone starts chart publication only after all three dependency pipelines succeed.

`deploy-prod` now depends only on `publish-helm`, so production deployment cannot begin until the chart version is published successfully.

## Fail-Closed Publication

`scripts/publish-helm-charts.sh`:

1. Validates the release tag and GCS credential.
2. Fetches the existing `index.yaml`; a missing or failed fetch stops publication.
3. Renders and packages exactly `helix-controlplane-$VERSION.tgz` and `helix-sandbox-$VERSION.tgz`.
4. Uploads both exact archives.
5. Merges the existing index and uploads `index.yaml` last with `Cache-Control:no-cache,no-store,max-age=0`.

Uploading the index last makes it the atomic discovery point. A package upload failure leaves the existing index unchanged, so clients cannot discover the partial release.

If chart publication fails, the pipeline runs the existing `scripts/release-rollback.sh` cleanup. Partially published image tags are intentionally retained: the failed release is removed from its tag, GitHub release, and Helm index, so it is undiscoverable through supported release channels, while deleting registry artifacts would add risky cleanup with no customer benefit.

## Verification

```text
cd /Users/psamuel/helix/helix-worktrees/chart-after-images
sh scripts/test-publish-helm-charts.sh
publish helm charts test passed

sh -n scripts/publish-helm-charts.sh scripts/test-publish-helm-charts.sh
drone lint --trusted .drone.yml
ruby -e 'require "yaml"; YAML.load_stream(File.read(".drone.yml")); YAML.load_file("cloudbuild.yaml")'
git diff --check
```

The publisher test verifies the existing-index fetch, exact archive names, archive-before-index upload order, no-cache index upload, and fail-closed behavior when the existing index cannot be fetched. Shell syntax, trusted Drone lint, YAML parsing, and diff checks passed.

NOT live tested: a real release tag still needs to verify Drone dependency scheduling, manifest completion, chart visibility, rollback on publication failure, and deployment only after `publish-helm` succeeds.
