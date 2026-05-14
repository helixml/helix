# Design: Skip helix-sway Pull at Sandbox Startup Unless Enabled

## Where the eager pull lives today

Three things conspire to produce the observed behavior:

1. **CI always builds and ships sway refs.** `.drone.yml:1664-1668` (amd64)
   and `.drone.yml:1987-1989` (arm64) unconditionally write
   `sandbox-images/helix-sway.ref` and `sandbox-images/helix-sway.version`
   alongside the equivalent `helix-ubuntu` files.
2. **Those files get baked into the sandbox image.**
   `Dockerfile.sandbox:204` does `COPY sandbox-images/ /opt/images/`,
   so the new sandbox container always boots with
   `/opt/images/helix-sway.{ref,version}` present.
3. **Startup unconditionally walks every known desktop name.**
   `sandbox/04-start-dockerd.sh:248-251`:

   ```bash
   load_desktop_image "sway"   "false"
   load_desktop_image "zorin"  "false"
   load_desktop_image "ubuntu" "false"
   load_desktop_image "kde"    "false"
   ```

   `load_desktop_image` (lines 160–245) finds the version file, doesn't
   find the local image (fresh upgrade), and pulls
   `ghcr.io/helixml/helix-sway:VERSION` — that's the log line the user
   sees.

## Existing pattern we're mirroring

The build-side `stack` script already separates production from
experimental desktops (`stack:29-30`):

```bash
PRODUCTION_DESKTOPS=(ubuntu)
AVAILABLE_EXPERIMENTAL_DESKTOPS=(sway zorin xfce kde)
# EXPERIMENTAL_DESKTOPS can be set as space-separated string, …
```

The fix mirrors this categorization at runtime so build-time and
startup-time stay consistent.

## Decision: gate at startup-script level via env var

Add the same production/experimental split to
`sandbox/04-start-dockerd.sh`, gated by a new env var
`HELIX_EXPERIMENTAL_DESKTOPS` (default empty).

```bash
# Production desktops always loaded.
PRODUCTION_DESKTOPS=(ubuntu)
# Experimental desktops only loaded if listed in HELIX_EXPERIMENTAL_DESKTOPS.
AVAILABLE_EXPERIMENTAL_DESKTOPS=(sway zorin xfce kde)

for d in "${PRODUCTION_DESKTOPS[@]}"; do
    load_desktop_image "$d" "false"
done

# Convert space-separated env var into a quick lookup.
declare -A enabled_experimental=()
for d in $HELIX_EXPERIMENTAL_DESKTOPS; do
    enabled_experimental[$d]=1
done

for d in "${AVAILABLE_EXPERIMENTAL_DESKTOPS[@]}"; do
    if [ -n "${enabled_experimental[$d]:-}" ]; then
        load_desktop_image "$d" "false"
    else
        echo "ℹ️  helix-${d} is experimental; skipping pull (set HELIX_EXPERIMENTAL_DESKTOPS=\"${d} ...\" to enable)"
    fi
done
```

The skip log uses the same `ℹ️` prefix style as the existing
"`${IMAGE_NAME} not configured (no version file)`" branch, so operators
recognise it as a benign info message.

The `Dockerfile.sandbox` env block (lines 270–279) gets a default:

```dockerfile
HELIX_EXPERIMENTAL_DESKTOPS="" \
```

`docker-compose.dev.yaml` may pass `HELIX_EXPERIMENTAL_DESKTOPS` through
to the `sandbox-nvidia` service so a developer working on sway can opt in
without rebuilding.

## Why not remove sway from the .ref/.version files in CI?

Considered and rejected:

- Operators or developers may legitimately want to flip sway on for a
  given deployment without re-running CI. Keeping the `.ref` file
  baked in means opt-in is a single env-var flip, not a rebuild.
- The CI side is shared between amd64 and arm64 in two near-duplicate
  blocks; touching both for what is fundamentally a runtime concern
  spreads the change unnecessarily.
- The `.ref` files are tiny (~100 bytes), so the cost of always
  shipping them is negligible.

## Why not lazy pull at session-create time?

Considered and rejected as the *primary* mechanism:

