# Implementation Tasks: Persist Browser State and Auto-Restore on Session Resume

- [x] In `desktop/shared/helix-workspace-setup.sh`, add a `~/.config/google-chrome` and `~/.config/chromium` symlink block targeting `$WORK_DIR/.chrome-state`, mirroring the existing `.claude` block (lines 535-567). Include the "preserve existing ephemeral files before symlinking" cp step. Seed first-run sentinels on initial setup so the "skip welcome dialog" behaviour still works.
- [x] In `Dockerfile.sway-helix`, change `"RestoreOnStartup": 5` → `1` in the Chrome policy JSON and add `"RestoreOnStartup":1` to the arm64 Chromium policy JSON.
- [x] In `Dockerfile.ubuntu-helix`, make the same `RestoreOnStartup` changes (Chrome and Chromium policy blocks).
- [x] In `desktop/sway-config/startup-app.sh`, add an auto-launch block + heartbeat-touch loop for Chrome (after Sway is up and Chrome workspace assignment rules are written).
- [x] Mirror the same auto-launch + heartbeat-touch block in `desktop/ubuntu-config/startup-app.sh`.
- [x] Add a short note to `desktop/sway-config/SWAY-USER-GUIDE.md` explaining that browser state now persists across sessions and that any saved passwords (Chrome on Linux uses `--password-store=basic`, unencrypted) will also persist.
- [x] Bash syntax check (`bash -n`) on all three modified shell scripts + heredoc expansion check for `desktop/ubuntu-config/startup-app.sh` start_gnome body. All pass.
- [x] Write per-repo PR description in `pull_request_helix.md`.
- [ ] **Deferred to reviewer (cannot self-test):** open Chrome on `helix-sway` with 3 tabs → restart container → verify tabs restored and Chrome on workspace 3. Restarting kills the implementing agent's own session, see design.md "Why manual verification can't run from the implementing agent".
- [ ] **Deferred to reviewer:** same end-to-end test on `helix-ubuntu` (GNOME).
- [ ] **Deferred to reviewer:** close Chrome before container restart → verify Chrome stays closed.
- [ ] **Deferred to reviewer (arm64):** confirm `~/.config/chromium` symlink path works using XtraDeb-PPA Chromium → google-chrome-stable wrapper.
