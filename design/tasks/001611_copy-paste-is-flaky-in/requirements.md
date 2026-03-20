# Requirements: Fix Flaky First Paste (Ctrl+V types "v")

## Problem

When a user presses Ctrl+V to paste into the desktop viewer, the **first attempt** types a literal "v" character instead of pasting. Subsequent presses work correctly.

## User Story

As a user of the desktop viewer, when I press Ctrl+V to paste, I expect the clipboard contents to be pasted in the remote app on the first attempt — not a literal "v" character.

## Acceptance Criteria

- [ ] Pressing Ctrl+V in the desktop viewer pastes correctly on the first attempt after connecting
- [ ] No spurious "v" character appears in the remote application when pasting
- [ ] Subsequent Ctrl+V presses continue to work correctly
- [ ] Copy (Ctrl+C) is unaffected
