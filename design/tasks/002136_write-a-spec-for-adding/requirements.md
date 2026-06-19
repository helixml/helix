# Requirements: Add Keyboard Shortcuts to the File Browser

## User Stories

- As a user, I want to navigate the file list with arrow keys so I can browse without reaching for the mouse.
- As a user, I want to press Enter to open the focused file or enter the focused directory.
- As a user, I want to press Backspace or Alt+Left to go up to the parent directory.
- As a user, I want to press Home or the `~` key to jump back to the root directory.
- As a user, I want to press Delete (or Fn+Delete on Mac) on a focused item to trigger the delete confirmation.
- As a user, I want to press Escape to close any open context menu or dialog.

## Acceptance Criteria

1. When the file list has focus, `ArrowUp` / `ArrowDown` moves the highlighted item through the list (wrapping at the ends).
2. Pressing `Enter` on a highlighted directory navigates into it; pressing `Enter` on a highlighted file opens it (same as clicking).
3. Pressing `Backspace` or `Alt+ArrowLeft` navigates to the parent directory (equivalent to clicking the back arrow).
4. Pressing `Home` navigates to the root directory (equivalent to clicking the Home icon).
5. Pressing `Delete` on a highlighted item opens the delete confirmation dialog.
6. Pressing `Escape` closes the context menu (if open) or the delete dialog (if open).
7. Keyboard focus is visible — the highlighted row shows a clear visual focus indicator consistent with the existing active-item style.
8. Shortcuts only fire when focus is inside the sidebar file list, not globally, to avoid conflicts with the Monaco editor or other inputs.
