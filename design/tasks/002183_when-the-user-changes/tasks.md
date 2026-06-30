# Implementation Tasks: Fix Zed Theme Not Updating on Repeated Light/Dark Toggle

## Confirm the root cause
- [ ] In the inner Helix, open a live spec-task Zed session and click the light/dark toggle twice; confirm the theme changes on click 1 but not click 2.
- [ ] Tail the settings-sync-daemon log; confirm `theme sync: branch=managed_overwrite ...` and `Updated settings.json` appear on BOTH clicks (daemon writes correctly twice).
- [ ] Run `ls -i ~/.config/zed/settings.json` before and after a toggle; confirm the inode number changes (atomic-rename replacement).
- [ ] Confirm Zed re-reads settings.json on click 1 but not click 2 (the watcher is the break).

## Primary fix — make Zed's settings watcher survive atomic-rename replacement
- [ ] In `crates/fs/src/fs.rs` `RealFs::watch`, also register a watch on the parent directory for regular files (today only symlinks get it, lines ~1088-1104), filtering delivered events to the target path.
- [ ] Alternatively/additionally route user + global settings through directory-based watching using the existing `watch_config_dir` (`crates/settings/src/settings_file.rs:200`), which already filters per-file.
- [ ] Verify keymap and global-settings watchers still work and there is no duplicate-reload storm.

## Regression test (required)
- [ ] Add a Zed test next to the existing watcher tests in `crates/settings/src/settings_file.rs` using `RealFs` + a tempdir: start `watch_config_file`, then replace the file via tmp-write + rename three times, asserting a reload is delivered after EACH replacement (not just the first).

## Alternative (only if the Zed change is rejected) — daemon-side inode-preserving write
- [ ] Change `writeSettings` in `helix/api/cmd/settings-sync-daemon/main.go` to truncate-and-write `settings.json` in place (preserve inode) instead of tmp-write + `os.Rename`.
- [ ] Add a Go test asserting the inode is stable across repeated `writeSettings` calls.

## Build, verify, ship
- [ ] Build Zed: `./stack build-zed release`, then `./stack build-ubuntu`; start a NEW session.
- [ ] End-to-end verify in the inner Helix: toggle light/dark ≥ 3 times and confirm Zed's theme changes every time with no reload/restart.
- [ ] Confirm a user-picked custom Zed theme is still preserved (not clobbered) after toggling.
- [ ] If Zed changed: commit Zed, capture `git rev-parse HEAD`, bump `ZED_COMMIT` in `sandbox-versions.txt`, and follow the CLAUDE.md PR ordering (open Helix PR before pushing Zed branch).
- [ ] Record any modified upstream Zed files in `portingguide.md`.
- [ ] Check CI (Drone) green after pushing.
