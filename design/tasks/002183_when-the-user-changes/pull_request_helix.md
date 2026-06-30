# Fix Zed theme not updating on repeated light/dark toggle

## Summary
Clicking the Helix light/dark toggle (moon/sun) changed the embedded Zed editor's
theme on the first click but never again until Zed was restarted. The Helix side
of the pipeline was already writing the correct theme to `settings.json` on every
toggle — Zed simply stopped reading it after the first change.

Root cause: the settings-sync-daemon wrote `settings.json` via temp-file +
`os.Rename`, which **replaces the file's inode on every write**. Zed watches the
user `settings.json` by inode (its file watcher only adds a parent-directory
watch for symlinks; a regular file is watched by its own inode). inotify removes
that watch when the inode is replaced and never re-arms it, so only the first
daemon write per session was ever observed. This also explains why several prior
daemon-side fixes (HELIX_MANAGED_THEMES, effectiveTheme, USER_PREFERENCE_FIELDS)
never resolved it — the written value was never the problem.

Fix: `writeSettings` now writes in place (`O_WRONLY|O_CREATE|O_TRUNC` + `Write` +
`Sync`) instead of temp-file + rename, keeping the inode stable so Zed's inotify
watch survives across every write. Reads remain safe: Zed debounces file events
~100ms before loading and we write the small JSON in a single `Write`/`Sync`.

NOTE: trigger is the in-UI button driving the user's `color_scheme` preference —
not the OS appearance toggle. No OS/portal appearance path is touched.

## Changes
- `api/cmd/settings-sync-daemon/main.go`: `writeSettings` writes settings.json in
  place (inode-preserving) instead of atomic rename; comment explains why.
- `api/cmd/settings-sync-daemon/main_test.go`: add `TestWriteSettingsPreservesInode`
  asserting the inode stays stable and contents are correct across repeated writes.

## Testing
- `go test ./cmd/settings-sync-daemon/` passes (incl. the new inode test).
- End-to-end in helix-in-helix: rebuilt the desktop image (`build-ubuntu`), started
  a fresh spec-task session, toggled light/dark repeatedly and confirmed Zed's
  theme switches every time. (See task design doc for details.)

## Follow-up (not in this PR)
- Harden Zed's settings watcher to survive atomic-rename inode replacement (watch
  parent dir / re-arm) so manual external edits by atomic-saving editors are also
  covered. Upstream candidate.
