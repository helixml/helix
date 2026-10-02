# Building QEMU for Helix for Mac

## Following UTM

**Helix for Mac follows UTM.** UTM is our upstream for QEMU, its macOS
dependencies, and how QEMU is launched. We do not track upstream QEMU directly,
even when it is newer: UTM ports QEMU to macOS (shared-library build, Venus/Metal
scanout, SPICE IOSurface, HVF fixes) and we build on that port rather than
maintain our own.

Everything is pinned to one UTM release, in [`UTM_VERSION`](UTM_VERSION):

| What | Source | Pin |
|---|---|---|
| QEMU source | [`utmapp/qemu`](https://github.com/utmapp/qemu) branch `utm-edition`, **merged** into [`helixml/qemu-utm`](https://github.com/helixml/qemu-utm) `utm-edition-venus-helix` with our patches on top | `QEMU_UTM_COMMIT` |
| Sysroot QEMU compiles against (virglrenderer, SPICE, glib, ...) | UTM's `scripts/build_dependencies.sh` at `UTM_COMMIT` | `UTM_VERSION` |
| Frameworks + `virgl_render_server` bundled in Helix.app | `UTM.dmg` of `UTM_RELEASE` (`fetch-utm-app.sh`) | `UTM_VERSION` |
| Launch environment (`RENDER_SERVER_EXEC_PATH`, `ANGLE_DEFAULT_PLATFORM`, `VK_DRIVER_FILES`) | UTM's `QEMUHelper/QEMUHelper.m` | `for-mac/vm.go` `buildQEMUEnv` |

All three must come from the same UTM release. A QEMU from one release with
frameworks from another fails in confusing ways (for example: blob resources
fail with `ERR_UNSPEC` and the video stream never starts).

### Updating to a new UTM release

1. **QEMU:** in `helixml/qemu-utm`, branch from `utm-edition-venus-helix` and
   `git merge utmapp/utm-edition`. Merge, never rebase: our branch carries UTM
   developers' commits as well as ours, and rebasing a subset silently drops
   code. Resolve conflicts by authorship (`git log --format=%an <base>..HEAD -- <file>`):
   files only UTM developers touched take UTM's version; for Helix changes, keep
   ours unless UTM now solves the same problem, in which case take theirs and
   drop our workaround. Bump the version string in
   `hw/display/helix/helix-frame-export.m`.
2. **Pins:** set `UTM_RELEASE` and `UTM_COMMIT` in `UTM_VERSION` (`UTM_COMMIT`
   must carry the same `patches/sources` as the release tag), and
   `QEMU_UTM_COMMIT` to the merge commit.
3. **Launch:** diff UTM's `QEMUHelper.m` / `UTMQemuSystem.m` against
   `vm.go` for new environment or arguments.
4. **Test** before pinning: video stream, multiple desktops, idle CPU
   (`sudo powermetrics --samplers tasks -i 5000 -n 3 | grep qemu`).

UTM's QEMU release tarball plus its `patches/qemu-*-utm.patch` equals the
`utm-edition` branch tip, so merging the branch is equivalent to what UTM ships.

## Local build

```bash
# One-time (and whenever UTM_VERSION changes): ~1-1.5h
./for-mac/qemu-helix/build-utm-sysroot.sh

# Build QEMU from ~/pm/qemu-utm and install into the app bundle + dev-qemu.
# Stop the VM first.
cd for-mac && make rebuild-qemu
```

`build-utm-sysroot.sh` checks out `UTM_COMMIT` in `~/pm/UTM` (refusing if it has
local changes; never patch UTM) and stamps the sysroot. `build-qemu-ci.sh`
refuses a sysroot built from a different commit.

## CI

The tag pipeline (`build-macos-dmg` in `.drone.yml`) runs
`build-utm-sysroot.sh` (no-op unless the pin changed), `build-qemu-ci.sh`
(cached per `QEMU_UTM_COMMIT` + build script + `UTM_VERSION`), then
`build-helix-app.sh`, which takes frameworks and the render server from the
pinned UTM.app.

## Troubleshooting

- **Video stream stuck on "Starting video", `RESOURCE_CREATE_BLOB` errors:**
  render server not found or mismatched. Check `virgl_render_server` sits next
  to `qemu-system-aarch64` and comes from the same UTM release.
- **`-spice: invalid option`:** QEMU built without SPICE; the sysroot is missing
  or incomplete. Re-run `build-utm-sysroot.sh`.
- **`Library not loaded: @rpath/...` from `build/dev-qemu`:** stale dev QEMU.
  Run `make rebuild-qemu`. The app prefers `build/dev-qemu` relative to its
  working directory, so launching it from a shell inside `for-mac/` uses it.
