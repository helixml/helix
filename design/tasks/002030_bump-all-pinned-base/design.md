# Design: Bump All Pinned Base Image SHA Digests Across All Dockerfiles

## Overview

This is a pure refactor: regenerate every `@sha256:...` digest pinned in the repo's Dockerfiles to the current multi-arch manifest digest for the same tag, and update the few comment lines that reference those digests. No tags, ARGs, layers, or other content change.

## Inventory (discovered from the codebase, 2026-05-18)

**11 unique image+tag combinations**, 24 occurrences across 11 Dockerfiles:

| Image + tag | Current digest (short) | Files |
| --- | --- | --- |
| `golang:1.25-bookworm` | `sha256:e3a54b77…` | `Dockerfile:6`, `Dockerfile.sway-helix:39`, `Dockerfile.ubuntu-helix:27`, `Dockerfile.sandbox:14`, `operator/Dockerfile:2` |
| `ubuntu:25.10` | `sha256:4a9232cc…` | `Dockerfile.zed-build:15`, `Dockerfile.sway-helix:19,77,140,196,269`, `Dockerfile.ubuntu-helix:76,185` |
| `node:20-slim` | `sha256:2cf067cf…` | `Dockerfile.qwen-build:12`, `Dockerfile.qwen-code-build:12`, `scripts/sse-mcp-server/Dockerfile:1` |
| `node:23-alpine` | `sha256:a34e14ef…` | `Dockerfile:95` |
| `debian:bookworm-slim` | `sha256:67b30a61…` | `Dockerfile:125` |
| `golang:1.25-alpine3.22` | `sha256:26b4d711…` | `Dockerfile.demos:1` |
| `nvidia/cuda:12.6.3-runtime-ubuntu24.04` | `sha256:92906d87…` | `Dockerfile.ubuntu-helix:18` |
| `golangci/golangci-lint:v1.62-alpine` | `sha256:0f3af392…` | `Dockerfile.lint:3` |
| `golang:1.23-alpine3.21` | `sha256:4bb4be21…` | `Dockerfile.lint:5` |
| `gcr.io/distroless/static:nonroot` | `sha256:e3f94564…` | `operator/Dockerfile:28` |
| `ubuntu:25.04` | `sha256:27771fb7…` | `Dockerfile.sandbox:44` |

Comment blocks that quote digests (must be kept in sync):
- `Dockerfile.sway-helix` lines 10–12 — quotes `ubuntu:25.10` and `golang:1.25-bookworm` digests.
- `Dockerfile.ubuntu-helix` lines 11–13 — quotes `ubuntu:25.10` and `golang:1.25-bookworm` digests.

No other in-file comments inline a full digest (verified by grep for `sha256:` in non-`FROM`/`ARG` lines).

## Approach

### 1. Build the digest map first, then edit

Resolve the 11 unique image+tag combinations to their new multi-arch manifest digests **before** touching any Dockerfile. This guarantees consistency (AC-4) — every occurrence of the same image+tag receives the exact same digest, because the digest is looked up once per combination.

```
declare -A NEW_DIGEST
for img in <11 unique image:tag values>; do
  NEW_DIGEST[$img]=$(docker buildx imagetools inspect "$img" \
                       --format '{{json .Manifest.Digest}}' \
                       | tr -d '"')
done
```

Then do the edits driven by the map. Editing without the map first risks drift if a new image is pushed mid-task.

### 2. Verify multi-arch before accepting a digest

For each resolved digest, confirm the manifest list contains both architectures:

```
docker buildx imagetools inspect <image>:<tag>
# Look for both: linux/amd64 and linux/arm64 entries under "Manifests:"
```

If only amd64 is present (expected case: `nvidia/cuda`), record this explicitly in `tasks.md` as an accepted exception (see OQ-1). Do **not** silently use a single-arch digest for an image that normally publishes multi-arch.

### 3. Edit with literal string replacement

Each digest occurrence is unique enough that `Edit` (literal `old_string` → `new_string`) is the right tool. Don't write a regex that catches "any sha256" because:
- Some lines also contain the digest in a comment (e.g. sway/ubuntu header comments) — those need their own targeted replacement so the surrounding context is preserved correctly.
- Edit's "must be unique in file" guarantee provides a built-in safety check that the line you intended to change is in fact the one being changed.

For each unique image+tag, run an `Edit` with `replace_all: true` on `OLD_DIGEST` → `NEW_DIGEST` within each affected file. Since each digest string is globally unique, `replace_all` is safe and handles both `FROM` lines and the comment blocks in one pass per file.

### 4. Validate after editing

Three cheap, mechanical checks before commit:

