# Implementation Tasks

## Phase 1: Pipeline Restructuring (Biggest Impact)

- [ ] Split `build-desktops` step in `build-sandbox-amd64` pipeline into two parallel steps: `build-desktop-sway` and `build-desktop-ubuntu`, each depending on `build-zed` + `build-qwen-code`
- [ ] Apply the same split to the `build-sandbox-arm64` pipeline
- [ ] Update `build-sandbox` step to depend on both new desktop steps instead of the single `build-desktops`
- [ ] Investigate whether `build-macos-dmg` actually needs `build-sandbox-arm64` — check if `provision-vm-light.sh` pulls the sandbox image or just the controlplane image
- [ ] If DMG doesn't need sandbox: remove `build-sandbox-arm64` from `build-macos-dmg`'s `depends_on`
- [ ] Verify the E2E test (`zed-e2e-test`) finishes before `build-sandbox` on the critical path — if not, consider running it as a non-blocking quality check that doesn't gate `push-sandbox`

## Phase 2: Docker Build Optimization

- [ ] Add `--mount=type=cache` to the `embedding-model` stage in `Dockerfile` to persist downloaded ONNX models across builds
- [ ] Fix `Dockerfile.qwen-build` layer ordering: copy `package.json`/`package-lock.json` first, run `npm ci`, then copy remaining source
- [ ] Consolidate scattered `apt-get update` calls in `Dockerfile.sway-helix` using BuildKit apt cache mounts (`--mount=type=cache,target=/var/cache/apt`)
- [ ] Apply the same apt cache mount pattern to `Dockerfile.ubuntu-helix`

## Phase 3: Registry Push Optimization

- [ ] Create a new `mirror-to-ghcr` Drone pipeline that runs after all build/manifest pipelines complete, moving all `ghcr-push.sh` and `ghcr-manifest.sh` calls out of the build pipelines
- [ ] Remove inline `scripts/ghcr-push.sh` calls from `build-controlplane-amd64`, `build-controlplane-arm64`, `build-sandbox-amd64`, `build-sandbox-arm64`, `build-desktops` (both arches), and `build-runner*` steps
- [ ] Keep GHCR pushes for manifests in the new pipeline (controlplane, sandbox, sway, ubuntu manifests)

## Phase 4: Conditional Builds (Optional, Medium Effort)

- [ ] Extend Zed commit-based cache-check pattern to qwen-code: before building, check if `qwen-code-build/` output matches the pinned commit hash and skip if unchanged
- [ ] Add registry-based caching for desktop images: before building sway/ubuntu, check if the image with the expected tag already exists in the registry and skip the build
- [ ] Document the caching strategy in a comment block at the top of `.drone.yml`

## Validation

- [ ] Run a test release build (RC tag) and measure end-to-end time — target under 45 minutes
- [ ] Verify all images are correctly pushed to both `registry.helixml.tech` and `ghcr.io`
- [ ] Verify macOS DMG is built, notarized, and uploaded to R2
- [ ] Confirm multi-arch manifests work correctly for controlplane, sandbox, sway, ubuntu
- [ ] Run the release-rollback pipeline in dry-run to confirm it still works with the restructured pipelines