- The hydra session-launch path already invokes `docker run` against
  the resolved image; if the image isn't local, Docker pulls it
  on-demand. So there's no functional gap — first sway session on a
  non-pre-pulled sandbox just takes longer.
- Adding an explicit "pull-before-run" hook in hydra introduces new
  failure modes (network errors at session-create time, instead of at
  startup) and a place we'd need a UX for "image is being pulled, hang
  on" — not worth it for an experimental feature.

So lazy-pull is the *fallback* for opted-out deployments, not a code
change we make.

## Cleanup logic interaction

`04-start-dockerd.sh` runs an "old image cleanup" pass after
`load_desktop_image` (the section that starts at line 268, "Cleaning up
old desktop images"). It keeps images that match the `.version` files
and the `.runtime-ref` files written by `load_desktop_image`.

For an opt-out sandbox:

- `helix-sway.version` exists (from the `COPY`), but no
  `helix-sway.runtime-ref` is written (because we skip the pull). Any
  pre-existing `helix-sway:OLD_VERSION` images on disk from a previous
  opted-in container will be pruned by the existing cleanup pass —
  matching the intent (free up disk).
- A `helix-sway:NEW_VERSION` image, if it somehow exists locally
  (devloop), is preserved by the version-file match. That's fine.

For an opted-in sandbox: behavior is identical to today.

No changes needed to the cleanup section.

## Files touched

| File | Change |
|---|---|
| `sandbox/04-start-dockerd.sh` | Replace lines 247–251 with production/experimental split keyed off `HELIX_EXPERIMENTAL_DESKTOPS`. |
| `Dockerfile.sandbox` | Add `HELIX_EXPERIMENTAL_DESKTOPS=""` to the env block (~line 270–279). |
| `docker-compose.dev.yaml` | Pass `HELIX_EXPERIMENTAL_DESKTOPS=${HELIX_EXPERIMENTAL_DESKTOPS:-}` through to `sandbox-nvidia` (and `sandbox`/`sandbox-macos` if equivalent service entries exist). |
| `CLAUDE.md` (sandbox section) or new `design/2026-05-14-skip-experimental-desktop-pull.md` | One-paragraph note documenting the new env var and the default. |

No CI / `.drone.yml` change. No Go code change. No API change. No frontend change.

## Test plan

This is a sandbox-startup-script change; the natural tests are:

1. **Default-config upgrade.** Bump the sandbox version, restart the
   sandbox container with no `HELIX_EXPERIMENTAL_DESKTOPS` set, watch
   logs:
   - Expect: `helix-ubuntu` pulls as today.
   - Expect: `ℹ️  helix-sway is experimental; skipping pull …` line.
   - Expect: no `🔄 Pulling …helix-sway…` line.
2. **Opt-in upgrade.** Restart with `HELIX_EXPERIMENTAL_DESKTOPS="sway"`:
   - Expect: identical sway pull behavior to today.
   - Expect: zorin/kde still skipped with info messages.
3. **Cleanup of stale sway image.** On a sandbox that previously had
   `helix-sway:OLD` pulled, restart with the new script and no
   experimental opt-in. Verify the cleanup pass removes
   `helix-sway:OLD` (no orphan multi-GB image left behind).
4. **Lazy-pull fallback.** On an opted-out sandbox, attempt to launch a
   sway desktop session via hydra. Verify Docker pulls
   `helix-sway:VERSION` on demand and the session starts.

Steps 1, 2, and 3 are bash-script verifications — no Go test changes
needed. Step 4 confirms we haven't broken the escape hatch for users
who do want sway.

## Notes for future agents

- The build-time/runtime split for desktops was already partially
  implemented (`stack:29-30`) but not extended to startup. If you find
  yourself repeating that hardcoded list of experimental desktops in a
  third place, factor it into a shared definition.
- `sandbox/04-start-dockerd.sh` is bash, copied into the image at
  `Dockerfile.sandbox:238`. It's not hot-reloadable — changes require
  `./stack build-sandbox` and a fresh sandbox container.
- The `🔄 Pulling` emoji is grep-able and the logs are user-visible.
  Keep that aesthetic when adding new info lines.
