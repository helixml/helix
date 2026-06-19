# Implementation Tasks: Add Keyboard Shortcuts to the File Browser

- [ ] Add `focusedIndex` state (`useState<number>(-1)`) to `FilesSidebar.tsx`
- [ ] Add `tabIndex={0}` and `onKeyDown={handleKeyDown}` to the `<List>` element
- [ ] Implement `handleKeyDown` mapping ArrowUp/Down, Enter, Backspace, Alt+ArrowLeft, Home, Delete, Escape to existing callbacks
- [ ] Apply visual focus highlight to the item at `focusedIndex` (reuse `backgroundColor: '#1a1a2f'` active style or add outline)
- [ ] Reset `focusedIndex` to `-1` when the directory changes (on `currentDirectory` change via `useEffect`)
- [ ] Wire Delete key to set `selectedFile` to the focused item and open `deleteDialogOpen`
- [ ] Wire Escape key to close context menu (`setMenuAnchorEl(null)`), delete dialog, or cheat-sheet overlay (whichever is open)
- [ ] Add `shortcutsOverlayOpen` boolean state; wire `?` key to open it
- [ ] Render a MUI `Dialog` for the cheat-sheet showing all shortcuts in a two-column key/description table, styled with dark theme colors (`#1a1a2f`, border `#333`)
- [ ] Add a small `?` icon button in the sidebar navigation header as a mouse-accessible trigger for the overlay
- [ ] Call `preventDefault()` for all handled keys to prevent browser scroll / history navigation
- [ ] Manual test: arrow navigation, Enter to open file/dir, Backspace to go up, Home to root, Delete to confirm dialog, `?` opens overlay, Escape dismisses each layer
