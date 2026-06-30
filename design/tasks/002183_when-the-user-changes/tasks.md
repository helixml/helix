# Implementation Tasks: Fix Zed Theme Not Updating on Repeated Light/Dark Toggle

## Implementation decision (see design.md "Implementation Notes")
Chosen fix: **daemon-side inode-preserving write** in the settings-sync-daemon
(primary repo `helix`). It is the correct root-cause fix for the Helix-driven
theme sync (Zed watches the settings.json inode; the daemon must stop replacing
that inode) and is end-to-end testable in helix-in-helix as the reviewer
required. The broader Zed-watcher hardening is recorded as a follow-up.

## Tasks
- [x] Confirm root cause from code trace (daemon atomic-rename replaces inode; Zed file-inode watch dies after first replacement).
- [x] Change `writeSettings` in `helix/api/cmd/settings-sync-daemon/main.go` to truncate-and-write `settings.json` in place (preserve inode) instead of tmp-write + `os.Rename`.
- [x] Add a Go unit test asserting the inode (`Ino`) is stable across repeated `writeSettings` calls and the contents are correct. (`TestWriteSettingsPreservesInode`, passing.)
- [x] Build: `./stack build-ubuntu` (helix-ubuntu:551562, pushed to local registry).
- [~] End-to-end verify in inner Helix (start a NEW session so the new daemon is used).
- [ ] End-to-end verify in inner Helix: toggle light/dark ≥ 3 times via the UI button; confirm Zed's theme changes every time with no reload/restart.
- [ ] Confirm a user-picked custom Zed theme is still preserved (not clobbered) after toggling.
- [ ] Merge latest `origin/main` into the feature branch; push `feature/002183-fix-zed-theme-not`.
- [ ] Write per-repo PR description (`pull_request_helix.md`).

## Follow-up (not in this PR)
- [ ] Harden Zed's settings watcher (`crates/fs`/`crates/settings`) to survive atomic-rename inode replacement (watch parent dir / re-arm), so manual external edits and any future atomic-rename writer are also covered. Upstream candidate.
