# Requirements: Bump All Pinned Base Image SHA Digests Across All Dockerfiles

## Background

The repository pins base images by `@sha256:...` digest for build cache stability. Over time, those pinned digests accumulate unpatched OS- and package-level CVEs because newer image rebuilds (under the same tag) are not picked up automatically. This task refreshes every pinned digest to the latest multi-arch manifest list digest for the **same tag**, closing that security gap without changing any version baseline.

## User Stories

### US-1: Refresh pinned digests for security
**As a** Helix maintainer
**I want** every `@sha256:` digest in every Dockerfile refreshed to the current multi-arch manifest digest for its existing tag
**So that** rebuilt images inherit the latest OS / package security patches without changing the version baseline.

### US-2: Preserve reproducible builds
**As a** developer building Helix on either amd64 or arm64
**I want** identical image+tag pairs to resolve to the same digest everywhere they appear
**So that** builds stay reproducible and the cache behaves predictably across Dockerfiles.

### US-3: Keep documentation in sync
**As a** future reader of these Dockerfiles
**I want** inline comments that reference pinned digests to match the actual pinned digest
**So that** the documentation is not misleading.

## Acceptance Criteria

### AC-1: Every digest in every Dockerfile is refreshed
- Every `@sha256:...` occurrence in every Dockerfile under the repo has been replaced with a digest fetched on or after **2026-05-18**.
- No `@sha256:` line keeps its prior digest.

### AC-2: Tags are unchanged
- For every `FROM` / `ARG` line, the substring before `@sha256:` (image name + tag) is byte-identical to before the change.
- No upgrade of major/minor versions, no tag flavour change (e.g. `bookworm` → `bullseye`), no registry change.

### AC-3: All new digests are multi-arch manifest list digests
- Each new digest is the top-level manifest list digest reported by `docker buildx imagetools inspect <image>:<tag>` — not a per-architecture image digest.
- The manifest list must resolve to both `linux/amd64` and `linux/arm64` platforms.
- Exception (subject to the open question below): if an image legitimately has no multi-arch manifest (e.g. `nvidia/cuda` may be amd64-only), the amd64-only digest is acceptable and must be explicitly noted in the implementation tasks.

### AC-4: Same image+tag → same digest everywhere
- For every image+tag combination that appears in more than one Dockerfile, all occurrences carry the **same** new digest.
- Verifiable with a simple `grep` per `image:tag` showing one unique digest.

### AC-5: Comments referencing digests are updated
- Comment blocks in `Dockerfile.sway-helix` and `Dockerfile.ubuntu-helix` (header comments at the top of the file listing `image -> sha256:...`) reflect the new digests.
- Any other in-file comment that quotes a pinned digest is similarly updated.

### AC-6: No collateral changes
- No changes to `ARG` / `ENV` values, package versions, layer order, build steps, instructions, or any non-digest content.
- A `git diff` shows only `sha256:...` strings and the corresponding comment updates.

### AC-7: Digest strings are valid
- Each new digest matches `^sha256:[0-9a-f]{64}$` (64-character lowercase hex with `sha256:` prefix). No typos.

### AC-8: Build still works on both architectures
- CI builds (existing pipeline) pass on `linux/amd64` and `linux/arm64` for every affected Dockerfile.
- No new build-time errors caused by missing layers / wrong architecture.

## Scope

**In scope** — every Dockerfile in the repo that contains `@sha256:`. Discovered set (11 files, 24 pin occurrences, 11 unique image+tag combinations):

| File | Pin count |
| --- | --- |
| `Dockerfile` | 3 |
| `Dockerfile.demos` | 1 |
| `Dockerfile.lint` | 2 |
| `Dockerfile.qwen-build` | 1 |
| `Dockerfile.qwen-code-build` | 1 |
| `Dockerfile.sandbox` | 2 |
| `Dockerfile.sway-helix` | 6 |
| `Dockerfile.ubuntu-helix` | 4 |
| `Dockerfile.zed-build` | 1 |
| `operator/Dockerfile` | 2 |
| `scripts/sse-mcp-server/Dockerfile` | 1 |

**Out of scope**
- Upgrading any image tag (`golang:1.25` → `1.26`, `node:23` → `node:24`, etc.).
- Changing any version pinned via build args (Docker, containerd, Go toolchain versions, etc.).
- Any application code, Helm chart, docker-compose, or CI/build script changes.
- Refactoring or reorganising Dockerfile contents.

## Open Questions

- **OQ-1: `nvidia/cuda:12.6.3-runtime-ubuntu24.04`** — confirm during implementation whether a multi-arch manifest list exists. If only amd64 is published, the amd64-only digest is acceptable and the implementation note in `tasks.md` should record that choice. Existing file comments in `Dockerfile.ubuntu-helix` already acknowledge amd64-only as expected for this image.
- **OQ-2: `gcr.io/distroless/static:nonroot`** — verify that the manifest list digest resolves on both amd64 and arm64. The distroless project publishes multi-arch images for this tag, so this is expected to succeed; flag as exception only if not.
