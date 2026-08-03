# Implementation Tasks: Refresh Pinned Base Image Digests Across All Dockerfiles

- [ ] Re-run the inventory to confirm nothing moved since planning: `grep -rn '@sha256:' Dockerfile* operator/Dockerfile scripts/sse-mcp-server/Dockerfile .drone.yml`
- [ ] Re-resolve every unique image:tag with `docker buildx imagetools inspect <image:tag>` and record the top-level `Digest:` (do not trust the table in design.md as final)
- [ ] For each resolved digest, confirm the `MediaType` is an image index / manifest list and that the `Manifests:` list includes both `linux/amd64` and `linux/arm64(/v8)`
- [ ] Update `golang:1.25-bookworm` everywhere: `Dockerfile:6`, `Dockerfile.sandbox:14`, `Dockerfile.sway-helix:39`, `Dockerfile.ubuntu-helix:35`, `operator/Dockerfile:2`
- [ ] Update `ubuntu:25.10` everywhere: `Dockerfile.zed-build:15`, `Dockerfile.sway-helix:19,77,140,196,269`, `Dockerfile.ubuntu-helix:84,191,244`
- [ ] Update `golang:1.25-alpine3.22` in `Dockerfile.demos:1`
- [ ] Update `debian:bookworm-slim` in `Dockerfile:111`
- [ ] Update `gcr.io/distroless/static:nonroot` in `operator/Dockerfile:28`
- [ ] Re-check the images expected to be unchanged (`node:20-slim`, `node:23-alpine`, `golang:1.23-alpine3.21`, `golangci/golangci-lint:v1.62-alpine`, `ubuntu:25.04`, `nvidia/cuda:12.6.3-runtime-ubuntu24.04`, `registry.helixml.tech/helix/controlplane:2.11.52`) and update any that have in fact moved
- [ ] Update the digest ledger comments and the `(2026-04-13)` date stamp in the header blocks of `Dockerfile.sway-helix` (lines 11–12) and `Dockerfile.ubuntu-helix` (lines 12–13)
- [ ] Update the `ARG CUDA_BASE_IMAGE` default in `Dockerfile.ubuntu-helix:18` if the CUDA index digest has moved
- [ ] Update the two `--build-arg CUDA_BASE_IMAGE=…@sha256:…` values in `.drone.yml:1671` and `.drone.yml:2012` to multi-arch index digests, and call out in the PR that line 2012 previously carried an amd64-only digest in the arm64 job
- [ ] Verify no tag, version, `GOOSE_COMMIT`, Rust/Docker version arg, `RUN` line, or comment other than the digest ledger was touched: review `git diff` line by line
- [ ] Consistency check: extract all `image:tag@sha256:…` strings across all files, `sort -u`, and confirm exactly one digest per image:tag
- [ ] Typo check: run `docker buildx imagetools inspect <image>@<written-digest>` for every distinct written pin and confirm each resolves to an index
- [ ] Multi-arch build check: `docker buildx build --platform linux/amd64,linux/arm64 -o type=cacheonly` for `operator/Dockerfile` and `Dockerfile.lint` (and `Dockerfile` if the environment allows)
- [ ] Commit with a message listing each image and its old→new digest, and note the expected cold-cache rebuild on the next CI run
