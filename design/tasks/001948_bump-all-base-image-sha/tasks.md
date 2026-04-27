# Implementation Tasks

- [ ] Re-confirm the inventory: `grep -rn '^FROM .*@sha256:' --include='Dockerfile*' /home/retro/work/helix` matches the 9 lines in `design.md` (plus the commented-out line at `Dockerfile.runner:10`). If the inventory has drifted, update plan first.
- [ ] Build the unique-tag set from the inventory (expect ~7 unique `image:tag` values).
- [ ] For each unique tag, run `docker buildx imagetools inspect <image>:<tag>` and capture: top-level digest, manifest mediaType, list of platforms.
- [ ] Verify each result is a manifest **list/index** (`...image.index.v1+json` or `...manifest.list.v2+json`) AND covers both `linux/amd64` and `linux/arm64`. If not, stop on that tag and add it to the human-review flag list — do not commit a digest for it.
- [ ] Validate every new digest against the regex `^sha256:[0-9a-f]{64}$`. Reject and re-fetch on any mismatch.
- [ ] Update each pinned `FROM` line in place — replace only the SHA suffix. Tag, image name, alias (`AS xxx`), and any trailing args must be byte-identical.
- [ ] Apply identical digests for repeated tags: `golang:1.25-bookworm` at `Dockerfile.runner:28` and `operator/Dockerfile:2` must match.
- [ ] Refresh the commented-out pin at `Dockerfile.runner:10` (`ghcr.io/astral-sh/uv:0.5.4`) to the same digest used at line 22.
- [ ] Update pin-date comments to `2026-04-27` in `Dockerfile.ubuntu-helix` (lines 10, 25) and `Dockerfile.sway-helix` (lines 9, 37). Also refresh the digest values shown in the comment blocks (`ubuntu:25.10`, `golang:1.25-bookworm`) to match what `imagetools inspect` returns today, even though the FROM lines in those files are not actually pinned.
- [ ] Run `git diff` on the helix repo and visually confirm: only SHA-hex-suffixes and the listed comment lines have changed. No tag, no logic, no whitespace drift, no stage names.
- [ ] Run `grep -rn '^FROM .*@sha256:' --include='Dockerfile*' /home/retro/work/helix` again and confirm every digest matches the freshly-fetched values from the verification step.
- [ ] Sanity-check: attempt `docker buildx build --platform linux/amd64,linux/arm64 --pull --no-cache --target <first-stage> -f <each touched Dockerfile> .` for at least one stage per touched Dockerfile, to confirm the registry resolves the new pins on both arches. If full multi-arch build is too heavy, at minimum run `docker pull --platform linux/arm64 <image>@<new-digest>` and `--platform linux/amd64` for each updated pin.
- [ ] Write a short report listing: (a) every digest update with file:line and old → new SHA, (b) any tag that failed multi-arch verification (human review needed), (c) the floating-image inventory from `design.md` (note-only, not modified), (d) the `Dockerfile.ubuntu-helix` / `Dockerfile.sway-helix` comment-vs-FROM-line discrepancy.
- [ ] Open a PR in the helix repo with the diff and the report. Title: routine base-image digest refresh, dated 2026-04-27.
