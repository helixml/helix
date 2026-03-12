# Requirements: Disable Zed Auto-Formatting (Except Go)

## Problem

Zed's default `format_on_save: "on"` setting is actively reformatting source files in Helix sessions. This is particularly harmful for JavaScript/TypeScript/TSX in our own codebases, where the formatter mangles intentional formatting. Go is the one language where format-on-save (via `gofmt`/`gopls`) is expected and desired.

The settings-sync-daemon controls what goes into `~/.config/zed/settings.json` inside desktop containers, but currently sets no `format_on_save` or `languages` config — so Zed's default (`"on"`) applies globally.

Additionally, the daemon's user override mechanism is fundamentally broken. `writeSettings()` does an atomic write (`os.Rename` from a temp file), which replaces the inode at the settings path. On Linux, inotify watches inodes — so after the first atomic write, the fsnotify watcher is watching a deleted inode and never receives events again. `onFileChanged()` never fires, `d.userOverrides` stays empty forever, and user changes to any setting (theme, context_servers, anything) are silently reverted on the next poll cycle.

On top of this, hardcoded Helix defaults (like `text_rendering_mode`) are only set in `syncFromHelix()` (startup) but not in `checkHelixUpdates()` (30-second poll), so they get silently dropped. And inject mutations (`injectLanguageModelAPIKey`, etc.) cause `deepEqual` to always fail, so the daemon rewrites the file every 30 seconds even when nothing changed.

## User Stories

1. **As a Helix user**, I want Zed to stop reformatting my JS/TS/TSX files on save so my code stays as I wrote it.
2. **As a Helix user writing Go**, I want `gofmt` format-on-save to keep working because that's the standard Go workflow.
3. **As a Helix user**, I want my Zed settings changes (e.g. theme) to survive the daemon's 30-second poll cycle, not get silently reverted.
4. **As a Helix user**, I want to be able to override `format_on_save` per-language (e.g. re-enable for Rust) and have that override stick.
5. **As a Helix user**, I want new sessions to start with a sensible default theme ("Ayu Dark") but be able to change it without it reverting.

## Acceptance Criteria

### Formatting defaults
- [ ] `format_on_save` is `"off"` globally in the Zed settings written by the settings-sync-daemon
- [ ] Go language has `format_on_save: "on"` as a per-language override

### fsnotify watcher fix
- [ ] The fsnotify watcher watches the settings **directory** (not the file) so atomic renames don't kill it — same pattern already used for the Claude credentials watcher
- [ ] `onFileChanged()` fires when Zed writes `settings.json`, and user overrides are captured in `d.userOverrides`

### Poll-cycle bug fix
- [ ] Hardcoded Helix defaults (`text_rendering_mode`, `suggest_dev_container`, `format_on_save`, `languages`, `agent.tool_permissions`) persist across the 30-second poll cycle — they must not be dropped by `checkHelixUpdates()`
- [ ] The daemon does not detect a spurious "config changed" diff on every poll due to inject mutations (`api_key`, `available_models`) polluting the comparison baseline

### User preference persistence
- [ ] `theme` is set to "Ayu Dark" as the initial default on new sessions, but once the user changes it via Zed UI, the daemon preserves the user's choice and never reverts it
- [ ] `mergeSettings()` deep-merges the `languages` key (same pattern as `context_servers`) so user per-language overrides don't clobber Helix's Go formatting override and vice versa
- [ ] `extractUserOverrides()` diffs `languages` per-language key (same pattern as `context_servers`) so only actual user customizations are captured
- [ ] A user who changes `format_on_save` to `"on"` for a specific language via Zed UI has that change persist across poll cycles

### Deployment
- [ ] Change requires `build-ubuntu` + new session to take effect (expected for settings-sync-daemon changes)

## Out of Scope

- Per-project formatting preferences (users can still set these via Zed's project settings)
- Adding formatter-specific config (e.g. prettier rules) — just controlling the on/off toggle
- Changes to the Zed fork itself — this is purely a settings-sync-daemon change