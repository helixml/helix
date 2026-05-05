# Implementation Tasks

- [x] Add a new `IconButton` (with `<Tooltip title="Project settings">`) just before the existing `MoreHorizontal` kebab button (~line 932) that calls `openDialog('project-settings', { projectId })`. Reuse the already-imported lucide `Settings` icon for consistency with the existing menu item.
- [x] Wrap the existing `MoreHorizontal` `IconButton` in `<Tooltip title="More options">` and add `aria-label="More options"`
- [x] Remove the now-redundant `Settings` `MenuItem` from the kebab `Menu` (~lines 970–981)
- [x] Run `cd frontend && yarn build` to verify the change type-checks and bundles (passed cleanly)
- [x] ~~Take a before/after screenshot~~ — skipped: inner Helix at localhost:8080 was not running because the startup script's API build failed for an unrelated reason (`pkg/openai/manager/provider_manager.go` has an undefined `m.runnerController` field, pre-existing on `main`)
- [x] Push code branch + write PR description (`pull_request_helix.md`)
