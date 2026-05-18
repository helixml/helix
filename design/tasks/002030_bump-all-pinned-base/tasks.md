# Implementation Tasks: Bump All Pinned Base Image SHA Digests Across All Dockerfiles

## Preparation

- [ ] Confirm Docker buildx is available (`docker buildx version`) and that the local environment can reach Docker Hub, `gcr.io`, and `nvcr.io` / `nvidia/cuda` registries.
- [ ] Re-grep the repo for `@sha256:` in `Dockerfile*` and `**/Dockerfile` to confirm the inventory of 24 pin occurrences across 11 files still holds. If it does not, update the digest map (next step) accordingly before editing anything.

## Build the digest map

- [ ] For each of the 11 unique image+tag combinations below, run `docker buildx imagetools inspect <image>:<tag>` and record (a) the top-level manifest list digest and (b) the list of platforms supported.
  - [ ] `golang:1.25-bookworm`
  - [ ] `ubuntu:25.10`
  - [ ] `node:20-slim`
  - [ ] `node:23-alpine`
  - [ ] `debian:bookworm-slim`
  - [ ] `golang:1.25-alpine3.22`
  - [ ] `nvidia/cuda:12.6.3-runtime-ubuntu24.04`
  - [ ] `golangci/golangci-lint:v1.62-alpine`
  - [ ] `golang:1.23-alpine3.21`
  - [ ] `gcr.io/distroless/static:nonroot`
  - [ ] `ubuntu:25.04`
- [ ] Verify each new digest matches `^sha256:[0-9a-f]{64}$` (regex check on the recorded values).
- [ ] Verify each new digest's manifest list covers **both** `linux/amd64` and `linux/arm64`. For any image where only `amd64` is published, record it as an accepted exception with a one-line rationale (see OQ-1 / OQ-2 in `requirements.md`).
- [ ] Record the final image+tag → new-digest map in a scratch file (e.g. PR description). This is the audit trail.

## Apply the edits

For each image+tag, do one `Edit` per affected file using literal `old_digest` → `new_digest`, `replace_all: true`. Since each digest string is globally unique, `replace_all` safely covers both `FROM` / `ARG` lines and any quoting comment block.

- [ ] `golang:1.25-bookworm` — update in: `Dockerfile`, `Dockerfile.sway-helix` (FROM line **and** header comment at lines 10–12), `Dockerfile.ubuntu-helix` (FROM line **and** header comment at lines 11–13), `Dockerfile.sandbox`, `operator/Dockerfile`.
- [ ] `ubuntu:25.10` — update in: `Dockerfile.zed-build`, `Dockerfile.sway-helix` (5 FROM lines **and** header comment at lines 10–12), `Dockerfile.ubuntu-helix` (2 FROM lines **and** header comment at lines 11–13).
- [ ] `node:20-slim` — update in: `Dockerfile.qwen-build`, `Dockerfile.qwen-code-build`, `scripts/sse-mcp-server/Dockerfile`.
- [ ] `node:23-alpine` — update in: `Dockerfile`.
- [ ] `debian:bookworm-slim` — update in: `Dockerfile`.
- [ ] `golang:1.25-alpine3.22` — update in: `Dockerfile.demos`.
- [ ] `nvidia/cuda:12.6.3-runtime-ubuntu24.04` — update in: `Dockerfile.ubuntu-helix` (the `ARG CUDA_BASE_IMAGE=` line at line 18, not a `FROM` line).
- [ ] `golangci/golangci-lint:v1.62-alpine` — update in: `Dockerfile.lint`.
- [ ] `golang:1.23-alpine3.21` — update in: `Dockerfile.lint`.
- [ ] `gcr.io/distroless/static:nonroot` — update in: `operator/Dockerfile`.
- [ ] `ubuntu:25.04` — update in: `Dockerfile.sandbox`.

## Validate

- [ ] `grep -oE '@sha256:[0-9a-f]{64}' Dockerfile* operator/Dockerfile scripts/sse-mcp-server/Dockerfile | wc -l` returns **24** (matches original pin count).
- [ ] For each of the 11 image+tag combinations, `grep "<image>:<tag>@sha256:" <files> | grep -oE 'sha256:[0-9a-f]{64}' | sort -u` returns exactly one digest (consistency check across files).
- [ ] `git diff` shows only `sha256:` substitutions and the two header comment block updates (in `Dockerfile.sway-helix` and `Dockerfile.ubuntu-helix`). No other changes.
- [ ] No old digest survives anywhere: grep the repo for each *old* digest (short prefix is enough) and confirm zero hits.

## Build verification

- [ ] Run `docker buildx build --platform linux/amd64,linux/arm64 --pull <one Dockerfile per unique base image>` locally where possible, OR rely on the existing CI pipeline.
- [ ] Confirm CI builds succeed on both `linux/amd64` and `linux/arm64` for every changed Dockerfile.
- [ ] If any architecture fails, do NOT bypass — investigate whether the upstream image regressed, document the finding, and decide whether to revert that one digest or escalate.

## Commit

- [ ] Single commit per the spec workflow: `chore(docker): refresh pinned base image digests (security)`. Commit body lists the 11 image+tag combinations updated and notes any accepted exceptions (e.g. `nvidia/cuda` amd64-only).
- [ ] Push and open PR; ensure CI is green before merge.
