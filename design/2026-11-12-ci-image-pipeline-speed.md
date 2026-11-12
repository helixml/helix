# CI image pipeline speed and tag failures (2026-11-12)

Data: the last 250 Drone builds (5286–5499), every main push and tag build, step
timings from `/builds/<n>` and step logs. Commands are in "Reproducing" below.

## 1. The slow manifest steps are a broken GHCR token, and the failure is hidden

Healthy main builds (5286–5421) take seconds for each manifest step and 1–4 min
for each publish step:

| step | healthy (5286–5421) | since 5425 (2026-11-12 ~03:00) |
|---|---|---|
| manifest-controlplane/manifest | 6–23 s | 1357–1369 s |
| manifest-sandbox/manifest-helix-ubuntu | 6–14 s | 1357–1371 s |
| build-controlplane-amd64/publish-image | 45–233 s | 941–1134 s |
| build-sandbox-amd64/build-desktops | 9–380 s | 923–991 s |
| build-sandbox-amd64/push-sandbox | 21–137 s | 916–1862 s |

The 8–11 min manifest averages in the brief come entirely from this window.
Every log since 5425 shows:

```
Error response from daemon: Get "https://ghcr.io/v2/": denied: denied
[ghcr-manifest] attempt 1/6 failed (rc=0), retrying in 30s: ghcr_login
...
No such manifest: ghcr.io/helixml/controlplane:53c3be4
GHCR manifest pushed (amd64-only): ghcr.io/helixml/controlplane:53c3be4
```

- The `github_token` Drone secret is being rejected by ghcr.io. A rejected
  credential cannot recover, but the script retried login, then `manifest
  create`, then `manifest push` 6 times each: 3 × 450 s = 1350 s per manifest
  call, ~900 s per `ghcr-push.sh` call.
- `retry()` read `rc=$?` after `if "$@"; then …; fi`. A failed `if` without an
  `else` sets `$?` to 0, so `rc` was always 0 and an exhausted retry returned
  success. The scripts' documented "exit 1 so the build fails" never happened.
- Impact: **2.12.58 is not on ghcr.io** (`controlplane`, `helix-sandbox`,
  `helix-ubuntu` all 404; 2.12.57 returns 200), yet its publish steps were
  green. install.sh and the helm charts pull from ghcr.io. Main commit images
  are missing there too, and the sandbox's `helix-ubuntu.ref` points at
  `ghcr.io/helixml/helix-ubuntu:<sha>`.

**Action needed (not code): rotate the `github_token` Drone secret** (PAT with
`write:packages` for the helixml org), then re-publish 2.12.58 to GHCR.
With this PR's fix, every main and tag build fails at the first GHCR call
until the secret is rotated. Before, they passed without publishing.

## 2. Why tag builds fail (13 of 18 in this window)

`release-rollback` deletes the GitHub release **and the git tag** whenever a
core stage fails. The tag is then re-cut, so one release attempt shows up as
several failed builds. 2.12.55 alone was cut 8 times.

| build | tag | failed step | cause |
|---|---|---|---|
| 5341 | 2.12.53 | deploy-prod runner steps | `ssh: Could not resolve hostname code.helix.ml` (control plane deployed fine) |
| 5376 | 2.12.54 | zed-e2e-live-claude-latest | e2e flake: `thread_created arrived 6.2s after chat_message (expected >= 8s)` |
| 5382, 5387, 5404 | 2.12.54/55 | build-macos-dmg/provision-vm | `curl: (28) Failed to connect to cloud-images.ubuntu.com port 443 after 150019 ms` from flight-arm64 |
| 5420 | 2.12.56 | build-macos-dmg/provision-vm | R2 endpoint unreachable from flight-arm64 during VM upload |
| 5400 | 2.12.55 | clone | `couldn't find remote ref refs/tags/2.12.55`: the tag was deleted by a rollback while this run was queued |
| 5401 | 2.12.55 | build-frontend + rollback | same race |
| 5406, 5409, 5412 | 2.12.55 | build-frontend | vitest tests hitting the 5 s timeout on node03 (a different set each time: OrgApiKeys, NewBotDialog, EditProviderModelsDialog…) |
| 5415 | 2.12.55 | build-desktops ×2, publish-image | registry.helixml.tech (R2 backend) returned `500` / `unexpected status from HEAD` to amd64 and arm64 pushes at the same moment |
| 5440 | 2.12.58 | deploy-prod/clone | `fatal: pack has bad object … inflate returned -3`. Drone clone has 0 retries. |

