# Design: Refresh Pinned Base Image Digests Across All Dockerfiles

## Approach

Pure text edit. No scripts, no tooling, no automation added — this is a periodic
manual maintenance pass and a generator would be over-engineering for ~20 lines.

Work image-first, not file-first:

1. Build the set of unique `image:tag` values across all pin sites (done — see table).
2. For each, resolve the multi-arch index digest once with
   `docker buildx imagetools inspect <image:tag>`.
3. Apply that one digest to every occurrence of that image:tag with a
   scoped `sed`/replace-all, so all copies move together by construction (AC-7 is then
   satisfied structurally rather than by careful manual repetition).
4. Re-verify each written digest by inspecting it back.

## Key Decisions

**Why one-digest-per-image-tag, applied repo-wide.** `golang:1.25-bookworm` appears in
5 files (6 `FROM` lines + 2 comment lines) and `ubuntu:25.10` in 3 files (9 `FROM`
lines + 2 comment lines). Editing file-by-file is how a stale copy survives. Resolving
once and replacing all occurrences of the *old* digest string makes divergence
impossible.

**Why the index digest, never the platform digest.** `docker buildx imagetools inspect`
prints the top-level `Digest:` (the index) plus a per-platform `Manifests:` list. Pinning
a per-platform entry makes the other architecture fail or silently build the wrong arch —
which is exactly the pre-existing defect found in `.drone.yml` line 2012. Sanity check:
an index has `MediaType: application/vnd.oci.image.index.v1+json` or
`application/vnd.docker.distribution.manifest.list.v2+json`; a platform digest has
`application/vnd.oci.image.manifest.v1+json` or `…manifest.v2+json`.

**Why the ledger comments count as code.** `Dockerfile.sway-helix` and
`Dockerfile.ubuntu-helix` carry a header block listing each base image and its digest.
It is the documented source of truth for the next person doing this task; leaving it
stale is worse than not having it.

**Why the internal image is a no-op.** `registry.helixml.tech/helix/controlplane:2.11.52`
is a released version tag — refreshing its digest is meaningless unless the tag was
re-pushed. Verified unchanged.

## Environment Findings (verified 2026-08-03 from this machine)

- `docker` + `docker buildx v0.35.0` are on PATH; Docker Hub, `gcr.io`, and
  **`registry.helixml.tech` are all reachable and inspectable without extra auth**.
  Open Question 2 from the task brief is answered: no blocker on the internal registry.
- `.github/workflows/` contains only `codeql.yml` and `stainless_action.yml`; neither
  pins a base image digest. There is no `.buildkite/`. CI is Drone (`.drone.yml`).
- `Dockerfile.ubuntu-helix:18` is the one ARG-based digest reference
  (`ARG CUDA_BASE_IMAGE=nvidia/cuda:…@sha256:…`), answering brief Open Question 3.
- The `uv`/`python:3.11-slim` images mentioned in
  `design/2026-03-02-golden-cache-miss-investigation.md` no longer exist in any
  Dockerfile — that doc describes an older tree. Do not go hunting for them.

## Resolved Digests (live `imagetools inspect`, 2026-08-03 — re-resolve at implementation time)

All "new" values below were confirmed to be index/manifest-list media types carrying
both `linux/amd64` and `linux/arm64(/v8)`.

