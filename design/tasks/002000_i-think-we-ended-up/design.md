# Design

## Summary

Replace `window.minimize()` with `window.change_workspace_by_index(last, false)`
inside the `_onWindowCreated` handler of the
`helix-cursor@helix.ml` GNOME shell extension. Update the metadata
and comments to match.

## Files Touched

- `desktop/ubuntu-config/gnome-extension/helix-cursor@helix.ml/extension.js`
  — change handler logic, comments, log messages.
- `desktop/ubuntu-config/gnome-extension/helix-cursor@helix.ml/metadata.json`
  — update the `description` field, bump `version` from 2 → 3 so
  GNOME Shell reloads the manifest cleanly.

No changes to `dconf-settings.ini` (4 fixed workspaces is already
what we want), no changes to `start-zed-core.sh`.

## Key Decision: Move, Don't Minimize

GNOME Mutter exposes
[`Meta.Window.change_workspace_by_index(space_index, append)`](https://gjs-docs.gnome.org/meta14/meta.window#method-change_workspace_by_index).
Calling it with `append = false` moves the window onto the given
workspace without creating a new one (we have a fixed count, so we
must not append).

We deliberately do **not** call `window.activate()` or
`workspace.activate()` afterwards — the user starts on workspace 1
(Zed) and we want them to stay there. The window simply lives on
workspace 4 until the user navigates there.

## Determining "Last" Workspace

Use `global.workspace_manager.get_n_workspaces() - 1` rather than the
hard-coded constant `3`. Reasons:

- Defensive: if a future change reduces the workspace count, we don't
  blindly target a non-existent index.
- Symmetric with the rest of the extension's GNOME 45+ API usage
  (e.g. `global.backend.get_cursor_tracker()`).

If `n_workspaces < 2`, log a warning and bail — leaving the window
on its default workspace is preferable to either minimizing (the bug
we're fixing) or crashing the shell.

## Why the Old Behavior Misbehaves

`window.minimize()` is honored, but Mutter then bookkeeps the window
as "minimized but pending attention" because the terminal keeps
emitting output (the `tail -F` from `start-zed-core.sh:150`). Each
new line triggers an `urgent` / `demands-attention` hint, which the
Ubuntu Dock surfaces as the jiggling icon, and the app-switcher
draws the entry without compositing a preview because the surface is
unmapped. Moving rather than minimizing keeps the surface mapped, so
Mutter renders the preview normally and the dock has nothing to
nag about.

## Timing

We keep the existing 100ms `GLib.timeout_add` delay before reading
`window.get_title()` — terminals (Kitty in our setup) sometimes set
their title slightly after the `window-created` signal fires. The
move-workspace call is just as safe inside that callback as the
old minimize call was.

## Rollout

1. Edit `extension.js` and `metadata.json`.
2. `./stack build-ubuntu` to bake the extension into the image.
3. Start a new spec-task session — existing sessions keep the old
   image.
4. Verify in the GNOME activities overview that the ACP terminal is
   on workspace 4 (last) and that the dock icon is calm.

No DB migration, no config flag, no flag flip.

## Risks / Notes

- If the launcher changes to spawn the terminal *already* on a
  particular workspace, this code becomes a no-op (still safe).
- If a user manually drags the window back to workspace 1, we do
  nothing — we only act on `window-created`. That's intentional;
  user intent wins.
- We are NOT adding a "should I move?" preference. The whole reason
  this code exists is for the Helix sandbox UX; if a future task
  needs configurability, add it then.

## Implementation Notes (post-build)

- The actual terminal that hosts "ACP Agent Logs" is **ghostty**,
  not kitty (`launch_terminal` in `desktop/shared/start-zed-core.sh`
  was changed at some point; current invocation is `ghostty.real
  --title=ACP Agent Logs ...`). The substring match
  `title.includes("ACP Agent Logs")` still matches, so no code
  change was needed in the extension — the title-based match is
  terminal-agnostic.
- Build cycle: `./stack build-ubuntu` produced
  `helix-ubuntu:688422`. The inner Helix sandbox auto-pulls new
  images on next session start; existing sessions keep their
  pinned image (no need to restart the sandbox).
- Verification approach: instead of trying to screenshot the
  GNOME activities overview through the streaming pipeline (which
  required a live spec-task session and a perfectly-timed click),
  I queried Mutter's window state directly via
  `org.gnome.Shell.Eval` over the session D-Bus. This gave a
  ground-truth result: `workspace_index: 3, minimized: false,
  active_workspace_index: 0`. See
  `screenshots/01-mutter-dbus-verification.txt`.
- Gotcha: D-Bus session address has to come from the
  `gnome-shell` process's `/proc/$PID/environ` because
  `dbus-run-session` doesn't bind a well-known socket (it picks a
  random `/tmp/dbus-xxxxx`). Connecting from `bash -c` requires
  exporting that env var; `gdbus` from `root` won't work even with
  the right address — use `docker exec -u retro` so the UID
  matches.
