# Design: Refresh All Dockerfile Base Image SHA Digest Pins

## Inventory of Pins (snapshot at planning time)

Eleven Dockerfiles. Eight distinct `image:tag` references pinned by digest:

| image:tag | Current digest (to be refreshed) | Files |
| --- | --- | --- |
| `golang:1.25-bookworm` | `sha256:29e59af9...da1a6d8c` | `Dockerfile`, `Dockerfile.sandbox`, `Dockerfile.sway-helix`, `Dockerfile.ubuntu-helix`, `operator/Dockerfile` |
| `node:23-alpine` | `sha256:a34e14ef...af09456` | `Dockerfile` |
| `debian:bookworm-slim` | `sha256:4724b8cc...642182655d` | `Dockerfile` |
| `golangci/golangci-lint:v1.62-alpine` | `sha256:0f3af392...0640bc310d` | `Dockerfile.lint` |
| `golang:1.23-alpine3.21` | `sha256:4bb4be21...632b0fc6` | `Dockerfile.lint` ⚠ version anomaly |
| `node:20-slim` | `sha256:f93745c1...0c40313a55` | `Dockerfile.qwen-code-build`, `Dockerfile.qwen-build`, `scripts/sse-mcp-server/Dockerfile` |
| `ubuntu:25.04` | `sha256:27771fb7...294dac20` | `Dockerfile.sandbox` ⚠ version anomaly |
| `ubuntu:25.10` | `sha256:4a9232cc...0b810a31e` | `Dockerfile.sway-helix` (×4 stages), `Dockerfile.ubuntu-helix` (×2 stages), `Dockerfile.zed-build` |
| `golang:1.25-alpine3.22` | `sha256:2c16ac01...fad2ebfb75f884c` | `Dockerfile.demos` |
| `gcr.io/distroless/static:nonroot` | `sha256:e3f94564...22a80a39` | `operator/Dockerfile` |
| `nvidia/cuda:12.6.3-runtime-ubuntu24.04` | `sha256:92906d87...3884ba924` | `Dockerfile.ubuntu-helix` (ARG default, single-arch by design) |

## Approach

Resolve each unique `image:tag` once with `docker buildx imagetools inspect`,
then do a textual find-and-replace of the old digest with the new digest
across the matching files. Because the same digest appears in multiple
files, a per-image replace is safer and self-checks requirement (3)
automatically — every occurrence of the old hash gets the same new hash.

### Resolution command

```
docker buildx imagetools inspect <image>:<tag> --format '{{json .Manifest}}'
```

The top-level `.Manifest.digest` is the multi-arch manifest-list digest
(media type `application/vnd.oci.image.index.v1+json` or
`application/vnd.docker.distribution.manifest.list.v2+json`). That value
is what we pin.

If the inspect output's top-level mediaType is a single-arch manifest
(e.g. nvidia/cuda for arm64-less images), record the digest as the
intentional single-arch pin and note it in the PR description.

### Replacement strategy

For each `image:tag`:

1. `OLD=<current digest>` (from inventory above, re-grepped at run time).
2. `NEW=$(docker buildx imagetools inspect <image>:<tag> --format '{{.Manifest.Digest}}')`.
3. Assert `NEW` matches `^sha256:[0-9a-f]{64}$`.
4. `grep -rl "$OLD" <dockerfile-set> | xargs sed -i "s|$OLD|$NEW|g"`.
5. Re-grep `$OLD` across the repo — must return zero hits.

### Verification before commit

- `git diff` shows only digest-string changes; no tag, image, or
  whitespace changes.
- For each refreshed image:
  `docker buildx imagetools inspect <image>:<tag>@<NEW>` succeeds
  (proves the digest is reachable on the registry).
- For multi-arch images: inspect output lists both `linux/amd64` and
  `linux/arm64` platforms.

## Key Decisions

- **Single source of truth per image.** Resolve once, replace everywhere.
  Prevents drift between `Dockerfile.sway-helix` and `Dockerfile.ubuntu-helix`.
- **Manifest-list digests, never per-arch.** Helix CI builds for both
  amd64 and arm64; using a per-arch digest would silently break one platform.
