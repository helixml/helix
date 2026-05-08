# Requirements

## User Story

As a maintainer of the Helix repository, I want every SHA-pinned base image
digest in our Dockerfiles refreshed to the latest available multi-arch digest
for its named tag, so that builds pick up the latest OS-level and runtime
security patches without changing image versions.

## In Scope

- Every `FROM ... @sha256:<hex>` line across all Dockerfiles in the
  `/home/retro/work/helix/` repo.
- Inline date-stamp comments that record when the pin was last refreshed.
- Cross-Dockerfile consistency: the same image tag must resolve to the same
  digest everywhere it is pinned.

## Out of Scope

- Bumping image **tags or versions** (e.g. `golang:1.25` → `golang:1.26`,
  `node:20` → `node:22`). Tag names are immutable for this task.
- Adding pins to images that are currently floating (no `@sha256:`). These are
  flagged for human review only — never auto-pinned.
- Pinning images supplied via build-time `ARG` (e.g. `${CUDA_BASE_IMAGE}`,
  `${TAG}` in `runner-base`). These are intentionally dynamic.
- Any change to build logic, build args, stage structure, RUN steps, or
  COPY semantics.

## Acceptance Criteria

1. Every previously-pinned `FROM` line in the repo has been updated to the
   **current** digest for its tag (verified against the upstream registry on
   2026-04-27).
2. Every refreshed digest is a **multi-arch manifest list** covering both
   `linux/amd64` **and** `linux/arm64`, verified with
   `docker buildx imagetools inspect`. Single-arch image digests are rejected.
3. Where the same `image:tag` appears pinned in multiple Dockerfiles, all
   occurrences carry the **same** new digest.
4. Every SHA hash in the diff is exactly **64 lowercase hex characters**
   (`^[0-9a-f]{64}$`) — no typos, no truncation.
5. No `FROM` tag name changed. No build logic, ARG, RUN, COPY, or stage
   structure changed.
6. Inline pin-date comments (`(2026-03-30)`, `BASE IMAGE DIGESTS: ...`) are
   updated to `2026-04-27`.
7. A short report (in the PR body or an attached note) lists:
   - Each digest that was updated (file:line, old → new).
   - Any image tag for which a multi-arch manifest list could **not** be
     found (flagged for human review, not modified).
   - Any floating (unpinned) image observed but intentionally not modified.
