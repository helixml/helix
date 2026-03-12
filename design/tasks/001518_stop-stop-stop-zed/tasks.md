# Implementation Tasks

## Fix broken fsnotify watcher (root cause of all user override failures)

- [ ] In `startWatcher()`, watch the settings **directory** (`filepath.Dir(SettingsPath)`) instead of the file itself — atomic renames (`os.Rename` in `writeSettings`) replace the inode, killing a file-level inotify watcher permanently. This is the same pattern already used for the Claude credentials watcher.
- [ ] In the fsnotify event handler, match settings changes with `filepath.Base(event.Name) == "settings.json"` instead of `event.Name == SettingsPath`
- [ ] Verify `onFileChanged()` fires after Zed writes `settings.json` — add a log line and confirm it appears in daemon logs after changing a setting in the Zed UI

## Extract hardcoded defaults to shared helper

- [ ] Create a `helixDefaults()` function in `api/cmd/settings-sync-daemon/main.go` that returns the static Helix defaults map: `text_rendering_mode`, `suggest_dev_container`, `format_on_save: "off"`, and `languages: {"Go": {"format_on_save": "on"}}`
- [ ] Refactor `syncFromHelix()` to call `helixDefaults()` as the base for `d.helixSettings`, then layer on API response fields (`context_servers`, `language_models`, `assistant`, `agent`, `theme`)
- [ ] Refactor `checkHelixUpdates()` to call `helixDefaults()` as the base for `newHelixSettings`, then layer on API response fields — same as `syncFromHelix`
- [ ] Move the `agent.tool_permissions` injection (currently only in `syncFromHelix`) into a shared spot or into `helixDefaults()` so `checkHelixUpdates` also applies it

## User-preference fields (theme etc.)

- [ ] Add a `USER_PREFERENCE_FIELDS` map: `{"theme": true}` — settings the daemon sets as initial defaults but never overwrites once the user has changed them
- [ ] In `syncFromHelix()`, set `theme: "Ayu Dark"` (from the API response) as the initial default in a fresh `settings.json`
- [ ] In `mergeSettings()`, for each `USER_PREFERENCE_FIELDS` key, read the on-disk value from `settings.json` and preserve it in the merged output instead of using the Helix-provided value — same pattern already used for `telemetry`
- [ ] In `checkHelixUpdates()`, exclude `USER_PREFERENCE_FIELDS` keys from `newHelixSettings` so they don't trigger spurious diffs
- [ ] In `extractUserOverrides()`, skip `USER_PREFERENCE_FIELDS` keys — the daemon reads them from disk, no need to sync back to the API

## Fix spurious rewrite every 30 seconds

- [ ] Store a `d.helixSettingsBaseline` field that holds the pre-injection version of helix settings (before `injectLanguageModelAPIKey`, `injectAvailableModels`, etc. mutate the map)
- [ ] In `checkHelixUpdates()`, compare `newHelixSettings` against `d.helixSettingsBaseline` instead of `d.helixSettings` so inject mutations don't cause a false diff every poll cycle
- [ ] After detecting a real change, set `d.helixSettingsBaseline = newHelixSettings` and copy into `d.helixSettings` before running inject functions

## Deep merge `languages` in merge/extract functions

- [ ] In `mergeSettings()`, add a deep-merge block for `languages` matching the existing `context_servers` pattern — merge user language overrides per-language key instead of flat overwrite
- [ ] Skip `languages` from the flat user-override loop in `mergeSettings()` (same as `context_servers` is skipped)
- [ ] In `extractUserOverrides()`, add a per-language diff block for `languages` matching the existing `context_servers` pattern — only capture languages the user actually customized

## Build and verify

- [ ] Run `go build ./cmd/settings-sync-daemon/` from `api/` to verify compilation
- [ ] `./stack build-ubuntu` to rebuild the desktop image with the new daemon
- [ ] Start a new session — confirm theme is "Ayu Dark" (initial default)
- [ ] Change theme to something else, wait >30 seconds — confirm theme is NOT reverted
- [ ] Open a JS/TS file, edit and save — confirm no auto-formatting
- [ ] Open a Go file, edit and save — confirm `gofmt` runs
- [ ] Wait >30 seconds, verify `text_rendering_mode` and `format_on_save` are still present in `~/.config/zed/settings.json` (poll cycle didn't drop them)
- [ ] Check daemon logs — confirm no "Detected Helix config change" spam every 30 seconds (spurious rewrite fixed)
- [ ] Change a per-language setting via Zed UI, wait >30 seconds, verify it persists