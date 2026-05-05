# Surface project Settings as a header IconButton

## Summary

Project settings on the project management page (`SpecTasksPage`) were hidden behind the three-dot kebab menu in the top-right corner, with no tooltip or label. Users reported they had trouble finding it. This PR promotes Settings to a dedicated gear `IconButton` directly in the header so it's reachable in one click, and adds a tooltip to the kebab so it's no longer mystery-meat.

## Changes

- `frontend/src/pages/SpecTasksPage.tsx`
  - New `IconButton` with the lucide `Settings` icon and a `<Tooltip title="Project settings">`, placed immediately to the left of the existing `MoreHorizontal` kebab. Calls the same `openDialog('project-settings', { projectId })` as before.
  - Wrapped the existing kebab `IconButton` in `<Tooltip title="More options">` and added `aria-label="More options"`.
  - Removed the now-redundant `Settings` `MenuItem` from the kebab menu. Files / Sharing / Show Archived / Show Metrics / Show Merged remain.
  - Header `Box` switched to `display: flex` + `gap: 0.5` to space the two icon buttons cleanly. Mobile/xs behaviour is unchanged.

## Notes

- The settings dialog itself is unchanged — only its entry point.
- Right-click menu on project cards in `Projects.tsx` is unchanged.
- Reused the lucide `Settings` icon that was already imported in the file (matches the icon previously used in the kebab menu item) rather than introducing a separate MUI gear, to keep the visual identical to what users have seen in the menu.
