# Implementation Tasks: Fix Chrome Auto-Relaunch on Session Resume

- [~] In `desktop/sway-config/startup-app.sh` (around line 597-603), replace the `[ ... -lt 300 ]` freshness check with a plain `[ -f "$CHROME_MARKER" ]` existence check. Route the auto-launch decision and Chrome's own stdout/stderr to `/tmp/chrome-autolaunch.log` (append both branches: launching and skipping).
- [ ] In `desktop/ubuntu-config/startup-app.sh` (around line 313-335, **inside** the `<<GNOME_EOF` heredoc), make the same change, preserving the `\$` / `\${...}` / `\$(...)` heredoc escapes. Emit a `gow_log` line in both branches so the result also lands in the standard start log.
- [ ] Heartbeat blocks below the auto-launch in both files: leave untouched. They're correct.
- [ ] `bash -n desktop/sway-config/startup-app.sh` passes.
- [ ] For the Ubuntu file: verify the modified heredoc body still expands to syntactically valid bash (extract the heredoc with `sed -n '/<<GNOME_EOF/,/^GNOME_EOF$/p'`, strip the markers, `bash -n` the result). 002027 took the same step.
- [ ] Add a one-line breadcrumb at the top of `design/tasks/002027_can-we-persist-browser/design.md` pointing at this task: "See [002071](../002071_since-feature002027/) for the auto-relaunch fix — the 5-minute freshness check on the marker mtime was wrong across container restarts." Optional but cheap.
- [ ] Write per-repo PR description in `pull_request_helix.md` inside this task directory, mirroring the 002027 format.
- [ ] **Deferred to reviewer (cannot self-test — restart kills the implementing agent's own session):** end-to-end test on `helix-sway`: open Chrome with tabs → stop session → wait > 5 min → resume → expect Chrome with tabs.
- [ ] **Deferred to reviewer:** same E2E test on `helix-ubuntu`.
- [ ] **Deferred to reviewer (arm64):** same E2E test on an arm64 sandbox running Chromium (which the Dockerfiles already symlink to `google-chrome-stable`, so this code path covers it with no additional changes) — confirm Chromium auto-relaunches with tabs and that `pgrep -x chromium` in the unchanged heartbeat block keeps the marker fresh.
- [ ] **Deferred to reviewer:** close Chrome cleanly mid-session, wait > 30 s for heartbeat, stop, resume → expect Chrome stays closed.
- [ ] **Deferred to reviewer:** inspect `/tmp/chrome-autolaunch.log` after each resume to confirm the script logged whether it tried to launch and what (if anything) Chrome wrote.
