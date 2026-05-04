# Requirements

## User Story

As a release engineer, I want every `FROM` line in every Dockerfile pinned to a freshly resolved multi-arch manifest digest so that builds remain reproducible while picking up accumulated OS/runtime CVE patches.

## In-scope Dockerfiles (11 files)

```
Dockerfile
Dockerfile.demos
Dockerfile.lint
Dockerfile.qwen-build
Dockerfile.qwen-code-build
Dockerfile.sandbox
Dockerfile.sway-helix
Dockerfile.ubuntu-helix
Dockerfile.zed-build
operator/Dockerfile
scripts/sse-mcp-server/Dockerfile
```

`*.dockerignore` files are excluded — they are companions, not Dockerfiles.

## Acceptance Criteria

1. Every `FROM <image>:<tag>` line in the listed Dockerfiles is rewritten to `FROM <image>:<tag>@sha256:<digest>`. Stage references (e.g. `FROM api-base AS ...`, `FROM ${CUDA_BASE_IMAGE} AS ...`, `FROM scratch`, `FROM builder`) are not changed — only references to remote registry images.
2. Tags are unchanged. Only the `@sha256:...` portion is added or refreshed.
3. Each digest is the **multi-arch manifest list** digest (output of `docker buildx imagetools inspect <image>:<tag>` → top-level `Digest:` field), not a single-platform layer digest.
4. Where the same `image:tag` appears in multiple `FROM` lines of the same Dockerfile (e.g. `ubuntu:25.10` repeated across stages of `Dockerfile.sway-helix`), all occurrences use the **same** digest.
5. Comment-only digest documentation (e.g. the `# - ubuntu:25.10 -> sha256:...` block at the top of `Dockerfile.sway-helix` and `Dockerfile.ubuntu-helix`) is updated to match the new digests applied in the active `FROM` directives. No digest may exist only in a comment.
6. The "pinned as of" date comment in `Dockerfile.sway-helix` (line 9, line 37) and `Dockerfile.ubuntu-helix` (line 10, line 25) is updated from `2026-03-30` to `2026-05-04`.
7. The `ARG CUDA_BASE_IMAGE=nvidia/cuda:12.6.3-runtime-ubuntu24.04` default in `Dockerfile.ubuntu-helix` is pinned with a digest. The arm64 build path (`--build-arg CUDA_BASE_IMAGE=ubuntu:25.10@sha256:...`) is set in `.drone.yml` and is **out of scope** for this Dockerfile-only refresh, but the `ubuntu:25.10` digest applied in the Dockerfile must match what the drone build-arg uses (so cache and reproducibility stay aligned).
8. `FROM scratch` lines are left as-is — `scratch` is not a registry image and has no digest.
9. No typos: every digest is exactly 64 lowercase hex chars after `sha256:`. Spot-check by grepping that every `@sha256:` occurrence is followed by `[a-f0-9]{64}\b`.
10. After the change, both `docker build --platform linux/amd64` and `docker build --platform linux/arm64` of each Dockerfile pull and start successfully (the existing build flows in `.drone.yml` continue to work without modification).

## Out of Scope

- Changing image tags or major versions (e.g. `golang:1.25` → `golang:1.26`).
- Refreshing digests embedded in `.drone.yml` `--build-arg` flags. They live outside Dockerfiles and the task scope is "every Dockerfile". (Note in design that they exist and may drift.)
- Pinning images referenced from non-Dockerfile sources (`docker-compose*.yaml`, helm charts, CI step images like `docker:cli`).
