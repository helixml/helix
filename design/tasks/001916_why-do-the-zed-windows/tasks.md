# Implementation Tasks

- [x] In `desktop/ubuntu-config/start-zed-helix.sh`, add the dynamic `ZED_WINDOW_SIZE`/`ZED_WINDOW_POSITION` block from `design.md` (computes 80% of `GAMESCOPE_WIDTH/HEIGHT` divided by `GDK_SCALE`, with a 10% margin) just before the `source "$CORE_SCRIPT"` line, with a short comment explaining the upstream `a0d0195ca9` regression and why these env vars sidestep GNOME auto-maximise
- [x] In `api/pkg/external-agent/zed_config.go` (~line 253), change `--viewport` value from `1600x1080` to `1280x800` and update the adjacent comment to note the new dimensions still trigger desktop-mode rendering and keep Chrome below Mutter's auto-maximise threshold
- [x] Rebuild + verify deferred — user will test live (`build-ubuntu` for the start-zed-helix.sh change; `zed_config.go` Air hot-reloads on next spectask start)
- [x] Code pushed on `feature/001916-why-do-the-zed-windows`; PR description written; user will open the PR from the UI

## Verification (user, live)

- Fresh spectask at default 1920×1080 / 100% zoom — Zed should launch centred 1536×864 at origin (192, 108)
- 4K display (`GAMESCOPE_WIDTH=3840 GAMESCOPE_HEIGHT=2160`) — Zed should scale to ~3072×1728 centred
- `HELIX_ZOOM_LEVEL=200` — Zed should appear at the scaled logical size
- Chrome via chrome-devtools MCP — window ~1280×880, desktop sites still render in desktop mode
- Stream in a small browser viewport (≈1280×720) on a 1920×1080 session — Zed not clipped at the bottom
- Sway sessions unaffected (no Sway changes)
