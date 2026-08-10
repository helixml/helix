# Implementation Tasks: Refresh All Dockerfile Base Image SHA Digests to Latest Multi-Arch Manifests

## Resolve

- [ ] Re-confirm the pin inventory: `grep -rn "sha256:" --include="Dockerfile*" /home/retro/work/helix` (expect 12 distinct image:tag pairs across 11 files)
- [ ] For each distinct image:tag, run `docker buildx imagetools inspect <image>:<tag>` and record the top-level `Digest:` value — never use `docker pull --platform`
- [ ] For each resolved digest, confirm `MediaType` is an OCI index or Docker manifest list, and that both `linux/amd64` and `linux/arm64` appear under `Platform:`
- [ ] Build a single old-digest → new-digest mapping table; note which images are already current (expect several to be unchanged)
- [ ] Resolve `registry.helixml.tech/helix/controlplane:2.11.52`; if it fails, leave it unchanged and record it for the PR description

## Apply

- [ ] Update `golang:1.25-bookworm` in `Dockerfile`, `Dockerfile.sandbox`, `Dockerfile.sway-helix`, `Dockerfile.ubuntu-helix`, `operator/Dockerfile`
- [ ] Update `ubuntu:25.10` in all 5 `FROM` lines of `Dockerfile.sway-helix`, both in `Dockerfile.ubuntu-helix`, and in `Dockerfile.zed-build`
- [ ] Update `node:20-slim` in `Dockerfile.qwen-build`, `Dockerfile.qwen-code-build`, `scripts/sse-mcp-server/Dockerfile`
- [ ] Update the single-site pins: `node:23-alpine` and `debian:bookworm-slim` (`Dockerfile`), `ubuntu:25.04` (`Dockerfile.sandbox`), `golang:1.25-alpine3.22` (`Dockerfile.demos`), `golang:1.23-alpine3.21` and `golangci/golangci-lint:v1.62-alpine` (`Dockerfile.lint`), `gcr.io/distroless/static:nonroot` (`operator/Dockerfile`)
- [ ] Update the `CUDA_BASE_IMAGE` ARG default digest at `Dockerfile.ubuntu-helix:18` (digest only — do not touch the ARG name or the `nvidia/cuda:12.6.3-runtime-ubuntu24.04` tag)
- [ ] Update the digest-restating header comments at `Dockerfile.sway-helix:11-12` and `Dockerfile.ubuntu-helix:12-13` so they match the new `FROM` digests
- [ ] Update the pin-date comments from `2026-04-13` to today at `Dockerfile.sway-helix:9`, `Dockerfile.sway-helix:37`, `Dockerfile.ubuntu-helix:10`, `Dockerfile.ubuntu-helix:33`
- [ ] Decide on `.drone.yml` (Open Question 1): update the `--build-arg CUDA_BASE_IMAGE=…` digests at lines 1671 and 2012 to match, or explicitly document why they were left stale

## Verify

- [ ] Consistency: for each image, `grep -rho '<image>:<tag>@sha256:[a-f0-9]*' --include='Dockerfile*' . | sort -u` returns exactly one line
- [ ] Multi-arch: re-inspect each new digest by digest reference and confirm amd64 + arm64 are both present
- [ ] No drift: review `git diff` and confirm only 64-hex digest strings and the four dates changed — no tags, versions, ARG/ENV values, or RUN steps
- [ ] Confirm no `sha256:` outside a base-image pin was touched (e.g. download checksums inside RUN steps)
- [ ] Run a local build smoke test on at least `Dockerfile` and `Dockerfile.ubuntu-helix` (or rely on CI) and confirm the base images pull cleanly

## Ship

- [ ] Commit on a feature branch with a message naming the images whose digests moved
- [ ] In the PR description, list old → new digest per image, state that all digests are multi-arch manifest lists, and flag any image that could not be resolved or that lacks a genuine multi-arch manifest
- [ ] Confirm CI is green before merge
