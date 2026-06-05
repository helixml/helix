# Implementation Tasks

- [x] Re-grep `helix/` for every `FROM ... @sha256:` line and the `CUDA_BASE_IMAGE` ARG default; confirm the inventory in `design.md` matches the live repo
- [x] Resolve current multi-arch manifest digest for `golang:1.25-bookworm` via `docker buildx imagetools inspect` and verify both `linux/amd64` and `linux/arm64` are present
- [x] Resolve current multi-arch manifest digest for `node:23-alpine`
- [x] Resolve current multi-arch manifest digest for `debian:bookworm-slim`
- [x] Resolve current multi-arch manifest digest for `golangci/golangci-lint:v1.62-alpine`
- [x] Resolve current multi-arch manifest digest for `golang:1.23-alpine3.21`
- [x] Resolve current multi-arch manifest digest for `node:20-slim`
- [x] Resolve current multi-arch manifest digest for `ubuntu:25.04`
- [x] Resolve current multi-arch manifest digest for `ubuntu:25.10`
- [x] Resolve current multi-arch manifest digest for `golang:1.25-alpine3.22`
- [x] Resolve current multi-arch manifest digest for `gcr.io/distroless/static:nonroot`
- [x] Resolve current digest for `nvidia/cuda:12.6.3-runtime-ubuntu24.04` (note: actually publishes both amd64 and arm64; record this in PR)
- [x] For each image, replace the old digest with the new one across all matching Dockerfiles using a grep-then-sed sweep so every occurrence updates atomically
- [x] Re-grep each old digest across the repo to prove zero remaining occurrences
- [x] `git diff` review: confirm only the 64-hex chars after `@sha256:` changed; no tag, image, whitespace, or structural changes
- [x] For every refreshed image, run `docker buildx imagetools inspect <image>:<tag>@<new-digest>` to prove the new digest is reachable on the registry
- [x] Open the PR with the digest changes; in the body, list each image with old → new digest, confirm multi-arch coverage, call out the CUDA single-arch pin as intentional, and surface the version-drift open questions (`Dockerfile.lint` Go 1.23, `Dockerfile.sandbox` Ubuntu 25.04, `Dockerfile.demos` alpine variant) for stakeholder review
