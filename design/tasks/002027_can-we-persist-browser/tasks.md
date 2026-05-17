# Implementation Tasks: Persist Browser State and Auto-Restore on Session Resume

- [x] In `desktop/shared/helix-workspace-setup.sh`, add a `~/.config/google-chrome` and `~/.config/chromium` symlink block targeting `$WORK_DIR/.chrome-state`, mirroring the existing `.claude` block (lines 535-567). Include the "preserve existing ephemeral files before symlinking" cp step. Seed first-run sentinels on initial setup so the "skip welcome dialog" behaviour still works.
- [x] In `Dockerfile.sway-helix`, change `"RestoreOnStartup": 5` → `1` in the Chrome policy JSON and add `"RestoreOnStartup":1` to the arm64 Chromium policy JSON.
- [x] In `Dockerfile.ubuntu-helix`, make the same `RestoreOnStartup` changes (Chrome and Chromium policy blocks).
- [~] In `desktop/sway-config/startup-app.sh`, add an auto-launch block + heartbeat-touch loop for Chrome (after Sway is up and Chrome workspace assignment rules are written).
- [ ] Mirror the same auto-launch + heartbeat-touch block in `desktop/ubuntu-config/startup-app.sh`.
- [ ] Manual test on `helix-sway`: open Chrome with 3 distinct tabs, restart container, verify tabs restored and Chrome is up on workspace 3 without user action.
- [ ] Manual test on `helix-ubuntu` (GNOME): same scenario.
- [ ] Manual test: close Chrome before container restart, verify Chrome stays closed on next session.
- [ ] Verify `ls -la /home/retro/.config/google-chrome` inside a running container shows a symlink to `/home/retro/work/.chrome-state` (and `~/.config/chromium` similarly on arm64).
- [ ] Add a short note to `desktop/sway-config/SWAY-USER-GUIDE.md` explaining that browser state now persists across sessions and that any saved passwords (Chrome on Linux uses `--password-store=basic`, unencrypted) will also persist.
- [ ] Write per-repo PR description in `pull_request_helix.md`.
