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
- [~] In `Dockerfile.sandbox` (env block around lines 270–279), add
  `HELIX_EXPERIMENTAL_DESKTOPS=""` as the default.
- [ ] In `docker-compose.dev.yaml`, plumb
  `HELIX_EXPERIMENTAL_DESKTOPS=${HELIX_EXPERIMENTAL_DESKTOPS:-}` through
  to every sandbox service (`sandbox-nvidia`, plus any sibling services
  like `sandbox` / `sandbox-macos` that use the same image).
- [ ] Run `./stack build-sandbox` and start a fresh sandbox container with
  no `HELIX_EXPERIMENTAL_DESKTOPS` set. Confirm `helix-ubuntu` pulls as
  today and `helix-sway` does NOT pull (info message instead).
- [ ] Restart the sandbox with `HELIX_EXPERIMENTAL_DESKTOPS="sway"` and
  confirm `helix-sway` pulls exactly as it did before the change.
- [ ] On a sandbox that has a stale `helix-sway:OLD` image from before
  the fix, restart with default config and confirm the existing cleanup
  pass prunes the stale image (no multi-GB orphan left).
- [ ] On an opted-out sandbox, launch a sway desktop session via hydra
  and confirm Docker pulls `helix-sway:VERSION` lazily at `docker run`
  time so the escape hatch still works.
- [ ] Add a one-paragraph note to `CLAUDE.md` (sandbox / Build Pipeline
  section) documenting `HELIX_EXPERIMENTAL_DESKTOPS`, default empty, and
  pointing at this design doc.
- [ ] Open the PR against `helixml/helix` with the conventional commit
  prefix `fix(sandbox):` and reference this spec task in the description.
- [ ] After pushing, watch CI (`gh pr checks` / Drone MCP) and confirm
  the sandbox-build pipeline still passes — no `.drone.yml` change is
  expected, but the sandbox image rebuild needs to succeed.
