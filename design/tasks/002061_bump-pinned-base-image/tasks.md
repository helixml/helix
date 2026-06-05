# Implementation Tasks: Bump Pinned Base Image SHA Digests Across All Dockerfiles

- [ ] Enumerate all `FROM`/`ARG` base-image pins (already inventoried in design.md — 27 pin sites across 11 Dockerfiles, 11 unique `image:tag` references).
- [ ] For each unique `image:tag`, resolve the multi-arch manifest list digest via `docker buildx imagetools inspect <image>:<tag>` and record the new digest.
- [ ] Confirm each resolved manifest is a multi-arch list (linux/amd64 + linux/arm64); flag any image lacking a multi-arch manifest and pause for human input before pinning.
- [ ] Update every matching `FROM`/`ARG` line across all Dockerfiles with the new digest, keeping tag names byte-identical.
- [ ] Verify internal consistency — same `image:tag` resolves to the identical digest string everywhere (grep/sort/uniq check across all Dockerfiles).
- [ ] Update pin-date comments to `2026-06-01` in `Dockerfile.sandbox:12`, `Dockerfile.sandbox:42`, and `Dockerfile.ubuntu-helix:10`.
- [ ] Run `docker buildx build --platform linux/amd64,linux/arm64 --print -f Dockerfile .` as a dry-run verification on the main `Dockerfile`.
- [ ] Confirm no toolchain version ARGs were touched, no tag names were changed, and no `.dockerignore` or CI files were edited.
- [ ] Commit with a single message documenting the bump and the resolution date (2026-06-01).
