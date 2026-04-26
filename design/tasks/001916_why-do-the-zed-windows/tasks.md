# Implementation Tasks

- [~] In `desktop/ubuntu-config/start-zed-helix.sh`, add the dynamic `ZED_WINDOW_SIZE`/`ZED_WINDOW_POSITION` block from `design.md` (computes 80% of `GAMESCOPE_WIDTH/HEIGHT` divided by `GDK_SCALE`, with a 10% margin) just before the `source "$CORE_SCRIPT"` line, with a short comment explaining the upstream `a0d0195ca9` regression and why these env vars sidestep GNOME auto-maximise
- [ ] In `api/pkg/external-agent/zed_config.go` (~line 253), change `--viewport` value from `1600x1080` to `1280x800` and update the adjacent comment to note the new dimensions still trigger desktop-mode rendering and keep Chrome below Mutter's auto-maximise threshold
- [ ] Rebuild the Ubuntu desktop image: `./stack build-ubuntu` (the `zed_config.go` change is API-side and Air hot-reloads on next spectask start)
- [ ] Start a fresh spectask at the default 1920×1080 / 100% zoom; confirm Zed launches as a centred 1536×864 windowed window at origin (192, 108)
- [ ] Start a spectask with a 4K display (`GAMESCOPE_WIDTH=3840 GAMESCOPE_HEIGHT=2160`); confirm Zed scales proportionally to ~3072×1728 centred and is still windowed
- [ ] Start a spectask with `HELIX_ZOOM_LEVEL=200`; confirm Zed is windowed at the appropriately scaled logical size
- [ ] Open Chrome via the chrome-devtools MCP; confirm the window is ~1280×880 and desktop sites (e.g. github.com) still render in desktop mode
- [ ] Stream the desktop in a small browser viewport (≈1280×720) on a 1920×1080 session and confirm Zed is not clipped at the bottom when un-maximised
- [ ] Verify Sway sessions are unaffected (no changes to Sway config)
- [ ] Open a PR titled `Force proportional windowed initial size for Zed and shrink Chrome MCP viewport in Ubuntu desktop` referencing task 001916