- **No tag bumps.** Even if the latest `golang:1.25-bookworm` digest is
  also reachable as `golang:1.25.4-bookworm`, we pin the same tag the
  Dockerfile already uses.
- **Anomalies flagged, not fixed.** `Dockerfile.lint` at `golang:1.23` and
  `Dockerfile.sandbox` at `ubuntu:25.04` look out of step with the rest of
  the repo. Refresh their digests on the existing tag; raise the version
  drift as an open question for stakeholders.
- **CUDA ARG default counts.** `Dockerfile.ubuntu-helix` uses
  `ARG CUDA_BASE_IMAGE=...@sha256:...`. The arm64 build path overrides
  this ARG to `ubuntu:25.10`, but the amd64 default still needs refreshing.
  This pin stays single-arch (NVIDIA does not publish arm64 for this tag).

## Open Questions To Surface In PR

- `Dockerfile.lint` is on `golang:1.23-alpine3.21`; everything else is on
  `golang:1.25`. Intentional (lint pinned to older Go for compat) or stale?
- `Dockerfile.sandbox` is on `ubuntu:25.04`; the rest of the Ubuntu-based
  Dockerfiles are on `25.10`. Intentional or stale?
- `Dockerfile.demos` is on `golang:1.25-alpine3.22`; siblings use
  `golang:1.25-bookworm`. Intentional choice of alpine, or drift?

## Implementation Notes

### Resolution results (2026-05-11)

| Image | Old digest (truncated) | New digest (truncated) | Changed? | Platforms |
| --- | --- | --- | --- | --- |
| `golang:1.25-bookworm` | `29e59af9...da1a6d8c` | `e3a54b77...d409547` | **YES** | amd64, arm64/v8, +6 |
| `node:23-alpine` | `a34e14ef...af09456` | `a34e14ef...af09456` | no | amd64, arm64/v8, +3 |
| `debian:bookworm-slim` | `4724b8cc...642182655d` | `67b30a61...8493d3` | **YES** | amd64, arm64/v8, +6 |
| `golangci/golangci-lint:v1.62-alpine` | `0f3af392...0640bc310d` | `0f3af392...0640bc310d` | no | amd64, arm64 |
| `golang:1.23-alpine3.21` | `4bb4be21...632b0fc6` | `4bb4be21...632b0fc6` | no | amd64, arm64/v8, +6 |
| `node:20-slim` | `f93745c1...0c40313a55` | `2cf067cf...febfc0` | **YES** | amd64, arm64/v8, +3 |
| `ubuntu:25.04` | `27771fb7...294dac20` | `27771fb7...294dac20` | no | amd64, arm64/v8, +4 |
| `ubuntu:25.10` | `4a9232cc...0b810a31e` | `4a9232cc...0b810a31e` | no | amd64, arm64/v8, +4 |
| `golang:1.25-alpine3.22` | `2c16ac01...fad2ebfb75f884c` | `26b4d711...e17cf95` | **YES** | amd64, arm64/v8, +6 |
| `gcr.io/distroless/static:nonroot` | `e3f94564...22a80a39` | `e3f94564...22a80a39` | no | amd64, arm64/v8, +3 |
| `nvidia/cuda:12.6.3-runtime-ubuntu24.04` | `92906d87...3884ba924` | `92906d87...3884ba924` | no | amd64, **arm64** |

Net effect: **4 images need a digest update** (golang 1.25-bookworm,
debian bookworm-slim, node 20-slim, golang 1.25-alpine3.22). The other
seven were already pinned to the current upstream manifest list.

### Discovery: NVIDIA CUDA does have arm64

The design assumed `nvidia/cuda:12.6.3-runtime-ubuntu24.04` was an
amd64-only image. Actual inspection shows the manifest list publishes
**both** `linux/amd64` and `linux/arm64`. We still leave the Dockerfile
structure alone — the `CUDA_BASE_IMAGE` ARG default + arm64 pipeline
override is existing convention — but we will note in the PR that the
underlying image is in fact multi-arch, in case the team wants to
simplify the build later.

### Replacement mechanics actually used

For each of the four changing images we ran `grep -rl OLD ./Dockerfile*
./operator ./scripts | xargs sed -i "s|OLD|NEW|g"` from the helix repo
root, then re-grepped each old digest to prove zero remaining hits.
