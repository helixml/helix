# Requirements: Refresh Pinned Base Image Digests Across All Dockerfiles

## Background

The `helix` repo pins every base image with `@sha256:` for reproducible, cache-stable
builds (see the "BASE IMAGE DIGESTS: Pinned for stable layer caching (2026-04-13)"
comment blocks in `Dockerfile.sway-helix` and `Dockerfile.ubuntu-helix`). The pins
were last refreshed on 2026-04-13, so ~4 months of upstream OS/toolchain security
patches are not being picked up.

This is a maintenance pass: refresh the digests, keep every tag/version identical.

## User Stories

**As a platform engineer**, I want every base image pin refreshed to the current
upstream digest so that production container images include the latest OS and
runtime CVE fixes without changing any image version.

**As a build engineer**, I want every refreshed digest to be a multi-arch manifest
index digest so that both `linux/amd64` and `linux/arm64` builds keep working.

**As a reviewer**, I want the same image:tag to resolve to the identical digest in
every file (Dockerfiles *and* the digest ledger comments) so no stale copy silently
diverges.

## Scope (discovered inventory)

Files in `/home/retro/work/helix/` containing `@sha256:` base-image pins:

| File | Lines | Images |
|---|---|---|
| `Dockerfile` | 6, 19, 81, 111 | golang:1.25-bookworm, registry.helixml.tech/helix/controlplane:2.11.52, node:23-alpine, debian:bookworm-slim |
| `Dockerfile.demos` | 1 | golang:1.25-alpine3.22 |
| `Dockerfile.lint` | 3, 5 | golangci/golangci-lint:v1.62-alpine, golang:1.23-alpine3.21 |
| `Dockerfile.sandbox` | 14, 57 | golang:1.25-bookworm, ubuntu:25.04 |
| `Dockerfile.qwen-build` | 12 | node:20-slim |
| `Dockerfile.qwen-code-build` | 12 | node:20-slim |
| `Dockerfile.zed-build` | 15 | ubuntu:25.10 |
| `Dockerfile.sway-helix` | 11–12 (comments), 19, 39, 77, 140, 196, 269 | ubuntu:25.10 ×5, golang:1.25-bookworm |
| `Dockerfile.ubuntu-helix` | 12–13 (comments), 18 (ARG `CUDA_BASE_IMAGE`), 35, 84, 191, 244 | ubuntu:25.10 ×4, golang:1.25-bookworm, nvidia/cuda:12.6.3-runtime-ubuntu24.04 |
| `operator/Dockerfile` | 2, 28 | golang:1.25-bookworm, gcr.io/distroless/static:nonroot |
| `scripts/sse-mcp-server/Dockerfile` | 1 | node:20-slim |
| `.drone.yml` | 1671, 2012 | `--build-arg CUDA_BASE_IMAGE=` pins (see AC-6) |

## Acceptance Criteria

- **AC-1 (freshness)** Every `@sha256:` pin listed above equals the digest reported by
  `docker buildx imagetools inspect <image:tag>` as its top-level `Digest:` on the day
  of the change. Images already at the current digest are left untouched.
- **AC-2 (multi-arch)** Every written digest has `MediaType` of
  `application/vnd.oci.image.index.v1+json` or
  `application/vnd.docker.distribution.manifest.list.v2+json`, and its manifest list
  contains both `linux/amd64` and `linux/arm64` (`linux/arm64/v8` counts). No
  platform-specific manifest digest is written into a `FROM` line or ARG default.
- **AC-3 (no version drift)** No image name or tag changes. `golang:1.23-alpine3.21`
  stays 1.23; `node:20-slim` stays 20; `ubuntu:25.10` stays 25.10; the CUDA
  12.6.3/ubuntu24.04 tag is unchanged. `GOOSE_COMMIT`, Rust toolchain versions,
  Docker Engine version args, and `GO_VERSION` handling are untouched.
- **AC-4 (structure untouched)** Only the 64-hex-char portion after `@sha256:` changes.
  No `RUN` lines, stage names, ordering, or rationale comments are edited — except the
  digest ledger comments in AC-5.
- **AC-5 (ledger comments in sync)** The `# - ubuntu:25.10 -> sha256:…` and
  `# - golang:1.25-bookworm -> sha256:…` comment lines in `Dockerfile.sway-helix` (11–12)
  and `Dockerfile.ubuntu-helix` (12–13) are updated to the new digests, and the
  "(2026-04-13)" date in those blocks is updated to the date of this change. These are
  documentation of the pins, so leaving them stale would be a correctness bug.
- **AC-6 (.drone.yml)** The two `--build-arg CUDA_BASE_IMAGE=…@sha256:…` values in
  `.drone.yml` are refreshed. Both currently hold **platform-specific** digests (verified
  2026-08-03 — see design.md); they are replaced with the corresponding multi-arch index
  digests unless the user directs otherwise (see Open Questions).
- **AC-7 (consistency)** `grep -rn '@sha256:' Dockerfile* operator/ scripts/ .drone.yml`
  shows exactly one distinct digest per image:tag combination across the whole repo.
- **AC-8 (typo-proof)** Every new digest is produced by copy from tool output, and a
  verification pass re-resolves each written digest with
  `docker buildx imagetools inspect <image>@<written-digest>` and confirms it resolves
  and is an index. No hand-typed hex.
- **AC-9 (builds)** `docker buildx build --platform linux/amd64,linux/arm64` succeeds at
  least through the base-image pull/resolve stage for the primary Dockerfiles
  (`Dockerfile`, `Dockerfile.sandbox`, `operator/Dockerfile`, `Dockerfile.lint`), or CI
  passes.

## Out of Scope

- `helix-next` Dockerfiles — verified to have **no** digest pins at all
  (`FROM golang:1.25-alpine`, `FROM node:20-alpine`, …). Adding pins there is a
  separate decision.
- `.github/workflows/` (`codeql.yml`, `stainless_action.yml`) — verified to contain no
  base-image digest pins.
- `registry.helixml.tech/helix/controlplane:2.11.52` is an immutable internal version
  tag; its digest is refreshed only if the tag was re-pushed (it has not been — verified
  identical on 2026-08-03).

## Open Questions

1. **`.drone.yml` arm64 CUDA arg looks wrong today.** Line 2012 passes
   `ubuntu:25.10@sha256:91e7ac4c…` to the **arm64** build, and that digest is the
   **linux/amd64** manifest of the old ubuntu:25.10 index. Line 1671 passes an
   amd64-only `nvidia/cuda` manifest to the amd64 build (harmless but arch-locked).
   Should this task replace both with multi-arch index digests (recommended, and the
   assumption written into AC-6), or is the arch-specific pinning deliberate and to be
   preserved with only a freshness bump?
2. **Is `.drone.yml` in scope at all?** The objective says "all Dockerfiles"; the drone
   pipeline is the only CI config that embeds base image digests. Assumption: yes,
   in scope, otherwise `Dockerfile.ubuntu-helix`'s ARG default and the pipeline's
   override would drift apart.
3. **Snapshot vs. re-resolve.** design.md records the digests resolved live on
   2026-08-03. If implementation happens days later, should the agent re-resolve
   (assumption: **yes, always re-resolve**; the table is a cross-check, not the source
   of truth)?
4. **CUDA runtime image freshness.** `nvidia/cuda:12.6.3-runtime-ubuntu24.04` still
   resolves to the exact digest already pinned (`92906d87…`) — NVIDIA does not appear
   to rebuild that tag. Confirm no action is expected there.
