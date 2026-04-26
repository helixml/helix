# Implementation Tasks

- [ ] In `desktop/ubuntu-config/start-zed-helix.sh`, add `export ZED_WINDOW_SIZE="${ZED_WINDOW_SIZE:-1600,900}"` and `export ZED_WINDOW_POSITION="${ZED_WINDOW_POSITION:-160,90}"` just before the `source "$CORE_SCRIPT"` line, with a short comment explaining they keep Zed below GNOME's auto-maximise threshold on first launch
- [ ] In `api/pkg/external-agent/zed_config.go` (~line 253), change `--viewport` value from `1600x1080` to `1280x800` and update the adjacent comment to note the new dimensions still trigger desktop-mode rendering and keep Chrome below Mutter's auto-maximise threshold
- [ ] Rebuild the Ubuntu desktop image: `./stack build-ubuntu` (the `zed_config.go` change is API-side and Air hot-reloads on next spectask start)
- [ ] Start a fresh spectask; confirm Zed launches as a centred ~1600×900 windowed window with title bar visible and ample margin around it
- [ ] Open Chrome via the chrome-devtools MCP; confirm the window is ~1280×880 and desktop sites (e.g. github.com) still render in desktop mode
- [ ] Stream the desktop in a small browser viewport (≈1280×720) and confirm Zed is not clipped at the bottom when un-maximised
- [ ] Verify Sway sessions are unaffected (no changes to Sway config)
- [ ] Open a PR titled `Force windowed initial size for Zed and shrink Chrome MCP viewport in Ubuntu desktop` referencing task 001916
