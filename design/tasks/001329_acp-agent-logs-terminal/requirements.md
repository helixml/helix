# Requirements: ACP Agent Logs Terminal Minimized by Default

## Overview

The ACP agent logs terminal window (Ghostty) currently appears on top of the Zed window when `SHOW_ACP_DEBUG_LOGS=true`. This terminal shows tailed Zed log output for debugging purposes. Users want this terminal to be **minimized by default** so it doesn't obstruct the Zed IDE view.

## User Stories

1. **As a developer**, I want the ACP logs terminal to start minimized so that I can see the full Zed IDE without obstruction while still having access to logs when needed.

2. **As a debugger**, I want to be able to restore/maximize the logs terminal when I need to check agent activity without restarting the session.

## Acceptance Criteria

- [ ] When `SHOW_ACP_DEBUG_LOGS=true`, the ACP Agent Logs terminal launches minimized
- [ ] The terminal can be restored from the taskbar/dock when needed
- [ ] The terminal continues to tail logs even while minimized
- [ ] No changes to the terminal behavior when manually restored by the user
- [ ] Works on Ubuntu GNOME desktop (primary target)
- [ ] Does not affect other terminal windows (e.g., Helix Setup terminal)

## Non-Requirements

- Sway desktop support (different window management model)
- Persistent minimize state across session restarts
- UI toggle to control minimize-by-default behavior