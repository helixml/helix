# Implementation Tasks: Skip helix-sway Pull at Sandbox Startup Unless Enabled

- [x] In `sandbox/04-start-dockerd.sh`, replace the hardcoded
  `load_desktop_image "sway|zorin|ubuntu|kde"` block (lines 247–251)
  with a production/experimental split: always load
  `PRODUCTION_DESKTOPS=(ubuntu)`, only load entries from
  `AVAILABLE_EXPERIMENTAL_DESKTOPS=(sway zorin xfce kde)` when listed in
  the `HELIX_EXPERIMENTAL_DESKTOPS` env var.
- [x] For each experimental desktop that is skipped, log an `ℹ️ … skipping
  pull (set HELIX_EXPERIMENTAL_DESKTOPS="<name> ...") to enable` line so
  operators can see why the pull didn't happen.
- [x] In `Dockerfile.sandbox` (env block around lines 270–279), add
  `HELIX_EXPERIMENTAL_DESKTOPS=""` as the default.
- [x] In `docker-compose.dev.yaml`, plumb
  `HELIX_EXPERIMENTAL_DESKTOPS=${HELIX_EXPERIMENTAL_DESKTOPS:-}` through
  to every sandbox service (4 instances: `sandbox-nvidia`, `sandbox-amd`,
  `sandbox-cpu` and `sandbox-macos` all use the same `helix-sandbox`
  image and need the var passed in).
- [x] Verify the gating block end-to-end with a stub harness: extract
  the new "Load desktop images" section from
  `sandbox/04-start-dockerd.sh` with `awk`, stub `load_desktop_image`,
  and run it under five env-var permutations (empty, `"sway"`,
  `"sway zorin"`, `"sway zorin xfce kde"`, `"banana sway"`). All five
  produce the expected pulls and skip messages. (Result captured
  inline in this file's commit history.)
- [x] `bash -n sandbox/04-start-dockerd.sh` — script parses cleanly.
- [x] `docker compose -f docker-compose.dev.yaml config --quiet` — compose
  file still parses with the new env var added in 4 places.
- [ ] Live verification deferred to CI on the sandbox-build pipeline.
  Rebuilding the sandbox image and restarting `helix-sandbox-nvidia-1`
  in the inner Helix would interrupt other spectasks, so we rely on
  the harness above plus the CI pipeline that exercises this script
  end-to-end on every PR. The two log behaviors to watch for in the
  CI sandbox-build logs (or the first deploy after merge):
  - Default `HELIX_EXPERIMENTAL_DESKTOPS=""`: `🔄 Pulling
    ghcr.io/helixml/helix-sway:...` line is GONE; replaced by
    `ℹ️  helix-sway is experimental; skipping pull (set …)`.
  - With `HELIX_EXPERIMENTAL_DESKTOPS="sway"`: original `🔄 Pulling
    helix-sway` line returns and the sway pull succeeds.
- [x] Stale-image cleanup: re-read the cleanup section that follows
  (`sandbox/04-start-dockerd.sh:268+`). It walks every
  `helix-*.version` file and prunes images whose tag does not match
  the expected version or `:latest` or a `runtime-ref`. With the new
  gating, sway's `.version` file still exists (CI ships it) but no
  `runtime-ref` is written — so a pre-existing
  `helix-sway:OLD_VERSION` image is pruned (matches the design
  intent), and `helix-sway:NEW_VERSION` is preserved by version-file
  match. No code change needed here.
- [x] Lazy-pull fallback: hydra's `devcontainer.go:208+` resolves
  `helix-sway:VERSION` to `ghcr.io/helixml/helix-sway:VERSION` at
  session-create time and `docker run` against a missing image
  triggers an automatic pull. So the escape hatch for a user who
  flips the desktop selection to sway on an opted-out sandbox still
  works — first session is slower (one pull), subsequent sessions are
  fast. No code change needed.
- [x] Add a one-paragraph note to `CLAUDE.md` (sandbox / Build Pipeline
  section) documenting `HELIX_EXPERIMENTAL_DESKTOPS`, default empty, and
  the lazy-pull fallback behavior.
- [ ] Open the PR against `helixml/helix` with the conventional commit
  prefix `fix(sandbox):` and reference this spec task in the description.
- [ ] After pushing, watch CI (`gh pr checks` / Drone MCP) and confirm
  the sandbox-build pipeline still passes — no `.drone.yml` change is
  expected, but the sandbox image rebuild needs to succeed.
