# Requirements: Skip helix-sway Pull at Sandbox Startup Unless Enabled

## Background

When the sandbox version is bumped and a new sandbox container starts, the
startup script (`sandbox/04-start-dockerd.sh`) eagerly pulls every desktop
image referenced under `/opt/images/*.version` — including `helix-sway`.
That pull is large (multi-GB) and currently happens on **every** upgrade,
on **every** deployment, even though `helix-sway` is an experimental image
that is not selected for use in production today (`helix-ubuntu` is the
only production desktop).

Observed log on upgrade:

```
🔄 Pulling ghcr.io/helixml/helix-sway:2.11.4-linux-amd64 from registry...
2.11.4-linux-amd64: Pulling from helixml/helix-sway
0962d14975c5: Already exists
...
```

The build-time stack already classifies desktops as
`PRODUCTION_DESKTOPS=(ubuntu)` vs `AVAILABLE_EXPERIMENTAL_DESKTOPS=(sway
zorin xfce kde)` (see `stack:29-30`), but this distinction is not honored
at runtime — the startup script unconditionally tries to load all four.

## User Story

**As an** operator upgrading a Helix sandbox to a new version,
**I want** the startup script to skip pulling experimental desktop images
that I'm not using,
**so that** my upgrades complete faster and don't waste bandwidth/disk on
images that will never be exercised.

## Acceptance Criteria

1. By default, a sandbox upgrade does **not** pull `helix-sway` (or any
   other experimental desktop image) from the registry. Only production
   desktops (currently `helix-ubuntu`) are pulled at startup.
2. An operator can opt **in** to pre-pulling experimental desktops via a
   single environment variable, e.g.
   `HELIX_EXPERIMENTAL_DESKTOPS="sway zorin"`. Listed names are pulled at
   startup exactly as they are today.
3. The `🔄 Pulling ghcr.io/helixml/helix-sway:...` log line does **not**
   appear on a default-config upgrade. A short informational message is
   logged instead, so operators can see *why* the image was skipped (e.g.
   `ℹ️  helix-sway is experimental; skipping pull (set HELIX_EXPERIMENTAL_DESKTOPS=sway to enable)`).
4. Existing behavior is preserved when the operator opts in: every name in
   `HELIX_EXPERIMENTAL_DESKTOPS` is loaded via the existing
   `load_desktop_image` flow, including registry pull, re-tag, and the
   `.runtime-ref` write that downstream cleanup depends on.
5. The build-time CI pipeline is unchanged — `helix-sway` continues to be
   built and pushed to the registry. The fix is purely a runtime gating
   change inside the sandbox container.
6. The cleanup logic later in `04-start-dockerd.sh` (which prunes old
   image versions) continues to behave correctly: skipped images that
   *do* exist on disk from a previous, pre-fix container are pruned (no
   stale sway images linger forever); skipped images that don't exist on
   disk are simply absent.
7. Hydra's on-demand desktop launch path is **not** required to change.
   If a user ever does request a sway desktop on a sandbox where sway
   was not pre-pulled, Docker pulls the image lazily at `docker run`
   time (acceptable trade-off — first sway session is slower, default
   upgrade is fast).

## Non-Goals

- Removing `helix-sway` from the build pipeline or from the registry.
- Adding a UI surface to toggle experimental desktops per-org. The
  opt-in is operator-level, set via env on the sandbox container.
- Changing the API-side desktop selection logic (`ZED_IMAGE` default,
  hydra image resolution).
- Lazy/deferred pull at first-session-request time inside the startup
  script. Docker's own pull-on-`run` covers that case adequately.
