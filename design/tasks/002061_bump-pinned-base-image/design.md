# Design: Bump Pinned Base Image SHA Digests Across All Dockerfiles

## Approach

For each unique `image:tag` referenced anywhere in the repo's Dockerfiles, query the registry with `docker buildx imagetools inspect <image>:<tag>` and capture the top-level manifest list (image index) digest. Apply that identical digest string to every occurrence of the same `image:tag` across all files, leaving tag names and surrounding instructions byte-identical. Verify the refreshed pins with a multi-arch `docker buildx` dry-run on one representative Dockerfile.

## Current pin inventory

| File | Line | Kind | image:tag | Current digest |
|------|------|------|-----------|----------------|
| Dockerfile | 6 | FROM | golang:1.25-bookworm | sha256:e3a54b77385b4f8a31c1db4d12429ffb3718ea76865731a787c497755d409547 |
| Dockerfile | 97 | FROM | node:23-alpine | sha256:a34e14ef1df25b58258956049ab5a71ea7f0d498e41d0b514f4b8de09af09456 |
| Dockerfile | 127 | FROM | debian:bookworm-slim | sha256:67b30a61dc87758f0caf819646104f29ecbda97d920aaf5edc834128ac8493d3 |
| Dockerfile.demos | 1 | FROM | golang:1.25-alpine3.22 | sha256:26b4d7113039cd51356bd7930ecafd1031d2975dc3b6940ec8ed09457e17cf95 |
| Dockerfile.sandbox | 14 | FROM | golang:1.25-bookworm | sha256:e3a54b77385b4f8a31c1db4d12429ffb3718ea76865731a787c497755d409547 |
| Dockerfile.sandbox | 44 | FROM | ubuntu:25.04 | sha256:27771fb7b40a58237c98e8d3e6b9ecdd9289cec69a857fccfb85ff36294dac20 |
| Dockerfile.lint | 3 | FROM | golangci/golangci-lint:v1.62-alpine | sha256:0f3af3929517ed4afa1f1bcba4eae827296017720e08ecd5c68b9f0640bc310d |
| Dockerfile.lint | 5 | FROM | golang:1.23-alpine3.21 | sha256:4bb4be21ac98da06bc26437ee870c4973f8039f13e9a1a36971b4517632b0fc6 |
| Dockerfile.qwen-build | 12 | FROM | node:20-slim | sha256:2cf067cfed83d5ea958367df9f966191a942351a2df77d6f0193e162b5febfc0 |
| Dockerfile.qwen-build | 28 | FROM | scratch | (none — excluded) |
| Dockerfile.qwen-code-build | 12 | FROM | node:20-slim | sha256:2cf067cfed83d5ea958367df9f966191a942351a2df77d6f0193e162b5febfc0 |
| Dockerfile.sway-helix | 19 | FROM | ubuntu:25.10 | sha256:4a9232cc47bf99defcc8860ef6222c99773330367fcecbf21ba2edb0b810a31e |
| Dockerfile.sway-helix | 39 | FROM | golang:1.25-bookworm | sha256:e3a54b77385b4f8a31c1db4d12429ffb3718ea76865731a787c497755d409547 |
| Dockerfile.sway-helix | 77 | FROM | ubuntu:25.10 | sha256:4a9232cc47bf99defcc8860ef6222c99773330367fcecbf21ba2edb0b810a31e |
| Dockerfile.sway-helix | 140 | FROM | ubuntu:25.10 | sha256:4a9232cc47bf99defcc8860ef6222c99773330367fcecbf21ba2edb0b810a31e |
| Dockerfile.sway-helix | 196 | FROM | ubuntu:25.10 | sha256:4a9232cc47bf99defcc8860ef6222c99773330367fcecbf21ba2edb0b810a31e |
| Dockerfile.sway-helix | 269 | FROM | ubuntu:25.10 | sha256:4a9232cc47bf99defcc8860ef6222c99773330367fcecbf21ba2edb0b810a31e |
| Dockerfile.ubuntu-helix | 18 | ARG | nvidia/cuda:12.6.3-runtime-ubuntu24.04 | sha256:92906d87596d638d35015c6353053121bd299d25943b875763321653884ba924 |
| Dockerfile.ubuntu-helix | 35 | FROM | golang:1.25-bookworm | sha256:e3a54b77385b4f8a31c1db4d12429ffb3718ea76865731a787c497755d409547 |
| Dockerfile.ubuntu-helix | 84 | FROM | ubuntu:25.10 | sha256:4a9232cc47bf99defcc8860ef6222c99773330367fcecbf21ba2edb0b810a31e |
| Dockerfile.ubuntu-helix | 191 | FROM | ubuntu:25.10 | sha256:4a9232cc47bf99defcc8860ef6222c99773330367fcecbf21ba2edb0b810a31e |
| Dockerfile.ubuntu-helix | 244 | FROM | ubuntu:25.10 | sha256:4a9232cc47bf99defcc8860ef6222c99773330367fcecbf21ba2edb0b810a31e |
| Dockerfile.zed-build | 15 | FROM | ubuntu:25.10 | sha256:4a9232cc47bf99defcc8860ef6222c99773330367fcecbf21ba2edb0b810a31e |
| Dockerfile.zed-build | 115 | FROM | scratch | (none — excluded) |
| operator/Dockerfile | 2 | FROM | golang:1.25-bookworm | sha256:e3a54b77385b4f8a31c1db4d12429ffb3718ea76865731a787c497755d409547 |
| operator/Dockerfile | 28 | FROM | gcr.io/distroless/static:nonroot | sha256:e3f945647ffb95b5839c07038d64f9811adf17308b9121d8a2b87b6a22a80a39 |
| scripts/sse-mcp-server/Dockerfile | 1 | FROM | node:20-slim | sha256:2cf067cfed83d5ea958367df9f966191a942351a2df77d6f0193e162b5febfc0 |

