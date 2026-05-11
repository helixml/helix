# Requirements

## Background

The `helix-cursor@helix.ml` GNOME extension currently auto-minimizes the
"ACP Agent Logs" terminal window on creation
(`extension.js:192-208`, calls `window.minimize()`). This produces
visible regressions:

- The window's icon in the dock/app-bar jiggles continuously.
- In the GNOME app-switcher (Alt+Tab), the entry shows up as a "ghost"
  with no visible content preview.
- In Activities / window overview the window is awkwardly hidden.

The window count of the GNOME shell is fixed at **4 workspaces**
(`dconf-settings.ini:34` — `num-workspaces=4`,
 `dconf-settings.ini:39` — `dynamic-workspaces=false`). So a stable
"last workspace" exists.

## User Story

As a Helix desktop user, I want the ACP Agent Logs terminal to stay
un-minimized but tucked away on the last virtual desktop, so that:

- It does not clutter my primary workspace where Zed runs.
- The dock icon stays calm — no jiggling.
- The app-switcher behaves normally — no ghost entries.
- I (or a curious user) can still discover and inspect the logs by
  switching to workspace 4 in the GNOME activities view.

## Acceptance Criteria

1. When a window with title containing `"ACP Agent Logs"` is created,
   the extension moves it to the **last** virtual workspace (index
   `n_workspaces - 1`, currently workspace 4 / index 3) and does
   **not** minimize it.
2. The dock/app-bar icon for the ACP Agent Logs terminal does not
   jiggle / animate continuously after the move.
3. The window appears as a normal entry on workspace 4 in the GNOME
   activities overview (no ghost rendering, has a content preview).
4. The window does **not** auto-focus or auto-switch the user's
   active workspace away from workspace 1 (Zed).
5. If the workspace count is unexpectedly < 2, the extension logs a
   warning and leaves the window on the current workspace (no crash,
   no minimize).
6. `metadata.json` description and inline comments in `extension.js`
   reflect the new behavior (no stale references to "auto-minimizes").
7. After rebuilding the helix-ubuntu image
   (`./stack build-ubuntu`) and starting a new spec-task session
   with `SHOW_ACP_DEBUG_LOGS=true` (or `HELIX_DEBUG` set), the
   behavior is observed end-to-end.

## Out of Scope

- Changing the launcher in `desktop/shared/start-zed-core.sh` to
  start the terminal directly on workspace 4 (would couple shell
  startup to GNOME workspace API; keeping the placement logic in the
  extension keeps responsibility in one place).
- Adding configuration knobs (target workspace index, alternate
  windows). The constant `last workspace` is enough for now.
- Hiding / restyling the dock icon further.
