# Implementation Tasks: Persist Browser State and Auto-Restore on Session Resume

- [~] In `desktop/shared/helix-workspace-setup.sh`, add a `~/.config/google-chrome` and `~/.config/chromium` symlink block targeting `$WORK_DIR/.chrome-state`, mirroring the existing `.claude` block (lines 535-567). Include the "preserve existing ephemeral files before symlinking" cp step. Seed first-run sentinels on initial setup so the "skip welcome dialog" behaviour still works.
- [ ] In `Dockerfile.sway-helix`, change `"RestoreOnStartup": 5` → `1` in the Chrome policy JSON (line 707) and in the arm64 Chromium policy JSON (line 687).
- [ ] In `Dockerfile.ubuntu-helix`, make the same `RestoreOnStartup` change in the equivalent Chrome policy block.
- [ ] In `desktop/sway-config/startup-app.sh`, add (after Sway is up and Chrome workspace assignment rules are written, around line 469):
  - a background loop that, while a `google-chrome` process exists, touches `$WORK_DIR/.chrome-state/.was-running` every ~30s
  - a startup check that, if `.was-running` exists and is fresher than ~5 minutes, removes any stale `Singleton*` lock files and launches `google-chrome-stable &` in the background.
- [ ] Mirror the same auto-launch + heartbeat-touch block in `desktop/ubuntu-config/startup-app.sh`.
- [ ] Rebuild the affected images (`./stack build-ubuntu`, plus sway equivalent) and start a fresh session — existing containers keep the old policy/scripts.
- [ ] Manual test on `helix-sway`: open Chrome with 3 distinct tabs, restart container, verify tabs restored and Chrome is up on workspace 3 without user action.
- [ ] Manual test on `helix-ubuntu` (GNOME): same scenario.
- [ ] Manual test: close Chrome before container restart, verify Chrome stays closed on next session.
- [ ] Manual test on arm64 (Chromium): confirm the same flow works using the `chromium-browser` → `google-chrome-stable` compatibility symlink.
- [ ] Verify `ls -la /home/retro/.config/google-chrome` inside a running container shows a symlink to `/home/retro/work/.chrome-state` (and `~/.config/chromium` similarly on arm64).
- [ ] Add a short note to `desktop/sway-config/SWAY-USER-GUIDE.md` explaining that browser state (including any saved passwords stored via `--password-store=basic`) now persists across sessions.
