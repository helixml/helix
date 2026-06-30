# Design: Fix Zed Theme Not Updating on Repeated Light/Dark Toggle

## Summary

The Helix UI light/dark button drives the user's `color_scheme` preference all
the way to a freshly-written Zed `settings.json`. The Helix side of that pipeline
is correct and writes the right theme on **every** toggle. The bug is purely on
the **Zed side**: its user-settings file watcher only observes the *first* write
because the Helix daemon writes via atomic rename (which replaces the inode), and
Zed watches the file inode without re-arming after it is replaced. The fix makes
Zed's settings watcher resilient to atomic-rename replacement; a daemon-side
inode-preserving write is documented as an alternative.

## Full pipeline (verified end-to-end)

```
Helix UI moon/sun button
  helix/frontend/src/components/system/Page.tsx:342  → onClick=toggleMode
  helix/frontend/src/contexts/theme.tsx:236          → api.v1UsersMeColorSchemeUpdate({color_scheme})
        ↓ PUT /api/v1/users/me/color-scheme
  helix/api/.../user_handlers.go:403 updateUserColorScheme
        persists UserMeta.Config.ColorScheme; publishUserColorSchemeChange()
        ↓ WS "config_changed" {field:color_scheme}
  settings-sync-daemon  helix/api/cmd/settings-sync-daemon/main.go
    runConfigEventLoop (1082) → syncFromHelix (937)
      GET /sessions/{id}/zed-config
        API derives theme: light→"One Light", dark→"Ayu Dark"
        helix/api/pkg/server/zed_config_handlers.go:325-329
      effectiveTheme(config.Theme) (1225)  → managed_overwrite → "One Light"/"Ayu Dark"
      applyGNOMEColorScheme(config.ColorScheme) (1182)
      writeSettings(...) (1880)  → write settings.json.tmp, os.Rename over settings.json  ← ATOMIC RENAME (new inode)
        ↓ file change
  Zed  watch_settings_files → SettingsStore::watch_settings_files
        crates/settings/src/settings_store.rs:359 → watch_config_file (per FILE)
        crates/settings/src/settings_file.rs:169   → fs.watch(file); reload on each event
        crates/fs/src/fs.rs:1084 RealFs::watch      → watcher.add(FILE only; parent added only for symlinks 1088-1104)
        crates/fs/src/fs_watcher.rs:197,764         → native inotify, NonRecursive, on the file inode
```

## Root cause

`RealFs::watch` registers a **native inotify watch on the settings.json inode**
(regular file → no parent-directory watch; the parent is watched only when the
path is a symlink). The Helix daemon's `writeSettings` replaces that inode on
every write (`os.WriteFile(tmp)` + `os.Rename(tmp, settings.json)`,
main.go:1899-1906).

inotify semantics: when the watched inode is unlinked by the rename, the kernel
delivers a final event (Removed) and then `IN_IGNORED`, which tears the watch
down. Zed's `watch_config_file` reloads on that one Removed event (so the **first**
toggle works), but nothing re-arms the watch on the new inode (`fs_watcher.rs`
only has re-arm/poll logic for paths that don't exist *at add time*, via
`add_pending_path`/`poll_path_until_created` — not for re-registration after a
Removed). Therefore **every toggle after the first produces no event and Zed
never re-reads settings.json.**

