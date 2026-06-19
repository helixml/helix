# Implementation Tasks: Add Keyboard Shortcuts to the File Browser

- [ ] Add `focusedIndex` state (`useState<number>(-1)`) to `FilesSidebar.tsx`
- [ ] Add `tabIndex={0}` and `onKeyDown={handleKeyDown}` to the `<List>` element
- [ ] Implement `handleKeyDown` mapping ArrowUp/Down, Enter, Backspace, Alt+ArrowLeft, Home, Delete, Escape to existing callbacks
- [ ] Apply visual focus highlight to the item at `focusedIndex` (reuse `backgroundColor: '#1a1a2f'` active style or add outline)
- [ ] Reset `focusedIndex` to `-1` when the directory changes (on `currentDirectory` change via `useEffect`)
- [ ] Wire Delete key to set `selectedFile` to the focused item and open `deleteDialogOpen`
- [ ] Wire Escape key to close context menu (`setMenuAnchorEl(null)`) or delete dialog (`setDeleteDialogOpen(false)`)
- [ ] Call `preventDefault()` for all handled keys to prevent browser scroll / history navigation
- [ ] Manual test: arrow navigation, Enter to open file/dir, Backspace to go up, Home to root, Delete to confirm dialog, Escape to dismiss
