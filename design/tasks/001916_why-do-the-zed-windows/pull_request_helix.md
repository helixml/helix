# Force proportional windowed initial size for Zed; shrink Chrome MCP viewport

## Summary

Zed has been launching maximised in every new spectask on the Ubuntu/GNOME desktop, and unmaximising it leaves a window taller than the streamed viewport. Root cause: upstream Zed commit [`a0d0195ca9`](https://github.com/zed-industries/zed/pull/52940) ("Add onboarding for parallel agents", 2026-04-07) bumped `DEFAULT_WINDOW_SIZE` from `1536×864` to `1536×1095`. That commit landed in the Helix Zed fork via the 001864 merge on 2026-04-24. On a 1920×1080 virtual monitor `default_bounds()` clips the new value to `1536×1080` — exactly the work-area height — which trips Mutter's `auto-maximize=true` and the user sees a fullscreen Zed.

Coincidentally, the chrome-devtools MCP `--viewport 1600x1080` set in task 001532 produced a ~1600×1160 Chrome window that also tripped the same auto-maximise threshold; shrinking it is a one-line follow-on that gets Chrome out of the same trap.

## Changes

- **`desktop/ubuntu-config/start-zed-helix.sh`**: export `ZED_WINDOW_SIZE` and `ZED_WINDOW_POSITION` computed dynamically as 80% of `GAMESCOPE_WIDTH/HEIGHT` divided by `GDK_SCALE`, with a 10% margin on each side. Zed wraps env-var bounds as `WindowBounds::Windowed`, which skips the auto-maximise path entirely. Formula lands on `1536×864` at 1920×1080/100% — exactly the dimensions Zed defaulted to before `a0d0195ca9` — and scales proportionally to 4K (3072×1728), 5K (2048×1152 at 200% zoom), etc.
- **`api/pkg/external-agent/zed_config.go`**: shrink the chrome-devtools MCP viewport from `1600x1080` to `1280x800`. `1280` is the canonical desktop-vs-mobile breakpoint, so sites still render in desktop mode, and the resulting Chrome window leaves a wide margin on a 1920×1080 monitor.

The Sway desktop is untouched — its tiling behaviour is a different dynamic and out of scope for this task.

## Test plan

- [ ] Fresh spectask at default 1920×1080 / 100% zoom: Zed launches centred 1536×864 at origin (192, 108), not maximised
- [ ] Spectask with `GAMESCOPE_WIDTH=3840 GAMESCOPE_HEIGHT=2160`: Zed scales to ~3072×1728 centred
- [ ] Spectask with `HELIX_ZOOM_LEVEL=200`: Zed at the scaled logical size
- [ ] Chrome opened via chrome-devtools MCP: window ~1280×880, desktop sites render in desktop mode
- [ ] Stream in a small browser viewport (≈1280×720) on a 1920×1080 session: Zed not clipped at the bottom
- [ ] Sway sessions: no behaviour change

Spec: `helix-specs:001916_why-do-the-zed-windows`
