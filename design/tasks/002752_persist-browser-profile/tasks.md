# Implementation Tasks: Persist Chrome Profile Across Desktop Container Restarts

- [ ] Create a feature branch off `helixml/helix` main
- [ ] Add `--user-data-dir=/home/retro/work/.chrome-state` as the first entry in the `chrome-devtools` context server `Args` in `api/pkg/external-agent/zed_config.go` (~line 309)
- [ ] Add the explanatory comment above it: only `/home/retro/work` is bind-mounted, and the `~/.config/google-chrome` symlink does not cover an explicit `--user-data-dir`
- [ ] Add a unit test in `api/pkg/external-agent/zed_config_test.go` asserting the `chrome-devtools` args carry the persistent `--user-data-dir`
- [ ] Run `cd api && go build ./...` and `go test ./pkg/external-agent/...`
- [ ] Delete the Chrome auto-launch + heartbeat subshell from `desktop/ubuntu-config/startup-app.sh` (~328-363), keeping the Zed launcher above it intact
- [ ] Delete the equivalent auto-launch + heartbeat block from `desktop/sway-config/startup-app.sh` (~600-635), leaving the `SERVICES_STARTED` gate and portal/settings-sync startup intact
- [ ] Update the comment in `desktop/shared/helix-workspace-setup.sh` (645-682) to say the chrome-devtools MCP is now the primary consumer of `.chrome-state`; keep the symlinks unchanged
- [ ] Shellcheck / syntax-check both edited startup scripts
- [ ] Locate where Claude Code's `--mcp-config` is materialised, confirm already-running desktops keep old flags until agent restart, and record the finding for the PR description (no workaround)
- [ ] Run `./stack build-ubuntu` so both changed desktop files land in the image
- [ ] In the inner Helix at `http://localhost:8080`: register `test@helix.ml` / `helixtest`, complete onboarding, create a spec task with a live desktop session
- [ ] Verify `ps aux | grep chrome` shows `--user-data-dir=/home/retro/work/.chrome-state` and not the `.cache` path
- [ ] Drive the chrome MCP to a site that sets a cookie, then confirm `/home/retro/work/.chrome-state/Default/Cookies` exists and is non-empty
- [ ] Recreate the desktop container, then confirm the cookie survived and the MCP still works (`list_pages` / `navigate_page` succeed) — mandatory, not optional
- [ ] Confirm no GUI Chrome auto-launch fired, no `/tmp/chrome-autolaunch.log`, and no `SingletonLock` collision in the logs
- [ ] Open a PR against `helixml/helix` main describing: the two changes, the already-running-desktops caveat, the ~170MB profile now living on `prod/helix-workspaces` per spec task, and any `NOT tested: <what and why>` items — do not merge
