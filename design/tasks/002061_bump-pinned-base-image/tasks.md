# Implementation Tasks: Bump Pinned Base Image SHA Digests Across All Dockerfiles

- [x] Re-verify the pin inventory in the working tree (27 pin sites, 11 unique `image:tag`, plus 4 date-bearing pin comments in sway-helix/ubuntu-helix).
- [x] Resolve the multi-arch manifest list digest for each unique `image:tag` via `docker buildx imagetools inspect <image>:<tag>`; record each new digest and confirm `linux/amd64` + `linux/arm64` support.
- [x] Flag any image lacking a multi-arch manifest and pause for human input before pinning. *(All 11 images publish multi-arch manifests covering linux/amd64 + linux/arm64 — nothing to flag.)*
- [x] Update every matching `FROM`/`ARG` line across all 11 Dockerfiles with the new digest, keeping tag names byte-identical. *(4 of 11 image:tags actually changed: `golang:1.25-bookworm`, `debian:bookworm-slim`, `golang:1.25-alpine3.22`, `gcr.io/distroless/static:nonroot`. The other 7 already pin the latest published digest.)*
- [x] Update pin-date comments to `2026-06-01` in `Dockerfile.sway-helix:9`, `Dockerfile.sway-helix:37`, `Dockerfile.ubuntu-helix:10`, and `Dockerfile.ubuntu-helix:33` (plus the documentary `# - golang:1.25-bookworm -> sha256:...` lines at `Dockerfile.sway-helix:12` and `Dockerfile.ubuntu-helix:13`).
- [x] Verify internal consistency — same `image:tag` resolves to one identical digest string everywhere (grep/sort/uniq across all Dockerfiles). *(All 11 image:tag → exactly 1 digest.)*
- [x] Run `docker buildx build --platform linux/amd64,linux/arm64 --check -f Dockerfile .` as a multi-arch dry-run on the main `Dockerfile`. *(Result: "Check complete, no warnings found." — all 3 new digests resolved on registry.)*
- [x] Confirm no toolchain version ARGs were touched, no tag names were changed, no `.dockerignore` or CI files were edited. *(Diff: only digest swaps + date-comment edits across 6 files.)*
- [~] Merge latest `origin/main`, commit, push the feature branch, and write per-repo PR description (`pull_request_helix.md`).
