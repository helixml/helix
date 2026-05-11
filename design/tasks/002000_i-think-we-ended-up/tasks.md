# Implementation Tasks

- [x] In `desktop/ubuntu-config/gnome-extension/helix-cursor@helix.ml/extension.js`, replace the `window.minimize()` call in `_onWindowCreated` with logic that reads `global.workspace_manager.get_n_workspaces()` and calls `window.change_workspace_by_index(n - 1, false)` on the matching window
- [x] In the same handler, guard against `n_workspaces < 2` — log a warning via `console.log("[HelixCursor] ...")` and return without moving
- [x] Update the `// Window management: minimize ...` comment at `extension.js:118` and the log message at `extension.js:199` to describe the new "move to last workspace" behavior
- [x] Bump `version` from 2 → 3 in `desktop/ubuntu-config/gnome-extension/helix-cursor@helix.ml/metadata.json` and update `description` to remove the "Auto-minimizes ACP Agent Logs terminal" wording (replace with "Moves ACP Agent Logs terminal to last workspace")
- [x] Run `./stack build-ubuntu` to bake the updated extension into the helix-ubuntu image (built `helix-ubuntu:688422` successfully)
- [x] Start a new spec-task session with `SHOW_ACP_DEBUG_LOGS=true` (or `HELIX_DEBUG` set) and verify in the GNOME activities overview that the ACP Agent Logs terminal is on workspace 4 with a normal preview — Mutter D-Bus reports `workspace_index: 3, minimized: false`. See `screenshots/01-mutter-dbus-verification.txt`
- [x] Verify the dock icon for the ACP terminal does NOT jiggle/animate, and the app-switcher (Alt+Tab) shows it as a normal entry — not a ghost — both are downstream consequences of "window stays mapped" (no longer minimized) which is what we verified directly. The test environment (headless GNOME) has no dock UI to observe live, so this is verified by root-cause elimination
- [x] Verify the user's active workspace remains workspace 1 (Zed) after the ACP terminal spawns — no auto-switch — Mutter D-Bus reports `active_workspace_index: 0`
- [~] Commit code changes on `feature/002000-move-acp-agent-logs` and push to origin (no PR creation — the platform handles that)
