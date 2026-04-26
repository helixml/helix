# Design: Force a windowed initial size for Zed via env vars

## Root cause (Ubuntu / GNOME desktop)

Scope: every spectask gets a fresh container with empty `~/` and `~/work` (so Zed always starts with no persisted window bounds). The fix only needs to control the **first launch in a new spectask**.

Two things combine:

1. **Zed defers to the WM for initial bounds.** `crates/zed/src/zed.rs:309 build_window_options` returns `window_bounds: None` (and `window_min_size: 360×240`). With no persisted bounds, Zed effectively asks GNOME for whatever GNOME considers a default size for an unconstrained window. On Wayland in headless GNOME with a 1920×1080 virtual monitor, that comes out at or above the work area.
2. **GNOME / Mutter auto-maximises large windows.** `org.gnome.mutter auto-maximize` defaults to `true`. The Helix `dconf-settings.ini` does not override it, so any window opened at ≥ work-area dimensions is silently promoted to Maximized before the user sees it.

Result on a fresh spectask: Zed appears Maximized. The "taller than the screen when un-maximised" symptom is the inner *windowed* bounds Mutter remembers for the maximised window — those default to roughly the requested size (≥ 1080 tall on a 1920×1080 virtual monitor minus the top bar), so when the user clicks unmaximize the bottom of the window falls off the streamed viewport.

