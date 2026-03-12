# Requirements: Don't Override User's Chosen Theme in Zed

## Problem

The settings-sync-daemon clobbers user customizations (theme, and potentially other settings) on every initial sync. When `syncFromHelix()` runs (on daemon startup or `/reload`), it:

1. Resets `d.userOverrides` to an empty map (line 768)
2. Writes `d.helixSettings` directly to `settings.json` without merging user overrides (line 789)
3. Never fetches persisted user overrides from the Helix API (`GET /zed-config/user` endpoint doesn't exist; overrides are saved via `POST /zed-config/user` but never restored)

The merge path (`mergeSettings`) only runs in `checkHelixUpdates()` (the poll loop), which means user overrides are respected during runtime — but lost on every restart.

Additionally, the server hardcodes `config.Theme = "Ayu Dark"` in `zed_config.go:169`, meaning even the poll path will re-assert this theme if the Helix config hasn't changed structurally.

## User Stories

1. **As a user**, I want my chosen Zed theme to persist across session restarts, so I don't have to reconfigure it every time.
2. **As a user**, I want any settings I change in Zed's UI (font size, vim mode, etc.) to survive daemon restarts and Helix config updates.
3. **As a user**, I want Helix to provide a sensible default theme on first launch, but respect my choice once I've changed it.

## Acceptance Criteria

- [ ] When the daemon starts, it fetches persisted user overrides from the Helix API before writing `settings.json`
- [ ] Initial sync merges Helix settings with user overrides (same as the poll path already does)
- [ ] User-set `theme` is treated as a user override and persists across restarts
- [ ] The hardcoded `"Ayu Dark"` theme is only applied when the user has no theme override
- [ ] Existing merge behavior in `checkHelixUpdates` continues to work correctly
- [ ] `/reload` endpoint also respects user overrides (it calls `syncFromHelix` which has the same bug)
- [ ] Unit tests cover: initial sync with existing overrides, theme override persistence, merge precedence

## Out of Scope

- Changing which settings are considered "Helix-managed" vs "user-owned"
- UI for managing settings sync preferences
- Per-project theme settings