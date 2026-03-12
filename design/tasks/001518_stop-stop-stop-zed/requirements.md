# Requirements: Disable Zed Auto-Formatting (Except Go)

## Problem

Zed's default `format_on_save: "on"` setting is actively reformatting source files in Helix sessions. This is particularly harmful for JavaScript/TypeScript/TSX in our own codebases, where the formatter mangles intentional formatting. Go is the one language where format-on-save (via `gofmt`/`gopls`) is expected and desired.

The settings-sync-daemon controls what goes into `~/.config/zed/settings.json` inside desktop containers, but currently sets no `format_on_save` or `languages` config — so Zed's default (`"on"`) applies globally.

## User Stories

1. **As a Helix user**, I want Zed to stop reformatting my JS/TS/TSX files on save so my code stays as I wrote it.
2. **As a Helix user writing Go**, I want `gofmt` format-on-save to keep working because that's the standard Go workflow.

## Acceptance Criteria

- [ ] `format_on_save` is `"off"` globally in the Zed settings written by the settings-sync-daemon
- [ ] Go language has `format_on_save: "on"` as a per-language override
- [ ] Existing user overrides (if any) are not clobbered — the `mergeSettings` flow still lets users re-enable formatting per-language if they choose
- [ ] Change requires `build-ubuntu` + new session to take effect (this is expected for settings-sync-daemon changes)

## Out of Scope

- Per-project formatting preferences (users can still set these via Zed's project settings)
- Adding formatter-specific config (e.g. prettier rules) — just controlling the on/off toggle
- Changes to the Zed fork itself — this is purely a settings-sync-daemon change