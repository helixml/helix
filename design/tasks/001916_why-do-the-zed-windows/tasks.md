# Implementation Tasks

- [ ] In `desktop/ubuntu-config/start-zed-helix.sh`, add `export ZED_WINDOW_SIZE="${ZED_WINDOW_SIZE:-1600,900}"` and `export ZED_WINDOW_POSITION="${ZED_WINDOW_POSITION:-160,90}"` just before the `source "$CORE_SCRIPT"` line, with a short comment explaining they override GNOME auto-maximize and stale persisted bounds
- [ ] In `api/pkg/external-agent/zed_config.go` (~line 253), change `--viewport` value from `1600x1080` to `1280x800` and update the adjacent comment to note the new dimensions are still wide enough for desktop-mode rendering and small enough that Chrome doesn't trip Mutter's auto-maximise
- [ ] Rebuild the Ubuntu desktop image: `./stack build-ubuntu` (the `zed_config.go` change is API-side and Air hot-reloads on next session start)
- [ ] Reuse a session that currently shows the bug; confirm the fix wipes the stale Maximised state on first launch (Zed comes up windowed even though the persisted state was Maximised)
- [ ] Start a fresh session; confirm Zed launches as a centred ~1600×900 windowed window with title bar visible and ample margin around it
- [ ] Open Chrome via the chrome-devtools MCP; confirm the window is ~1280×880 and desktop sites (e.g. github.com) still render in desktop mode
- [ ] Manually maximise Zed, close the session, reopen it; confirm Zed comes up windowed again (env override beats persisted state on every launch)
- [ ] Stream the desktop in a small browser viewport (≈1280×720) and confirm Zed is no longer clipped at the bottom when un-maximised
- [ ] Verify Sway sessions are unaffected (no changes to Sway config)
- [ ] Open a PR titled `Force windowed initial size for Zed and shrink Chrome MCP viewport in Ubuntu desktop` referencing task 001916
