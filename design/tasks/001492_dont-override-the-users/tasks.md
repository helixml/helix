# Implementation Tasks

- [ ] Add `UserOverrides json.RawMessage` field to `ZedConfigResponse` in `api/pkg/types/types.go` (with `json:"user_overrides,omitempty"`)
- [ ] In `getZedConfig()` in `api/pkg/server/zed_config_handlers.go`, fetch persisted user overrides via `external_agent.GetUserZedOverrides()` and include them in the response
- [ ] Add `UserOverrides map[string]interface{}` field to `helixConfigResponse` in `api/cmd/settings-sync-daemon/main.go` to deserialize the new response field
- [ ] Rewrite `syncFromHelix()` to populate `d.userOverrides` from the API response instead of resetting to empty map
- [ ] Add fallback in `syncFromHelix()`: if API returns no overrides, read on-disk `settings.json` and call `extractUserOverrides()` to recover any existing user customizations
- [ ] Change `syncFromHelix()` to call `d.mergeSettings(d.helixSettings, d.userOverrides)` before `d.writeSettings()`, matching the pattern in `checkHelixUpdates()`
- [ ] Add unit test: initial sync with persisted user theme override preserves user's theme
- [ ] Add unit test: initial sync with no persisted overrides but existing on-disk theme preserves it
- [ ] Add unit test: initial sync with no overrides at all writes Helix default theme ("Ayu Dark")
- [ ] Add unit test: `mergeSettings` gives user override precedence over Helix `theme` value
- [ ] Run `go build ./api/cmd/settings-sync-daemon/` and `go test ./api/cmd/settings-sync-daemon/` to verify
- [ ] Regenerate swagger docs with `./stack update_openapi` (ZedConfigResponse changed)
- [ ] Manual test: start a session, change theme in Zed UI, restart the settings-sync-daemon (or start new session), verify theme persists