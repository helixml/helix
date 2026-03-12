# Implementation Tasks

- [ ] In `api/cmd/settings-sync-daemon/main.go` `syncFromHelix()`, add `"format_on_save": "off"` to the `d.helixSettings` map (around line 722, alongside existing defaults like `text_rendering_mode`)
- [ ] In the same map literal, add a `"languages"` key with a Go override: `"languages": map[string]interface{}{"Go": map[string]interface{}{"format_on_save": "on"}}`
- [ ] Run `go build ./cmd/settings-sync-daemon/` from `api/` to verify compilation
- [ ] `./stack build-ubuntu` to rebuild the desktop image with the new daemon
- [ ] Start a new session, open a JS/TS file, make an edit, save — confirm no auto-formatting occurs
- [ ] In the same session, open a Go file, make an edit, save — confirm `gofmt` formatting still runs