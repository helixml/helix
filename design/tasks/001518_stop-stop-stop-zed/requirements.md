# Requirements: Disable Zed Auto-Formatting (Except Go)

## Problem

Zed's default `format_on_save: "on"` setting is actively reformatting source files in Helix sessions. This is particularly harmful for JavaScript/TypeScript/TSX in our own codebases, where the formatter mangles intentional formatting. Go is the one language where format-on-save (via `gofmt`/`gopls`) is expected and desired.

The settings-sync-daemon controls what goes into `~/.config/zed/settings.json` inside desktop containers, but currently sets no `format_on_save` or `languages` config — so Zed's default (`"on"`) applies globally.

Additionally, there is a pre-existing bug: hardcoded Helix defaults (like `text_rendering_mode`) are only set in `syncFromHelix()` (startup) but not in `checkHelixUpdates()` (30-second poll). This means all hardcoded defaults — including any new `format_on_save` setting — get silently dropped ~30 seconds after session start. Users observe their settings being overridden shortly after changing them.

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

### Poll-cycle bug fix
- [ ] Hardcoded Helix defaults (`text_rendering_mode`, `suggest_dev_container`, `format_on_save`, `languages`, `agent.tool_permissions`) persist across the 30-second poll cycle — they must not be dropped by `checkHelixUpdates()`
- [ ] The daemon does not detect a spurious "config changed" diff on every poll due to missing hardcoded defaults in the comparison baseline

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