The 11 unique `image:tag` references to re-resolve are: `golang:1.25-bookworm`, `node:23-alpine`, `debian:bookworm-slim`, `golang:1.25-alpine3.22`, `ubuntu:25.04`, `golangci/golangci-lint:v1.62-alpine`, `golang:1.23-alpine3.21`, `node:20-slim`, `ubuntu:25.10`, `nvidia/cuda:12.6.3-runtime-ubuntu24.04`, `gcr.io/distroless/static:nonroot`. The `FROM scratch` lines have no digest and are excluded.

## Resolution procedure

1. For each unique `image:tag` listed above, run `docker buildx imagetools inspect <image>:<tag>` and capture the top-level `Digest:` line (the manifest list digest, not any per-platform child digest).
2. Confirm the manifest is a list — mediaType `application/vnd.docker.distribution.manifest.list.v2+json` or `application/vnd.oci.image.index.v1+json` — supporting both `linux/amd64` and `linux/arm64`. If it is NOT a multi-arch list, STOP and flag the image for human review (see open question 1) rather than pinning to a single-platform digest.
3. Replace the `@sha256:...` suffix on every matching `FROM` and `ARG` line across all Dockerfiles with the new digest. Tag names stay byte-identical; only the 64-character hex string after `@sha256:` changes.
4. Update the pin-date comments to `2026-06-01`. Implementation-phase grep found the date-bearing comments live in `Dockerfile.sway-helix:9`, `Dockerfile.sway-helix:37`, `Dockerfile.ubuntu-helix:10`, and `Dockerfile.ubuntu-helix:33` (each currently says `2026-04-13`). The "Pin to specific digest for stable layer caching." comments in `Dockerfile`, `Dockerfile.sandbox`, etc. carry no date and need no edit.
5. Run `docker buildx build --platform linux/amd64,linux/arm64 --print -f Dockerfile .` (or equivalent `--check`/dry-run) to verify the refreshed digests resolve on both architectures. The main `Dockerfile` is the recommended verification target because it exercises three distinct base images (`golang:1.25-bookworm`, `node:23-alpine`, `debian:bookworm-slim`) in a single multi-stage build.

## Internal consistency rule

The same `image:tag` MUST map to exactly one digest string across the whole repository. After the bump, a `grep -E '@sha256:' -h **/Dockerfile* | sort -u` (or equivalent) grouped by `image:tag` must show exactly one digest per tag. This matters for layer cache reuse: divergent digests for the same tag force redundant base-image pulls and break CI caching across builds.

## Single-arch fallback policy

If an image genuinely has no multi-arch manifest published, do NOT silently pin to a single-platform digest. Surface the image to the requester first. If the requester confirms single-arch is acceptable, add an inline comment in the Dockerfile (`# NOTE: single-arch image — no multi-arch manifest published for <image>:<tag> as of 2026-06-01`) immediately above the pinned line. Of the 11 currently-listed images, `gcr.io/distroless/static:nonroot` is the most likely candidate to double-check given how distroless variants are sometimes published per-platform.

## Open questions

1. Are any base images in this inventory published without a multi-arch manifest list? If so, flag each one to a human before pinning.
2. Should linter / pinned-tool tags (e.g. `golangci/golangci-lint:v1.62-alpine`) also be reviewed for a newer tag, or is this strictly a digest-only refresh? Default: digest-only, leave tags alone unless told otherwise.

## Notes for future digest-bump tasks

- This same procedure works unchanged on the next bump — only the resolution date and the digest values differ. Keep the inventory table up to date when files are added or removed.
- The pin-date comments are the easiest thing to forget; grep for `Pinned for stable layer caching` and `Pin to specific digest` to find them all.
- `docker buildx imagetools inspect` is preferred over `docker pull` + `docker inspect` because it returns the manifest list digest directly without pulling layers and without resolving to the local platform's child digest.
