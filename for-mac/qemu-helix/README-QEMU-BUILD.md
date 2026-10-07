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

Host Vulkan driver behind Venus: **MoltenVK**, UTM's default
(`UTMQemuSystem.m` `setVulkanDriver`), built with our SPIRV-Cross patches (see
"Patches on top of UTM"). KosmicKrisp is bundled but opt-in
(`HELIX_VULKAN_DRIVER=kosmickrisp`) and currently crashes on images.

Every component comes from UTM as shipped, except MoltenVK and
`virgl_render_server` (see below).

All three must come from the same UTM release. A QEMU from one release with
frameworks from another fails in confusing ways (for example: blob resources
fail with `ERR_UNSPEC` and the video stream never starts).

### Patches on top of UTM

**MoltenVK** is rebuilt from the commit UTM pins (`MOLTENVK_COMMIT` in UTM's
`patches/sources`) with [`patches/moltenvk/`](patches/moltenvk) and the
SPIRV-Cross fixes in [`patches/spirv-cross/`](patches/spirv-cross) applied, by
[`build-moltenvk.sh`](build-moltenvk.sh) (cached per commit + patch set + build
flags, ~10 minutes cold). `build-helix-app.sh` bundles it in place of UTM.app's
MoltenVK.

It is built with `MVK_USE_METAL_PRIVATE_API=1`, MoltenVK's supported option for
features only Metal's private interfaces provide: `VK_EXT_provoking_vertex`
(GL's last-vertex flat shading), logicOp, wide lines, non-seamless cube maps.
It only rules out App Store distribution, which Helix for Mac doesn't use (UTM
leaves it off because UTM is on the App Store). UTM's fork never built with it,
so `patches/moltenvk` adds the private-API properties its geometry-shader
emulation sets on mesh pipeline descriptors.

Without them, UTM's MoltenVK generates invalid Metal for the shaders of wgpu
apps (Zed, and wgpu's own validation shaders): pipeline creation fails on the
host, Venus doesn't report it, the render server kills the context at the next
`vkCmdBindPipeline` (`vkr: failed to look up object N of type 19`) and the
guest app waits forever, e.g. Zed's window never maps.

1. robustBufferAccess2 (UTM's SPIRV-Cross `a9a8d4a4`, which wgpu enables
   whenever offered) zero-fills out-of-bounds struct loads with `T(0)` and
   bounds-checks struct arrays with a stride of 4.
2. Passing an array taken out of a struct to a function doesn't compile
   (also in upstream SPIRV-Cross).
3. robustBufferAccess2 on descriptor arrays of buffers (Zink's uniforms)
   checks the descriptor index against the buffer size and uses the
   per-descriptor size array as a single size.
4. The geometry-to-mesh wrapper passes a discrete buffer descriptor array by
   its (undeclared) array name instead of per element.

All leave SPIRV-Cross's own MSL reference tests unchanged. Drop a patch once
the UTM release we follow ships the fix; check with `build-moltenvk.sh` —
`git am` fails on a patch that is already applied.

**`virgl_render_server`** is rebuilt from the virglrenderer commit UTM pins
(`VIRGLRENDERER_COMMIT`) with [`patches/virglrenderer/`](patches/virglrenderer)
by [`build-render-server.sh`](build-render-server.sh), with UTM's meson cross
file and options (needs the sysroot). On macOS dma_buf is emulated with shm,
and UTM's render server rejected importing a shm resource
(`VK_ERROR_INVALID_EXTERNAL_HANDLE`), so a guest that shares buffers between
processes (Chromium on Zink) loses its Venus context. The patch imports such
resources as host memory.

### Chromium

Chromium in the desktop renders with ANGLE's Vulkan backend directly on Venus
(`--use-angle=vulkan`, features `Vulkan,VulkanFromANGLE,DefaultANGLEVulkan`),
set by `desktop/shared/helix-chromium.sh` when the render node is virtio-gpu.
Its GPU process still initialises through native GL and needs robust contexts,
which virgl doesn't provide (it falls back to software on virgl), so the
wrapper also selects Zink (GL on Vulkan) for that, with
`ZINK_DEBUG=optimal_keys` to keep Zink off geometry-shader emulation, which
MoltenVK can't compile. ANGLE-on-GL over Zink is not an option: it renders
anti-aliased concave paths (SVG icons, Chromium's toolbar icons, `clip-path`)
as nothing. With ANGLE-Vulkan, WebGL 1/2 and WebGPU work; canvas benchmark
~47 fps (software: ~11), page scrolling at ~0.6 host cores (software: ~1.1).

Why not KosmicKrisp: since UTM 5.0.6 the render server runs out of process and
backs all host-visible memory with shm imported via
`VK_EXT_external_memory_host`. KosmicKrisp has a single, host-visible memory
type and can only use imported host memory for buffers, so every image gets a
nil texture and the render server crashes. MoltenVK gives images their own
storage and is unaffected.

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
   `QEMU_UTM_COMMIT` to the merge commit. Run `build-moltenvk.sh` and drop or
   rebase any `patches/spirv-cross` patch the new MoltenVK pin already has.
3. **Launch:** diff UTM's `QEMUHelper.m` / `UTMQemuSystem.m` against
   `vm.go` for new environment or arguments.
4. **Test** before pinning: video stream, multiple desktops, idle CPU
   (`sudo powermetrics --samplers tasks -i 5000 -n 3 | grep qemu`), and that
   Zed's window maps and renders (a `vkcube` that runs proves little: it
   doesn't use the shader features wgpu does).

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
