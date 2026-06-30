# Requirements: Fix Zed Theme Not Updating on Repeated Light/Dark Toggle

## Background

The Helix web UI has a light/dark mode toggle button (the moon/sun icon in the
page header). Clicking it switches the embedded Zed editor's theme. The first
click works (e.g. dark → light), but clicking again to go back (light → dark)
does **not** update Zed's theme. Only the first transition per Zed session takes
effect. This has been attempted multiple times — all prior attempts were on the
Helix settings-sync-daemon side and did not fix it, because the actual fault is
that **Zed stops noticing changes to `settings.json` after the first one**, not
that the daemon writes the wrong value.

Root cause (confirmed by code trace): the daemon writes `settings.json` with an
atomic rename, which replaces the file's inode on every write. Zed's user-settings
file watcher watches the *file inode* (only symlinks get a parent-directory
watch), and the native inotify watch is removed when that inode is replaced and
is never re-armed. So the first write is observed; every write after it is
silently missed.

NOTE: The trigger is the **moon/sun button in the Helix web UI**, which drives the
user's `color_scheme` preference through the Helix pipeline to Zed. It is not the
operating-system appearance toggle, and the fix does not touch any OS/portal
appearance path.

## User Stories

### US-1: Theme follows the UI toggle in both directions
As a Helix user clicking the light/dark toggle button,
I want Zed's theme to switch every time I click it,
so that the editor always matches the mode I selected — not just the first time.

### US-2: Reliable repeated toggling
As a user,
I want light→dark→light→dark (repeated any number of times) to keep working
within a single session,
so that I never have to reload or restart to recover correct theming.

### US-3: Protected by a regression test
As a maintainer,
I want an automated test proving the settings watcher survives atomic-rename
replacement of `settings.json`,
so that this class of bug (which also affects agent-switch settings reloads and
manual external edits) cannot silently regress.

## Acceptance Criteria

- [ ] Clicking the Helix light/dark toggle changes Zed's theme on the first click.
- [ ] Clicking it again changes Zed's theme back (the bug under repair).
- [ ] Repeating the toggle ≥ 3 times in a row switches correctly every time, with
      no Zed reload/restart and no manual settings edit.
- [ ] A user-picked custom Zed theme (set in Zed's own UI) is still preserved and
      not clobbered by the toggle (existing `HELIX_MANAGED_THEMES` behaviour).
- [ ] A regression test exercises the watcher against repeated atomic-rename
      replacement of the watched file and asserts a reload is delivered for the
      2nd and 3rd replacement, not just the 1st.
- [ ] No regression to the initial theme applied when a session first opens.

## Out of Scope

- Zed's operating-system appearance / portal `color-scheme` observation path
  (not involved — the trigger is the Helix UI button, not the OS).
- Adding new theme settings or changing the managed light/dark theme names.
- Reworking the Helix color-scheme API or the daemon's merge logic (it already
  computes and writes the correct value).
