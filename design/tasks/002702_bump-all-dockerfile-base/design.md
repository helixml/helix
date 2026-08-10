# Design: Refresh All Dockerfile Base Image SHA Digests to Latest Multi-Arch Manifests

## Approach

This is a mechanical find-and-replace task with one hard correctness constraint
(multi-arch digests only). No new tooling, no scripts, no abstractions — resolve
each distinct `image:tag` once, then apply the resulting digest everywhere it
appears.

The work is deliberately organised **per distinct image+tag, not per file**. That
is what guarantees US-3 consistency: `golang:1.25-bookworm` appears in 5 files and
`ubuntu:25.10` in 3, so a file-by-file pass invites divergence.

## Inventory (discovered during planning)

13 Dockerfiles exist; 11 contain digest pins (`Dockerfile.qwen-build` and
`Dockerfile.qwen-code-build` do, `*.dockerignore` files are not Dockerfiles).

12 distinct `image:tag` pairs, 21 pin sites total:

| # | Image:tag | Files (pin sites) |
|---|---|---|
| 1 | `golang:1.25-bookworm` | `Dockerfile:6`, `Dockerfile.sandbox:14`, `Dockerfile.sway-helix:39`, `Dockerfile.ubuntu-helix:35`, `operator/Dockerfile:2` + comment lines `sway-helix:12`, `ubuntu-helix:13` |
| 2 | `ubuntu:25.10` | `Dockerfile.sway-helix:19,77,140,196,269`, `Dockerfile.ubuntu-helix:191,244`, `Dockerfile.zed-build:15` + comment lines `sway-helix:11`, `ubuntu-helix:12` |
| 3 | `node:20-slim` | `Dockerfile.qwen-build:12`, `Dockerfile.qwen-code-build:12`, `scripts/sse-mcp-server/Dockerfile:1` |
| 4 | `node:23-alpine` | `Dockerfile:81` |
| 5 | `debian:bookworm-slim` | `Dockerfile:111` |
| 6 | `ubuntu:25.04` | `Dockerfile.sandbox:57` |
| 7 | `golang:1.25-alpine3.22` | `Dockerfile.demos:1` |
| 8 | `golang:1.23-alpine3.21` | `Dockerfile.lint:5` |
| 9 | `golangci/golangci-lint:v1.62-alpine` | `Dockerfile.lint:3` |
| 10 | `nvidia/cuda:12.6.3-runtime-ubuntu24.04` | `Dockerfile.ubuntu-helix:18` (ARG default) |
| 11 | `gcr.io/distroless/static:nonroot` | `operator/Dockerfile:28` |
| 12 | `registry.helixml.tech/helix/controlplane:2.11.52` | `Dockerfile:19` (private registry) |

## Snapshot of resolved digests (2026-08-10 — REFERENCE ONLY, re-resolve at implementation time)

Resolved with `docker buildx imagetools inspect <image>:<tag>`; all confirmed as
manifest lists / OCI indexes containing both amd64 and arm64.

| Image:tag | Current pin (prefix) | Latest as of 2026-08-10 | Action |
|---|---|---|---|
| `golang:1.25-bookworm` | `e3a54b77…` | `908f8ff2ec296df2f349563072c7925775cd28b50361a52ed834a8a37399b9bf` | **UPDATE** (5 FROM + 2 comments) |
| `ubuntu:25.10` | `4a9232cc…` | `7cc5e35f6567ee8c66d2abb4aab0fd866669e6207c237c3a8f0947a5c7f17092` | **UPDATE** (8 FROM + 2 comments) |
| `debian:bookworm-slim` | `67b30a61…` | `abd67ffcfa541b485a3dff59865ab629aa048a6c613e639d36e7456b0b229241` | **UPDATE** |
| `golang:1.25-alpine3.22` | `26b4d711…` | `65b4400aee0927412e9ed791a11893273a49d55df24841f7599660fb80dae464` | **UPDATE** |
| `gcr.io/distroless/static:nonroot` | `e3f94564…` | `f7f8f729987ad0fdf6b05eeeae94b26e6a0f613bdf46feea7fc40f7bd72953e6` | **UPDATE** |
| `node:20-slim` | `2cf067cf…` | `2cf067cfed83…` | already current |
| `node:23-alpine` | `a34e14ef…` | `a34e14ef1df2…` | already current |
| `ubuntu:25.04` | `27771fb7…` | `27771fb7b40a…` | already current |
| `golang:1.23-alpine3.21` | `4bb4be21…` | `4bb4be21ac98…` | already current |
| `golangci/golangci-lint:v1.62-alpine` | `0f3af392…` | `0f3af3929517…` | already current |
| `nvidia/cuda:12.6.3-runtime-ubuntu24.04` | `92906d87…` | `92906d8759…` | already current |
| `registry.helixml.tech/helix/controlplane:2.11.52` | `58705ba0…` | `58705ba01f…` | already current (resolves fine — no auth issue) |