| image:tag | current pin | current index digest | action |
|---|---|---|---|
| `golang:1.25-bookworm` | `e3a54b77…9547` | `ea341baa9bd5ba6784f6d7161ace70544349a6242d54d34a0fbfd2c4d51c9d58` | **UPDATE** — 6 FROM + 2 comments |
| `ubuntu:25.10` | `4a9232cc…a31e` | `7cc5e35f6567ee8c66d2abb4aab0fd866669e6207c237c3a8f0947a5c7f17092` | **UPDATE** — 9 FROM + 2 comments |
| `golang:1.25-alpine3.22` | `26b4d711…cf95` | `65b4400aee0927412e9ed791a11893273a49d55df24841f7599660fb80dae464` | **UPDATE** — `Dockerfile.demos:1` |
| `debian:bookworm-slim` | `67b30a61…93d3` | `7b140f374b289a7c2befc338f42ebe6441b7ea838a042bbd5acbfca6ec875818` | **UPDATE** — `Dockerfile:111` |
| `gcr.io/distroless/static:nonroot` | `e3f94564…0a39` | `f7f8f729987ad0fdf6b05eeeae94b26e6a0f613bdf46feea7fc40f7bd72953e6` | **UPDATE** — `operator/Dockerfile:28` |
| `node:20-slim` | `2cf067cf…bfc0` | identical | no-op |
| `node:23-alpine` | `a34e14ef…9456` | identical | no-op |
| `golang:1.23-alpine3.21` | `4bb4be21…0fc6` | identical | no-op |
| `golangci/golangci-lint:v1.62-alpine` | `0f3af392…310d` | identical | no-op |
| `ubuntu:25.04` | `27771fb7…ac20` | identical | no-op |
| `nvidia/cuda:12.6.3-runtime-ubuntu24.04` | `92906d87…a924` | identical | no-op (`Dockerfile.ubuntu-helix:18`) |
| `registry.helixml.tech/helix/controlplane:2.11.52` | `58705ba0…ca6a` | identical | no-op |

So ~5 distinct digests move, touching ~22 lines across 9 files (+ `.drone.yml`).

### `.drone.yml` special case

| line | value | media type |
|---|---|---|
| 1671 (amd64 job) | `nvidia/cuda:…@sha256:2c819353…` | `manifest.v2+json` → **amd64-only** |
| 2012 (arm64 job) | `ubuntu:25.10@sha256:91e7ac4c…` | `manifest.v1+json`, **linux/amd64** child of the old `4a9232cc` index |

Line 2012 is a latent defect: the arm64 pipeline is handed an amd64 base image digest.
Per AC-6 the default plan is to replace both with the index digests
(`92906d87…a924` for CUDA — unchanged upstream — and `7cc5e35f…7092` for ubuntu:25.10),
which fixes the arm64 job and keeps the ARG default and the pipeline override consistent.
Flag this in the PR description; if the user says the arch-specific pinning was
deliberate, the fallback is to substitute the corresponding platform child digest of the
*new* index instead.

## Verification Plan

```bash
cd /home/retro/work/helix

# 1. Resolve (repeat per image) — take the top-level Digest:, confirm MediaType is an index
docker buildx imagetools inspect golang:1.25-bookworm | head -5

# 2. Apply repo-wide by old→new digest string
grep -rln '<OLD_DIGEST>' Dockerfile* operator/ scripts/ .drone.yml \
  | xargs sed -i 's/<OLD_DIGEST>/<NEW_DIGEST>/g'

# 3. Consistency — one digest per image:tag
grep -rhno '[a-z0-9./-]*:[a-zA-Z0-9._-]*@sha256:[0-9a-f]\{64\}' \
  Dockerfile* operator/Dockerfile scripts/sse-mcp-server/Dockerfile .drone.yml \
  | sed 's/^[0-9]*://' | sort -u

# 4. Typo + multi-arch re-check on every distinct written pin
docker buildx imagetools inspect <image>@sha256:<written> | head -3   # must resolve, must be an index

# 5. Build check (base resolution at minimum)
docker buildx build --platform linux/amd64,linux/arm64 -f operator/Dockerfile --load=false -o type=cacheonly .
docker buildx build --platform linux/amd64,linux/arm64 -f Dockerfile.lint    -o type=cacheonly .
```

Step 4 is the typo guard: a mistyped hex digest cannot resolve, so a successful
`imagetools inspect` of the *written* string proves the string is correct.

## Notes for Future Agents

- Pins in this repo exist for **BuildKit layer-cache stability**, not just supply-chain
  hygiene — see `design/2026-02-21-shared-buildkit-cache.md` and
  `design/2026-03-02-golden-cache-miss-investigation.md`. Expect the first CI build after
  this change to be a full cold rebuild of every touched stage; that is normal, not a
  regression.
- The convention when bumping: also update the `(YYYY-MM-DD)` date stamp in the
  "BASE IMAGE DIGESTS: Pinned for stable layer caching" header blocks.
- `Dockerfile.sway-helix.dockerignore` / `Dockerfile.ubuntu-helix.dockerignore` contain
  no digests — ignore them despite matching a `Dockerfile*` glob.
