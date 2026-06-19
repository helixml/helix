# Design: Add Keyboard Shortcuts to the File Browser

## Context

The file browser lives in `frontend/src/components/files/FilesSidebar.tsx`. It renders a MUI `List` of `ListItem` elements. Navigation and file selection are currently mouse-only. The parent `Files.tsx` page hosts the Monaco editor, so shortcuts must be scoped to the sidebar to avoid interfering with editor bindings.

## Approach

Add a `keydown` handler to the `<List>` element via `onKeyDown`. The list gets `tabIndex={0}` so it can receive keyboard focus. A `focusedIndex` state (integer) tracks which item is visually highlighted.

```
<List
  tabIndex={0}
  onKeyDown={handleKeyDown}
  ...
>
```

The `handleKeyDown` function maps keys to existing navigation callbacks already defined in the component:

| Key | Action |
|-----|--------|
| `ArrowDown` | `focusedIndex = (focusedIndex + 1) % items.length` |
| `ArrowUp` | `focusedIndex = (focusedIndex - 1 + items.length) % items.length` |
| `Enter` | Call existing directory/file open logic for the focused item |
| `Backspace` / `Alt+ArrowLeft` | Call `navigateBack()` |
| `Home` | Call `navigateToRoot()` |
| `Delete` | Open delete dialog for the focused item |
| `Escape` | Close context menu or delete dialog |

`preventDefault()` is called for all handled keys to suppress browser defaults (scrolling, history navigation).

## Visual Focus

The focused item receives `selected` or a highlighted border via the same `backgroundColor: '#1a1a2f'` style already used for the active file. A thin `outline` on the `List` itself signals sidebar focus to the user.

## Codebase Notes

- `FilesSidebar.tsx` already has `navigateBack`, `navigateToRoot`, `navigateToDirectory`, and delete state — no new callbacks needed.
- Item data comes from `filesData` (array of `FilestoreItem`). Index into this array directly.
- `deleteDialogOpen` / `setDeleteDialogOpen` and `selectedFile` / `setSelectedFile` state already exist for delete flow.
- `menuAnchorEl` null check + `setMenuAnchorEl(null)` already closes the context menu — Escape just calls that.
- The Monaco editor inside `Files.tsx` has its own focus; the sidebar list focus is separate, so no global event listener is needed.