Grouped: 4 macOS runner network failures, 3 flaky frontend tests, 3 caused by
deleting and re-cutting the tag, 2 registry/R2 blips, 2 deploy-step
infrastructure failures, 1 e2e flake. None came from a code change in the
release.

## 3. What the image builds cost when healthy

On a warm runner-local cache, the image work is small. The per-tag DMG and the
zed e2e steps (~3 min each) take longer. The real outliers are full cache misses:

- 5332 (tag) build-desktops 2083 s, 5296/5298 ~800–936 s: the whole
  `Dockerfile.ubuntu-helix` rebuilt (base apt layer, goose `cargo build` 8m22s,
  Go binaries 7 min). The cache is the runner's local Docker daemon, and amd64
  builds alternate between node03 and luke-node04. A change built on one node
  is a cold build on the other. A shared registry cache
  (`--cache-to/--cache-from type=registry`, needs a buildx docker-container
  builder) would fix this. NOT measured yet; it is the next step after the GHCR
  token is fixed.

## Changes in this PR

- `scripts/lib/image-publish.sh`: shared `retry` (correct exit code) and
  `ghcr_login`, which fails immediately when ghcr.io rejects the token. A bad
  token now costs seconds instead of 7.5–22 min, and the build goes red.
- `scripts/ghcr-push.sh`, `scripts/ghcr-manifest.sh`: use the shared helpers.
  Dropped the stale "(amd64-only)" wording.
- `scripts/publish-image.sh`: retried push to registry.helixml.tech, then the
  GHCR mirror. `.drone.yml` now calls it at all 8 push sites.
- `scripts/test-image-publish.sh`: fake-docker tests, run under dash and busybox
  ash.
- `for-mac/scripts/provision-vm-light.sh`: the pinned, checksummed Ubuntu cloud
  image is cached in `$(dirname VM_DIR)/cache`, which the CI step's
  `rm -rf …/ci-provision` does not delete. Downloads use `curl -f --retry 5`.
  Tag builds no longer need cloud-images.ubuntu.com.

## `.drone.yml` changes

1. Each `docker push X` + `scripts/ghcr-push.sh X` pair is now
   `scripts/publish-image.sh X` (8 sites).
2. `shell-script-tests` runs `sh scripts/test-image-publish.sh`.
3. `deploy-prod` clones with `retries: 3`.

These merge without conflict with ci/parallel-pipeline-graph (checked with
`git merge-tree`).

## Release rollback keeps the git tag

`scripts/release-rollback.sh` no longer deletes the tag. To retry a release
that failed on a flaky step, run `gh release create <tag> --verify-tag
--generate-notes`, then restart the Drone build (see CLAUDE.md, Releases).

## Frontend test failures

Of 51 failed `build-frontend` runs, 47 ran on node03, which was degraded
(yarn and tsc took 290 s instead of 38 s; the summed vitest collect time was
10 750 s instead of 205 s). The other 4 were real flakes: two "Worker exited
unexpectedly" crashes, the RobustPromptInput grace-window race, and a
HelixOrgBotDetail timeout. On 2026-11-12, luke-node04 (16 cores, runner
capacity 16) reached a load average of 91, and build-frontend took
1000–1300 s. https://github.com/helixml/helix/pull/3571 handles suite speed and
the known flakes. The runner's capacity relative to its core count is an infra
issue: capacity 16 on 16 cores lets concurrent builds starve each other.

## Reproducing

```bash
export DRONE_SERVER=https://drone.lukemarsden.net DRONE_TOKEN=…
curl -s -H "Authorization: Bearer $DRONE_TOKEN" "$DRONE_SERVER/api/repos/helixml/helix/builds/5459" \
  | jq '.stages[]|select(.name=="manifest-controlplane")|.steps[]|{name,d:(.stopped-.started)}'
# GHCR presence of a release
tok=$(curl -s "https://ghcr.io/token?scope=repository:helixml/controlplane:pull" | jq -r .token)
curl -s -o /dev/null -w '%{http_code}\n' -H "Authorization: Bearer $tok" \
  -H 'Accept: application/vnd.oci.image.index.v1+json' https://ghcr.io/v2/helixml/controlplane/manifests/2.12.58
sh scripts/test-image-publish.sh
```
