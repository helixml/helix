# Implementation Tasks

- [ ] In `frontend/src/pages/SpecTasksPage.tsx`, import `SettingsIcon` from `@mui/icons-material/Settings`
- [ ] Add a new `IconButton` (with `<Tooltip title="Project settings">`) just before the existing `MoreHorizontal` kebab button (~line 932) that calls `openDialog('project-settings', { projectId })`
- [ ] Wrap the existing `MoreHorizontal` `IconButton` in `<Tooltip title="More options">` and add `aria-label="More options"`
- [ ] Remove the now-redundant `Settings` `MenuItem` from the kebab `Menu` (~lines 970–981)
- [ ] Verify the header still renders correctly on desktop widths and is still hidden on `xs` breakpoint
- [ ] Manually click the new gear button and confirm the project settings dialog opens
- [ ] Manually open the kebab menu and confirm Files / Sharing / view-toggle items still work
- [ ] Run frontend type check / lint
