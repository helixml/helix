# Design: Force a windowed initial size for Zed via env vars

## Root cause (Ubuntu / GNOME desktop)

Scope: every spectask gets a fresh container with empty `~/` and `~/work`, so Zed always starts with no persisted window bounds. The fix only needs to control the **first launch in a new spectask**.

### When did this start: upstream Zed commit `a0d0195ca9`

`a0d0195ca9` ("Add onboarding for parallel agents", upstream PR #52940, **2026-04-07**) changed Zed's `DEFAULT_WINDOW_SIZE` constant in `crates/gpui/src/window.rs:69`:

```diff
- pub const DEFAULT_WINDOW_SIZE: Size<Pixels> = size(px(1536.), px(864.));
+ pub const DEFAULT_WINDOW_SIZE: Size<Pixels> = size(px(1536.), px(1095.));
```

This commit was pulled into the Helix Zed fork via the **001864 merge** (`ac4f4a9080` upstream-into-feature on 2026-04-23, PR `980a6f1dbc` merged on 2026-04-24), and CI started building the new binary the same day. That is exactly when the regression started — not the Chrome viewport task that landed alongside it.

Why it matters: on a fresh launch with no persisted bounds, Zed falls through to `default_bounds()` in `crates/gpui/src/window.rs:1162` and `crates/gpui/src/platform.rs:259-267`. That function does:

```rust
let clipped_window_size = DEFAULT_WINDOW_SIZE.min(&bounds.size);
```

On a 1920×1080 virtual monitor, the new `1536×1095` clips to `1536×1080` — **exactly the screen height**. GNOME's Mutter (with `org.gnome.mutter auto-maximize=true`, the upstream default and not overridden in our `dconf-settings.ini`) sees a window opening at the full work-area height and silently promotes it to `Maximized`. The user sees a fullscreen Zed.

Before the upstream change, `1536×864` clipped to `1536×864` on the same monitor — 216 px shorter than the work area, comfortably under the auto-maximise threshold, so it opened as a centred floating window. This is the "used to start nicely in the middle of the screen" behaviour the user remembers.

The "taller than the screen when un-maximised" symptom is Mutter remembering the requested `1536×1095` (or whatever the unmaximized bounds are) and snapping back to that when the user un-maximises — 15 px taller than the 1080-tall work area, so the bottom edge falls off the streamed viewport.

### The Chrome viewport task is not the cause

The Chrome viewport task (commit `53715951c`, also 2026-04-23) is unrelated as a *cause* but shares the same auto-maximise dynamic — Chrome's CDP `page.resize` to `1600x1080` produces a ~1600×1160 window (page + chrome decorations), which also trips the threshold. The user's intuition that the two are connected is correct in mechanism, even though the Chrome fix did not literally resize the Zed window. Shrinking the Chrome viewport is a one-line follow-on that complements the Zed fix and gets Chrome out of the same fullscreen trap.

## Fix part 1: pass `ZED_WINDOW_SIZE` and `ZED_WINDOW_POSITION` env vars

Zed already supports two env vars that set initial bounds and force the workspace into the `WindowBounds::Windowed` variant (so Mutter never sees an at-or-above-work-area request to auto-maximise):

| Env var | Format | Effect |
|---|---|---|
| `ZED_WINDOW_SIZE` | `WIDTH,HEIGHT` (pixels) | Sets the size half of `window_bounds` |
| `ZED_WINDOW_POSITION` | `X,Y` (pixels from top-left) | Sets the origin half |

Implementation: `crates/workspace/src/workspace.rs:171-183` (env parsing) and `:8011-8017` (override application). When both are set, `window_bounds_env_override` returns `Some(Bounds)` and `workspace.rs:1981-1985` wraps it as `WindowBounds::Windowed(bounds)` — explicitly **not** Maximized or Fullscreen.

Set the env vars in **`desktop/ubuntu-config/start-zed-helix.sh`** (the GNOME-specific Zed launcher), computed dynamically from the virtual monitor size and zoom level so the proportions hold for 1920×1080, 4K, 5K, and any HiDPI session:

```bash
# In desktop/ubuntu-config/start-zed-helix.sh, before sourcing the core script.
#
# Force a centred, ~80%-of-screen windowed initial size for Zed.
# Without this, Zed (since upstream commit a0d0195ca9, merged via 001864) defaults
# to 1536x1095, which clips to the screen height and triggers GNOME's auto-maximize.
# These env vars wrap the bounds as WindowBounds::Windowed and skip auto-maximize.
#
# ZED_WINDOW_SIZE/POSITION are in *logical* pixels, so divide by GDK_SCALE
# (the integer GNOME scaling-factor that startup-app.sh exports when zoom > 100%;
# unset at 100% zoom, hence the :-1 default). HELIX_SCALE_FACTOR itself is not
# exported by startup-app.sh, so don't read it directly.
ZED_SCALE=${GDK_SCALE:-1}
ZED_LOGICAL_W=$(( ${GAMESCOPE_WIDTH:-1920} / ZED_SCALE ))
ZED_LOGICAL_H=$(( ${GAMESCOPE_HEIGHT:-1080} / ZED_SCALE ))
ZED_W=$(( ZED_LOGICAL_W * 80 / 100 ))
ZED_H=$(( ZED_LOGICAL_H * 80 / 100 ))
ZED_X=$(( ZED_LOGICAL_W * 10 / 100 ))
ZED_Y=$(( ZED_LOGICAL_H * 10 / 100 ))
export ZED_WINDOW_SIZE="${ZED_WINDOW_SIZE:-${ZED_W},${ZED_H}}"
export ZED_WINDOW_POSITION="${ZED_WINDOW_POSITION:-${ZED_X},${ZED_Y}}"
```

Resulting bounds at common configurations:

| GAMESCOPE | Scale | Logical | Zed window | Origin |
|---|---|---|---|---|
| 1920×1080 | 1× | 1920×1080 | 1536×864 | (192, 108) |
| 1920×1080 | 2× | 960×540 | 768×432 | (96, 54) |
| 3840×2160 (4K) | 1× | 3840×2160 | 3072×1728 | (384, 216) |
| 3840×2160 (4K) | 2× | 1920×1080 | 1536×864 | (192, 108) |
| 5120×2880 (5K) | 2× | 2560×1440 | 2048×1152 | (256, 144) |

Note that `1536×864` is **exactly the size Zed defaulted to before `a0d0195ca9`** — at 1920×1080 the formula reproduces the old, working dimensions; at any other display size it scales proportionally. `HELIX_SCALE_FACTOR` is exported earlier in `startup-app.sh` (search for `HELIX_SCALE_FACTOR=$((${ZOOM_LEVEL}/100))`); when zoom is 100% the variable is unset, so the `:-1` default kicks in.

The Sway script (`desktop/sway-config/start-zed-helix.sh`) is **not** changed — Sway's tiling behaviour is a different dynamic and out of scope.

## Fix part 2: shrink the chrome-devtools MCP viewport

The Chrome viewport setting in `api/pkg/external-agent/zed_config.go:253` is currently:

```go
"--viewport", "1600x1080",
```

Change it to:

```go
"--viewport", "1280x800",
```

`1280×800` is the canonical "small desktop" viewport — wider than the 1024-px threshold most sites use to switch to mobile, at the 1280-px breakpoint that the 001532 design doc itself called out for full desktop mode (e.g. GitHub), and small enough that the resulting Chrome window (~1280×880 page + chrome decorations) leaves a wide margin on a 1920×1080 monitor and stays well below Mutter's auto-maximise threshold. The `--viewport` is still passed as a CLI arg, so the underlying 001532 fix is preserved.

Update the comment on the preceding line of `zed_config.go` to reflect the new dimensions and the rationale (still desktop-mode, no longer fills the screen).

**Why static and not dynamic for Chrome.** `zed_config.go` runs in the API server process, not in the desktop container, so it doesn't see the session's `GAMESCOPE_WIDTH/HEIGHT` env vars. Plumbing them through (per-session MCP config) is more invasive than it's worth: the Chrome MCP browser is an automation surface, not the user's main browser, and `1280×800` works on every realistic display — it sits centred (or wherever GNOME places it) on 1920×1080, and on 4K/5K it's a small but perfectly serviceable window. If a future task needs a proportional Chrome viewport too, the right place to add the math is in a wrapper script under `desktop/ubuntu-config/` that exec's `chrome-devtools-mcp@latest --viewport ${COMPUTED}`, not in the API.

## Decisions and rationale

**Why env-var override and not a dconf change to disable `auto-maximize`.** Disabling `auto-maximize` globally in `dconf-settings.ini` would change behaviour for every GTK/GNOME app in the desktop and could leave Zed at whatever oversized default it requested (just unmaximised, still off-screen). The env-var override is scoped to Zed and gives an explicit, deterministic position and size — strictly more targeted.

**Why dynamic 80% sizing rather than hard-coding `1600×900`.** Users can pick any virtual monitor (1920×1080, 4K, 5K) and any zoom level. A hardcoded value that looks right at 1920×1080 looks tiny on 4K and tinier on 5K. 80% of the logical work area scales sensibly across all real configurations and matches the perception of "centered with margin around it" the user remembers — and it lands on `1536×864` at 1920×1080, which is the exact size Zed used to default to before `a0d0195ca9`.

**Why `1280×800` for Chrome and not a percentage.** The Chrome `--viewport` value is the rendered *page* size, not the window size — the window is `viewport + ~80px` of chrome decorations. `1280` is the canonical desktop-vs-mobile breakpoint, so it's the smallest value that still triggers desktop-mode rendering everywhere. The Go-side static value is acceptable here because (a) the viewport drives page rendering which doesn't need to scale with the user's monitor, and (b) the resulting Chrome window fits comfortably on every supported display size.

**Why use the `${VAR:-default}` pattern.** Lets a session override the size by setting the env var before launching the script, which is useful for debugging without rebuilding the image.

**Why not change `build_window_options` in Zed.** It would mean carrying another upstream-divergent patch in the Helix Zed fork (already a chronic merge-conflict source — see 001864/001909 design docs). The env-var path is a supported upstream API.

## Files to change

| File | Change |
|---|---|
| `desktop/ubuntu-config/start-zed-helix.sh` | Add the two `export` lines (`ZED_WINDOW_SIZE`, `ZED_WINDOW_POSITION`) just before the `source "$CORE_SCRIPT"` line near the bottom. |
| `api/pkg/external-agent/zed_config.go` | Change `--viewport` value from `1600x1080` to `1280x800` on line 253 and update the adjacent comment. |

No changes in `/home/retro/work/zed/`. No changes in `dconf-settings.ini`. No changes in the Sway config.

## Verification

1. Rebuild the Ubuntu desktop image (`./stack build-ubuntu`). The `zed_config.go` change is API-side and hot-reloads via Air on the next spectask start; the `start-zed-helix.sh` change ships in the desktop image.
2. **Start a fresh spectask at the default 1920×1080 / 100% zoom** — confirm Zed launches as a centred 1536×864 windowed window at origin (192, 108), not maximised.
3. **Start a spectask with a 4K display (`GAMESCOPE_WIDTH=3840 GAMESCOPE_HEIGHT=2160`, 100% zoom)** — confirm Zed is ~3072×1728 centred, still windowed (proportional check).
4. **Start a spectask with `HELIX_ZOOM_LEVEL=200`** — confirm Zed is windowed at the appropriately scaled logical size (e.g., 1536×864 on a 4K monitor at 200% zoom, since logical work area is 1920×1080).
5. Open Chrome via the chrome-devtools MCP — confirm the window is ~1280×880 (page 1280×800 + chrome decorations) and that desktop sites (e.g. github.com) still render in desktop mode.
6. Drag Zed to fill the screen by hand — confirm normal GNOME maximise/unmaximise still works.
7. Stream the desktop in a small browser viewport (≈1280×720) on a 1920×1080 session — confirm Zed is not clipped at the bottom.

## Notes for future agents

- **Spectasks always start with empty `~/` and `~/work`.** Don't design fixes around state persisting between spectasks — for the user it doesn't. (Zed *does* persist window bounds across launches *within* a single spectask, via `~/.config/zed` → `$WORK_DIR/.zed-state`, but every new spectask starts blank.) Design for the first-launch case.
- **Zed has `ZED_WINDOW_SIZE` and `ZED_WINDOW_POSITION` env vars** that set the workspace bounds and force the `WindowBounds::Windowed` variant. Format: `WIDTH,HEIGHT` and `X,Y` (integer pixels, comma-separated). They are in **logical** pixels — divide physical pixels by `GDK_SCALE` when computing. Implementation in `crates/workspace/src/workspace.rs:171-183` and `:8011-8017`. Use these whenever you need to control Zed's window size from outside Zed — don't patch `build_window_options`.
- **Zed's `DEFAULT_WINDOW_SIZE` (in `crates/gpui/src/window.rs:69`) is what Zed asks for when there are no persisted bounds and no env-var override.** It changed from `1536×864` to `1536×1095` in upstream commit `a0d0195ca9` (2026-04-07, PR #52940 "Add onboarding for parallel agents"), which is what triggered this regression. If a future Zed merge changes it again, this fix's percentage-of-screen formula keeps working — it doesn't depend on the upstream default at all.
- **`HELIX_SCALE_FACTOR` is set in `startup-app.sh` but not exported.** Use `GDK_SCALE` (which is exported) for any scale-aware bash math in scripts that run downstream of `start_gnome`.
- **`GAMESCOPE_WIDTH/HEIGHT` are exported in `startup-app.sh` and inherited via `dbus-run-session`** all the way down to `start-zed-helix.sh`. The defaults are `1920` and `1080`; users can override them per session.
- **GNOME's `auto-maximize` is on by default** in the Helix Ubuntu desktop. Apps launching with a window ≥ work-area dimensions get silently promoted to Maximised. If a future app shows the same regression, the env-override pattern (or app-specific equivalent) is the cleaner fix than disabling `auto-maximize` globally.
- **There are two Helix desktops, Sway and Ubuntu/GNOME**, with separate `start-zed-helix.sh` wrappers (`desktop/sway-config/` and `desktop/ubuntu-config/`) that source the shared `desktop/shared/start-zed-core.sh`. Per-WM tweaks belong in the wrappers, not the shared core.
- **`dconf-settings.ini` (`desktop/ubuntu-config/dconf-settings.ini`) is loaded once at session start** via `dconf load /`. Settings here are baked into the GSettings DB before `gnome-shell` starts, so they take effect for the first window of every session.
- **The Chrome viewport task `53715951c` is not the cause of this Zed bug** but shares the auto-maximise dynamic, and the dimensions chosen there (`1600x1080`) were independently too big. Shrinking the viewport (here, to `1280x800`) is a one-line follow-on, not a revert.
- **`--viewport WxH` in `chrome-devtools-mcp` is the *page* size, not the *window* size.** Add ~80 px of chrome decorations to estimate the actual window footprint. The 1280-px page-width breakpoint is what desktop sites use to switch out of mobile mode (called out in the 001532 design doc).
