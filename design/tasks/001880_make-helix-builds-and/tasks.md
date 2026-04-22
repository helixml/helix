# Implementation Tasks

## Phase 1: Pre-built Base Images (Biggest Impact — gets sandbox to ~10 min)

- [ ] Split `Dockerfile.sway-helix` at the "FREQUENTLY CHANGING CODE" comment (~line 912): everything above becomes `Dockerfile.sway-base`, everything below stays in a new slim `Dockerfile.sway-helix` that starts with `FROM helix-sway-base:latest`
- [ ] Same split for `Dockerfile.ubuntu-helix` at ~line 1100: create `Dockerfile.ubuntu-base` and slim `Dockerfile.ubuntu-helix`
- [ ] The Go build stage (desktop-bridge, settings-sync-daemon, docker-wrapper) needs to stay in the app-layer Dockerfile since it compiles current Go source — keep it as a build stage in the slim Dockerfile
- [ ] Build and push base images to `registry.helixml.tech/helix/helix-sway-base:latest` and `helix-ubuntu-base:latest`
- [ ] Create a new Drone pipeline `build-desktop-bases` triggered by: (a) changes to Dockerfile base sections, (b) weekly cron for security updates
- [ ] Update `build-sandbox-amd64` and `build-sandbox-arm64` pipelines to use the slim app-layer Dockerfiles

## Phase 2: Parallelize Desktop Builds (Saves ~15-25 min)

- [ ] Split the `build-desktops` step in `build-sandbox-amd64` into two parallel steps: `build-desktop-sway` and `build-desktop-ubuntu`, each depending on `build-zed` + `build-qwen-code`
- [ ] Apply the same split to `build-sandbox-arm64`
- [ ] Update `build-sandbox` step to depend on both new desktop steps

## Phase 3: Pre-built CI Test Image (Saves ~4-5 min on default pipeline)

- [ ] Create `Dockerfile.ci` based on `golang:1.25-bookworm` with `build-essential`, `git`, and ORT library pre-installed
- [ ] Build and push to `registry.helixml.tech/helix/helix-ci:bookworm`
- [ ] Update `build-api-binary` to use `helix-ci:bookworm` — remove apt-get and ORT download commands
- [ ] Update `unit-test` to use `helix-ci:bookworm` — remove apt-get and ORT download commands
- [ ] Update `api-integration-test` to use `helix-ci:bookworm` — remove apt-get and ORT download commands
- [ ] Add a cron pipeline to rebuild `helix-ci:bookworm` weekly (or when Go version changes)

## Phase 4: Decouple macOS DMG (Saves ~25-40 min on release critical path)

- [ ] Read `for-mac/scripts/provision-vm-light.sh` to verify whether it pulls the sandbox image or only controlplane
- [ ] If sandbox not needed: remove `build-sandbox-arm64` from `build-macos-dmg`'s `depends_on`
- [ ] If sandbox is needed: investigate whether `provision-vm-light.sh` can use a pre-built sandbox image instead of waiting for the current build

## Phase 5: Async GHCR Mirroring (Saves ~5-10 min)

- [ ] Create a new `mirror-to-ghcr` Drone pipeline that depends on all build/manifest pipelines
- [ ] Move all `scripts/ghcr-push.sh` calls from build steps into this new pipeline
- [ ] Move all `scripts/ghcr-manifest.sh` calls into this new pipeline
- [ ] Verify GHCR images appear correctly after a release

## Phase 6: Minor Docker Optimizations

- [ ] Add `--mount=type=cache` for embedding model downloads in the main `Dockerfile`'s `embedding-model` stage
- [ ] Fix `Dockerfile.qwen-build` layer ordering: copy `package.json`/`package-lock.json` first, run `npm ci`, then copy source
- [ ] Add BuildKit apt cache mounts to the base desktop Dockerfiles to speed up base image rebuilds

## Validation

- [ ] Run a main branch build and verify it completes in under 15 minutes
- [ ] Run a test release (RC tag) and verify it completes in under 45 minutes
- [ ] Verify all images pushed correctly to `registry.helixml.tech` and `ghcr.io`
- [ ] Verify macOS DMG build, notarization, and R2 upload still work
- [ ] Verify multi-arch manifests (controlplane, sandbox, sway, ubuntu) are correct
- [ ] Test the release-rollback pipeline still works with restructured pipelines
