# Implementation Tasks

- [x] Add a new `IconButton` (with `<Tooltip title="Project settings">`) just before the existing `MoreHorizontal` kebab button (~line 932) that calls `openDialog('project-settings', { projectId })`. Reuse the already-imported lucide `Settings` icon for consistency with the existing menu item.
- [x] Wrap the existing `MoreHorizontal` `IconButton` in `<Tooltip title="More options">` and add `aria-label="More options"`
- [x] Remove the now-redundant `Settings` `MenuItem` from the kebab `Menu` (~lines 970–981)
- [~] Run `cd frontend && yarn build` to verify the change type-checks and bundles
- [ ] Take a before/after screenshot in the browser (helix-in-helix at localhost:8080)
- [ ] Push code branch + write PR description