1. **Digest validity** — every new digest matches `^sha256:[0-9a-f]{64}$`. A `grep -oE '@sha256:[0-9a-f]{64}'` across all Dockerfiles, piped through `wc -l`, must equal **24** (the original pin count). Any other count = something went wrong.
2. **Consistency** — for each of the 11 image+tag combinations, every occurrence carries the same new digest. `grep` per image+tag and pipe to `sort -u`; each must collapse to exactly one digest.
3. **Diff hygiene** — `git diff` shows only `sha256:` changes and the two comment-block updates. Anything else means the edit went wrong.

### 5. Build verification

For changed Dockerfiles, run `docker buildx build --platform linux/amd64,linux/arm64 --dry-run` (or the existing CI pipeline if available locally) to confirm the new digests resolve on both architectures. If buildx isn't set up locally, rely on CI; either way, do not consider the task done until both-arch CI is green.

## Key Decisions

### D-1: One digest map, derived once, applied many times
**Decision:** Resolve digests once into a map, then apply.
**Alternative considered:** Looking up the digest per `Edit` operation.
**Why:** Multi-occurrence images would otherwise risk drift if `docker pull` runs at slightly different times and the upstream tag is being updated mid-task. The map also serves as the audit trail recorded in `tasks.md`.

### D-2: Use `docker buildx imagetools inspect`, not `docker pull` + `inspect`
**Decision:** Resolve via `buildx imagetools inspect`.
**Why:** `docker pull` + `docker image inspect` returns the *per-architecture* digest of whatever was pulled (host arch). The manifest list digest — the one we want — only surfaces from `buildx imagetools inspect`. Using the wrong source is the most common way to accidentally pin amd64-only.

### D-3: Preserve all tags exactly, even if a newer tag is available
**Decision:** No tag changes whatsoever, even when a newer minor / patch tag has shipped (e.g. don't bump `golang:1.23-alpine3.21` to `1.23-alpine3.22`).
**Why:** Out-of-scope per the user request. Tag changes carry behavioural risk (toolchain bumps, base OS changes); digest refresh under a stable tag is a security-only change.

### D-4: Accept amd64-only digest only for documented exceptions
**Decision:** If an image's manifest list doesn't include arm64, record it explicitly in `tasks.md` and proceed with the amd64 digest. Do not skip the image.
**Why:** The repo today already builds amd64-only for `nvidia/cuda` (see the file-level comment in `Dockerfile.ubuntu-helix`), so dropping that pin would be a regression. Explicit acknowledgement avoids the "silent exception" trap.

### D-5: Don't refactor while we're in here
**Decision:** No consolidation of duplicated `FROM golang:1.25-bookworm@…` lines into a shared base, no comment cleanup beyond keeping referenced digests accurate, no version bumps.
**Why:** Single-purpose changes are easier to review and revert. CVE backports masked by a refactor is exactly the kind of mistake this task is trying to prevent.

## Risks & Mitigations

| Risk | Mitigation |
| --- | --- |
| Accidentally pin a per-arch digest instead of a manifest list digest | Always use `docker buildx imagetools inspect` (D-2); verify both `linux/amd64` and `linux/arm64` are present in the manifest list before accepting. |
| Inconsistent digest across files for the same image+tag | Build digest map first (D-1), then apply; post-edit consistency grep (validation step 3). |
| Stale comment lines still quoting old digests | Explicit list of affected comment blocks (sway-helix lines 10–12, ubuntu-helix lines 11–13) included in `tasks.md`. |
| A new digest contains a typo or wrong length | Regex check in validation; CI build will fail on a malformed digest. |
| Upstream pushes a new image mid-task, causing partial drift | Resolve all digests in one batch at the start (D-1). Re-run the full map lookup if more than a few hours elapse before commit. |
| CI fails on arm64 after the bump | Run a multi-arch buildx dry-run locally where possible; otherwise treat CI failure as the trigger to investigate that specific image (likely a real upstream regression worth flagging upstream, not worked around). |

## Notes for Future Agents

- This task pattern (refresh pinned digests) will recur periodically. The digest-map-then-edit approach scales well and is the same flow regardless of how many Dockerfiles exist.
- The comment-block convention in `Dockerfile.sway-helix` and `Dockerfile.ubuntu-helix` (listing pinned digests at the top of the file) is a maintainability liability — future refreshes must remember to update them. Consider in a separate task whether those header comments should be removed in favour of the inline pin being the single source of truth.
- `Dockerfile.ubuntu-helix:18` uses `ARG CUDA_BASE_IMAGE=nvidia/cuda:…@sha256:…` rather than `FROM …@sha256:…`. Treat ARG-level pins the same as FROM-level pins for this task.