This precisely matches the symptom ("changes on the first transition but doesn't
change back") and explains why prior daemon-side fixes failed: the daemon was
already writing the correct theme each time — Zed simply stopped reading it.
(The same fault silently affects any post-first settings.json change, e.g.
in-place agent switches and manual external edits by atomic-saving editors.)

## Why prior attempts missed it

All prior work was daemon-side (see main.go comments around HELIX_MANAGED_THEMES
1206-1214, effectiveTheme 1216-1278, the removed USER_PREFERENCE_FIELDS 1289-1297,
structured_replace 1264-1269). Each made the *written value* more correct, but the
value was never the problem — Zed not re-reading was. There is also no test: Zed's
existing watcher test only covers the symlink-parent case
(`settings_file.rs:63 test_watch_config_file_reloads_when_parent_dir_is_symlink`),
never atomic-rename replacement of a regular file.

## Approach

### Step 1 — Confirm (cheap, decisive)
Reproduce by toggling twice and observe:
- Daemon log shows `theme sync: branch=managed_overwrite wrote="Ayu Dark"...` and
  `Updated settings.json` on BOTH toggles (proves daemon writes correctly twice).
- `ls -i ~/.config/zed/settings.json` before/after a toggle shows the inode number
  changing (proves atomic-rename inode replacement).
- Zed reloads on the 1st toggle but not the 2nd (proves the watcher is the break).

### Step 2 — Fix (primary, Zed-side: make the watcher survive inode replacement)
Make the user/global settings watch resilient to atomic rename. Preferred:
watch the **parent directory** for regular files (filtering events to the target
path), mirroring what the daemon itself already does for the same reason
(main.go:1650-1656 "watch the directory ... os.Rename replaces the inode, making a
file-level watcher permanently dead"). Zed already has `watch_config_dir`
(`settings_file.rs:200`) which does exactly per-file filtering over a directory
watch — route user/global settings through a directory-based watch, or extend
`RealFs::watch` to add the parent watch for regular files too. Alternative within
the same step: re-arm the native watch after a Removed event (re-call
`watcher.add(path)`), but the directory-watch approach is simpler and also covers
the brief window where the file does not exist mid-rename.

Keep the change minimal and upstream-shaped (it is a general watcher-robustness
fix, candidate for upstreaming); record any modified upstream files in
`portingguide.md`.

### Step 3 — Alternative / defense-in-depth (Helix daemon-side)
If the Zed change is deemed too invasive for now, make `writeSettings`
inode-preserving: truncate-and-write the existing `settings.json` in place rather
than write-tmp-then-rename, so the existing inotify watch keeps firing. Trade-off:
loses write atomicity (a reader could observe a partial file); mitigate by keeping
the JSON small and accepting that Zed re-reads on the next event and tolerates a
transient parse error. This is smaller and Helix-owned but strictly worse than
fixing the watcher; prefer Step 2.

### Step 4 — Regression test (required)
Add a Zed test (alongside `settings_file.rs` watcher tests, using `RealFs` + a
tempdir) that:
1. Creates a file, starts `watch_config_file`, drains the initial load.
2. Writes new contents via tmp-file + rename (atomic replace) and asserts a reload
   is received.
3. Repeats the atomic replace a 2nd and 3rd time and asserts a reload is received
   each time (this is what fails today).
If the chosen fix is daemon-side instead, add a Go test asserting `writeSettings`
keeps the inode stable across writes.

## Files in scope

| File | Role |
|------|------|
| `zed: crates/fs/src/fs.rs` (`RealFs::watch`, ~1084) | **Primary fix** — add parent-dir watch for regular files |
| `zed: crates/settings/src/settings_file.rs` (`watch_config_file` 169 / `watch_config_dir` 200) | Watcher wiring + regression test |
| `zed: crates/fs/src/fs_watcher.rs` | Native watch registration / optional re-arm |
| `helix: api/cmd/settings-sync-daemon/main.go` (`writeSettings` 1880) | Alternative inode-preserving write (Step 3) |

## Risks & notes

- The Zed-side watcher change touches shared `fs`/`settings` code; scope it to the
  regular-file watch path and lean on the existing `watch_config_dir` per-file
  filtering to limit blast radius. Test other config watchers (keymap, global
  settings) still behave.
- This is the Helix Zed fork; building Zed requires `./stack build-zed release`
  on ARM, then `build-ubuntu`, and bumping `sandbox-versions.txt` with the new
  ZED_COMMIT (see CLAUDE.md). A daemon-only fix (Step 3) instead needs
  `./stack build-ubuntu` + a new session.
- Verify end-to-end in the inner Helix (register, create a spec task so a live Zed
  session exists, click the toggle ≥ 3 times) — not just via unit test.
