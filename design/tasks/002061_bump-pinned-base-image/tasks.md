# Implementation Tasks: Bump Pinned Base Image SHA Digests Across All Dockerfiles

- [~] Re-verify the pin inventory in the working tree (27 pin sites, 11 unique `image:tag`, plus 4 date-bearing pin comments in sway-helix/ubuntu-helix).
- [ ] Resolve the multi-arch manifest list digest for each unique `image:tag` via `docker buildx imagetools inspect <image>:<tag>`; record each new digest and confirm `linux/amd64` + `linux/arm64` support.
- [ ] Flag any image lacking a multi-arch manifest and pause for human input before pinning.
- [ ] Update every matching `FROM`/`ARG` line across all 11 Dockerfiles with the new digest, keeping tag names byte-identical.
- [ ] Update pin-date comments to `2026-06-01` in `Dockerfile.sway-helix:9`, `Dockerfile.sway-helix:37`, `Dockerfile.ubuntu-helix:10`, and `Dockerfile.ubuntu-helix:33`.
- [ ] Verify internal consistency — same `image:tag` resolves to one identical digest string everywhere (grep/sort/uniq across all Dockerfiles).
- [ ] Run `docker buildx build --platform linux/amd64,linux/arm64 --check -f Dockerfile .` as a multi-arch dry-run on the main `Dockerfile`.
- [ ] Confirm no toolchain version ARGs were touched, no tag names were changed, no `.dockerignore` or CI files were edited.
- [ ] Merge latest `origin/main`, commit, push the feature branch, and write per-repo PR description (`pull_request_helix.md`).
