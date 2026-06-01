# Implementation Tasks: Diagnose Helix→Zed Theme Sync Stuck After Toggle Post-Resume

## Phase 1 — Reproduce and Capture

- [ ] In the inner Helix, start a session, wait for desktop, close it, then resume.
- [ ] Confirm the deployed image tag: `cat /home/retro/work/helix/sandbox-images/helix-ubuntu.version` and cross-check the sandbox has it: `docker compose -f /home/retro/work/helix/docker-compose.dev.yaml exec -T sandbox-nvidia docker images helix-ubuntu --format "{{.Tag}}"`.
- [ ] Verify the running daemon binary corresponds to a commit at or after `8053d6948`: `docker exec <ubuntu-external-container> /usr/local/bin/settings-sync-daemon --version` (or compare binary `mtime` if no version flag).
- [ ] Capture daemon logs from container start through three toggles: `docker compose exec sandbox-nvidia docker logs -f <name> 2>&1 | grep -E "config event|config_changed|applied GNOME|Updated settings.json|theme"`
- [ ] Toggle Helix dark → light → dark → light (4 transitions). After each, record: GNOME `gsettings get org.gnome.desktop.interface color-scheme`, contents of `~/.config/zed/settings.json` `.theme` field, and Zed's visibly-rendered theme.
- [ ] Save captures under `screenshots/` and `logs/` in this task directory.

## Phase 2 — Root-cause

- [ ] Compare per-toggle: did the daemon write the correct `theme` value? did Zed's rendered theme change?
- [ ] Classify the failure against hypotheses H1–H5 in design.md (or document H6 if none fit).
- [ ] Update this task's design.md with the confirmed hypothesis, evidence, and dismissed alternatives.

## Phase 3 — Fix (conditional on Phase 2)

### If H1 (Zed-side reactive-reload / structured-theme stale state)
- [ ] Open a follow-up task in helix-specs targeting the `zed` repo for the Zed-side bug.
- [ ] In the daemon, switch to writing `theme` as a structured `{mode: "...", light: "...", dark: "..."}` value that matches the user's color scheme, so Zed's `Dynamic` selection path picks the right slot deterministically — only if writing a bare string proves insufficient.

### If H2 (stale deployed image)
- [ ] Re-run `./stack build-ubuntu` and verify the transfer to the sandbox (per `2026-03-12-settings-sync-daemon-fixes.md` recommendations: re-run `./stack transfer-ubuntu-to-sandbox`, confirm image is present in sandbox docker before declaring success).
- [ ] Document the failure path so future operators catch it.

### If H3 (session-resume timing race)
- [ ] Either: have the daemon's initial `syncFromHelix()` complete synchronously before Zed launches (sequencing change in `start-zed-core.sh`), OR have the daemon proactively re-write `settings.json` once Zed signals readiness.

### If H4 (own-write guard race)
- [ ] Replace the `time.Since(d.lastModified) < 1*time.Second` heuristic with a deterministic guard — e.g. a counter of pending self-writes, decremented when the matching fsnotify event arrives.

### If H5 (user has a custom theme — working as designed)
- [ ] Add a tooltip / inline help in the Helix UI's color-scheme toggle explaining that the inner Zed theme is preserved when the user has selected a custom theme outside `One Light` / `Ayu Dark`.
- [ ] Consider a UI affordance to "reset Zed theme to Helix-managed" — out of scope unless requested.

## Phase 4 — Observability (do this regardless of which hypothesis lands)

- [~] Add a single structured INFO log line emitted by `effectiveTheme` (or its callers) describing the branch taken: `branch=managed_overwrite|preserve_custom|structured_replace|no_api_theme`, on-disk value, written value, API value.
- [ ] Audit `mergeSettings` for any path that touches `theme` without going through `effectiveTheme`; if any exist, route them through the helper.
- [ ] Add unit tests in `main_test.go` covering: bare-string managed theme, bare-string custom theme, structured `{mode,light,dark}` on disk, missing settings file, unparseable settings file.

## Phase 5 — Verify

- [ ] In the inner Helix, after the fix, repeat Phase 1's capture sequence. Each of the 4 transitions must flip Zed within ~2s; `~/.config/zed/settings.json`'s `.theme` field must match the chosen color scheme; Zed's rendered theme must match.
- [ ] Update `pull_request_helix.md` with summary, refs to 001998, and the confirmed root cause.
