# Design: Float Zed (and key apps) by default in Sway

## Root cause

The Helix Desktop's Sway config (`desktop/sway-config/config` + the dynamically-appended config in `desktop/sway-config/startup-app.sh`) contains **no `for_window ... floating enable` rules**. In Sway, that means every window — Zed, Chrome, kitty, ghostty — tiles by default and fills its assigned workspace.

Workspace assignment from `startup-app.sh:362-374`:

```
assign [app_id="dev.zed.Zed-Dev"] workspace number 1
assign [class="Zed"]              workspace number 1
assign [app_id="kitty"]           workspace number 2
assign [app_id="google-chrome"]   workspace number 3
assign [class="Google-chrome"]    workspace number 3
```

Each app is the only window in its workspace → tiled to 100% of the workspace = full screen (minus the 8px border set at `startup-app.sh:381`).

The headless output is `${GAMESCOPE_WIDTH}x${GAMESCOPE_HEIGHT}` (default `1920x1080`, `startup-app.sh:511-512`). If the user's browser viewport is shorter than `GAMESCOPE_HEIGHT`, Zed appears "taller than the screen".

**Why the Chrome viewport task (001532) made this visible now:** before commit `53715951c`, chrome-devtools-mcp was launching Chrome at its built-in default ~921×896 (because the env-var viewport setting was a no-op). At that small size the user's eye read Chrome as a "windowed" app even though Sway was technically tiling it. After the fix Chrome's CDP `page.resize({contentWidth:1600,contentHeight:1080})` makes Chrome physically big — and then the user notices that *every* window in this desktop is the same way, including Zed. Zed's tiling behavior didn't change; the user's reference point did.

## Fix: add Sway floating rules

Append to the dynamically-generated Sway config in `desktop/sway-config/startup-app.sh`, near the existing `assign [...]` and `default_border` blocks (~lines 362–384):

```bash
# Default to floating, sub-screen-sized windows so the desktop feels
# windowed rather than tiled-fullscreen. Users can still maximize/fullscreen
# manually with $mod+Shift+space / $mod+f.
echo "for_window [app_id=\"dev.zed.Zed-Dev\"] floating enable, resize set 1600 900, move position center" >> $HOME/.config/sway/config
echo "for_window [class=\"Zed\"]              floating enable, resize set 1600 900, move position center" >> $HOME/.config/sway/config
echo "for_window [app_id=\"google-chrome\"]   floating enable, resize set 1600 900, move position center" >> $HOME/.config/sway/config
echo "for_window [class=\"Google-chrome\"]    floating enable, resize set 1600 900, move position center" >> $HOME/.config/sway/config
echo "for_window [app_id=\"kitty\"]           floating enable, resize set 1100 700, move position center" >> $HOME/.config/sway/config
echo "for_window [app_id=\"ghostty\"]         floating enable, resize set 1100 700, move position center" >> $HOME/.config/sway/config
echo "for_window [app_id=\"acp-log-viewer\"]  floating enable, resize set 1100 700, move position center" >> $HOME/.config/sway/config
```

Sizes chosen for a `1920×1080` headless output:
- Zed/Chrome `1600×900` → leaves a ~160×90 margin on each side, comfortably below `GAMESCOPE_HEIGHT=1080` so it can never appear "taller than the screen".
- Terminals `1100×700` → smaller because users typically want them as a side panel.

## Decisions and rationale

**Why fix in Sway config, not in Zed.** `build_window_options` in `crates/zed/src/zed.rs:309` already sets `window_bounds: None` and defers to the WM. The user might also have other apps with the same regression (Chrome already complained about); the WM is the natural single point of control.

**Why floating + explicit size, not just floating.** Sway's `floating enable` alone keeps the window's *requested* size. Zed and Chrome will request very large sizes when given a 1920×1080 output; we need an explicit `resize set` to bound them.

**Why not also revert the Chrome viewport.** Task 001532 was a deliberate fix — Chrome's *page* viewport should be 1600×1080 so desktop sites render in desktop mode. The new floating rule on the Chrome window is independent of (and complementary to) the page viewport.

**Why no fullscreen/maximize keybinding.** Sway already provides:
- `$mod+f` → fullscreen toggle
- `$mod+Shift+space` → toggle floating ↔ tiling

These work out of the box; no new bindings needed.

## Files to change

| File | Change |
|---|---|
| `desktop/sway-config/startup-app.sh` | Append the 7 `for_window ... floating enable, resize set ...` lines after the existing `assign` block (around line 374) and before the `default_border` block (around line 381). |

No changes in `/home/retro/work/zed/`. No changes in `api/`.

## Verification

1. Rebuild the sway-helix desktop image and start a fresh session.
2. Confirm Zed launches as a centred ~1600×900 floating window with visible borders on all sides.
3. Confirm `$mod+f` still toggles fullscreen, `$mod+Shift+space` still toggles tiling.
4. Open Chrome from waybar → confirm it also opens floating ~1600×900 (not full screen).
5. Open kitty (`$mod+Shift+Return`) → confirm it floats at ~1100×700.
6. Stream the desktop in a small browser window (e.g., 1280×720) → Zed window is no longer clipped at the bottom.

## Notes for future agents

- **Sway tiling is the default** in this codebase. Any new app launched in the desktop will tile to fill its workspace unless a `for_window ... floating enable` rule is added in `startup-app.sh`. Add a rule whenever you introduce a new GUI app.
- **All Sway config lives in `startup-app.sh` (dynamic, runs every session)** — the static `desktop/sway-config/config` is included *very* early by GOW and only sets things that don't depend on `$mod` or env vars.
- **Window sizing in Zed itself is WM-driven.** `build_window_options` returns `window_bounds: None`. Don't try to "fix" window sizing in Zed code; fix it in the compositor.
- **`GAMESCOPE_WIDTH`/`GAMESCOPE_HEIGHT` set the inner desktop resolution.** They default to `1920x1080`. The streaming layer scales/crops to the user's browser viewport, so any window taller than `GAMESCOPE_HEIGHT` will be clipped.
