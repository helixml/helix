# Design: Refresh All Pinned Base Image SHA Digests in Dockerfiles

## Overview

A digest-refresh sweep across 12 Dockerfiles and 11 unique `image:tag` combinations. No code changes, no tag bumps — only the `@sha256:…` hex string after each pinned base image is updated to the current upstream multi-arch index digest. A small handful of pin-date comments are also bumped to today (2026-05-25).

## Current State Inventory

Discovered via `grep -nE '^FROM\s' helix/**/Dockerfile*`. **11 unique pinned `image:tag` combinations** referenced from **12 Dockerfiles** (`Dockerfile.demos` adds an 11th unique pin via `golang:1.25-alpine3.22`; counts of file vs. pin differ because several files share pins):

| # | Image:tag | Current digest (truncated) | Files using it |
|---|---|---|---|
| 1 | `golang:1.25-bookworm` | `sha256:e3a54b77…d409547` | `Dockerfile`, `Dockerfile.sandbox`, `Dockerfile.sway-helix`, `Dockerfile.ubuntu-helix`, `operator/Dockerfile` |
| 2 | `golang:1.23-alpine3.21` | `sha256:4bb4be21…32b0fc6` | `Dockerfile.lint` |
| 3 | `golang:1.25-alpine3.22` | `sha256:26b4d711…e17cf95` | `Dockerfile.demos` |
| 4 | `ubuntu:25.04` | `sha256:27771fb7…294dac20` | `Dockerfile.sandbox` |
| 5 | `ubuntu:25.10` | `sha256:4a9232cc…810a31e` | `Dockerfile.sway-helix` (5 FROMs), `Dockerfile.ubuntu-helix` (2 FROMs), `Dockerfile.zed-build` |
| 6 | `node:20-slim` | `sha256:2cf067cf…b5febfc0` | `Dockerfile.qwen-build`, `Dockerfile.qwen-code-build`, `scripts/sse-mcp-server/Dockerfile` |
| 7 | `node:23-alpine` | `sha256:a34e14ef…f09af09456` | `Dockerfile` |
| 8 | `debian:bookworm-slim` | `sha256:67b30a61…ac8493d3` | `Dockerfile` |
| 9 | `golangci/golangci-lint:v1.62-alpine` | `sha256:0f3af392…0bc310d` | `Dockerfile.lint` |
| 10 | `gcr.io/distroless/static:nonroot` | `sha256:e3f94564…22a80a39` | `operator/Dockerfile` |
| 11 | `nvidia/cuda:12.6.3-runtime-ubuntu24.04` | `sha256:92906d87…3884ba924` | `Dockerfile.ubuntu-helix` (ARG default; amd64 only — see below) |

Excluded (intentional, not pinnable / not external):
- `FROM scratch` — `Dockerfile.qwen-build:28`, `Dockerfile.qwen-code-build:35`, `Dockerfile.zed-build:115`.
- `FROM api-base AS …` (×4) and `FROM ui-base AS …` (×2) in `Dockerfile` — local stages.
- `FROM builder-env AS builder` — `Dockerfile.zed-build:72` — local stage.

## Approach

A digest-only sweep, one image at a time:

1. **Resolve.** For each of the 11 unique `image:tag` combos, fetch the current multi-arch index manifest digest from the live registry.
2. **Verify multi-arch.** Confirm the resolved digest lists both `linux/amd64` and `linux/arm64`. The NVIDIA CUDA image is the only one expected to fail this check (see below) — flag and document any others.
3. **Substitute.** For each Dockerfile, replace the old digest with the new one. Where the same `image:tag` appears in multiple files, the same new digest is used everywhere — verified after substitution with a `grep` consistency check.
4. **Refresh pin-date comments.** Update the four `2026-04-13` comment occurrences in `Dockerfile.ubuntu-helix` and `Dockerfile.sway-helix` to `2026-05-25`, and refresh the `# - <image> -> sha256:…` summary lines in their headers to the new digests.
5. **Validate.** Run `docker buildx build --platform linux/amd64,linux/arm64` on the primary `Dockerfile` as a smoke test.

