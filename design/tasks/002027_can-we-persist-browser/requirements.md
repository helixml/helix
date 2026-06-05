# Requirements: Persist Browser State and Auto-Restore on Session Resume

## Background

In the Helix desktop containers (Sway and GNOME/Ubuntu variants), Chrome (amd64) or Chromium (arm64) is the default browser. The user's question:

> Can we persist browser state? When Chrome or Chromium was previously open, it would be nice if we could automatically reopen it as well, so that when you resume a session you get all your tabs back.

### Persistence model (verified)

Only `/home/retro/work/` is bind-mounted into the container as a persistent volume — everything else under `/home/retro/` (including `~/.config/google-chrome/`) is **ephemeral** and lost on container restart. This is confirmed by:

- `api/pkg/sandbox/controller_provision.go:252-257` — only `workspaceHostDir` is mounted at `/home/retro/work`.
- `desktop/shared/helix-workspace-setup.sh:535-567` — already symlinks `~/.claude` → `$WORK_DIR/.claude-state` for the same reason. This is the canonical pattern in this codebase.

### Current Chrome behaviour

- Enterprise policy at `Dockerfile.sway-helix:707` / `Dockerfile.ubuntu-helix` sets `"RestoreOnStartup": 5` (open New Tab Page) — even *if* the profile survived, tabs would not auto-restore.
- Chrome is launched on demand (user click on Sway bar globe icon, `$mod+Shift+f`, GNOME favourite). Nothing auto-launches it.

## User stories

### Story 1: Tabs survive container restart
**As a** user resuming a spec-task session
**I want** my open Chrome tabs and browsing history to still be there
**So that** I don't lose context when the container restarts

### Story 2: Browser auto-reopens on resume
**As a** user resuming a session where Chrome was open
**I want** Chrome to be running automatically when the desktop comes up
**So that** I don't have to manually re-launch it every time

### Story 3: Quietly skip auto-launch when not wanted
**As a** user who closed Chrome before the session ended
**I want** Chrome to stay closed on the next resume
**So that** the desktop isn't cluttered with apps I didn't want

## Acceptance criteria

- [ ] Closing Chrome with tabs open, then restarting the container, restores the same tabs (verified for both `helix-sway` and `helix-ubuntu` runtimes).
- [ ] Profile data (bookmarks, history, saved logins, extensions) survives a container restart.
- [ ] If Chrome was running when the previous session ended, it is auto-launched on session start (background, minimised or on workspace 3 per existing Sway rule at `startup-app.sh:468`).
- [ ] If Chrome was *not* running when the session ended (user closed it), it does **not** auto-launch.
- [ ] Works on both amd64 (Google Chrome) and arm64 (Chromium) — same code path via the existing `google-chrome-stable` compatibility symlink.
- [ ] Works in both desktop variants: `Dockerfile.sway-helix` and `Dockerfile.ubuntu-helix` (the GNOME variant).
- [ ] No regression for users who never open the browser — no errors at startup, no empty Chrome window on first launch.
- [ ] First-run sentinel logic in `Dockerfile.sway-helix:724-729` continues to work — users get a clean first launch with no welcome dialog.

## Out of scope

- Saving open terminal sessions, Zed window state, or other application state (separate concern).
- Cross-host migration of browser state (persistent sandboxes are sticky to their original host per `controller_provision.go:336-364`; this task does not change that).
- Syncing browser state between different sandboxes (each sandbox keeps its own profile).
