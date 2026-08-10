# Requirements: Refresh All Dockerfile Base Image SHA Digests to Latest Multi-Arch Manifests

## Background

The Helix repository pins every base image to an `@sha256:…` digest for build
reproducibility and stable BuildKit layer caching. The trade-off is that upstream
security patches are silently skipped until someone manually refreshes the pins.
The oldest pins date from **2026-04-13**, so roughly four months of upstream CVE
fixes are currently not being picked up.

This task refreshes the digests only. No tag, version, or build logic changes.

## User Stories

### US-1 — Pick up upstream security patches

**As a** Helix maintainer
**I want** every SHA-pinned base image in every Dockerfile refreshed to the latest
digest for its existing tag
**So that** our container images include upstream security fixes released since
the last pin refresh.

**Acceptance criteria:**
- [ ] Every Dockerfile in the repo (root and subdirectories) has been checked.
- [ ] For each `image:tag@sha256:…` pin, the digest is the latest published digest
      for that exact tag at implementation time.
- [ ] Images already at their latest digest are left byte-identical (no churn).
- [ ] No image tag or version number changes anywhere.

### US-2 — Keep arm64 builds working

**As a** Helix CI pipeline building for both `linux/amd64` and `linux/arm64`
**I want** every new digest to be a multi-architecture manifest list / OCI image
index digest
**So that** arm64 builds do not break with a `no match for platform` error.

**Acceptance criteria:**
- [ ] Every new digest was resolved with `docker buildx imagetools inspect <image>:<tag>`
      and taken from the top-level `Digest:` line.
- [ ] `docker pull --platform …` was **not** used to source any digest.
- [ ] For each updated image, `MediaType` is `application/vnd.oci.image.index.v1+json`
      or `application/vnd.docker.distribution.manifest.list.v2+json`, and the
      manifest list contains both `linux/amd64` and `linux/arm64` (or `linux/arm64/v8`).
- [ ] Any image lacking a genuine multi-arch manifest is called out explicitly in
      the PR description.

### US-3 — Consistent digests across files

**As a** developer reading the Dockerfiles
**I want** the same `image:tag` to carry the identical digest in every file it
appears in
**So that** builds share cache layers and there is one obvious answer to "what
version of ubuntu:25.10 do we use?".

**Acceptance criteria:**
- [ ] `golang:1.25-bookworm` uses one digest across all 5 files it appears in.
- [ ] `ubuntu:25.10` uses one digest across all 3 files (8 `FROM` lines) plus the
      2 comment blocks that restate it.
- [ ] `node:20-slim` uses one digest across all 3 files.
- [ ] A grep for each image name shows exactly one distinct digest.

### US-4 — Accurate pin-date comments

**As a** future maintainer
**I want** the `Pinned for stable layer caching (YYYY-MM-DD)` comments to reflect
when the digests were actually refreshed
**So that** I can tell at a glance how stale the pins are.

**Acceptance criteria:**
- [ ] The 4 dated comment lines in `Dockerfile.sway-helix` and
      `Dockerfile.ubuntu-helix` are updated from `2026-04-13` to the
      implementation date.
- [ ] The 2 digest-restating comment blocks (`# - ubuntu:25.10 -> sha256:…`) match
      the new `FROM` digests exactly.
- [ ] No unrelated comments, dates, or incident notes are touched.

### US-5 — Handle unresolvable / private images safely

**As a** reviewer
**I want** any base image whose digest cannot be resolved from the build
environment left unchanged and flagged
**So that** a network or auth failure never produces a silently wrong pin.

**Acceptance criteria:**
- [ ] `registry.helixml.tech/helix/controlplane:2.11.52` is either refreshed (if
      resolvable) or left untouched with an explicit note in the PR description.
- [ ] No digest is invented, guessed, or copied from a stale source.

## Out of Scope

- Upgrading image tags or versions (`golang:1.23-alpine3.21` stays 1.23;
  `node:20-slim` stays 20; `ubuntu:25.10` stays 25.10).
- Application-level pins: Rust toolchains, npm/apt packages, Go modules,
  `GOOSE_COMMIT`, `sandbox-versions.txt`.
- Any `ARG`/`ENV` value other than the digest inside `CUDA_BASE_IMAGE`.
- Any `RUN` step or build logic.
- Non-Dockerfile files — see Open Question 1 about `.drone.yml`.

## Open Questions

1. **`.drone.yml` carries its own digest overrides that shadow the Dockerfile.**
   `.drone.yml:1671` passes `--build-arg CUDA_BASE_IMAGE=nvidia/cuda:12.6.3-runtime-ubuntu24.04@sha256:2c8193…`
   and `.drone.yml:2012` passes `--build-arg CUDA_BASE_IMAGE=ubuntu:25.10@sha256:91e7ac4c…`.
   Both override the Dockerfile values at CI build time, and the `ubuntu:25.10`
   digest there **already differs** from the one in `Dockerfile.ubuntu-helix`.
   Refreshing only the Dockerfiles means CI keeps building against stale/divergent
   bases. The stated scope is "all Dockerfiles" — should `.drone.yml` be included
   too? *Assumption if unanswered: update `.drone.yml` as well and note it clearly
   in the PR, since leaving it out defeats the security purpose for the CI images.*

2. **Roughly half the pins are already current.** As of 2026-08-10, 6 of 12
   distinct image+tags already carry the latest multi-arch digest (see design.md
   for the table). Should the PR touch only the 5 that changed, or is a no-op
   re-write expected everywhere? *Assumption: only genuinely changed digests are
   edited; pin-date comments are still refreshed to today.*

3. **The private controlplane image resolves fine from this environment** and is
   already at its latest multi-arch digest. Confirming that no action is needed
   there — unless a newer controlplane tag is intended, which would be a version
   bump and is out of scope.

4. **Digests will drift between spec-writing and implementation.** The table in
   design.md is a snapshot from 2026-08-10 and must be treated as reference, not
   as values to paste. The implementer re-resolves everything. Confirming that is
   the expectation.

5. **No automated verification exists for digest freshness.** Should this task
   also add a CI check or a `scripts/` helper that reports stale pins, or is a
   one-off manual refresh sufficient? *Assumption: one-off refresh only; tooling
   would be a separate task.*