## Key Decisions

### D1: Use `docker buildx imagetools inspect` as the source of truth
`docker buildx imagetools inspect <image>:<tag>` reports the index manifest digest and the list of platforms in one call — ideal for both "what is the current digest?" and "does this digest cover amd64 and arm64?". The fallback is `docker manifest inspect`, but `imagetools` gives clearer multi-arch output and is the documented buildx workflow.

**Why this over `docker pull` + `docker inspect`:** `docker pull` resolves to the platform-specific digest for the host architecture, which is exactly what we must NOT use. `imagetools inspect` returns the index manifest, which is what we want.

### D2: Same image:tag → same digest, everywhere
Multi-arch index digests are stable and global (one digest per image:tag at any moment in time). Two files referencing `golang:1.25-bookworm` MUST therefore receive the same new SHA. The implementation must produce a deterministic mapping `image:tag → new_digest` once per image and apply it across files. A post-substitution `grep` of each new digest should show a count equal to the row's "Files using it" entry.

### D3: NVIDIA CUDA is documented as a known single-arch case
The existing header comment in `Dockerfile.ubuntu-helix` (lines 10–18) already explains that amd64 uses `nvidia/cuda:…` and arm64 is overridden to `ubuntu:25.10` via `--build-arg CUDA_BASE_IMAGE=ubuntu:25.10` from the pipeline. If the CUDA tag's `imagetools inspect` confirms it is amd64-only (likely), the existing comment is already correct — only the digest needs refreshing, and a single brief line should be added to the header comment block making the single-arch nature explicit: `# Note: nvidia/cuda tag publishes only linux/amd64; arm64 path uses ubuntu:25.10 (see ARG override).` If the CUDA tag DOES publish a multi-arch index, no comment change is needed.

### D4: Pin-date comments are part of the change
The comments in `Dockerfile.ubuntu-helix` and `Dockerfile.sway-helix` currently say `2026-04-13`. Leaving them stale after refreshing the digests would lie to future readers about when the pin was last validated. Comments are bumped to `2026-05-25` and the `# - <image> -> sha256:<digest>` summary lines updated to the new SHAs.

### D5: Tag/version bumps are deliberately excluded
The user request is unambiguous: do not change tags. `golang:1.23-alpine3.21` (used only by `Dockerfile.lint`) and `ubuntu:25.04` (used only by `Dockerfile.sandbox`) stay on those tags even though the rest of the repo has moved on to `golang:1.25` and `ubuntu:25.10`. Tag drift inside the repo is a separate concern and is not addressed here.

## Risks & Mitigations

- **Risk:** A new digest is pulled for one architecture but the manifest list is actually broken on the other. **Mitigation:** the multi-arch `docker buildx build` smoke test on the primary `Dockerfile` exercises both platforms end-to-end.
- **Risk:** Inconsistent digests across files for the same image:tag. **Mitigation:** D2 mandates a per-image lookup once, applied everywhere; post-substitution `grep` verifies count.
- **Risk:** A typo in a 64-char hex string passes inspection but breaks the build. **Mitigation:** copy digests directly from `imagetools inspect` output (no manual retyping); the multi-arch smoke build catches malformed pins as a "manifest not found" pull error.
- **Risk:** Upstream registry rate-limits / outages mid-refresh. **Mitigation:** the inspect calls are cheap and idempotent — re-run as needed.

## Notes for Future Refresh Sweeps

- The inventory above is the authoritative starting list for the next refresh; re-grep `^FROM\s` to confirm nothing was added.
- The pin-date convention used in this repo lives only in `Dockerfile.ubuntu-helix` and `Dockerfile.sway-helix`. If the convention spreads to other Dockerfiles, future sweeps must include them.
- `Dockerfile.sandbox` and `Dockerfile.lint` lag the rest of the repo on tag versions — flag for the team but do not "fix" as part of a digest refresh.
- `nvidia/cuda` is the only known single-arch base. If a new arm64-incompatible base is ever added, it MUST follow the same `ARG`-override-with-ubuntu pattern.
