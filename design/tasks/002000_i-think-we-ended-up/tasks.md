# Implementation Tasks

- [x] In `desktop/ubuntu-config/gnome-extension/helix-cursor@helix.ml/extension.js`, replace the `window.minimize()` call in `_onWindowCreated` with logic that reads `global.workspace_manager.get_n_workspaces()` and calls `window.change_workspace_by_index(n - 1, false)` on the matching window
- [x] In the same handler, guard against `n_workspaces < 2` — log a warning via `console.log("[HelixCursor] ...")` and return without moving
- [x] Update the `// Window management: minimize ...` comment at `extension.js:118` and the log message at `extension.js:199` to describe the new "move to last workspace" behavior
- [x] Bump `version` from 2 → 3 in `desktop/ubuntu-config/gnome-extension/helix-cursor@helix.ml/metadata.json` and update `description` to remove the "Auto-minimizes ACP Agent Logs terminal" wording (replace with "Moves ACP Agent Logs terminal to last workspace")
- [~] Run `./stack build-ubuntu` to bake the updated extension into the helix-ubuntu image
- [ ] Start a new spec-task session with `SHOW_ACP_DEBUG_LOGS=true` (or `HELIX_DEBUG` set) and verify in the GNOME activities overview that the ACP Agent Logs terminal is on workspace 4 with a normal preview
- [ ] Verify the dock icon for the ACP terminal does NOT jiggle/animate, and the app-switcher (Alt+Tab) shows it as a normal entry — not a ghost
- [ ] Verify the user's active workspace remains workspace 1 (Zed) after the ACP terminal spawns — no auto-switch
- [ ] Commit changes with a message describing the fix; do NOT amend or force-push
