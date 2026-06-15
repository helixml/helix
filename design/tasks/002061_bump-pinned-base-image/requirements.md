# Requirements: Bump Pinned Base Image SHA Digests Across All Dockerfiles

## Background

Every Dockerfile in this repository pins its base images by both tag and `@sha256:` digest. The digest pins lock builds to a specific image content hash, which gives us deterministic builds, stable Docker layer caching, and a clear audit trail. Over time, upstream maintainers publish updated images for the same tag (security patches, library updates, base OS fixes) and the pinned digests fall behind. This refactor refreshes every pinned digest to the latest currently-published multi-arch manifest list digest for the same image:tag. Tag names, toolchain versions, package version ARGs, and Dockerfile structure are unchanged — this is a digest-refresh only.

## User stories

- As a platform engineer, I want our pinned base image digests to reflect the latest published image for each tag, so that our containers pick up upstream security and bugfix patches without me chasing them image-by-image.
- As a security reviewer, I want every Dockerfile in the repo to resolve to a current, verifiable multi-arch manifest digest, so that I can attest the build inputs are reproducible and match what upstream publishes today.

## Acceptance criteria

1. Every `@sha256:` digest on a `FROM` or `ARG` line in every Dockerfile is replaced with the digest currently published for the same `image:tag`.
2. Each replacement digest is the top-level manifest list (image index) digest, not a single-platform image digest.
3. The resolved manifest list supports both `linux/amd64` and `linux/arm64`. Any image without a multi-arch manifest is flagged for human input rather than silently re-pinned.
4. Tag names are byte-identical before and after — only the `@sha256:...` suffix changes.
5. Internal consistency: the same `image:tag` resolves to one and only one digest string across the whole repository.
6. No toolchain version ARGs (Go, Node, Rust, Docker, gosu, containerd, Goose commit, Ghostty tag, etc.) are modified.
7. Pin-date comments listed in design.md are updated to `2026-06-01`:
   - `Dockerfile.sway-helix:9`
   - `Dockerfile.sway-helix:37`
   - `Dockerfile.ubuntu-helix:10`
   - `Dockerfile.ubuntu-helix:33`
8. At least one Dockerfile is verified with a `docker buildx` multi-arch dry-run after the digest swap to confirm the new digests resolve.
9. The change lands as a single commit whose message documents the bump and the resolution date.

## Out of scope

- Bumping any toolchain version (Go 1.25, Node 23, Node 20, Debian bookworm, Ubuntu 25.04, Ubuntu 25.10, Rust 1.87.0, Rust 1.92.0, CUDA 12.6.3, Alpine 3.21, Alpine 3.22, golangci-lint v1.62, etc.).
- Bumping any package-version ARG (`GOSU_VERSION`, `DOCKER_VERSION`, `CONTAINERD_VERSION`, `GHOSTTY_TAG`, `GOOSE_COMMIT`, `chrome-devtools-mcp@0.25.0`, `@modelcontextprotocol/server-github@2025.4.8`, etc.).
- Edits to `.dockerignore`, build scripts, or CI configuration.
- Restructuring multi-stage builds, renaming stages, or reordering instructions.
- Reviewing whether the tag itself is appropriate (e.g. moving from `v1.62-alpine` to a newer linter tag) — strictly digest-only unless explicitly requested.
