# Move ACP Agent Logs terminal to last workspace instead of minimizing

## Summary

Replaces the auto-minimize behavior in the `helix-cursor@helix.ml`
GNOME extension with a "move to the last workspace" behavior. The
old approach (`window.minimize()`) caused the dock icon to jiggle
continuously (because `tail -F` keeps emitting output and Mutter
treats those as `demands-attention` hints on a minimized surface)
and made the window appear as a ghost in the GNOME app-switcher
because its surface is unmapped while minimized.

The new behavior keeps the surface mapped on the last virtual
workspace (workspace 4 in our 4-workspace setup configured by
`dconf-settings.ini`), so:

- Dock icon stays calm — no jiggling.
- App-switcher shows a normal preview.
- The window is still discoverable from the GNOME activities
  overview.
- The user's active workspace (workspace 1, Zed) is not changed.

## Changes

- `desktop/ubuntu-config/gnome-extension/helix-cursor@helix.ml/extension.js`
  - `_onWindowCreated`: replaced `window.minimize()` with
    `window.change_workspace_by_index(n_workspaces - 1, false)`.
  - Added a defensive check that bails (with a log line, no crash)
    if `n_workspaces < 2`.
  - Updated the surrounding comment and the log messages to match
    the new behavior.
- `desktop/ubuntu-config/gnome-extension/helix-cursor@helix.ml/metadata.json`
  - Bumped `version` 2 → 3 so GNOME Shell reloads the manifest.
  - Updated `description` from "Auto-minimizes ACP Agent Logs
    terminal" to "Moves ACP Agent Logs terminal to last workspace".

## Test plan

- [ ] `./stack build-ubuntu` baked the new extension into the image.
- [ ] Started a new spec-task session with `SHOW_ACP_DEBUG_LOGS=true`.
- [ ] Confirmed the ACP Agent Logs terminal appears on workspace 4
  in the GNOME activities overview with a normal preview.
- [ ] Confirmed the dock icon does not jiggle.
- [ ] Confirmed the app-switcher shows it as a normal entry, not a
  ghost.
- [ ] Confirmed the user's active workspace stays workspace 1 (Zed)
  on session start.
