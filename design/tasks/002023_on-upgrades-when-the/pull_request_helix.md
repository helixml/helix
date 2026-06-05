# fix(sandbox): only pull experimental desktop images when enabled

## Summary

On every sandbox upgrade, `sandbox/04-start-dockerd.sh` was eagerly
pulling **every** desktop image referenced under `/opt/images/*.version`
— including `helix-sway`, an experimental image that production isn't
even using. Each upgrade burned multi-GB of bandwidth and disk on an
image no session would touch:

```
🔄 Pulling ghcr.io/helixml/helix-sway:2.11.4-linux-amd64 from registry...
2.11.4-linux-amd64: Pulling from helixml/helix-sway
0962d14975c5: Already exists
…
```

The build-side `stack` script already classified desktops as
`PRODUCTION_DESKTOPS=(ubuntu)` vs
`AVAILABLE_EXPERIMENTAL_DESKTOPS=(sway zorin xfce kde)`, but that split
wasn't honoured at runtime. This PR mirrors it inside the sandbox
startup script, gated by a new `HELIX_EXPERIMENTAL_DESKTOPS` env var
(default empty).

After this change:

- Default upgrade pulls only `helix-ubuntu`. Each experimental desktop
  logs a single `ℹ️ helix-<name> is experimental; skipping pull (set
  HELIX_EXPERIMENTAL_DESKTOPS="<name> ...") to enable` line.
- Operators / developers who use sway can opt in with
  `HELIX_EXPERIMENTAL_DESKTOPS="sway"`; behaviour is then identical to
  before this PR.
- A user who flips a session to a non-pre-pulled desktop still works —
  hydra resolves the image name to its `ghcr.io/helixml/...` ref and
  Docker pulls on `docker run`. First session is slower, subsequent
  sessions cached.

CI still builds and pushes `helix-sway` to the registry; nothing about
the build pipeline or distribution changes.

## Changes

- `sandbox/04-start-dockerd.sh` — replace the hard-coded
  `load_desktop_image "sway|zorin|ubuntu|kde"` block with a
  production/experimental split keyed off `HELIX_EXPERIMENTAL_DESKTOPS`.
  Skipped desktops log a single info line so operators can see the
  opt-in path.
- `Dockerfile.sandbox` — declare `HELIX_EXPERIMENTAL_DESKTOPS=""` as
  the env default.
- `docker-compose.dev.yaml` — pass `HELIX_EXPERIMENTAL_DESKTOPS` through
  to all four sandbox services (`sandbox-nvidia`, `sandbox-amd`,
  `sandbox-cpu`, `sandbox-macos`).
- `CLAUDE.md` — short note in the Build Pipeline section explaining the
  new env var and the lazy-pull fallback.

## Test plan

- [x] `bash -n sandbox/04-start-dockerd.sh` — script parses cleanly.
- [x] Stub-`load_desktop_image` harness across five env-var
  permutations (empty, `"sway"`, `"sway zorin"`, `"sway zorin xfce
  kde"`, `"banana sway"` to test unknown-name handling). All five
  produce the expected pull/skip output.
- [x] `docker compose -f docker-compose.dev.yaml config --quiet` — the
  compose file still parses with the new env var added in 4 places.
- [ ] CI sandbox-build pipeline (Drone) — please confirm the build
  still passes and the new info log line shows up on a fresh sandbox
  start.
- [ ] One opted-in deploy (`HELIX_EXPERIMENTAL_DESKTOPS="sway"`) to
  confirm the opt-in path is unchanged from current behaviour.

Spec: `helix-specs/design/tasks/002023_on-upgrades-when-the/`
