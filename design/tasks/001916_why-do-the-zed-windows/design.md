# Design: Force a windowed initial size for Zed via env vars

## Root cause (Ubuntu / GNOME desktop)

Three things combine:

1. **Zed defers to the WM for initial bounds.** `crates/zed/src/zed.rs:309 build_window_options` returns `window_bounds: None` (and `window_min_size: 360×240`). On a fresh launch with no persisted bounds, Zed effectively asks GNOME for whatever GNOME considers a default size for an unconstrained window.
2. **GNOME / Mutter auto-maximises large windows.** `org.gnome.mutter auto-maximize` defaults to `true`. The Helix `dconf-settings.ini` does not override it, so any window opened at ≥ work-area dimensions is silently maximised.
3. **Zed persists the resulting Maximised state per-workspace.** `crates/workspace/src/workspace.rs:1981-1985` reads `workspace.window_bounds` and restores the `WindowBounds::Maximized(...)` variant on next launch. The Zed state directory is symlinked to `$WORK_DIR/.zed-state/`, which is bind-mounted from persistent workspace storage (`startup-app.sh:50-58`), so the state survives container restarts indefinitely.

That cycle ratchets the window into Maximised once and never lets it out. The "taller than the screen when un-maximised" symptom is the persisted *windowed* bounds inside that Maximised state — saved when the work area was a different size (e.g., before a recent zoom-level change, or when chrome borders were different) — being too tall for the current 1920×1080 virtual monitor.

The Chrome viewport task (commit `53715951c`) is **not the cause** of the Zed regression but it shares the same dynamic (Chrome's CDP `page.resize` enlarges the window past the work-area threshold → Mutter auto-maximises). The user's intuition that the two are connected is correct in mechanism, even though the Chrome fix did not literally resize the Zed window. Separately, the `1600x1080` value picked in 001532 is itself too big — the CDP resize gives Chrome a ~1600×1160 window (1080 page + ~80 chrome decorations) on a 1920×1080 monitor, wider and taller than necessary for desktop-mode rendering and right on the auto-maximise threshold. Shrinking it is a one-line follow-on that complements the Zed fix and gets Chrome out of fullscreen as a side effect.

## Fix part 1: pass `ZED_WINDOW_SIZE` and `ZED_WINDOW_POSITION` env vars

Zed already supports two env vars that override **all** persisted bounds and force a windowed size:

| Env var | Format | Effect |
|---|---|---|
| `ZED_WINDOW_SIZE` | `WIDTH,HEIGHT` (pixels) | Overrides the size half of `window_bounds` |
| `ZED_WINDOW_POSITION` | `X,Y` (pixels from top-left) | Overrides the origin half |

Implementation: `crates/workspace/src/workspace.rs:171-183` (env parsing) and `:8011-8017` (override application). When both are set, `window_bounds_env_override` returns `Some(Bounds)` and `workspace.rs:1981-1985` wraps it as `WindowBounds::Windowed(bounds)` — explicitly **not** Maximized or Fullscreen. The override applies on every workspace open, so it also wipes any stale persisted Maximised state.

Set the env vars in **`desktop/ubuntu-config/start-zed-helix.sh`** (the GNOME-specific Zed launcher), so they only affect the Ubuntu desktop and are visible in one place:

```bash
# In desktop/ubuntu-config/start-zed-helix.sh, before sourcing the core script.
#
# Force a centred, windowed initial size for Zed inside the GNOME virtual monitor.
# Without this, GNOME's auto-maximize promotes Zed to Maximized on first launch and
# Zed then persists that state forever in $WORK_DIR/.zed-state/local-share/zed/db.
# Setting these env vars wraps the bounds as WindowBounds::Windowed and overrides
# any persisted Maximized/Fullscreen state on every launch.
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

**Why env-var override and not a dconf change to disable `auto-maximize`.** Disabling `auto-maximize` in `dconf-settings.ini` would prevent GNOME from auto-maximising apps in general. That sounds appealing, but (a) it doesn't fix the existing persisted Maximised state in `~/.zed-state/local-share/zed/db/...`, so users with stale state would still see the bug, and (b) it changes behaviour for all apps. The env-var override is **scoped to Zed only** and **also resets stale state on every launch** — strictly better.

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

1. Rebuild the Ubuntu desktop image (`./stack build-ubuntu`). The `zed_config.go` change is API-side and hot-reloads via Air on the next session start; the `start-zed-helix.sh` change ships in the desktop image.
2. **Reuse a session that currently shows the bug** — confirm the fix wipes the stale Maximised state on first launch (Zed comes up windowed even though the persisted state was Maximised).
3. Start a fresh session — confirm Zed launches as a centred ~1600×900 windowed window with title-bar buttons visible.
4. Open Chrome via the chrome-devtools MCP — confirm the window is ~1280×880 (page 1280×800 + chrome decorations) and that desktop sites (e.g. github.com) still render in desktop mode.
5. Maximise Zed manually, then close and reopen the session — confirm Zed comes up windowed again (env override beats persisted state on every launch). This is the regression-prevention check.
6. Drag Zed to fill the screen by hand — confirm normal GNOME behaviour still works (title bar visible, can be unmaximised normally). Note: this windowed-size is not persisted because the env override re-applies on every launch.
7. Stream the desktop in a small browser viewport (≈1280×720) — confirm Zed is no longer clipped at the bottom.

## Notes for future agents

- **Zed has `ZED_WINDOW_SIZE` and `ZED_WINDOW_POSITION` env vars** that beat persisted bounds. Format: `WIDTH,HEIGHT` and `X,Y` (integer pixels, comma-separated). Implementation in `crates/workspace/src/workspace.rs:171-183` and `:8011-8017`. Use these whenever you need to force a window size from outside Zed — don't patch `build_window_options`.
- **Zed state in this product is persistent.** `~/.config/zed`, `~/.local/share/zed`, `~/.cache/zed` are all symlinked to `$WORK_DIR/.zed-state/`, which is the workspace bind-mount. Anything Zed persists (window bounds, settings, ACP threads) survives container/session restarts. Don't assume "fresh container" means "fresh Zed state".
- **GNOME's `auto-maximize` is on by default** in the Helix Ubuntu desktop. Apps launching with a window ≥ work-area dimensions get silently promoted to Maximised. If a future app shows the same regression, the env-override pattern (or app-specific equivalent) is the cleaner fix than disabling `auto-maximize` globally.
- **There are two Helix desktops, Sway and Ubuntu/GNOME**, with separate `start-zed-helix.sh` wrappers (`desktop/sway-config/` and `desktop/ubuntu-config/`) that source the shared `desktop/shared/start-zed-core.sh`. Per-WM tweaks belong in the wrappers, not the shared core.
- **`dconf-settings.ini` (`desktop/ubuntu-config/dconf-settings.ini`) is loaded once at session start** via `dconf load /`. Settings here are baked into the GSettings DB before `gnome-shell` starts, so they take effect for the first window of every session.
- **The Chrome viewport task `53715951c` is not the cause of this Zed bug** but shares the auto-maximise dynamic, and the dimensions chosen there (`1600x1080`) were independently too big. Shrinking the viewport (here, to `1280x800`) is a one-line follow-on, not a revert.
- **`--viewport WxH` in `chrome-devtools-mcp` is the *page* size, not the *window* size.** Add ~80 px of chrome decorations to estimate the actual window footprint. The 1280-px page-width breakpoint is what desktop sites use to switch out of mobile mode (called out in the 001532 design doc).
