# Design

## Approach

This is a mechanical, low-risk refactor: walk every Dockerfile, find each
`FROM <image>:<tag>@sha256:<old>` line, look up the **current** multi-arch
manifest list digest for that exact `<image>:<tag>`, and rewrite the SHA in
place. Tags never change. Floating images are reported, never pinned.

No new tooling, scripts, or abstractions. Edit the Dockerfiles directly.

## Inventory of Pinned Lines (as of discovery)

These are the **only** lines that should be modified by this task. Nine pins
across six files; two image tags appear more than once and must end up with
matching digests.

| # | File | Line | Image tag |
|---|------|------|-----------|
| 1 | `Dockerfile.runner` | 22 | `ghcr.io/astral-sh/uv:0.5.4` |
| 2 | `Dockerfile.runner` | 28 | `golang:1.25-bookworm` |
| 3 | `Dockerfile.runner` | 61 | `alpine:3.21` |
| 4 | `Dockerfile.lint`   | 3  | `golangci/golangci-lint:v1.62-alpine` |
| 5 | `Dockerfile.lint`   | 5  | `golang:1.23-alpine3.21` |
| 6 | `operator/Dockerfile` | 2 | `golang:1.25-bookworm` |
| 7 | `operator/Dockerfile` | 28 | `gcr.io/distroless/static:nonroot` |
| 8 | `scripts/sse-mcp-server/Dockerfile` | 1 | `node:20-slim` |
| 9 | `Dockerfile.demos`  | 1  | `golang:1.25-alpine3.22` |

**Cross-file consistency:** `golang:1.25-bookworm` appears at #2 and #6 — both
must receive the **same** new digest.

There is also a **commented-out** pinned reference at `Dockerfile.runner:10`
(`ghcr.io/astral-sh/uv:0.5.4@sha256:4993...`). Per spec we refresh it too so
it doesn't drift, but it is non-load-bearing — flag if uncertain.

## Date-Comment Updates

Comments that record a pin date and the implementation agent should bump to
`2026-04-27`:

- `Dockerfile.ubuntu-helix:10` `BASE IMAGE DIGESTS: Pinned for stable layer caching (2026-03-30).`
- `Dockerfile.ubuntu-helix:25` `(golang:1.25-bookworm as of 2026-03-30)`
- `Dockerfile.sway-helix:9`    `BASE IMAGE DIGESTS: Pinned for stable layer caching (2026-03-30).`
- `Dockerfile.sway-helix:37`   `(golang:1.25-bookworm as of 2026-03-30)`

**Discrepancy to flag for human review:** The `BASE IMAGE DIGESTS` comment
blocks in `Dockerfile.ubuntu-helix` and `Dockerfile.sway-helix` document
digests for `ubuntu:25.10` and `golang:1.25-bookworm`, but the actual `FROM`
lines in those files do **not** carry `@sha256:` pins. The comments either
lie or document an aspirational pin policy that was never applied. Refresh
the digest values listed in the comments to match what would be the current
multi-arch digests, but do **not** add `@sha256:` to the FROM lines (per
"don't pin without confirmation"). Note the inconsistency in the report.

## Floating Images (Flag, Do Not Pin)

These appear in `FROM` lines without `@sha256:`. Spec says: report them, do
not auto-pin. List in the PR / report:

- `Dockerfile`: `golang:1.25-bookworm`, `node:23-alpine`, `debian:bookworm-slim`
- `Dockerfile.sandbox`: `golang:1.25-bookworm`, `ubuntu:25.04`
- `Dockerfile.sway-helix`: `ubuntu:25.10` (×4), `golang:1.25-bookworm`
- `Dockerfile.ubuntu-helix`: `golang:1.25-bookworm`, `ubuntu:25.10` (×2)
- `Dockerfile.zed-build`: `ubuntu:25.10`
- `Dockerfile.qwen-build`: `node:20-slim`
- `Dockerfile.qwen-code-build`: `node:20-slim`

Note: several of these have header comments like `# Pin to specific digest
for stable layer caching` but no actual pin. Same flag as above.

## Intentionally Dynamic — Do NOT Touch

- `Dockerfile.runner:102` — `ghcr.io/helixml/runner-base:${TAG}` (the inline
  comment explicitly explains why it must stay unpinned).
- `Dockerfile.ubuntu-helix:165` — `${CUDA_BASE_IMAGE}` (set by `--build-arg`).
  The default value `nvidia/cuda:12.6.3-runtime-ubuntu24.04` is itself
  unpinned — flag for human review but do not modify.
- `FROM scratch`, and any `FROM <stage-name>` (`api-base`, `ui-base`,
  `builder-env`, `base`) — N/A, internal stages.

## Verifying Multi-Arch Manifest Lists

Tooling: `docker buildx imagetools inspect <image>:<tag>` is available on the
implementation host (verified during planning).

For each tag, the workflow is:

1. `docker buildx imagetools inspect <image>:<tag> --format '{{json .Manifest}}'`
   — confirm `mediaType` is `application/vnd.oci.image.index.v1+json` or
   `application/vnd.docker.distribution.manifest.list.v2+json` (i.e., a
   manifest **list/index**, not a single-arch manifest).
2. Confirm the platforms list contains **both** `linux/amd64` and
   `linux/arm64`. Other platforms (e.g. `s390x`, `ppc64le`) may be present
   and are fine; what matters is that both required platforms are covered.
3. Read the new digest from `.Manifest.digest`.
4. Validate the digest format: `sha256:` prefix + 64 lowercase hex chars.
5. Replace the old SHA in the FROM line with the new one. Tag is unchanged.

If a tag's top-level digest is not a manifest list, or the list is missing
either `amd64` or `arm64`, **stop**. Do not commit that update — flag for
human review.

## Risks & Mitigations

- **Single-arch digest accidentally pinned** → would silently break the other
  arch. Mitigation: explicit manifest-list verification step above; refuse
  to commit if not satisfied.
- **Typo in 64-char hex** → registry pull fails with cryptic error.
  Mitigation: regex-validate every new SHA before writing the file.
- **Same tag, different digests across files** → cache invalidation, drift.
  Mitigation: deduplicate by `image:tag` and apply the same fetched digest
  everywhere.
- **Upstream registry rate limits** (Docker Hub, ghcr.io, gcr.io) → may need
  authenticated pulls. Mitigation: keep lookups to one per unique
  `image:tag`, not one per occurrence (only ~7 unique tags here).
- **Commented-out pin at `Dockerfile.runner:10`** → not load-bearing, but
  refreshing it keeps the file consistent. Low risk either way.

## Notes for Future Agents

Patterns observed in this repo, useful next time:

- Helix uses two pin styles: **active** (`@sha256:` directly on the FROM
  line, in `Dockerfile.runner`, `Dockerfile.lint`, `Dockerfile.demos`,
  `operator/Dockerfile`, `scripts/sse-mcp-server/Dockerfile`) and
  **comment-only documentation** (`BASE IMAGE DIGESTS:` header block in
  `Dockerfile.ubuntu-helix` and `Dockerfile.sway-helix`, with no actual pin
  on the FROM line). The latter is inconsistent — flag during refreshes.
- The repo must build for both `linux/amd64` and `linux/arm64`. Any pin must
  be a multi-arch manifest list; this is non-negotiable.
- A useful inventory command:
  `grep -rn '^FROM .*@sha256:' --include='Dockerfile*'`
- `docker buildx imagetools inspect` is the right tool for digest +
  manifest-list verification (no need for skopeo/crane/regctl, none of which
  are installed here).
