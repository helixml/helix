# Implementation Tasks: Fix Chrome Auto-Relaunch on Session Resume

- [x] In `desktop/sway-config/startup-app.sh` (around line 597-603), replace the `[ ... -lt 300 ]` freshness check with a plain `[ -f "$CHROME_MARKER" ]` existence check. Route the auto-launch decision and Chrome's own stdout/stderr to `/tmp/chrome-autolaunch.log` (append both branches: launching and skipping).
- [x] In `desktop/ubuntu-config/startup-app.sh` (around line 313-335, **inside** the `<<GNOME_EOF` heredoc), make the same change, preserving the `\$` / `\${...}` / `\$(...)` heredoc escapes. Emit a `gow_log` line in both branches so the result also lands in the standard start log.
- [x] Heartbeat blocks below the auto-launch in both files: leave untouched. They're correct.
- [x] `bash -n desktop/sway-config/startup-app.sh` passes; also ran `bash -n desktop/ubuntu-config/startup-app.sh`.
- [x] For the Ubuntu file: verify the modified heredoc body still expands to syntactically valid bash. Extracted heredoc body, applied the `\$` → `$` unescaping that bash does at heredoc-write time, then `bash -n` on the result — passes.
- [x] Add a one-line breadcrumb at the top of `design/tasks/002027_can-we-persist-browser/design.md` pointing at this task.
- [~] Write per-repo PR description in `pull_request_helix.md` inside this task directory, mirroring the 002027 format.
- [ ] **Deferred to reviewer (cannot self-test — restart kills the implementing agent's own session):** end-to-end test on `helix-sway`: open Chrome with tabs → stop session → wait > 5 min → resume → expect Chrome with tabs.
- [ ] **Deferred to reviewer:** same E2E test on `helix-ubuntu`.
- [ ] **Deferred to reviewer (arm64):** same E2E test on an arm64 sandbox running Chromium (which the Dockerfiles already symlink to `google-chrome-stable`, so this code path covers it with no additional changes) — confirm Chromium auto-relaunches with tabs and that `pgrep -x chromium` in the unchanged heartbeat block keeps the marker fresh.
- [ ] **Deferred to reviewer:** close Chrome cleanly mid-session, wait > 30 s for heartbeat, stop, resume → expect Chrome stays closed.
- [ ] **Deferred to reviewer:** inspect `/tmp/chrome-autolaunch.log` after each resume to confirm the script logged whether it tried to launch and what (if anything) Chrome wrote.
