# Implementation Tasks

## Extract hardcoded defaults to shared helper

- [ ] Create a `helixDefaults()` function in `api/cmd/settings-sync-daemon/main.go` that returns the static Helix defaults map: `text_rendering_mode`, `suggest_dev_container`, `format_on_save: "off"`, and `languages: {"Go": {"format_on_save": "on"}}`
- [ ] Refactor `syncFromHelix()` to call `helixDefaults()` as the base for `d.helixSettings`, then layer on API response fields (`context_servers`, `language_models`, `assistant`, `agent`, `theme`)
- [ ] Refactor `checkHelixUpdates()` to call `helixDefaults()` as the base for `newHelixSettings`, then layer on API response fields — same as `syncFromHelix`
- [ ] Move the `agent.tool_permissions` injection (currently only in `syncFromHelix`) into a shared spot or into `helixDefaults()` so `checkHelixUpdates` also applies it

## Deep merge `languages` in merge/extract functions

- [ ] In `mergeSettings()`, add a deep-merge block for `languages` matching the existing `context_servers` pattern — merge user language overrides per-language key instead of flat overwrite
- [ ] Skip `languages` from the flat user-override loop in `mergeSettings()` (same as `context_servers` is skipped)
- [ ] In `extractUserOverrides()`, add a per-language diff block for `languages` matching the existing `context_servers` pattern — only capture languages the user actually customized

## Build and verify

- [ ] Run `go build ./cmd/settings-sync-daemon/` from `api/` to verify compilation
- [ ] `./stack build-ubuntu` to rebuild the desktop image with the new daemon
- [ ] Start a new session, open a JS/TS file, edit and save — confirm no auto-formatting
- [ ] In the same session, open a Go file, edit and save — confirm `gofmt` runs
- [ ] Wait >30 seconds, verify `text_rendering_mode` and `format_on_save` are still present in `~/.config/zed/settings.json` (poll cycle didn't drop them)
- [ ] Change a per-language setting via Zed UI, wait >30 seconds, verify it persists