The Chrome viewport task (commit `53715951c`) is **not the cause** of the Zed regression but it shares the same dynamic (Chrome's CDP `page.resize` enlarges the window past the work-area threshold → Mutter auto-maximises). The user's intuition that the two are connected is correct in mechanism, even though the Chrome fix did not literally resize the Zed window. Separately, the `1600x1080` value picked in 001532 is itself too big — the CDP resize gives Chrome a ~1600×1160 window (1080 page + ~80 chrome decorations) on a 1920×1080 monitor, wider and taller than necessary for desktop-mode rendering and right on the auto-maximise threshold. Shrinking it is a one-line follow-on that complements the Zed fix and gets Chrome out of fullscreen as a side effect.

## Fix part 1: pass `ZED_WINDOW_SIZE` and `ZED_WINDOW_POSITION` env vars

Zed already supports two env vars that set initial bounds and force the workspace into the `WindowBounds::Windowed` variant (so Mutter never sees an at-or-above-work-area request to auto-maximise):

| Env var | Format | Effect |
|---|---|---|
| `ZED_WINDOW_SIZE` | `WIDTH,HEIGHT` (pixels) | Sets the size half of `window_bounds` |
| `ZED_WINDOW_POSITION` | `X,Y` (pixels from top-left) | Sets the origin half |

Implementation: `crates/workspace/src/workspace.rs:171-183` (env parsing) and `:8011-8017` (override application). When both are set, `window_bounds_env_override` returns `Some(Bounds)` and `workspace.rs:1981-1985` wraps it as `WindowBounds::Windowed(bounds)` — explicitly **not** Maximized or Fullscreen.

Set the env vars in **`desktop/ubuntu-config/start-zed-helix.sh`** (the GNOME-specific Zed launcher), so they only affect the Ubuntu desktop and live in one place:

```bash
# In desktop/ubuntu-config/start-zed-helix.sh, before sourcing the core script.
#
# Force a centred, windowed initial size for Zed inside the GNOME virtual monitor.
# Without this, Zed's first launch in a new spectask asks the WM for an
# unconstrained size, GNOME's auto-maximize promotes it to Maximized, and the
# user sees a fullscreen Zed. These env vars wrap the bounds as
# WindowBounds::Windowed and skip the auto-maximize path entirely.
# Sized for a 1920x1080 GAMESCOPE virtual monitor: 1600x900 leaves a 160x90 margin.
export ZED_WINDOW_SIZE="${ZED_WINDOW_SIZE:-1600,900}"
export ZED_WINDOW_POSITION="${ZED_WINDOW_POSITION:-160,90}"
```

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

`1280×800` is the canonical "small desktop" viewport — it is wider than the 1024-px threshold most sites use to switch to mobile, comfortably above the 1280-px threshold that the original 001532 spec called out for full desktop mode (e.g. GitHub), and small enough on a 1920×1080 monitor that the Chrome window leaves a wide margin and stays well below Mutter's auto-maximise threshold (work area ≈ 1920×1050 with the dock; 1280×880 window fits with room to spare). The `--viewport` is still passed as a CLI arg, so the underlying 001532 fix is preserved.

Update the comment on the preceding line of `zed_config.go` to reflect the new dimensions and the rationale (still desktop-mode, no longer fills the screen).

## Decisions and rationale

**Why env-var override and not a dconf change to disable `auto-maximize`.** Disabling `auto-maximize` globally in `dconf-settings.ini` would change behaviour for every GTK/GNOME app in the desktop and could leave Zed at whatever oversized default it requested (just unmaximised, still off-screen). The env-var override is scoped to Zed and gives an explicit, deterministic position and size — strictly more targeted.

**Why hard-code 1600×900 instead of computing from `GAMESCOPE_WIDTH/HEIGHT`.** The virtual monitor is overwhelmingly 1920×1080 in production; the few sessions with `HELIX_DISPLAY_SCALE>1` get a smaller logical work area but Zed still needs at least 1280×800 to be usable. A static `1600×900` works in every realistic scenario. If a future task wants dynamic sizing, the env-var hook is in the right place to compute it from `$GAMESCOPE_WIDTH`/`$GAMESCOPE_HEIGHT`.

**Why `1280x800` for Chrome and not `1600x900` (matching Zed).** The Chrome `--viewport` value is the rendered *page* size, not the window size — the window is `viewport + ~80px` of chrome decorations. Picking `1280x800` keeps the window comfortably below 1920×1080 in both axes, and `1280` is the canonical desktop-vs-mobile breakpoint that the 001532 design doc itself called out. Matching Zed's 1600 width here would leave Chrome at ~1600×880 — roughly the same fullscreen-ish footprint we are trying to escape.

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
2. **Start a fresh spectask** — confirm Zed launches as a centred ~1600×900 windowed window with title-bar buttons visible (not maximised, ample margin around it).
3. Open Chrome via the chrome-devtools MCP — confirm the window is ~1280×880 (page 1280×800 + chrome decorations) and that desktop sites (e.g. github.com) still render in desktop mode.
4. Drag Zed to fill the screen by hand — confirm normal GNOME maximise/unmaximise still works.
5. Stream the desktop in a small browser viewport (≈1280×720) — confirm Zed is not clipped at the bottom.

## Notes for future agents

- **Spectasks always start with empty `~/` and `~/work`.** Don't design fixes around state persisting between spectasks — for the user it doesn't. (Zed *does* persist window bounds across launches *within* a single spectask, via `~/.config/zed` → `$WORK_DIR/.zed-state`, but every new spectask starts blank.) Design for the first-launch case.
- **Zed has `ZED_WINDOW_SIZE` and `ZED_WINDOW_POSITION` env vars** that set the workspace bounds and force the `WindowBounds::Windowed` variant. Format: `WIDTH,HEIGHT` and `X,Y` (integer pixels, comma-separated). Implementation in `crates/workspace/src/workspace.rs:171-183` and `:8011-8017`. Use these whenever you need to control Zed's window size from outside Zed — don't patch `build_window_options`.
- **GNOME's `auto-maximize` is on by default** in the Helix Ubuntu desktop. Apps launching with a window ≥ work-area dimensions get silently promoted to Maximised. If a future app shows the same regression, the env-override pattern (or app-specific equivalent) is the cleaner fix than disabling `auto-maximize` globally.
- **There are two Helix desktops, Sway and Ubuntu/GNOME**, with separate `start-zed-helix.sh` wrappers (`desktop/sway-config/` and `desktop/ubuntu-config/`) that source the shared `desktop/shared/start-zed-core.sh`. Per-WM tweaks belong in the wrappers, not the shared core.
- **`dconf-settings.ini` (`desktop/ubuntu-config/dconf-settings.ini`) is loaded once at session start** via `dconf load /`. Settings here are baked into the GSettings DB before `gnome-shell` starts, so they take effect for the first window of every session.
- **The Chrome viewport task `53715951c` is not the cause of this Zed bug** but shares the auto-maximise dynamic, and the dimensions chosen there (`1600x1080`) were independently too big. Shrinking the viewport (here, to `1280x800`) is a one-line follow-on, not a revert.
- **`--viewport WxH` in `chrome-devtools-mcp` is the *page* size, not the *window* size.** Add ~80 px of chrome decorations to estimate the actual window footprint. The 1280-px page-width breakpoint is what desktop sites use to switch out of mobile mode (called out in the 001532 design doc).