So the expected blast radius is **5 images across 8 files**, plus 4 pin-date
comment lines. Digests will have moved by implementation time; the counts may
differ, but the method does not.

## Key Decisions

**Use `docker buildx imagetools inspect`, not `docker pull`.**
`docker pull --platform linux/amd64` resolves to a *single-platform* manifest
digest. Pinning that would make `docker build --platform linux/arm64` fail with
`no match for platform in manifest`. `imagetools inspect` prints the manifest
index digest on its top-level `Digest:` line, which is what a `FROM` pin needs.
Verified working in this environment (buildx v0.36.0), including against the
private `registry.helixml.tech` registry.

**Resolve per image+tag, apply per occurrence.**
Build the digest table first, then apply. `sed -i` with the old digest as the
search key is the safest mechanism: it updates `FROM` lines and comment lines in
one pass and cannot accidentally rewrite an unrelated `sha256:` (e.g. a checksum
in a `RUN` step) because the old digest is a unique 64-hex string.

**Do not touch images already at their latest digest.**
Rewriting an identical value adds diff noise for no benefit. Pin-date comments
are still refreshed, since the *verification* happened today even where the
digest did not move.

**Leave unresolvable images untouched and flag them.**
A failed `imagetools inspect` (network, auth, rate limit) must abort that image,
not fall back to a guess. In practice the private image resolved cleanly during
planning, so this is a safety net rather than an expected path.

## Verification Strategy

Three checks, all cheap and all runnable by a reviewer:

1. **Multi-arch assertion** — for each new digest, re-run
   `docker buildx imagetools inspect <image>:<tag>@<new-digest>` and confirm the
   `MediaType` is an index/manifest-list and that both `linux/amd64` and
   `linux/arm64` appear under `Platform:`.
2. **Consistency assertion** — `grep -rho '<image>:<tag>@sha256:[a-f0-9]*' --include='Dockerfile*' . | sort -u`
   returns exactly one line per image.
3. **No-version-drift assertion** — `git diff` shows only 64-hex digest strings
   and date strings changing. Any diff hunk touching a tag, `ARG` name, `ENV`, or
   `RUN` line is a bug.

A full image build is *not* required for merge — digest refreshes within a tag
are by definition the same OS/toolchain release — but CI will exercise the main
`Dockerfile`, `Dockerfile.ubuntu-helix`, and `Dockerfile.sway-helix` paths anyway,
and a red build there is a hard blocker.

## Gotchas Found During Planning

- **`.drone.yml` shadows the Dockerfile pins.** Lines 1671 and 2012 pass
  `--build-arg CUDA_BASE_IMAGE=…@sha256:…`, overriding `Dockerfile.ubuntu-helix:18`
  at CI time. The `ubuntu:25.10` digest at `.drone.yml:2012` (`91e7ac4c…`) already
  differs from the Dockerfile's (`4a9232cc…`). See Open Question 1 in
  requirements.md — this is the single most likely thing for an implementer to miss.
- **`ubuntu:25.10` is the widest blast radius**: 8 `FROM` lines across 3 files plus
  2 comment restatements. Miss one and cache stability is lost for that stage.
- **Two files restate digests in header comments** (`Dockerfile.sway-helix:11-12`,
  `Dockerfile.ubuntu-helix:12-13`). These are documentation, not build inputs, but
  a stale comment here is worse than no comment.
- **`Dockerfile.ubuntu-helix:18` is an `ARG` default, not a `FROM`.** The digest
  still needs updating; the `ARG` name and structure must not change.
- **`Dockerfile.ubuntu-helix` also contains unrelated dates** (`2026-05-02` CI
  incident notes at lines 344 and 782) and unrelated pins (`GOOSE_COMMIT`, Rust
  1.92). None of these are pin-date comments — leave them alone.
- **`golang:1.25-bookworm`'s manifest list no longer advertises `riscv64`/`s390x`**
  in the current index. Irrelevant to Helix (amd64 + arm64 only) but worth not
  being alarmed by.
- The repo pins `golang:1.23-alpine3.21` in `Dockerfile.lint` while everything
  else is on 1.25. That is intentional (golangci-lint v1.62 compatibility) and
  explicitly out of scope.
